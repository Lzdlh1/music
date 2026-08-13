package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlog "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"github.com/musicflow/musicflow/internal/api/handlers"
	authmw "github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/proxy"
	"github.com/musicflow/musicflow/internal/scheduler"
	"github.com/musicflow/musicflow/internal/source"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Server API 服务器
type Server struct {
	app        *fiber.App
	db         *gorm.DB
	cfg        *config.Config
	scheduler  *scheduler.Scheduler
	aggregator *source.Aggregator
	storageMgr *storage.Manager
	tgBot      *telegram.Bot
	mtMgr      *telegram.MTProtoManager
	channelMgr *telegram.ChannelManager
	proxyMgr   *proxy.Manager
	wsHub      *handlers.WSHub
	log        *zap.Logger
}

// NewServer 创建 API 服务器
func NewServer(
	db *gorm.DB,
	cfg *config.Config,
	sched *scheduler.Scheduler,
	agg *source.Aggregator,
	sm *storage.Manager,
	tgBot *telegram.Bot,
	mtMgr *telegram.MTProtoManager,
	channelMgr *telegram.ChannelManager,
	proxyMgr *proxy.Manager,
	log *zap.Logger,
) *Server {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error":   true,
				"message": err.Error(),
			})
		},
		BodyLimit: 200 * 1024 * 1024, // 200MB，支持大文件上传
	})

	s := &Server{
		app:        app,
		db:         db,
		cfg:        cfg,
		scheduler:  sched,
		aggregator: agg,
		storageMgr: sm,
		tgBot:      tgBot,
		mtMgr:      mtMgr,
		channelMgr: channelMgr,
		proxyMgr:   proxyMgr,
		wsHub:      handlers.NewWSHub(log),
		log:        log,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.app.Use(recover.New())
	s.app.Use(fiberlog.New())
	s.app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))
}

