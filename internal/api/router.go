package api

import (
	"strings"

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
	// 静态文件（前端构建产物，html 禁用缓存防止旧 bundle 混用）
	s.app.Static("/", "./web/dist", fiber.Static{
		ModifyResponse: func(c *fiber.Ctx) error {
			if strings.Contains(string(c.Response().Header.ContentType()), "text/html") {
				c.Set("Cache-Control", "no-store")
			}
			return nil
		},
	})

	api := s.app.Group("/api/v1")

	// ---------- 公开路由（无需登录） ----------
	authHandler := handlers.NewAuthHandler(s.db, s.cfg.Auth.JWTSecret)
	api.Post("/auth/login", authHandler.Login)
	api.Get("/auth/status", authHandler.AuthStatus)
	api.Post("/auth/setup", authHandler.Setup)     // 首次使用创建管理员
	api.Post("/auth/register", authHandler.Register) // 邀请码注册

	// 云盘/曲库/在线试听流媒体播放（<audio> 无法携带 JWT header，用 query token 鉴权）
	cloudHandler := handlers.NewCloudHandler(s.storageMgr, s.db, s.log)
	api.Get("/storage/:id/stream", authmw.AuthQueryToken(s.db, s.cfg.Auth.JWTSecret), cloudHandler.Stream)
	api.Get("/library/:id/stream", authmw.AuthQueryToken(s.db, s.cfg.Auth.JWTSecret), cloudHandler.LibraryStream)
	api.Get("/library/:id/lyrics", authmw.AuthQueryToken(s.db, s.cfg.Auth.JWTSecret), cloudHandler.LibraryLyrics)
	trackHandler := handlers.NewTrackHandler(s.aggregator, s.db, s.log)
	api.Get("/track/:id/stream", authmw.AuthQueryToken(s.db, s.cfg.Auth.JWTSecret), trackHandler.Stream)

	// ---------- JWT 认证中间件（数据库驱动，始终启用） ----------
	api.Use(authmw.AuthMiddleware(s.db, s.cfg.Auth.JWTSecret))

	// 搜索与曲目（登录用户）
	searchHandler := handlers.NewSearchHandler(s.aggregator, s.log)
	api.Get("/search", searchHandler.Search)
	api.Get("/track/:id/sources", searchHandler.GetTrackSources)
	api.Get("/track/:id/lyrics", searchHandler.GetLyrics)
	api.Get("/track/:id/cover", searchHandler.GetCover)

	// 在线试听（128K 代理流，不落盘）与收藏
	api.Post("/library/favorite", trackHandler.Favorite)
	api.Delete("/library/favorite/:id", trackHandler.Unfavorite)

	// 当前用户
	usersHandler := handlers.NewUsersHandler(s.db, s.log)
	api.Get("/users/me", usersHandler.Me)
	api.Put("/users/me", usersHandler.UpdateMe)
	api.Get("/users/me/quota", usersHandler.MyQuota)

	// 下载任务（登录用户）
	taskHandler := handlers.NewTaskHandler(s.scheduler, s.db, s.log)
	api.Post("/tasks", taskHandler.Create)
	api.Get("/tasks", taskHandler.List)
	api.Get("/tasks/stats", taskHandler.Stats)
	api.Post("/tasks/batch", taskHandler.BatchCreate)
	api.Get("/tasks/:id", taskHandler.Get)
	api.Put("/tasks/:id/pause", taskHandler.Pause)
	api.Put("/tasks/:id/resume", taskHandler.Resume)
	api.Delete("/tasks/:id", taskHandler.Cancel)

	// 存储配置（用户创建归自己；共享只读）
	storageHandler := handlers.NewStorageHandler(s.storageMgr, s.db, s.log)
	api.Get("/storage", storageHandler.List)
	api.Post("/storage", storageHandler.Create)
	api.Put("/storage/:id", storageHandler.Update)
	api.Delete("/storage/:id", storageHandler.Delete)
	api.Post("/storage/:id/test", authmw.StorageReadGuard(s.db), storageHandler.Test)
	api.Get("/storage/:id/browse", authmw.StorageReadGuard(s.db), storageHandler.Browse)

	// 云盘文件操作（写需为自己的存储）
	api.Post("/storage/:id/mkdir", authmw.StorageWriteGuard(s.db), cloudHandler.Mkdir)
	api.Post("/storage/:id/rename", authmw.StorageWriteGuard(s.db), cloudHandler.Rename)
	api.Delete("/storage/:id/file", authmw.StorageWriteGuard(s.db), cloudHandler.DeleteFile)
	api.Post("/storage/:id/upload", authmw.StorageWriteGuard(s.db), cloudHandler.Upload)

	// 音乐库（登录用户，按归属过滤）
	libraryHandler := handlers.NewLibraryHandler(s.db, s.storageMgr, s.log)
	api.Get("/library", libraryHandler.List)
	api.Get("/library/:id", libraryHandler.Get)
	api.Delete("/library/:id", libraryHandler.Delete)

	// 歌单导入（登录用户）
	playlistHandler := handlers.NewPlaylistHandler(s.log)
	api.Post("/playlist/parse-url", playlistHandler.ParseURL)
	api.Post("/playlist/parse-text", playlistHandler.ParseText)
	api.Post("/playlist/parse-file", playlistHandler.ParseFile)

	// ---------- 仅管理员 ----------
	admin := api.Group("", authmw.RequireAdmin())

	// 用户与邀请码
	admin.Get("/users", usersHandler.List)
	admin.Put("/users/:id", usersHandler.Update)
	admin.Post("/users/invite", usersHandler.CreateInvite)
	admin.Get("/users/invites", usersHandler.ListInvites)
	admin.Delete("/users/invites/:id", usersHandler.DeleteInvite)

	// 音乐源配置
	sourceHandler := handlers.NewSourceHandler(s.db, s.aggregator, s.mtMgr, s.log)
	admin.Get("/sources", sourceHandler.List)
	admin.Post("/sources", sourceHandler.Create)
	admin.Put("/sources/:id", sourceHandler.Update)
	admin.Delete("/sources/:id", sourceHandler.Delete)
	admin.Post("/sources/:id/test", sourceHandler.Test)
	admin.Get("/sources/qq/quota", sourceHandler.GetQQQuota)

	// 系统设置
	settingsHandler := handlers.NewSettingsHandler(s.db, s.log)
	admin.Get("/settings", settingsHandler.Get)
	admin.Put("/settings", settingsHandler.Update)
	admin.Get("/settings/download", settingsHandler.GetDownload)
	admin.Put("/settings/download", settingsHandler.UpdateDownload)
	admin.Get("/settings/naming", settingsHandler.GetNaming)
	admin.Put("/settings/naming", settingsHandler.UpdateNaming)

	// 移动云盘（139）登录
	yun139Handler := handlers.NewYun139Handler(s.db, s.storageMgr, s.log)
	admin.Post("/yun139/sms/send", yun139Handler.SendSms)
	admin.Post("/yun139/sms/login", yun139Handler.SmsLogin)
	admin.Post("/yun139/password/login", yun139Handler.PasswordLogin)
	admin.Post("/yun139/qr/start", yun139Handler.StartQR)
	admin.Post("/yun139/qr/poll", yun139Handler.PollQR)

	// Telegram
	tgHandler := handlers.NewTelegramHandler(s.tgBot, s.mtMgr, s.db, s.log)
	tg := admin.Group("/telegram")
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
	tg.Post("/channels/:id/command", channelHandler.CommandSearch)
	tg.Get("/channels/files", channelHandler.ListAllFiles)
	tg.Get("/channels/files/:fileId/download", channelHandler.GetFileDownloadURL)
	tg.Post("/channels/files/:fileId/save", channelHandler.DownloadToLibrary)

	// 系统信息
	systemHandler := handlers.NewSystemHandler(s.db, s.log)
	admin.Get("/system/info", systemHandler.Info)
	admin.Get("/system/logs", systemHandler.Logs)
	admin.Get("/system/storage-usage", systemHandler.StorageUsage)
	admin.Post("/system/cleanup", systemHandler.Cleanup)

	// 代理配置
	proxyHandler := handlers.NewProxyHandler(s.proxyMgr, s.log)
	admin.Get("/proxy", proxyHandler.GetConfig)
	admin.Put("/proxy", proxyHandler.SetConfig)
	admin.Post("/proxy/test", proxyHandler.Test)

	// WebSocket
	s.app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})
	s.app.Get("/ws/tasks", websocket.New(s.wsHub.HandleTasksWS))

	// SPA 回退：前端路由（no-store 防止浏览器缓存旧版 index.html 导致 chunk 加载失败）
	s.app.Get("/*", func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store")
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
