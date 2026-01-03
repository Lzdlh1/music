package db

import (
	"github.com/Lzdlh1/music-backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Init(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&models.User{}, &models.Task{})
}
