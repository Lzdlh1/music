package telegram

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/tg"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChannelManager 管理 Telegram 频道资源抓取
type ChannelManager struct {
	db       *gorm.DB
	log      *zap.Logger
	proxyMgr *proxy.Manager
	mtMgr    *MTProtoManager
	cancel   context.CancelFunc
	mu       sync.Mutex
	token    string
}

// NewChannelManager 创建频道管理器
func NewChannelManager(db *gorm.DB, proxyMgr *proxy.Manager, mtMgr *MTProtoManager, log *zap.Logger) *ChannelManager {
	return &ChannelManager{db: db, proxyMgr: proxyMgr, mtMgr: mtMgr, log: log}
}

func (cm *ChannelManager) httpClient(timeout time.Duration) *http.Client {
	if cm.proxyMgr != nil {
		c := cm.proxyMgr.HTTPClient()
		c.Timeout = timeout
		return c
	}
	return &http.Client{Timeout: timeout}
}

// Start 启动频道轮询
func (cm *ChannelManager) Start(ctx context.Context) error {
	// 获取可用 bot token
	var bot models.TGBot
	if err := cm.db.Where("enabled = ?", true).Order("priority DESC").First(&bot).Error; err != nil {
		cm.log.Info("no bot configured, channel manager idle")
		return nil
	}

	var cfg BotConfig
	if err := json.Unmarshal(bot.Config, &cfg); err != nil || cfg.Token == "" {
		return nil
	}

	cm.mu.Lock()
	cm.token = cfg.Token
	cm.mu.Unlock()

	pollCtx, cancel := context.WithCancel(ctx)
	cm.cancel = cancel
	go cm.pollLoop(pollCtx)

	cm.log.Info("channel manager started")
	return nil
}

// Stop 停止轮询
func (cm *ChannelManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.cancel != nil {
		cm.cancel()
		cm.cancel = nil
	}
}

// ReloadToken 重新加载 token（当 bot 配置变更时调用）
func (cm *ChannelManager) ReloadToken() {
	var bot models.TGBot
	if err := cm.db.Where("enabled = ?", true).Order("priority DESC").First(&bot).Error; err != nil {
		return
	}
	var cfg BotConfig
	if err := json.Unmarshal(bot.Config, &cfg); err != nil || cfg.Token == "" {
		return
	}
	cm.mu.Lock()
	cm.token = cfg.Token
	cm.mu.Unlock()
}

// pollLoop 长轮询循环
func (cm *ChannelManager) pollLoop(ctx context.Context) {
	offset := cm.loadOffset()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cm.mu.Lock()
		token := cm.token
		cm.mu.Unlock()

		if token == "" {
			time.Sleep(30 * time.Second)
			continue
		}

		updates, err := cm.getUpdates(token, offset)
		if err != nil {
			cm.log.Warn("getUpdates failed", zap.Error(err))
			time.Sleep(10 * time.Second)
			continue
		}

		for _, u := range updates {
			if u.ChannelPost != nil {
				cm.processChannelPost(u)
			} else if u.Message != nil {
				cm.processPrivateMessage(u)
			}
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
				cm.saveOffset(offset)
			}
		}

		if len(updates) == 0 {
			time.Sleep(5 * time.Second)
		}
	}
}

// tgUpdate Telegram Update 结构
type tgUpdate struct {
	UpdateID    int64      `json:"update_id"`
	ChannelPost *tgMessage `json:"channel_post"`
	Message     *tgMessage `json:"message"`
}

// tgMessage Telegram 消息
type tgMessage struct {
	MessageID   int64           `json:"message_id"`
	Chat        tgChat          `json:"chat"`
	From        *tgUser         `json:"from"`
	Date        int64           `json:"date"`
	Audio       *tgAudioInfo    `json:"audio"`
	Document    *tgDocumentInfo `json:"document"`
	Caption     string          `json:"caption"`
	Text        string          `json:"text"`
	ForwardFrom *tgUser         `json:"forward_from"`
	ForwardChat *tgChat         `json:"forward_from_chat"`
}

// tgUser Telegram 用户
type tgUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// tgChat Telegram 聊天信息
type tgChat struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Username string `json:"username"`
	Type     string `json:"type"`
}

