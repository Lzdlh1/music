package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChannelHandler 频道资源处理器
type ChannelHandler struct {
	channelMgr *telegram.ChannelManager
	storageMgr *storage.Manager
	db         *gorm.DB
	log        *zap.Logger
}

// NewChannelHandler 创建频道处理器
func NewChannelHandler(cm *telegram.ChannelManager, sm *storage.Manager, db *gorm.DB, log *zap.Logger) *ChannelHandler {
	return &ChannelHandler{channelMgr: cm, storageMgr: sm, db: db, log: log}
}

// ListChannels 列出已订阅的频道
func (h *ChannelHandler) ListChannels(c *fiber.Ctx) error {
	var channels []models.TGChannel
	h.db.Order("created_at DESC").Find(&channels)
	return c.JSON(fiber.Map{"data": channels})
}

// AddChannel 添加频道订阅
func (h *ChannelHandler) AddChannel(c *fiber.Ctx) error {
	var req struct {
		ChatID string `json:"chat_id"` // @username 或数字 ID
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	identifier := telegram.GetChatIDStr(req.ChatID)
	if identifier == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "请输入频道用户名或 Chat ID"})
	}

	// 优先使用 Bot API 验证频道；未配置 Bot 时回退到 MTProto 账号解析
	var channel *models.TGChannel
	var err error
	if h.channelMgr.HasBotToken() {
		channel, err = h.channelMgr.ScanChannel(identifier)
	} else {
		channel, err = h.channelMgr.ResolveChannelMTProto(c.Context(), identifier)
	}
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	// 检查是否已存在
	var existing models.TGChannel
	if err := h.db.Where("chat_id = ?", channel.ChatID).First(&existing).Error; err == nil {
		return c.JSON(fiber.Map{"success": false, "message": "该频道已添加"})
	}

	if err := h.db.Create(channel).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.Status(201).JSON(fiber.Map{"data": channel})
}

// RemoveChannel 移除频道
func (h *ChannelHandler) RemoveChannel(c *fiber.Ctx) error {
	id := c.Params("id")
	// 同时删除该频道的文件记录
	h.db.Where("channel_id = ?", id).Delete(&models.TGChannelFile{})
	h.db.Delete(&models.TGChannel{}, "id = ?", id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// ToggleChannel 启用/禁用频道
func (h *ChannelHandler) ToggleChannel(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	h.db.Model(&models.TGChannel{}).Where("id = ?", id).Update("enabled", req.Enabled)
	return c.JSON(fiber.Map{"message": "updated"})
}

// ListFiles 列出频道文件
func (h *ChannelHandler) ListFiles(c *fiber.Ctx) error {
	channelID := c.Params("id")
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 50)
	keyword := c.Query("keyword")

	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	query := h.db.Model(&models.TGChannelFile{}).Where("channel_id = ?", channelID)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR artist LIKE ? OR file_name LIKE ?", like, like, like)
	}

	var total int64
	query.Count(&total)

	var files []models.TGChannelFile
	query.Order("posted_at DESC").Offset(offset).Limit(pageSize).Find(&files)

	return c.JSON(fiber.Map{
		"data":  files,
		"total": total,
		"page":  page,
	})
}

// ListAllFiles 列出所有频道的文件
func (h *ChannelHandler) ListAllFiles(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	pageSize := c.QueryInt("page_size", 50)
	keyword := c.Query("keyword")
	channelID := c.Query("channel_id")

	if page < 1 {
		page = 1
	}
	offset := (page - 1) * pageSize

	query := h.db.Model(&models.TGChannelFile{})
	if channelID != "" {
		query = query.Where("channel_id = ?", channelID)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR artist LIKE ? OR file_name LIKE ?", like, like, like)
	}

	var total int64
	query.Count(&total)

	var files []models.TGChannelFile
	query.Order("posted_at DESC").Offset(offset).Limit(pageSize).Find(&files)

	return c.JSON(fiber.Map{
		"data":  files,
		"total": total,
		"page":  page,
	})
}

