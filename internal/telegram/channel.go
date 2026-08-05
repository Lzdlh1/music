package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	if len(resolved.Chats) == 0 {
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
							ChannelID:    channel.ID,
							ChatID:       channel.ChatID,
							MessageID:    int64(msg.ID),
							FileID:       fileID,
							FileUniqueID: fileUniqueID,
							FileName:     fileName,
							FileSize:     doc.Size,
							MimeType:     doc.MimeType,
							Duration:     duration,
							Title:        title,
							Artist:       performer,
							Caption:      msg.Message,
							PostedAt:     time.Unix(int64(msg.Date), 0),
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
