package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChannelHandler 频道资源处理器
type ChannelHandler struct {
	channelMgr *telegram.ChannelManager
	db         *gorm.DB
	log        *zap.Logger
}

// NewChannelHandler 创建频道处理器
func NewChannelHandler(cm *telegram.ChannelManager, db *gorm.DB, log *zap.Logger) *ChannelHandler {
	return &ChannelHandler{channelMgr: cm, db: db, log: log}
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

	// 验证频道并获取信息
	channel, err := h.channelMgr.ScanChannel(identifier)
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

// GetFileDownloadURL 获取文件下载链接
func (h *ChannelHandler) GetFileDownloadURL(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	var file models.TGChannelFile
	if err := h.db.Where("id = ?", fileID).First(&file).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "file not found"})
	}

	downloadURL, err := h.channelMgr.GetFileURL(file.FileID)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	// 标记为已下载
	h.db.Model(&file).Update("downloaded", true)

	return c.JSON(fiber.Map{
		"success":      true,
		"download_url": downloadURL,
		"file_name":    file.FileName,
		"title":        file.Title,
		"artist":       file.Artist,
	})
}

// DownloadToLibrary 下载文件到存储目标
func (h *ChannelHandler) DownloadToLibrary(c *fiber.Ctx) error {
	fileID := c.Params("fileId")

	var file models.TGChannelFile
	if err := h.db.Where("id = ?", fileID).First(&file).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "file not found"})
	}

	downloadURL, err := h.channelMgr.GetFileURL(file.FileID)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": "获取下载链接失败: " + err.Error()})
	}

	// 标记为已下载
	h.db.Model(&file).Update("downloaded", true)

	return c.JSON(fiber.Map{
		"success":      true,
		"download_url": downloadURL,
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
