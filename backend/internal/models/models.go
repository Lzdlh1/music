package models

import (
	"time"

	"gorm.io/gorm"
)

// User - simple local user for auth
type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Username     string         `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string         `gorm:"not null" json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Task represents a download/upload job. Note: we DO NOT store user cookies.
type Task struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Title        string    `json:"title"`
	URL          string    `json:"url"`
	Status       string    `json:"status"` // queued, running, done, failed
	ErrorMessage string    `json:"error_message"`
	OwnerID      uint      `json:"owner_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
