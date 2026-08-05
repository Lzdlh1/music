package config

import (
	"fmt"
	"sync"

	"github.com/spf13/viper"
)

// Config 全局配置结构体
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Download  DownloadConfig  `mapstructure:"download"`
	Naming    NamingConfig    `mapstructure:"naming"`
	Log       LogConfig       `mapstructure:"log"`
	Auth      AuthConfig      `mapstructure:"auth"`
}

type ServerConfig struct {
	Port      int    `mapstructure:"port"`
	Host      string `mapstructure:"host"`
	SecretKey string `mapstructure:"secret_key"`
}

type AuthConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	JWTSecret string `mapstructure:"jwt_secret"`
}

type DatabaseConfig struct {
	Type string `mapstructure:"type"` // sqlite or postgres
	DSN  string `mapstructure:"dsn"`
}

type SchedulerConfig struct {
	MaxConcurrentDownloads int    `mapstructure:"max_concurrent_downloads"`
	MaxConcurrentUploads   int    `mapstructure:"max_concurrent_uploads"`
	DownloadSpeedLimit     int64  `mapstructure:"download_speed_limit"`
	UploadSpeedLimit       int64  `mapstructure:"upload_speed_limit"`
	RetryMax               int    `mapstructure:"retry_max"`
	RetryBackoff           string `mapstructure:"retry_backoff"`
	TempDir                string `mapstructure:"temp_dir"`
	AutoCleanup            bool   `mapstructure:"auto_cleanup"`
}

type DownloadConfig struct {
	DefaultQuality string   `mapstructure:"default_quality"`
	SourcePriority []string `mapstructure:"source_priority"`
	EmbedCover     bool     `mapstructure:"embed_cover"`
	EmbedLyrics    bool     `mapstructure:"embed_lyrics"`
	SaveLrcFile    bool     `mapstructure:"save_lrc_file"`
}

type NamingConfig struct {
	Template      string `mapstructure:"template"`
	CoverFilename string `mapstructure:"cover_filename"`
}

type LogConfig struct {
	Level   string `mapstructure:"level"`
	File    string `mapstructure:"file"`
	MaxSize int    `mapstructure:"max_size"`
}

var (
	cfg  *Config
	once sync.Once
)

// Load 加载配置文件
func Load(path string) (*Config, error) {
	var err error
	once.Do(func() {
		viper.SetConfigFile(path)
		viper.SetConfigType("yaml")

		// 默认值
		viper.SetDefault("server.port", 15698)
		viper.SetDefault("server.host", "0.0.0.0")
		viper.SetDefault("database.type", "sqlite")
		viper.SetDefault("database.dsn", "./data/musicflow.db")
		viper.SetDefault("scheduler.max_concurrent_downloads", 3)
		viper.SetDefault("scheduler.max_concurrent_uploads", 5)
		viper.SetDefault("scheduler.retry_max", 3)
		viper.SetDefault("scheduler.retry_backoff", "2s,10s,30s")
		viper.SetDefault("scheduler.temp_dir", "./temp")
		viper.SetDefault("scheduler.auto_cleanup", true)
		viper.SetDefault("download.default_quality", "FLAC")
		viper.SetDefault("download.source_priority", []string{"telegram", "netease", "qqmusic", "custom"})
		viper.SetDefault("download.embed_cover", true)
		viper.SetDefault("download.embed_lyrics", true)
		viper.SetDefault("download.save_lrc_file", true)
		viper.SetDefault("naming.template", "{artist}/{album} ({year})/{track_no:02d} - {title}.{ext}")
		viper.SetDefault("naming.cover_filename", "cover.jpg")
		viper.SetDefault("log.level", "info")
		viper.SetDefault("log.file", "./data/musicflow.log")
		viper.SetDefault("log.max_size", 100)
		viper.SetDefault("auth.enabled", false)
		viper.SetDefault("auth.username", "admin")
		viper.SetDefault("auth.password", "musicflow")
		viper.SetDefault("auth.jwt_secret", "change-me-in-production")

		if readErr := viper.ReadInConfig(); readErr != nil {
			// 配置文件不存在时使用默认值
			if _, ok := readErr.(viper.ConfigFileNotFoundError); !ok {
				err = fmt.Errorf("read config: %w", readErr)
				return
			}
		}

		cfg = &Config{}
		if unmarshalErr := viper.Unmarshal(cfg); unmarshalErr != nil {
			err = fmt.Errorf("unmarshal config: %w", unmarshalErr)
			return
		}
	})

	return cfg, err
}

// Get 获取当前配置
func Get() *Config {
	return cfg
}