func (s *Server) setupRoutes() {
	// 静态文件（前端构建产物）
	s.app.Static("/", "./web/dist")

	api := s.app.Group("/api/v1")

	// 公开路由（认证前可访问）
	authHandler := handlers.NewAuthHandler(&s.cfg.Auth)
	api.Post("/auth/login", authHandler.Login)
	api.Get("/auth/status", authHandler.AuthStatus)

	// 云盘流媒体播放（<audio> 无法携带 JWT header，故放在认证中间件之前）
	cloudHandler := handlers.NewCloudHandler(s.storageMgr, s.db, s.log)
	api.Get("/storage/:id/stream", cloudHandler.Stream)
	api.Get("/library/:id/stream", cloudHandler.LibraryStream)

	// JWT 认证中间件
	api.Use(authmw.AuthMiddleware(&s.cfg.Auth))

	// 搜索
	searchHandler := handlers.NewSearchHandler(s.aggregator, s.log)
	api.Get("/search", searchHandler.Search)
	api.Get("/track/:id/sources", searchHandler.GetTrackSources)
	api.Get("/track/:id/lyrics", searchHandler.GetLyrics)
	api.Get("/track/:id/cover", searchHandler.GetCover)

	// 下载任务
	taskHandler := handlers.NewTaskHandler(s.scheduler, s.log)
	api.Post("/tasks", taskHandler.Create)
	api.Get("/tasks", taskHandler.List)
	api.Get("/tasks/stats", taskHandler.Stats)
	api.Post("/tasks/batch", taskHandler.BatchCreate)
	api.Get("/tasks/:id", taskHandler.Get)
	api.Put("/tasks/:id/pause", taskHandler.Pause)
	api.Put("/tasks/:id/resume", taskHandler.Resume)
	api.Delete("/tasks/:id", taskHandler.Cancel)

	// 存储配置
	storageHandler := handlers.NewStorageHandler(s.storageMgr, s.db, s.log)
	api.Get("/storage", storageHandler.List)
	api.Post("/storage", storageHandler.Create)
	api.Put("/storage/:id", storageHandler.Update)
	api.Delete("/storage/:id", storageHandler.Delete)
	api.Post("/storage/:id/test", storageHandler.Test)
	api.Get("/storage/:id/browse", storageHandler.Browse)

	// 云盘文件管理
	api.Post("/storage/:id/mkdir", cloudHandler.Mkdir)
	api.Post("/storage/:id/rename", cloudHandler.Rename)
	api.Delete("/storage/:id/file", cloudHandler.DeleteFile)
	api.Post("/storage/:id/upload", cloudHandler.Upload)

	// 音乐源配置
	sourceHandler := handlers.NewSourceHandler(s.db, s.log)
	api.Get("/sources", sourceHandler.List)
	api.Post("/sources", sourceHandler.Create)
	api.Put("/sources/:id", sourceHandler.Update)
	api.Delete("/sources/:id", sourceHandler.Delete)
	api.Post("/sources/:id/test", sourceHandler.Test)

	// 系统设置
	settingsHandler := handlers.NewSettingsHandler(s.db, s.log)
	api.Get("/settings", settingsHandler.Get)
	api.Put("/settings", settingsHandler.Update)
	api.Get("/settings/download", settingsHandler.GetDownload)
	api.Put("/settings/download", settingsHandler.UpdateDownload)
	api.Get("/settings/naming", settingsHandler.GetNaming)
	api.Put("/settings/naming", settingsHandler.UpdateNaming)

	// 音乐库
	libraryHandler := handlers.NewLibraryHandler(s.db, s.log)
	api.Get("/library", libraryHandler.List)
	api.Get("/library/:id", libraryHandler.Get)
	api.Get("/library/:id/lyrics", cloudHandler.LibraryLyrics)
	api.Delete("/library/:id", libraryHandler.Delete)

	// 歌单导入
	playlistHandler := handlers.NewPlaylistHandler(s.log)
	api.Post("/playlist/parse-url", playlistHandler.ParseURL)
	api.Post("/playlist/parse-text", playlistHandler.ParseText)
	api.Post("/playlist/parse-file", playlistHandler.ParseFile)

	// Telegram
	tgHandler := handlers.NewTelegramHandler(s.tgBot, s.mtMgr, s.db, s.log)
	tg := api.Group("/telegram")
	tg.Get("/bots", tgHandler.ListBots)
	tg.Post("/bots", tgHandler.CreateBot)
	tg.Put("/bots/:id", tgHandler.UpdateBot)
	tg.Delete("/bots/:id", tgHandler.DeleteBot)
	tg.Post("/bots/:id/test", tgHandler.TestBot)
	tg.Post("/bots/test", tgHandler.TestBot)
	tg.Get("/accounts", tgHandler.ListAccounts)
	tg.Post("/accounts", tgHandler.CreateAccount)
	tg.Delete("/accounts/:id", tgHandler.DeleteAccount)
	tg.Post("/accounts/:id/start", tgHandler.StartAccount)
	tg.Post("/accounts/:id/code", tgHandler.SubmitCode)
	tg.Post("/accounts/:id/password", tgHandler.SubmitPassword)

	// 频道资源
	channelHandler := handlers.NewChannelHandler(s.channelMgr, s.storageMgr, s.db, s.log)
	tg.Get("/channels", channelHandler.ListChannels)
	tg.Post("/channels", channelHandler.AddChannel)
	tg.Delete("/channels/:id", channelHandler.RemoveChannel)
	tg.Put("/channels/:id/toggle", channelHandler.ToggleChannel)
	tg.Get("/channels/:id/files", channelHandler.ListFiles)
	tg.Post("/channels/:id/scan", channelHandler.ScanHistory)
	tg.Get("/channels/files", channelHandler.ListAllFiles)
	tg.Get("/channels/files/:fileId/download", channelHandler.GetFileDownloadURL)
	tg.Post("/channels/files/:fileId/save", channelHandler.DownloadToLibrary)

	// 系统信息
	systemHandler := handlers.NewSystemHandler(s.db, s.log)
	api.Get("/system/info", systemHandler.Info)
	api.Get("/system/logs", systemHandler.Logs)
	api.Get("/system/storage-usage", systemHandler.StorageUsage)
	api.Post("/system/cleanup", systemHandler.Cleanup)

	// 代理配置
	proxyHandler := handlers.NewProxyHandler(s.proxyMgr, s.log)
	api.Get("/proxy", proxyHandler.GetConfig)
	api.Put("/proxy", proxyHandler.SetConfig)
	api.Post("/proxy/test", proxyHandler.Test)

	// WebSocket
	s.app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	s.app.Get("/ws/tasks", websocket.New(s.wsHub.HandleTasksWS))

	// SPA 回退：前端路由
	s.app.Get("/*", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache")
		return c.SendFile("./web/dist/index.html")
	})
}

// Listen 启动服务器
func (s *Server) Listen(addr string) error {
	// 设置调度器进度监听 → WebSocket 推送
	s.scheduler.SetProgressListener(func(taskID, status string, progress scheduler.TaskProgress) {
		s.wsHub.BroadcastTaskUpdate(taskID, status, progress)
	})

	s.log.Info("server starting", zap.String("addr", addr))
	return s.app.Listen(addr)
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}