// tgAudioInfo Telegram 音频信息
type tgAudioInfo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	Performer    string `json:"performer"`
	Title        string `json:"title"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	FileName     string `json:"file_name"`
}

// tgDocumentInfo Telegram 文档信息
type tgDocumentInfo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

// getUpdates 调用 Telegram Bot API 获取更新
func (cm *ChannelManager) getUpdates(token string, offset int64) ([]tgUpdate, error) {
	url := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30&allowed_updates=[\"channel_post\",\"message\"]",
		token, offset,
	)

	client := cm.httpClient(40 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram API returned ok=false")
	}
	return result.Result, nil
}

// processChannelPost 处理频道消息
func (cm *ChannelManager) processChannelPost(u tgUpdate) {
	msg := u.ChannelPost
	if msg == nil {
		return
	}

	// 检查是否是已订阅的频道
	var channel models.TGChannel
	if err := cm.db.Where("chat_id = ? AND enabled = ?", msg.Chat.ID, true).First(&channel).Error; err != nil {
		return // 不是已订阅的频道
	}

	// 检查是否是音频文件
	var file *models.TGChannelFile

	if msg.Audio != nil {
		a := msg.Audio
		title, artist := a.Title, a.Performer
		if title == "" {
			title = parseFilenameTitle(a.FileName)
		}
		if artist == "" {
			artist = parseFilenameArtist(a.FileName)
		}

		file = &models.TGChannelFile{
			ChannelID:    channel.ID,
			ChatID:       msg.Chat.ID,
			MessageID:    msg.MessageID,
			FileID:       a.FileID,
			FileUniqueID: a.FileUniqueID,
			FileName:     a.FileName,
			FileSize:     a.FileSize,
			MimeType:     a.MimeType,
			Duration:     a.Duration,
			Title:        title,
			Artist:       artist,
			Caption:      msg.Caption,
			PostedAt:     time.Unix(msg.Date, 0),
		}
	} else if msg.Document != nil {
		d := msg.Document
		if !isAudioMime(d.MimeType) && !isAudioExt(d.FileName) {
			return
		}
		file = &models.TGChannelFile{
			ChannelID:    channel.ID,
			ChatID:       msg.Chat.ID,
			MessageID:    msg.MessageID,
			FileID:       d.FileID,
			FileUniqueID: d.FileUniqueID,
			FileName:     d.FileName,
			FileSize:     d.FileSize,
			MimeType:     d.MimeType,
			Title:        parseFilenameTitle(d.FileName),
			Artist:       parseFilenameArtist(d.FileName),
			Caption:      msg.Caption,
			PostedAt:     time.Unix(msg.Date, 0),
		}
	}

	if file == nil {
		return
	}

	// 去重：按 file_unique_id 检查
	var existing int64
	cm.db.Model(&models.TGChannelFile{}).Where("file_unique_id = ?", file.FileUniqueID).Count(&existing)
	if existing > 0 {
		return
	}

	if err := cm.db.Create(file).Error; err != nil {
		cm.log.Warn("save channel file", zap.Error(err))
		return
	}

	cm.log.Info("new audio from channel",
		zap.String("channel", channel.Title),
		zap.String("title", file.Title),
		zap.String("artist", file.Artist),
		zap.Int64("size", file.FileSize))

	// 更新频道文件计数
	cm.db.Model(&models.TGChannel{}).Where("id = ?", channel.ID).
		Update("file_count", gorm.Expr("file_count + 1"))
}

// processPrivateMessage 处理私聊消息（用户转发音频给 Bot）
func (cm *ChannelManager) processPrivateMessage(u tgUpdate) {
	msg := u.Message
	if msg == nil || msg.Chat.Type != "private" {
		return
	}

	// 处理 /start 命令
	if msg.Text == "/start" {
		cm.sendReply(msg.Chat.ID, "👋 欢迎使用 MusicFlow Bot！\n\n"+
			"🎵 <b>使用方法：</b>\n"+
			"1. 去 TG 资源频道的机器人搜索音乐\n"+
			"2. 把收到的音频文件<b>转发</b>给我\n"+
			"3. 我会自动保存到你的音乐库\n\n"+
			"📁 支持：MP3、FLAC、WAV、APE、M4A 等格式\n"+
			"💡 直接发送音频文件也可以")
		return
	}

	// 处理 /help 命令
	if msg.Text == "/help" {
		cm.sendReply(msg.Chat.ID, "📖 <b>命令列表</b>\n\n"+
			"/start - 开始使用\n"+
			"/help - 帮助信息\n"+
			"/list - 查看最近接收的文件\n\n"+
			"💡 转发或发送音频文件给我即可自动收录")
		return
	}

	// 处理 /list 命令
	if msg.Text == "/list" {
		var files []models.TGChannelFile
		cm.db.Where("channel_id = ?", "private").Order("created_at DESC").Limit(10).Find(&files)
		if len(files) == 0 {
			cm.sendReply(msg.Chat.ID, "📭 暂无收到的文件")
			return
		}
		text := "📋 <b>最近收到的文件：</b>\n\n"
		for i, f := range files {
			status := "⏳"
			if f.Downloaded {
				status = "✅"
			}
			text += fmt.Sprintf("%d. %s %s - %s (%s)\n", i+1, status, f.Artist, f.Title, formatFileSize(f.FileSize))
		}
		cm.sendReply(msg.Chat.ID, text)
		return
	}

	// 处理音频消息（直接发送或转发的）
	if msg.Audio == nil && msg.Document == nil {
		if msg.Text != "" {
			cm.sendReply(msg.Chat.ID, "💡 请发送或转发音频文件给我。\n文本消息暂不支持搜索功能。")
		}
		return
	}

	var file *models.TGChannelFile
	sourceInfo := ""

	if msg.ForwardFrom != nil {
		sourceInfo = fmt.Sprintf("（来自 @%s）", msg.ForwardFrom.Username)
	} else if msg.ForwardChat != nil {
		sourceInfo = fmt.Sprintf("（来自 %s）", msg.ForwardChat.Title)
	}

	if msg.Audio != nil {
		a := msg.Audio
		title, artist := a.Title, a.Performer
		if title == "" {
			title = parseFilenameTitle(a.FileName)
		}
		if artist == "" {
			artist = parseFilenameArtist(a.FileName)
		}

		file = &models.TGChannelFile{
			ChannelID:    "private",
			ChatID:       msg.Chat.ID,
			MessageID:    msg.MessageID,
			FileID:       a.FileID,
			FileUniqueID: a.FileUniqueID,
			FileName:     a.FileName,
			FileSize:     a.FileSize,
			MimeType:     a.MimeType,
			Duration:     a.Duration,
			Title:        title,
			Artist:       artist,
			Caption:      msg.Caption,
			PostedAt:     time.Unix(msg.Date, 0),
		}
	} else if msg.Document != nil {
		d := msg.Document
		if !isAudioMime(d.MimeType) && !isAudioExt(d.FileName) {
			cm.sendReply(msg.Chat.ID, "❌ 不支持的文件类型："+d.MimeType+"\n请发送音频文件（MP3/FLAC/WAV 等）")
			return
		}
		file = &models.TGChannelFile{
			ChannelID:    "private",
			ChatID:       msg.Chat.ID,
			MessageID:    msg.MessageID,
			FileID:       d.FileID,
			FileUniqueID: d.FileUniqueID,
			FileName:     d.FileName,
			FileSize:     d.FileSize,
			MimeType:     d.MimeType,
			Title:        parseFilenameTitle(d.FileName),
			Artist:       parseFilenameArtist(d.FileName),
			Caption:      msg.Caption,
			PostedAt:     time.Unix(msg.Date, 0),
		}
	}

	if file == nil {
		return
	}

	// 去重
	var existing int64
	cm.db.Model(&models.TGChannelFile{}).Where("file_unique_id = ?", file.FileUniqueID).Count(&existing)
	if existing > 0 {
		cm.sendReply(msg.Chat.ID, "ℹ️ 该文件已收录过了")
		return
	}

	if err := cm.db.Create(file).Error; err != nil {
		cm.log.Warn("save private file", zap.Error(err))
		cm.sendReply(msg.Chat.ID, "❌ 保存失败，请重试")
		return
	}

	reply := fmt.Sprintf("✅ <b>已收录</b> %s\n🎵 %s - %s\n📁 %s\n💾 %s",
		sourceInfo, file.Artist, file.Title,
		file.FileName, formatFileSize(file.FileSize))
	cm.sendReply(msg.Chat.ID, reply)

	cm.log.Info("audio received from private message",
		zap.String("title", file.Title),
		zap.String("artist", file.Artist),
		zap.Int64("size", file.FileSize))
}

// sendReply 回复消息
func (cm *ChannelManager) sendReply(chatID int64, text string) {
	cm.mu.Lock()
	token := cm.token
	cm.mu.Unlock()

	if token == "" {
		return
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := fmt.Sprintf(`{"chat_id":%d,"text":%q,"parse_mode":"HTML"}`, chatID, text)
	resp, err := cm.httpClient(10*time.Second).Post(apiURL, "application/json", strings.NewReader(payload))
	if err != nil {
		cm.log.Warn("send reply failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

func formatFileSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
}

// ScanChannel 手动扫描频道（通过 getChat 验证 + 录入信息）
func (cm *ChannelManager) ScanChannel(chatIdentifier string) (*models.TGChannel, error) {
	cm.mu.Lock()
	token := cm.token
	cm.mu.Unlock()

	if token == "" {
		return nil, fmt.Errorf("no bot token configured")
	}

	// 获取频道信息
	chatInfo, err := cm.getChat(token, chatIdentifier)
	if err != nil {
		return nil, fmt.Errorf("get chat info: %w", err)
	}

	if chatInfo.Type != "channel" && chatInfo.Type != "supergroup" {
		return nil, fmt.Errorf("not a channel or supergroup (type: %s)", chatInfo.Type)
	}

	channel := &models.TGChannel{
		ChatID:   chatInfo.ID,
		Title:    chatInfo.Title,
		Username: chatInfo.Username,
		Enabled:  true,
	}

	return channel, nil
}

// HasBotToken 是否已配置 Bot token
func (cm *ChannelManager) HasBotToken() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.token != ""
}

// ResolveChannelMTProto 使用 MTProto 账号解析频道（无 Bot 时使用）
func (cm *ChannelManager) ResolveChannelMTProto(ctx context.Context, identifier string) (*models.TGChannel, error) {
	clientInst, err := cm.mtMgr.GetClient()
	if err != nil {
		return nil, fmt.Errorf("get mtproto client failed: %w", err)
	}

	username := strings.TrimPrefix(identifier, "@")
	resolved, err := clientInst.API.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return nil, fmt.Errorf("resolve %s failed: %w", identifier, err)
	}

	// 频道/群组
	for _, chat := range resolved.Chats {
		if c, ok := chat.(*tg.Channel); ok {
			return &models.TGChannel{
				ChatID:   c.ID,
				Title:    c.Title,
				Username: "@" + c.Username,
				Enabled:  true,
			}, nil
		}
	}

	// 用户 / Bot（私聊资源）
	for _, u := range resolved.Users {
		if usr, ok := u.(*tg.User); ok {
			title := usr.Username
			if title == "" {
				title = usr.FirstName
			}
			if usr.Bot {
				title = "@" + title
			}
			return &models.TGChannel{
				ChatID:   usr.ID,
				Title:    title,
				Username: "@" + usr.Username,
				Enabled:  true,
			}, nil
		}
	}
	return nil, fmt.Errorf("not a valid channel or user")
}

// getChat 获取聊天信息
func (cm *ChannelManager) getChat(token, chatID string) (*tgChat, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getChat?chat_id=%s", token, chatID)

	client := cm.httpClient(10 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Result      tgChat `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram: %s", result.Description)
	}
	return &result.Result, nil
}

