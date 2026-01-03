package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	SecretKey     string
	DatabasePath  string
	RcloneRemote  string
	Port          string
	FrontendOrigin string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		SecretKey:     os.Getenv("SECRET_KEY"),
		DatabasePath:  os.Getenv("DATABASE_PATH"),
		RcloneRemote:  os.Getenv("RCLONE_REMOTE"),
		Port:          os.Getenv("PORT"),
		FrontendOrigin: os.Getenv("FRONTEND_ORIGIN"),
	}

	if cfg.SecretKey == "" {
		cfg.SecretKey = "change-me-please"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "./data/music.db"
	}
	if cfg.Port == "" {
		cfg.Port = "12233"
	}
	if cfg.FrontendOrigin == "" {
		cfg.FrontendOrigin = "http://localhost:12233"
	}

	return cfg, nil
}
