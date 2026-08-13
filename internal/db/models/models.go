package models

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Task 下载任务
type Task struct {
	ID             string     `json:"id" gorm:"primaryKey"`
	Type           string     `json:"type" gorm:"not null"`   // SINGLE, BATCH, PLAYLIST
	Status         string     `json:"status" gorm:"not null"` // PENDING, FETCHING_META, DOWNLOADING, PROCESSING, UPLOADING, DONE, FAILED, PAUSED, CANCELLED
	Priority       int        `json:"priority" gorm:"default:0"`
	TrackInfo      JSON       `json:"track_info" gorm:"type:json;not null"`
	SelectedSource JSON       `json:"selected_source,omitempty" gorm:"type:json"`
	UploadTargets  JSON       `json:"upload_targets,omitempty" gorm:"type:json"`
	Progress       JSON       `json:"progress,omitempty" gorm:"type:json"`
	Error          string     `json:"error,omitempty"`
	RetryCount     int        `json:"retry_count" gorm:"default:0"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

func (Task) TableName() string { return "tasks" }

func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// Library 音乐库记录
type Library struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Title       string    `json:"title" gorm:"not null"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	Year        int       `json:"year"`
	Genre       string    `json:"genre"`
	Quality     string    `json:"quality"`
	Format      string    `json:"format"`
	FileSize    int64     `json:"file_size"`
	Duration    int       `json:"duration"`
	Source      string    `json:"source"`
	RemotePaths JSON      `json:"remote_paths,omitempty" gorm:"type:json"`
	CoverURL    string    `json:"cover_url"`
	HasLyrics   bool      `json:"has_lyrics"`
	Metadata    JSON      `json:"metadata,omitempty" gorm:"type:json"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Library) TableName() string { return "library" }

func (l *Library) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// StorageTarget 存储目标配置
type StorageTarget struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"not null"`
	Type      string    `json:"type" gorm:"not null"` // webdav, local, sftp, s3, onedrive, aliyun, gdrive
	Config    JSON      `json:"config" gorm:"type:json;not null"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
}

func (StorageTarget) TableName() string { return "storage_targets" }

func (s *StorageTarget) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

// MusicSource 音乐源配置
type MusicSourceConfig struct {
	ID       string `json:"id" gorm:"primaryKey"`
	Name     string `json:"name" gorm:"not null"`
	Type     string `json:"type" gorm:"not null"`
	Config   JSON   `json:"config" gorm:"type:json;not null"`
	Priority int    `json:"priority" gorm:"default:0"`
	Enabled  bool   `json:"enabled" gorm:"default:true"`
}

func (MusicSourceConfig) TableName() string { return "music_sources" }

func (m *MusicSourceConfig) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// TGBot Telegram Bot 配置
type TGBot struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	Name        string     `json:"name"`
	Username    string     `json:"username" gorm:"not null"`
	Config      JSON       `json:"config" gorm:"type:json;not null"`
	Priority    int        `json:"priority" gorm:"default:0"`
	Enabled     bool       `json:"enabled" gorm:"default:true"`
	SuccessRate float64    `json:"success_rate" gorm:"default:0"`
	LastTested  *time.Time `json:"last_tested,omitempty"`
}

func (TGBot) TableName() string { return "tg_bots" }

func (b *TGBot) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// TGAccount Telegram 账号
type TGAccount struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Phone       string    `json:"phone"`
	Username    string    `json:"username"`
	ApiID       int       `json:"api_id"`
	ApiHash     string    `json:"api_hash"`
	SessionPath string    `json:"session_path"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (TGAccount) TableName() string { return "tg_accounts" }

func (a *TGAccount) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// TGChannel Telegram 频道订阅
type TGChannel struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	ChatID    int64     `json:"chat_id" gorm:"uniqueIndex;not null"`
	Title     string    `json:"title"`
	Username  string    `json:"username"`
	Enabled   bool      `json:"enabled" gorm:"default:true"`
	FileCount int       `json:"file_count" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at"`
}

func (TGChannel) TableName() string { return "tg_channels" }

func (ch *TGChannel) BeforeCreate(tx *gorm.DB) error {
	if ch.ID == "" {
		ch.ID = uuid.New().String()
	}
	return nil
}

// TGChannelFile 频道中发现的音频文件
type TGChannelFile struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	ChannelID    string    `json:"channel_id" gorm:"index;not null"`
	ChatID       int64     `json:"chat_id"`
	MessageID    int64     `json:"message_id"`
	FileID       string    `json:"file_id" gorm:"not null"`
	FileUniqueID string    `json:"file_unique_id" gorm:"uniqueIndex;not null"`
	FileName     string    `json:"file_name"`
	FileSize     int64     `json:"file_size"`
	MimeType     string    `json:"mime_type"`
	Duration     int       `json:"duration"`
	Title        string    `json:"title"`
	Artist       string    `json:"artist"`
	Caption      string    `json:"caption"`
	Downloaded   bool      `json:"downloaded" gorm:"default:false"`
	PostedAt     time.Time `json:"posted_at"`
	CreatedAt    time.Time `json:"created_at"`
	// MTProto 下载所需：document 的 access_hash 与 file_reference（hex 存储）
	FileAccessHash int64  `json:"file_access_hash" gorm:"default:0"`
	FileReference  string `json:"file_reference"`
}

func (TGChannelFile) TableName() string { return "tg_channel_files" }

// FileIDInt 解析 FileID 字符串为 int64（MTProto document id）
func (f *TGChannelFile) FileIDInt() int64 {
	id, _ := strconv.ParseInt(f.FileID, 10, 64)
	return id
}

func (f *TGChannelFile) BeforeCreate(tx *gorm.DB) error {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	return nil
}

// Setting 系统配置（KV存储）
type Setting struct {
	Key       string    `json:"key" gorm:"primaryKey"`
	Value     JSON      `json:"value" gorm:"type:json;not null"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Setting) TableName() string { return "settings" }