// GetFileURL 获取文件下载链接
func (cm *ChannelManager) GetFileURL(fileID string) (string, error) {
	cm.mu.Lock()
	token := cm.token
	cm.mu.Unlock()

	if token == "" {
		return "", fmt.Errorf("no bot token configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", token, fileID)

	client := cm.httpClient(10 * time.Second)
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
			FileSize int64  `json:"file_size"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK {
		return "", fmt.Errorf("getFile failed")
	}

	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, result.Result.FilePath)
	return downloadURL, nil
}

// DownloadFile 下载 Telegram 文件到本地
func (cm *ChannelManager) DownloadFile(ctx context.Context, fileID, destPath string) error {
	downloadURL, err := cm.GetFileURL(fileID)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	client := cm.httpClient(10 * time.Minute)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// loadOffset 加载轮询 offset
func (cm *ChannelManager) loadOffset() int64 {
	var setting models.Setting
	if err := cm.db.Where("key = ?", "tg_poll_offset").First(&setting).Error; err != nil {
		return 0
	}
	var offset int64
	json.Unmarshal(setting.Value, &offset)
	return offset
}

// saveOffset 保存轮询 offset
func (cm *ChannelManager) saveOffset(offset int64) {
	data, _ := json.Marshal(offset)
	cm.db.Where("key = ?", "tg_poll_offset").Assign(models.Setting{
		Key:       "tg_poll_offset",
		Value:     models.JSON(data),
		UpdatedAt: time.Now(),
	}).FirstOrCreate(&models.Setting{})
}

// isAudioMime 判断是否是音频 MIME
func isAudioMime(mime string) bool {
	return strings.HasPrefix(mime, "audio/")
}

// isAudioExt 判断文件扩展名是否是音频
func isAudioExt(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".ape", ".wma", ".m4a", ".opus", ".dsf", ".dff", ".aiff":
		return true
	}
	return false
}

// parseFilenameTitle 从文件名解析标题
func parseFilenameTitle(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	// 尝试 "Artist - Title" 格式
	parts := strings.SplitN(name, " - ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(name)
}

// parseFilenameArtist 从文件名解析艺术家
func parseFilenameArtist(filename string) string {
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	parts := strings.SplitN(name, " - ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// GetChatIDStr 将 chat identifier 转为字符串（支持 @username 或数字 ID）
func GetChatIDStr(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return ""
	}
	// 如果是纯数字（含负号），直接返回
	if _, err := strconv.ParseInt(identifier, 10, 64); err == nil {
		return identifier
	}
	// 否则加上 @ 前缀
	if !strings.HasPrefix(identifier, "@") {
		identifier = "@" + identifier
	}
	return identifier
}

// ScanChannelHistory 扫描频道历史记录
func (cm *ChannelManager) ScanChannelHistory(ctx context.Context, channelID string) error {
	var channel models.TGChannel
	if err := cm.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		return fmt.Errorf("channel not found: %w", err)
	}

	clientInst, err := cm.mtMgr.GetClient()
	if err != nil {
		return fmt.Errorf("get mtproto client failed: %w", err)
	}

	identifier := channel.Username
	if identifier == "" {
		identifier = fmt.Sprintf("%d", channel.ChatID)
	} else if !strings.HasPrefix(identifier, "@") {
		identifier = "@" + identifier
	}

	resolved, err := clientInst.API.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: strings.TrimPrefix(identifier, "@")})
	if err != nil {
		return fmt.Errorf("resolve channel %s failed: %w", identifier, err)
	}
	if len(resolved.Chats) == 0 && len(resolved.Users) == 0 {
		return fmt.Errorf("channel not found")
	}

	var inputPeer tg.InputPeerClass
	for _, chat := range resolved.Chats {
		if c, ok := chat.(*tg.Channel); ok {
			inputPeer = &tg.InputPeerChannel{
				ChannelID:  c.ID,
				AccessHash: c.AccessHash,
			}
			break
		}
	}
	if inputPeer == nil {
		// 私聊 / Bot 用户
		for _, u := range resolved.Users {
			if usr, ok := u.(*tg.User); ok {
				inputPeer = &tg.InputPeerUser{
					UserID:     usr.ID,
					AccessHash: usr.AccessHash,
				}
				break
			}
		}
	}
	if inputPeer == nil {
		return fmt.Errorf("not a valid channel")
	}

	// 开始获取历史
	offsetID := 0
	limit := 100
	totalSaved := 0

	cm.log.Info("start scanning channel history", zap.String("channel", channel.Title))

	for {
		req := &tg.MessagesGetHistoryRequest{
			Peer:     inputPeer,
			OffsetID: offsetID,
			Limit:    limit,
		}

		res, err := clientInst.API.MessagesGetHistory(ctx, req)
		if err != nil {
			return fmt.Errorf("get history failed: %w", err)
		}

		var messages []tg.MessageClass
		switch r := res.(type) {
		case *tg.MessagesMessages:
			messages = r.Messages
		case *tg.MessagesMessagesSlice:
			messages = r.Messages
		case *tg.MessagesChannelMessages:
			messages = r.Messages
		default:
			break
		}

		if len(messages) == 0 {
			break
		}

		for _, m := range messages {
			msg, ok := m.(*tg.Message)
			if !ok {
				continue
			}

			offsetID = msg.ID

			if msg.Media == nil {
				continue
			}

			// 解析媒体
			var file *models.TGChannelFile

			if docMedia, ok := msg.Media.(*tg.MessageMediaDocument); ok {
				if doc, ok := docMedia.Document.(*tg.Document); ok {
					var fileName, title, performer string
					var duration int

					for _, attr := range doc.Attributes {
						switch a := attr.(type) {
						case *tg.DocumentAttributeFilename:
							fileName = a.FileName
						case *tg.DocumentAttributeAudio:
							title = a.Title
							performer = a.Performer
							duration = a.Duration
						}
					}

					if isAudioMime(doc.MimeType) || isAudioExt(fileName) {
						if title == "" {
							title = parseFilenameTitle(fileName)
						}
						if performer == "" {
							performer = parseFilenameArtist(fileName)
						}

						fileID := fmt.Sprintf("%d", doc.ID)
						fileUniqueID := fmt.Sprintf("%d_%d", doc.ID, doc.AccessHash) // 用 ID+AccessHash 作为 UniqueID

						file = &models.TGChannelFile{
							ChannelID:      channel.ID,
							ChatID:         channel.ChatID,
							MessageID:      int64(msg.ID),
							FileID:         fileID,
							FileUniqueID:   fileUniqueID,
							FileAccessHash: doc.AccessHash,
							FileReference:  hex.EncodeToString(doc.FileReference),
							FileName:       fileName,
							FileSize:       doc.Size,
							MimeType:       doc.MimeType,
							Duration:       duration,
							Title:          title,
							Artist:         performer,
							Caption:        msg.Message,
							PostedAt:       time.Unix(int64(msg.Date), 0),
						}
					}
				}
			}

			if file != nil {
				// 去重
				var existing int64
				cm.db.Model(&models.TGChannelFile{}).Where("file_unique_id = ?", file.FileUniqueID).Count(&existing)
				if existing == 0 {
					if err := cm.db.Create(file).Error; err == nil {
						totalSaved++
						// 更新频道计数
						cm.db.Model(&models.TGChannel{}).Where("id = ?", channel.ID).
							Update("file_count", gorm.Expr("file_count + 1"))
					}
				}
			}
		}

		time.Sleep(1 * time.Second) // 避免触发限流
	}

	cm.log.Info("scan channel history completed", zap.String("channel", channel.Title), zap.Int("saved", totalSaved))
	return nil
}

// downloadChunkSize MTProto 分块下载的单块大小（1MB）
const downloadChunkSize = 1 << 20

// DownloadFileMTProto 通过 MTProto 账号（非 Bot token）分块下载文件到本地
func (cm *ChannelManager) DownloadFileMTProto(ctx context.Context, file *models.TGChannelFile, dest string) error {
	clientInst, err := cm.mtMgr.GetClient()
	if err != nil {
		return fmt.Errorf("get mtproto client failed: %w", err)
	}

	fileRef, err := hex.DecodeString(file.FileReference)
	if err != nil {
		return fmt.Errorf("decode file reference failed: %w", err)
	}

	location := &tg.InputDocumentFileLocation{
		ID:            file.FileIDInt(),
		AccessHash:    file.FileAccessHash,
		FileReference: fileRef,
	}

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest file failed: %w", err)
	}
	defer out.Close()

	var offset int64
	for {
		if offset >= file.FileSize && file.FileSize > 0 {
			break // 已下载完整文件
		}
		req := &tg.UploadGetFileRequest{
			Location: location,
			Offset:   offset,
			Limit:    downloadChunkSize,
			Precise:  true,
		}
		res, err := clientInst.API.UploadGetFile(ctx, req)
		if err != nil {
			if strings.Contains(err.Error(), "OFFSET_INVALID") {
				break // 已到文件末尾
			}
			return fmt.Errorf("download chunk at offset %d failed: %w", offset, err)
		}

		uf, ok := res.(*tg.UploadFile)
		if !ok {
			return fmt.Errorf("unexpected upload response type: %T", res)
		}
		if len(uf.Bytes) == 0 {
			break // 下载完成
		}
		if _, err := out.Write(uf.Bytes); err != nil {
			return fmt.Errorf("write chunk failed: %w", err)
		}
		offset += int64(len(uf.Bytes))
	}

	cm.log.Info("mtproto file downloaded",
		zap.String("file", file.FileName), zap.Int64("size", offset))
	return nil
}

// DownloadByCommand 向 Bot 发送搜索命令，自动选择匹配的曲目并点击下载按钮，
// 等待 Bot 返回音频文件后入库。适用于不支持/未配置 Bot token 的场景。
func (cm *ChannelManager) DownloadByCommand(ctx context.Context, channelID, query string, waitTimeout time.Duration) ([]*models.TGChannelFile, error) {
	var channel models.TGChannel
	if err := cm.db.Where("id = ?", channelID).First(&channel).Error; err != nil {
		return nil, fmt.Errorf("channel not found: %w", err)
	}

	clientInst, err := cm.mtMgr.GetClient()
	if err != nil {
		return nil, fmt.Errorf("get mtproto client failed: %w", err)
	}

	peer, err := cm.resolvePeer(ctx, clientInst.API, &channel)
	if err != nil {
		return nil, err
	}

	// 记录发送命令前的最新消息 id，之后只处理新消息
	baseline, err := cm.peerTopMessageID(ctx, clientInst.API, peer)
	if err != nil {
		return nil, err
	}

	// 发送搜索命令（直接发歌名）
	if _, err := clientInst.API.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  query,
		RandomID: time.Now().UnixNano(),
	}); err != nil {
		return nil, fmt.Errorf("send command failed: %w", err)
	}
	cm.log.Info("bot command sent", zap.String("channel", channel.Title), zap.String("query", query))

	ctx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	// 1. 等待搜索结果，选择匹配的曲目按钮
	tackData, tackMsgID, err := cm.waitForSearchResult(ctx, clientInst.API, peer, baseline, query)
	if err != nil {
		return nil, err
	}
	cm.log.Info("track selected", zap.String("data", tackData), zap.Int("msg_id", tackMsgID))

	if err := cm.clickCallback(ctx, clientInst.API, peer, tackMsgID, tackData); err != nil {
		return nil, fmt.Errorf("select track failed: %w", err)
	}

	// 2. 等待详情卡片，触发 Download Track
	dlData, dlMsgID, err := cm.waitForDownloadButton(ctx, clientInst.API, peer, baseline)
	if err != nil {
		return nil, err
	}
	cm.log.Info("download triggered", zap.String("data", dlData), zap.Int("msg_id", dlMsgID))

	if err := cm.clickCallback(ctx, clientInst.API, peer, dlMsgID, dlData); err != nil {
		return nil, fmt.Errorf("trigger download failed: %w", err)
	}

	// 3. 等待 Bot 返回音频文件并入库
	files, err := cm.waitForAudioFiles(ctx, clientInst.API, peer, baseline, &channel, dlData, dlMsgID)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("等待下载超时，未获取到音频文件（免费用户下载可能需要 1-3 分钟）")
	}
	return files, nil
}

// resolvePeer 解析频道的 MTProto peer（频道或用户/Bot）
func (cm *ChannelManager) resolvePeer(ctx context.Context, api *tg.Client, channel *models.TGChannel) (tg.InputPeerClass, error) {
	identifier := channel.Username
	if identifier == "" {
		identifier = fmt.Sprintf("%d", channel.ChatID)
	} else if !strings.HasPrefix(identifier, "@") {
		identifier = "@" + identifier
	}

	resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: strings.TrimPrefix(identifier, "@")})
	if err != nil {
		return nil, fmt.Errorf("resolve %s failed: %w", identifier, err)
	}
	for _, chat := range resolved.Chats {
		if c, ok := chat.(*tg.Channel); ok {
			return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}, nil
		}
	}
	for _, u := range resolved.Users {
		if usr, ok := u.(*tg.User); ok {
			return &tg.InputPeerUser{UserID: usr.ID, AccessHash: usr.AccessHash}, nil
		}
	}
	return nil, fmt.Errorf("not a valid channel or user")
}

// peerTopMessageID 获取对话中最新一条消息的 id
func (cm *ChannelManager) peerTopMessageID(ctx context.Context, api *tg.Client, peer tg.InputPeerClass) (int, error) {
	msgs, err := cm.getLatestMessages(ctx, api, peer, 1)
	if err != nil {
		return 0, err
	}
	if len(msgs) > 0 {
		return msgs[0].ID, nil
	}
	return 0, nil
}

// getLatestMessages 拉取对话最新的 N 条消息
func (cm *ChannelManager) getLatestMessages(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, limit int) ([]*tg.Message, error) {
	res, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:     peer,
		Limit:    limit,
		OffsetID: 0,
	})
	if err != nil {
		return nil, err
	}

	var out []*tg.Message
	switch r := res.(type) {
	case *tg.MessagesMessages:
		for _, m := range r.Messages {
			if mm, ok := m.(*tg.Message); ok {
				out = append(out, mm)
			}
		}
	case *tg.MessagesMessagesSlice:
		for _, m := range r.Messages {
			if mm, ok := m.(*tg.Message); ok {
				out = append(out, mm)
			}
		}
	case *tg.MessagesChannelMessages:
		for _, m := range r.Messages {
			if mm, ok := m.(*tg.Message); ok {
				out = append(out, mm)
			}
		}
	}
	return out, nil
}

// callbackButtons 提取消息中的内联 callback 按钮（data -> text）
func callbackButtons(msg *tg.Message) map[string]string {
	btns := map[string]string{}
	if msg == nil || msg.ReplyMarkup == nil {
		return btns
	}
	im, ok := msg.ReplyMarkup.(*tg.ReplyInlineMarkup)
	if !ok {
		return btns
	}
	for _, row := range im.Rows {
		for _, b := range row.Buttons {
			if cb, ok := b.(*tg.KeyboardButtonCallback); ok {
				btns[string(cb.Data)] = cb.Text
			}
		}
	}
	return btns
}

// stripEmoji 粗略移除按钮文本中的 emoji（非 BMP 字符）
func stripEmoji(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 0x1F000 {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// waitForSearchResult 轮询搜索结果，返回匹配 query 的曲目按钮（tack_*）
// 优先级：标题精确匹配 query 且非 Live/Remix 变体 > 包含 query 且非变体 > 第一个 tack_track
func (cm *ChannelManager) waitForSearchResult(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, baseline int, query string) (string, int, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))
	for {
		select {
		case <-ctx.Done():
			return "", 0, fmt.Errorf("等待搜索结果超时: %w", ctx.Err())
		default:
		}

		msgs, err := cm.getLatestMessages(ctx, api, peer, 30)
		if err != nil {
			return "", 0, err
		}
		for _, m := range msgs {
			if m.ID <= baseline {
				continue
			}
			btns := callbackButtons(m)
			var fallback string
			var containsExact string
			for data, text := range btns {
				if !strings.HasPrefix(data, "tack_") {
					continue
				}
				if strings.HasSuffix(data, "_track") && fallback == "" {
					fallback = data
				}
				// 变体检测基于完整按钮文本（"凄美地 - Live" 含 Live）
				clean := stripEmoji(text)
				if isTrackVariant(clean) {
					continue
				}
				title := extractTitle(clean)
				titleLower := strings.ToLower(title)
				if titleLower == queryLower {
					return data, m.ID, nil
				}
				if containsExact == "" && titleLower != "" && queryLower != "" &&
					strings.Contains(titleLower, queryLower) {
					containsExact = data
				}
			}
			if containsExact != "" {
				return containsExact, m.ID, nil
			}
			if fallback != "" {
				return fallback, m.ID, nil
			}
		}

		select {
		case <-ctx.Done():
			return "", 0, fmt.Errorf("等待搜索结果超时: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// extractTitle 从按钮文本中提取歌曲标题（去掉 emoji 与 " - 歌手" 部分）
func extractTitle(btnText string) string {
	s := stripEmoji(btnText)
	if i := strings.Index(s, " - "); i > 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// isTrackVariant 判断是否为 Live/Remix/翻唱等变体版本
func isTrackVariant(title string) bool {
	t := strings.ToLower(title)
	for _, kw := range []string{"live", "remix", "version", "伴奏", "翻唱", "cover", "instrumental", "mv", "音乐剧"} {
		if strings.Contains(t, kw) {
			return true
		}
	}
	return false
}

// waitForDownloadButton 轮询详情卡片中的 Download Track 按钮
func (cm *ChannelManager) waitForDownloadButton(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, baseline int) (string, int, error) {
	for {
		select {
		case <-ctx.Done():
			return "", 0, fmt.Errorf("等待下载按钮超时: %w", ctx.Err())
		default:
		}

		msgs, err := cm.getLatestMessages(ctx, api, peer, 30)
		if err != nil {
			return "", 0, err
		}
		for _, m := range msgs {
			if m.ID <= baseline {
				continue
			}
			btns := callbackButtons(m)
			for data := range btns {
				if strings.HasPrefix(data, "download_") && strings.HasSuffix(data, "_track") {
					return data, m.ID, nil
				}
			}
		}

		select {
		case <-ctx.Done():
			return "", 0, fmt.Errorf("等待下载按钮超时: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

// clickCallback 点击指定消息上的 callback 按钮。
// Bot 可能未在超时内应答（BOT_RESPONSE_TIMEOUT），但回调实际已触发、Bot 端仍在处理，
// 因此忽略该错误继续等待后续消息。
func (cm *ChannelManager) clickCallback(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, msgID int, data string) error {
	_, err := api.MessagesGetBotCallbackAnswer(ctx, &tg.MessagesGetBotCallbackAnswerRequest{
		Peer:  peer,
		MsgID: msgID,
		Data:  []byte(data),
	})
	if err != nil && strings.Contains(err.Error(), "BOT_RESPONSE_TIMEOUT") {
		cm.log.Warn("bot callback answer timeout, assuming triggered",
			zap.String("data", data), zap.Error(err))
		return nil
	}
	return err
}

var waitSecondsRe = regexp.MustCompile(`(?i)please\s+wait\s+(\d+)\s+seconds`)

// parseWaitSeconds 从限流提示文本中解析需要等待的秒数
func parseWaitSeconds(text string) int {
	m := waitSecondsRe.FindStringSubmatch(text)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// waitForAudioFiles 轮询等待 Bot 返回的音频文件并入库。
// 若 Bot 触发限流（"please wait N seconds"），等待 N 秒后自动重新触发下载按钮。
func (cm *ChannelManager) waitForAudioFiles(ctx context.Context, api *tg.Client, peer tg.InputPeerClass, baseline int, channel *models.TGChannel, dlData string, dlMsgID int) ([]*models.TGChannelFile, error) {
	var files []*models.TGChannelFile
	idleRounds := 0
	rateLimited := false
	seen := map[int]bool{} // 已处理的消息，避免同一文件重复返回
	for {
		select {
		case <-ctx.Done():
			return files, nil
		default:
		}

		msgs, err := cm.getLatestMessages(ctx, api, peer, 30)
		if err != nil {
			return files, err
		}

		// 限流处理：Bot 要求等待 N 秒，等待后重新点击下载按钮
		if !rateLimited {
			for _, m := range msgs {
				if m.ID <= baseline || seen[m.ID] {
					continue
				}
				if sec := parseWaitSeconds(m.Message); sec > 0 {
					cm.log.Info("bot rate limited, waiting then retry download", zap.Int("seconds", sec))
					select {
					case <-ctx.Done():
						return files, nil
					case <-time.After(time.Duration(sec+3) * time.Second):
					}
					if err := cm.clickCallback(ctx, api, peer, dlMsgID, dlData); err != nil {
						cm.log.Warn("retry download click failed", zap.Error(err))
					}
					rateLimited = true
					break
				}
			}
		}

		newFound := false
		for _, m := range msgs {
			if m.ID <= baseline || m.Media == nil || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			docMedia, ok := m.Media.(*tg.MessageMediaDocument)
			if !ok {
				continue
			}
			doc, ok := docMedia.Document.(*tg.Document)
			if !ok {
				continue
			}

			fileName := ""
			isAudio := isAudioMime(doc.MimeType)
			for _, attr := range doc.Attributes {
				if a, ok := attr.(*tg.DocumentAttributeFilename); ok {
					fileName = a.FileName
					if isAudioExt(fileName) {
						isAudio = true
					}
				}
			}
			if !isAudio {
				continue
			}

			file := cm.buildTGFile(channel, m, doc)
			var existing int64
			cm.db.Model(&models.TGChannelFile{}).Where("file_unique_id = ?", file.FileUniqueID).Count(&existing)
			if existing == 0 {
				if err := cm.db.Create(file).Error; err == nil {
					cm.db.Model(&models.TGChannel{}).Where("id = ?", channel.ID).
						Update("file_count", gorm.Expr("file_count + 1"))
				}
			}
			files = append(files, file)
			newFound = true
		}

		if newFound {
			idleRounds = 0
		} else {
			idleRounds++
			if idleRounds >= 3 && len(files) > 0 {
				break // 已有文件且连续无新文件，认为下载完成
			}
		}

		select {
		case <-ctx.Done():
			return files, nil
		case <-time.After(3 * time.Second):
		}
	}
	return files, nil
}

// buildTGFile 从 document 构造频道文件记录
func (cm *ChannelManager) buildTGFile(channel *models.TGChannel, msg *tg.Message, doc *tg.Document) *models.TGChannelFile {
	var fileName, title, performer string
	var duration int
	for _, attr := range doc.Attributes {
		switch a := attr.(type) {
		case *tg.DocumentAttributeFilename:
			fileName = a.FileName
		case *tg.DocumentAttributeAudio:
			title = a.Title
			performer = a.Performer
			duration = a.Duration
		}
	}
	if title == "" {
		title = parseFilenameTitle(fileName)
	}
	if performer == "" {
		performer = parseFilenameArtist(fileName)
	}

	return &models.TGChannelFile{
		ChannelID:      channel.ID,
		ChatID:         channel.ChatID,
		MessageID:      int64(msg.ID),
		FileID:         fmt.Sprintf("%d", doc.ID),
		FileUniqueID:   fmt.Sprintf("%d_%d", doc.ID, doc.AccessHash),
		FileAccessHash: doc.AccessHash,
		FileReference:  hex.EncodeToString(doc.FileReference),
		FileName:       fileName,
		FileSize:       doc.Size,
		MimeType:       doc.MimeType,
		Duration:       duration,
		Title:          title,
		Artist:         performer,
		Caption:        msg.Message,
		PostedAt:       time.Unix(int64(msg.Date), 0),
	}
}
