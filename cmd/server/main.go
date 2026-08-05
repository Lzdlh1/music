package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/musicflow/musicflow/internal/api"
	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/db"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/proxy"
	"github.com/musicflow/musicflow/internal/scheduler"
	"github.com/musicflow/musicflow/internal/source"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/storage/local"
	s3storage "github.com/musicflow/musicflow/internal/storage/s3"
	sftpstorage "github.com/musicflow/musicflow/internal/storage/sftp"
	"github.com/musicflow/musicflow/internal/storage/webdav"
	"github.com/musicflow/musicflow/internal/telegram"
	"github.com/musicflow/musicflow/internal/worker"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
)

func main() {
	// 初始化日志
	logger := initLogger()
	defer logger.Sync()

	logger.Info("MusicFlow starting...")

	// 加载配置
	cfgPath := "config.yaml"
	if envPath := os.Getenv("MF_CONFIG"); envPath != "" {
		cfgPath = envPath
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Warn("load config file failed, using defaults", zap.Error(err))
		cfg, _ = config.Load("")
	}

	// 确保临时目录存在
	if err := os.MkdirAll(cfg.Scheduler.TempDir, 0750); err != nil {
		logger.Fatal("create temp dir", zap.Error(err))
	}

	// 初始化数据库
	database, err := db.Init(&cfg.Database, logger)
	if err != nil {
		logger.Fatal("init database", zap.Error(err))
	}

	// 初始化任务调度器
	sched := scheduler.New(database, &cfg.Scheduler, logger)

	// 初始化音乐源聚合器
	aggregator := source.NewAggregator(logger)

	// 初始化存储管理器
	storageMgr := storage.NewManager(logger)

	// 从数据库加载已配置的存储后端
	loadStorageBackends(database, storageMgr, logger)

	// 初始化代理管理器
	proxyMgr := proxy.NewManager(database, logger)

	// 初始化 Telegram Bot
	tgBot := telegram.NewBot(database, proxyMgr, logger)
	if err := tgBot.Start(context.Background()); err != nil {
		logger.Warn("telegram bot init failed", zap.Error(err))
	}

	// 初始化 Telegram MTProto 管理器
	mtMgr := telegram.NewMTProtoManager(database, proxyMgr, logger)

	// 初始化 Telegram 频道管理器
	channelMgr := telegram.NewChannelManager(database, proxyMgr, mtMgr, logger)
	if err := channelMgr.Start(context.Background()); err != nil {
		logger.Warn("channel manager init failed", zap.Error(err))
	}

	// 初始化工作器并绑定到调度器
	w := worker.New(database, aggregator, storageMgr, mtMgr, cfg, logger)
	sched.SetWorkerFunc(w.Execute)

	// 从数据库加载并注册已配置的音乐源
	loadMusicSources(database, aggregator, mtMgr, logger)

	// 恢复未完成的任务
	sched.RecoverTasks()

	// 创建并启动 API 服务器
	server := api.NewServer(database, cfg, sched, aggregator, storageMgr, tgBot, mtMgr, channelMgr, proxyMgr, logger)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	// 优雅关闭
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		logger.Info("shutting down...")
		if err := server.Shutdown(); err != nil {
			logger.Error("shutdown error", zap.Error(err))
		}
	}()

	if err := server.Listen(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func initLogger() *zap.Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding:         "console",
		EncoderConfig:    encoderConfig,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := cfg.Build()
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	return logger
}

