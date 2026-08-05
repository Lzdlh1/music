package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/db/models"
	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// Init 初始化数据库连接并自动迁移
func Init(cfg *config.DatabaseConfig, log *zap.Logger) (*gorm.DB, error) {
	// 确保目录存在
	dir := filepath.Dir(cfg.DSN)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	var dialector gorm.Dialector
	switch cfg.Type {
	case "sqlite", "":
		dialector = sqlite.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", cfg.Type)
	}

	var err error
	db, err = gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// 自动迁移
	if err := db.AutoMigrate(
		&models.Task{},
		&models.Library{},
		&models.StorageTarget{},
		&models.MusicSourceConfig{},
		&models.TGBot{},
		&models.TGAccount{},
		&models.TGChannel{},
		&models.TGChannelFile{},
		&models.Setting{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	// 修复空 ID 的历史记录
	fixEmptyIDs(db)

	log.Info("database initialized", zap.String("type", cfg.Type), zap.String("dsn", cfg.DSN))
	return db, nil
}

// fixEmptyIDs 修复历史遗留的空 ID 记录
func fixEmptyIDs(d *gorm.DB) {
	tables := []struct {
		model interface{}
		name  string
	}{
		{&models.MusicSourceConfig{}, "music_sources"},
		{&models.StorageTarget{}, "storage_targets"},
		{&models.Task{}, "tasks"},
		{&models.TGBot{}, "tg_bots"},
	}
	for _, t := range tables {
		var count int64
		d.Table(t.name).Where("id = '' OR id IS NULL").Count(&count)
		if count > 0 {
			d.Table(t.name).Where("id = '' OR id IS NULL").Update("id", gorm.Expr("lower(hex(randomblob(4)))||'-'||lower(hex(randomblob(2)))||'-4'||substr(lower(hex(randomblob(2))),2)||'-a'||substr(lower(hex(randomblob(2))),2)||'-'||lower(hex(randomblob(6)))"))
		}
	}
}
