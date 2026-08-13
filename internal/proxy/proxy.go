package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/db/models"
	"golang.org/x/net/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Manager 代理管理器，提供全局代理感知的 HTTP 客户端
type Manager struct {
	db  *gorm.DB
	log *zap.Logger

	mu       sync.RWMutex
	proxyURL *url.URL
	enabled  bool
	rawURL   string
}

// NewManager 创建代理管理器并从数据库加载配置
func NewManager(db *gorm.DB, log *zap.Logger) *Manager {
	m := &Manager{db: db, log: log}
	m.loadFromDB()
	return m
}

func (m *Manager) loadFromDB() {
	var setting models.Setting
	if err := m.db.Where("`key` = ?", "proxy").First(&setting).Error; err != nil {
		return
	}
	var cfg struct {
		URL     string `json:"url"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal(setting.Value, &cfg); err != nil {
		return
	}
	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err == nil {
			m.mu.Lock()
			m.proxyURL = u
			m.rawURL = cfg.URL
			m.enabled = cfg.Enabled
			m.mu.Unlock()
			m.log.Info("proxy loaded from db", zap.String("url", cfg.URL), zap.Bool("enabled", cfg.Enabled))
		}
	}
}

// SetProxy 设置代理并持久化
func (m *Manager) SetProxy(rawURL string, enabled bool) error {
	if rawURL != "" {
		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
		// 验证 scheme
		switch u.Scheme {
		case "http", "https", "socks5", "socks5h":
		default:
			return fmt.Errorf("unsupported proxy scheme: %s (支持 http/https/socks5)", u.Scheme)
		}
		m.mu.Lock()
		m.proxyURL = u
		m.rawURL = rawURL
		m.enabled = enabled
		m.mu.Unlock()
	} else {
		m.mu.Lock()
		m.proxyURL = nil
		m.rawURL = ""
		m.enabled = false
		m.mu.Unlock()
	}

	// 持久化
	data, _ := json.Marshal(map[string]interface{}{
		"url":     rawURL,
		"enabled": enabled,
	})
	now := time.Now()
	m.db.Where("`key` = ?", "proxy").Assign(models.Setting{
		Key:       "proxy",
		Value:     models.JSON(data),
		UpdatedAt: now,
	}).FirstOrCreate(&models.Setting{})

	return nil
}

// HTTPClient 返回代理感知的 HTTP 客户端
func (m *Manager) HTTPClient() *http.Client {
	return &http.Client{
		Transport: m.Transport(),
		Timeout:   30 * time.Second,
	}
}

// Transport 返回代理感知的 HTTP Transport
func (m *Manager) Transport() *http.Transport {
	t := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	m.mu.RLock()
	if m.enabled && m.proxyURL != nil {
		t.Proxy = http.ProxyURL(m.proxyURL)
	}
	m.mu.RUnlock()
	return t
}

// Dialer 返回基于代理的 TCP 拨号函数（供 MTProto/gotd 等非 HTTP 协议使用）。
// 未启用代理时返回 nil。
func (m *Manager) Dialer() func(ctx context.Context, network, addr string) (net.Conn, error) {
	m.mu.RLock()
	enabled := m.enabled
	u := m.proxyURL
	m.mu.RUnlock()
	if !enabled || u == nil {
		return nil
	}

	d, err := proxy.FromURL(u, proxy.Direct)
	if err != nil {
		m.log.Warn("build proxy dialer failed", zap.String("url", u.Redacted()), zap.Error(err))
		return nil
	}

	if cd, ok := d.(proxy.ContextDialer); ok {
		return func(ctx context.Context, network, addr string) (net.Conn, error) {
			return cd.DialContext(ctx, network, addr)
		}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.Dial(network, addr)
	}
}

// GetConfig 获取当前代理配置
func (m *Manager) GetConfig() (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rawURL, m.enabled
}

// TestProxy 测试代理连通性
func (m *Manager) TestProxy(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	t := &http.Transport{
		Proxy:           http.ProxyURL(u),
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
	client := &http.Client{Transport: t, Timeout: 10 * time.Second}

	resp, err := client.Get("https://api.telegram.org")
	if err != nil {
		return fmt.Errorf("proxy connection failed: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
