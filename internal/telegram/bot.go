package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// BotConfig Telegram Bot 配置
type BotConfig struct {
	Token   string `json:"token"`
	ChatIDs []int64 `json:"chat_ids"`
}

// Bot Telegram Bot 管理器
type Bot struct {
	db       *gorm.DB
	log      *zap.Logger
	proxyMgr *proxy.Manager
	cancel   context.CancelFunc
	mu       sync.Mutex

	token  string
	chatID int64
}

// NewBot 创建 Bot 实例
func NewBot(db *gorm.DB, proxyMgr *proxy.Manager, log *zap.Logger) *Bot {
	return &Bot{db: db, proxyMgr: proxyMgr, log: log}
}

// httpClient 返回代理感知的 HTTP 客户端
func (b *Bot) httpClient() *http.Client {
	if b.proxyMgr != nil {
		return b.proxyMgr.HTTPClient()
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Start 启动 Bot（如配置存在）
func (b *Bot) Start(ctx context.Context) error {
	var bot models.TGBot
	if err := b.db.Where("enabled = ?", true).Order("priority DESC").First(&bot).Error; err != nil {
		b.log.Info("no enabled telegram bot configured, skipping")
		return nil
	}

	var cfg BotConfig
	if err := json.Unmarshal(bot.Config, &cfg); err != nil {
		return fmt.Errorf("parse bot config: %w", err)
	}

	if cfg.Token == "" {
		return fmt.Errorf("bot token is empty")
	}

	b.mu.Lock()
	b.token = cfg.Token
	if len(cfg.ChatIDs) > 0 {
		b.chatID = cfg.ChatIDs[0]
	}
	b.mu.Unlock()

	b.log.Info("telegram bot initialized",
		zap.String("username", bot.Username),
		zap.Int64("chat_id", b.chatID))

	return nil
}

// Stop 停止 Bot
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

// SendMessage 发送文本消息
func (b *Bot) SendMessage(chatID int64, text string) error {
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()

	if token == "" {
		return fmt.Errorf("bot not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := fmt.Sprintf(`{"chat_id":%d,"text":%q,"parse_mode":"HTML"}`, chatID, text)
	resp, err := b.httpClient().Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// SendAudio 发送音频文件到频道/群组
func (b *Bot) SendAudio(chatID int64, filePath string, title, artist string, duration int) error {
	b.mu.Lock()
	token := b.token
	b.mu.Unlock()

	if token == "" {
		return fmt.Errorf("bot not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendAudio", token)

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// 使用 multipart form 上传
	body, contentType, err := createMultipartAudio(f, filepath.Base(filePath), chatID, title, artist, duration)
	if err != nil {
		return fmt.Errorf("create multipart: %w", err)
	}

	resp, err := b.httpClient().Post(url, contentType, body)
	if err != nil {
		return fmt.Errorf("send audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram api error %d: %s", resp.StatusCode, string(respBody))
	}

	b.log.Info("audio sent to telegram",
		zap.Int64("chat_id", chatID),
		zap.String("title", title),
		zap.String("artist", artist))
	return nil
}

// NotifyDownloadComplete 通知下载完成
func (b *Bot) NotifyDownloadComplete(title, artist, quality string) error {
	b.mu.Lock()
	chatID := b.chatID
	b.mu.Unlock()

	if chatID == 0 {
		return nil
	}

	text := fmt.Sprintf("✅ <b>下载完成</b>\n🎵 %s - %s\n📀 %s", artist, title, quality)
	return b.SendMessage(chatID, text)
}

// NotifyDownloadFailed 通知下载失败
func (b *Bot) NotifyDownloadFailed(title, artist, errMsg string) error {
	b.mu.Lock()
	chatID := b.chatID
	b.mu.Unlock()

	if chatID == 0 {
		return nil
	}

	text := fmt.Sprintf("❌ <b>下载失败</b>\n🎵 %s - %s\n💬 %s", artist, title, errMsg)
	return b.SendMessage(chatID, text)
}

// TestBot 测试 Bot Token 是否有效
func (b *Bot) TestBot(token string) (*BotInfo, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", token)

	client := b.httpClient()
	client.Timeout = 10 * time.Second
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool    `json:"ok"`
		Result BotInfo `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("invalid bot token")
	}

	return &result.Result, nil
}

// BotInfo Bot 信息
type BotInfo struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}