// loadMusicSources 从数据库加载已配置的音乐源
func loadMusicSources(database *gorm.DB, aggregator *source.Aggregator, mtMgr *telegram.MTProtoManager, logger *zap.Logger) {
	var configs []models.MusicSourceConfig
	if err := database.Where("enabled = ?", true).Find(&configs).Error; err != nil {
		logger.Warn("load music sources failed", zap.Error(err))
		return
	}

	for _, cfg := range configs {
		switch cfg.Type {
		case "custom_api":
			var apiCfg source.CustomAPIConfig
			if err := models.UnmarshalTo(cfg.Config, &apiCfg); err != nil {
				logger.Warn("parse custom api config", zap.String("id", cfg.ID), zap.Error(err))
				continue
			}
			if apiCfg.Name == "" {
				apiCfg.Name = cfg.Name
			}
			apiCfg.Priority = cfg.Priority
			aggregator.Register(source.NewCustomAPISource(apiCfg, logger))
		case "netease":
			var ncfg source.NeteaseConfig
			if err := models.UnmarshalTo(cfg.Config, &ncfg); err != nil {
				logger.Warn("parse netease config", zap.String("id", cfg.ID), zap.Error(err))
				continue
			}
			if ncfg.Name == "" {
				ncfg.Name = cfg.Name
			}
			ncfg.Priority = cfg.Priority
			aggregator.Register(source.NewNeteaseSource(ncfg, logger))
			logger.Info("loaded music source", zap.String("name", cfg.Name), zap.String("type", cfg.Type))
		case "meting":
			var mcfg source.MetingConfig
			if err := models.UnmarshalTo(cfg.Config, &mcfg); err != nil {
				logger.Warn("parse meting config", zap.String("id", cfg.ID), zap.Error(err))
				continue
			}
			if mcfg.Name == "" {
				mcfg.Name = cfg.Name
			}
			mcfg.Priority = cfg.Priority
			aggregator.Register(source.NewMetingSource(mcfg, logger))
			logger.Info("loaded music source", zap.String("name", cfg.Name), zap.String("type", cfg.Type))
		case "tgbot":
			var bcfg source.TGBotSourceConfig
			if err := models.UnmarshalTo(cfg.Config, &bcfg); err != nil {
				logger.Warn("parse tgbot config", zap.String("id", cfg.ID), zap.Error(err))
				continue
			}
			if bcfg.Name == "" {
				bcfg.Name = cfg.Name
			}
			bcfg.Priority = cfg.Priority
			aggregator.Register(source.NewTGBotSource(bcfg, mtMgr, logger))
			logger.Info("loaded music source", zap.String("name", cfg.Name), zap.String("type", cfg.Type))
		default:
			logger.Warn("unknown music source type", zap.String("type", cfg.Type), zap.String("id", cfg.ID))
		}
	}
}

// loadStorageBackends 从数据库加载已配置的存储后端
func loadStorageBackends(database *gorm.DB, mgr *storage.Manager, logger *zap.Logger) {
	var targets []models.StorageTarget
	if err := database.Where("enabled = ?", true).Find(&targets).Error; err != nil {
		logger.Warn("load storage backends failed", zap.Error(err))
		return
	}

	for _, t := range targets {
		var cfgMap map[string]interface{}
		if err := json.Unmarshal(t.Config, &cfgMap); err != nil {
			logger.Warn("parse storage config", zap.String("id", t.ID), zap.Error(err))
			continue
		}

		switch storage.StorageType(t.Type) {
		case storage.StorageLocal:
			basePath, _ := cfgMap["base_path"].(string)
			if basePath == "" {
				logger.Warn("local storage missing base_path", zap.String("id", t.ID))
				continue
			}
			backend, err := local.New(t.ID, t.Name, basePath)
			if err != nil {
				logger.Warn("create local storage", zap.String("id", t.ID), zap.Error(err))
				continue
			}
			mgr.Register(backend)

		case storage.StorageWebDAV:
			var cfg webdav.Config
			if err := json.Unmarshal(t.Config, &cfg); err != nil {
				logger.Warn("parse webdav config", zap.String("id", t.ID), zap.Error(err))
				continue
			}
			mgr.Register(webdav.New(t.ID, t.Name, cfg))

		case storage.StorageSFTP:
			var cfg sftpstorage.Config
			if err := json.Unmarshal(t.Config, &cfg); err != nil {
				logger.Warn("parse sftp config", zap.String("id", t.ID), zap.Error(err))
				continue
			}
			mgr.Register(sftpstorage.New(t.ID, t.Name, cfg))

		case storage.StorageS3:
			var cfg s3storage.Config
			if err := json.Unmarshal(t.Config, &cfg); err != nil {
				logger.Warn("parse s3 config", zap.String("id", t.ID), zap.Error(err))
				continue
			}
			mgr.Register(s3storage.New(t.ID, t.Name, cfg))

		default:
			logger.Warn("unknown storage type", zap.String("type", t.Type), zap.String("id", t.ID))
		}

		logger.Info("loaded storage backend", zap.String("name", t.Name), zap.String("type", t.Type))
	}
}
