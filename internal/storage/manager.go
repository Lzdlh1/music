package storage

import (
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Manager 多存储目标分发管理器
type Manager struct {
	backends map[string]Backend
	mu       sync.RWMutex
	log      *zap.Logger
}

// NewManager 创建存储管理器
func NewManager(log *zap.Logger) *Manager {
	return &Manager{
		backends: make(map[string]Backend),
		log:      log,
	}
}

// Register 注册存储后端
func (m *Manager) Register(b Backend) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backends[b.ID()] = b
	m.log.Info("storage backend registered",
		zap.String("id", b.ID()),
		zap.String("name", b.Name()),
		zap.String("type", string(b.Type())))
}

// Remove 移除存储后端
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.backends, id)
}

// Get 获取存储后端
func (m *Manager) Get(id string) (Backend, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.backends[id]
	return b, ok
}

// List 列出所有存储后端
func (m *Manager) List() []Backend {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []Backend
	for _, b := range m.backends {
		list = append(list, b)
	}
	return list
}

// NamingTemplate 文件命名模板
type NamingTemplate struct {
	Template string
}

// TrackNamingInfo 用于命名模板的曲目信息
type TrackNamingInfo struct {
	Artist      string
	AlbumArtist string
	Album       string
	Title       string
	Year        string
	TrackNo     int
	DiscNo      int
	Genre       string
	Ext         string
	Quality     string
	Source      string
}

// Format 根据模板生成文件路径
func (t *NamingTemplate) Format(info TrackNamingInfo) string {
	result := t.Template
	result = strings.ReplaceAll(result, "{artist}", sanitizePath(info.Artist))
	result = strings.ReplaceAll(result, "{album_artist}", sanitizePath(info.AlbumArtist))
	result = strings.ReplaceAll(result, "{album}", sanitizePath(info.Album))
	result = strings.ReplaceAll(result, "{title}", sanitizePath(info.Title))
	result = strings.ReplaceAll(result, "{year}", info.Year)
	result = strings.ReplaceAll(result, "{genre}", sanitizePath(info.Genre))
	result = strings.ReplaceAll(result, "{ext}", info.Ext)
	result = strings.ReplaceAll(result, "{quality}", info.Quality)
	result = strings.ReplaceAll(result, "{source}", info.Source)
	result = strings.ReplaceAll(result, "{track_no:02d}", fmt.Sprintf("%02d", info.TrackNo))
	result = strings.ReplaceAll(result, "{track_no}", fmt.Sprintf("%d", info.TrackNo))
	result = strings.ReplaceAll(result, "{disc_no}", fmt.Sprintf("%d", info.DiscNo))
	return result
}

// sanitizePath 清理文件名中不合法的字符
func sanitizePath(s string) string {
	if s == "" {
		return "Unknown"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(s)
}
