package db

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/db/models"
	"go.uber.org/zap"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
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
		&models.User{},
		&models.InviteKey{},
		&models.Task{},
		&models.Library{},
		&models.StorageTarget{},
		&models.MusicSourceConfig{},
		&models.TGBot{},
		&models.TGAccount{},
		&models.TGChannel{},
		&models.TGChannelFile{},
		&models.Setting{},
		&models.QQQuota{},
	); err != nil {
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	// 修复空 ID 的历史记录
	fixEmptyIDs(db)

	// 兼容：存在旧 single-admin 配置时，自动将其导入为管理员账号
	bootstrapLegacyAdmin(db, log)

	log.Info("database initialized", zap.String("type", cfg.Type), zap.String("dsn", cfg.DSN))
	return db, nil
}

// bootstrapLegacyAdmin 兼容旧版单账号登录（config.yaml auth: admin/musicflow）：
// 若 users 表为空且配置了账号密码，自动创建 admin 账号，避免旧部署升级后无法登录。
func bootstrapLegacyAdmin(db *gorm.DB, log *zap.Logger) {
	if db == nil {
		return
	}
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil || count > 0 {
		return
	}
	cfg := config.Get()
	if cfg == nil || !cfg.Auth.Enabled || cfg.Auth.Username == "" {
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Auth.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Warn("bootstrap legacy admin: hash password failed", zap.Error(err))
		return
	}
	u := models.User{
		Username:     cfg.Auth.Username,
		PasswordHash: string(hash),
		Role:         "admin",
	}
	if err := db.Create(&u).Error; err != nil {
		log.Warn("bootstrap legacy admin failed", zap.Error(err))
		return
	}
	log.Info("bootstrapped legacy admin", zap.String("username", cfg.Auth.Username))
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