// GetFileDownloadURL 下载文件到浏览器（通过 MTProto 账号下载后直接返回文件流）
func (h *ChannelHandler) GetFileDownloadURL(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	var file models.TGChannelFile
	if err := h.db.Where("id = ?", fileID).First(&file).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "file not found"})
	}

	// 先下载到临时文件，再返回给浏览器
	tempFile := filepath.Join(os.TempDir(), "musicflow_"+file.ID+filepath.Ext(file.FileName))
	if err := h.channelMgr.DownloadFileMTProto(c.Context(), &file, tempFile); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": "下载失败: " + err.Error()})
	}
	defer os.Remove(tempFile)

	// 标记为已下载
	h.db.Model(&file).Update("downloaded", true)

	filename := file.FileName
	if filename == "" {
		filename = file.Title + filepath.Ext(file.FileName)
	}
	return c.Download(tempFile, filename)
}

// DownloadToLibrary 下载文件到存储目标并写入音乐库
func (h *ChannelHandler) DownloadToLibrary(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	var file models.TGChannelFile
	if err := h.db.Where("id = ?", fileID).First(&file).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "file not found"})
	}

	// 1. MTProto 下载到临时文件
	ext := filepath.Ext(file.FileName)
	if ext == "" {
		ext = ".mp3"
	}
	tempFile := filepath.Join(os.TempDir(), "musicflow_"+file.ID+ext)
	if err := h.channelMgr.DownloadFileMTProto(c.Context(), &file, tempFile); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": "下载失败: " + err.Error()})
	}
	defer os.Remove(tempFile)

	// 2. 上传到所有启用的存储后端
	remotePaths := make(map[string]string)
	namingTpl := &storage.NamingTemplate{Template: "{artist}/{title}.{ext}"}
	for _, backend := range h.storageMgr.List() {
		remotePath := namingTpl.Format(storage.TrackNamingInfo{
			Artist: file.Artist,
			Title:  file.Title,
			Ext:    strings.TrimPrefix(ext, "."),
		})

		dir := filepath.Dir(remotePath)
		if dir != "" && dir != "." {
			if err := backend.MkdirAll(c.Context(), dir); err != nil {
				h.log.Warn("tg file mkdir failed", zap.String("backend", backend.ID()), zap.Error(err))
			}
		}
		if err := backend.Upload(c.Context(), tempFile, remotePath, nil); err != nil {
			h.log.Error("tg file upload failed", zap.String("backend", backend.ID()), zap.Error(err))
			continue
		}
		remotePaths[backend.ID()] = remotePath
	}

	// 3. 写入音乐库记录
	if len(remotePaths) > 0 {
		remotePathsJSON, _ := json.Marshal(remotePaths)
		library := &models.Library{
			Title:       file.Title,
			Artist:      file.Artist,
			Format:      strings.TrimPrefix(ext, "."),
			FileSize:    file.FileSize,
			Duration:    file.Duration,
			Source:      "telegram",
			RemotePaths: models.JSON(remotePathsJSON),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := h.db.Create(library).Error; err != nil {
			h.log.Error("save tg library record", zap.Error(err))
		}
	}

	// 4. 标记为已下载
	h.db.Model(&file).Update("downloaded", true)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "已保存到音乐库",
		"file": fiber.Map{
			"id":        file.ID,
			"title":     file.Title,
			"artist":    file.Artist,
			"file_name": file.FileName,
			"file_size": file.FileSize,
			"duration":  file.Duration,
			"mime_type": file.MimeType,
		},
	})
}

// ScanHistory 扫描频道历史记录
func (h *ChannelHandler) ScanHistory(c *fiber.Ctx) error {
	channelID := c.Params("id")

	// 异步执行扫描，避免阻塞 HTTP 请求
	go func() {
		if err := h.channelMgr.ScanChannelHistory(context.Background(), channelID); err != nil {
			h.log.Error("scan channel history failed", zap.String("channel_id", channelID), zap.Error(err))
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "开始扫描历史记录，请稍后刷新查看",
	})
}
