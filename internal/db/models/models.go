package models

import (
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// User 平台用户（角色：admin 管理员 / user 普通用户）
type User struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	Username       string    `json:"username" gorm:"uniqueIndex;not null"`
	PasswordHash   string    `json:"-"` // bcrypt，不序列化
	Role           string    `json:"role" gorm:"default:user"` // admin / user
	DownloadDir    string    `json:"download_dir"`             // 用户默认下载目录（相对存储根）
	DefaultStorage string    `json:"default_storage"`          // 用户默认存储目标 ID
	QQLimit        int       `json:"qq_limit" gorm:"default:300"` // QQ 每月下载限额
	Disabled       bool      `json:"disabled" gorm:"default:false"`
	CreatedAt      time.Time `json:"created_at"`
}

func (User) TableName() string { return "users" }

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

// InviteKey 一次性邀请注册码（管理员生成，用户凭码注册）
type InviteKey struct {
	ID        string     `json:"id" gorm:"primaryKey"`
	Code      string     `json:"code" gorm:"uniqueIndex;not null"`
	CreatedBy string     `json:"created_by"`
	UsedBy    string     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

func (InviteKey) TableName() string { return "invite_keys" }

func (k *InviteKey) BeforeCreate(tx *gorm.DB) error {
	if k.ID == "" {
		k.ID = uuid.New().String()
	}
	return nil
}

// Task 下载任务
type Task struct {
	ID             string     `json:"id" gorm:"primaryKey"`
	Type           string     `json:"type" gorm:"not null"`   // SINGLE, BATCH, PLAYLIST
	Status         string     `json:"status" gorm:"not null"` // PENDING, FETCHING_META, DOWNLOADING, PROCESSING, UPLOADING, DONE, FAILED, PAUSED, CANCELLED
	Priority       int        `json:"priority" gorm:"default:0"`
	OwnerID        string     `json:"owner_id" gorm:"index"` // 创建者用户 ID（空=管理员/系统）
	TrackInfo      JSON       `json:"track_info" gorm:"type:json;not null"`
	SelectedSource JSON       `json:"selected_source,omitempty" gorm:"type:json"`
	UploadTargets  JSON       `json:"upload_targets,omitempty" gorm:"type:json"`
	UploadDir      string     `json:"upload_dir,omitempty"` // 上传目标文件夹（相对存储根）
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

// Library 音乐库记录（kind：download 已下载 / favorite 收藏在线，owner 隔离）
type Library struct {
	ID            string    `json:"id" gorm:"primaryKey"`
	OwnerID       string    `json:"owner_id" gorm:"index"` // 归属用户 ID（空=全局/管理员下载）
	Kind          string    `json:"kind" gorm:"default:download"` // download / favorite
	Title         string    `json:"title" gorm:"not null"`
	Artist        string    `json:"artist"`
	Album         string    `json:"album"`
	Year          int       `json:"year"`
	Genre         string    `json:"genre"`
	Quality       string    `json:"quality"`
	Format        string    `json:"format"`
	FileSize      int64     `json:"file_size"`
	Duration      int       `json:"duration"`
	Source        string    `json:"source"`
	SourceTrackID string    `json:"source_track_id"` // 原始音乐源曲目 ID（如 GD音乐台-JOOX:xxx）
	RemotePaths   JSON      `json:"remote_paths,omitempty" gorm:"type:json"`
	CoverURL      string    `json:"cover_url"`
	LyricsLRC     string    `json:"lyrics_lrc"` // 下载时保存的 LRC 歌词文本
	HasLyrics     bool      `json:"has_lyrics"`
	Metadata      JSON      `json:"metadata,omitempty" gorm:"type:json"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	OwnerID   string    `json:"owner_id" gorm:"index"` // 归属用户（空=管理员创建的共享存储）
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

// QQQuota QQ 音乐源每月下载限额本地统计（腾讯侧无公开真实剩余接口，仅作参考），按用户+源统计
type QQQuota struct {
	ID         string    `json:"id" gorm:"primaryKey"` // {source_name}_{user_id}_{yyyymm}
	SourceName string    `json:"source_name" gorm:"index"`
	UserID     string    `json:"user_id" gorm:"index"`
	YearMonth  string    `json:"year_month" gorm:"index"`
	Count      int       `json:"count"`
	Limit      int       `json:"limit"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (QQQuota) TableName() string { return "qq_quotas" }

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
