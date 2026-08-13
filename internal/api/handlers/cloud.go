package handlers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/metadata"
	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// CloudHandler 网盘文件管理与流媒体播放处理器
type CloudHandler struct {
	manager   *storage.Manager
	db        *gorm.DB
	lyricsMgr *metadata.LyricsManager
	log       *zap.Logger
}

// NewCloudHandler 创建云盘处理器
func NewCloudHandler(mgr *storage.Manager, db *gorm.DB, log *zap.Logger) *CloudHandler {
	return &CloudHandler{
		manager:   mgr,
		db:        db,
		lyricsMgr: metadata.NewLyricsManager(log),
		log:       log,
	}
}

// getBackend 获取存储后端
func (h *CloudHandler) getBackend(c *fiber.Ctx, id string) (storage.Backend, error) {
	backend, ok := h.manager.Get(id)
	if !ok {
		return nil, fiber.NewError(fiber.StatusNotFound, "storage not found")
	}
	return backend, nil
}

// Mkdir 新建文件夹
func (h *CloudHandler) Mkdir(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, err := h.getBackend(c, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil || body.Path == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "path is required"})
	}

	if err := backend.MkdirAll(c.Context(), body.Path); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "created"})
}

// Rename 重命名/移动文件或文件夹
func (h *CloudHandler) Rename(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, err := h.getBackend(c, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	var body struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := c.BodyParser(&body); err != nil || body.OldPath == "" || body.NewPath == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "old_path and new_path are required"})
	}

	if err := backend.Rename(c.Context(), body.OldPath, body.NewPath); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "renamed"})
}

// DeleteFile 删除文件或文件夹
func (h *CloudHandler) DeleteFile(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, err := h.getBackend(c, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	p := c.Query("path")
	if p == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "path is required"})
	}

	if err := backend.Delete(c.Context(), p); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "deleted"})
}

// Upload 上传文件到网盘
func (h *CloudHandler) Upload(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, err := h.getBackend(c, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "file is required (form field 'file')"})
	}
	dir := c.FormValue("path", "/")

	// 保存到临时文件
	src, err := file.Open()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "mf-upload-*")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	tmp.Close()

	remotePath := path.Join(dir, file.Filename)
	if err := backend.Upload(c.Context(), tmpName, remotePath, nil); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "uploaded", "data": fiber.Map{"path": remotePath}})
}

// Stream 流式播放/下载网盘文件（支持 HTTP Range）
func (h *CloudHandler) Stream(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, err := h.getBackend(c, id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	p := c.Query("path")
	if p == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "path is required"})
	}

	return h.streamBackend(c, backend, p, "")
}

// LibraryStream 流式播放音乐库歌曲（从存储后端读取，支持 Range）
func (h *CloudHandler) LibraryStream(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Library
	if err := h.db.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "not found"})
	}

	var paths map[string]string
	if len(item.RemotePaths) > 0 {
		if err := json.Unmarshal(item.RemotePaths, &paths); err != nil {
			h.log.Warn("parse library remote_paths failed", zap.String("id", id), zap.Error(err))
		}
	}
	if len(paths) == 0 {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "no remote file for this track"})
	}

	// 优先使用指定存储
	storageID := c.Query("storage")
	if storageID != "" {
		if p, ok := paths[storageID]; ok {
			if backend, ok := h.manager.Get(storageID); ok {
				return h.streamBackend(c, backend, p, item.Title)
			}
		}
	}

	// 否则按配置顺序尝试
	for sid, p := range paths {
		backend, ok := h.manager.Get(sid)
		if !ok {
			continue
		}
		return h.streamBackend(c, backend, p, item.Title)
	}

	return c.Status(404).JSON(fiber.Map{"error": true, "message": "no playable storage backend"})
}

// LibraryLyrics 获取音乐库歌曲歌词
func (h *CloudHandler) LibraryLyrics(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Library
	if err := h.db.First(&item, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "not found"})
	}

	lyrics, err := h.lyricsMgr.FetchLyrics(c.Context(), item.Title, item.Artist)
	if err != nil {
		return c.JSON(fiber.Map{"data": nil, "message": "lyrics not found"})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"lrc": lyrics.LRC, "source": lyrics.Source}})
}

// streamBackend 从存储后端拉流输出，支持 Range 与附件下载
func (h *CloudHandler) streamBackend(c *fiber.Ctx, backend storage.Backend, p, displayName string) error {
	size, err := backend.Size(c.Context(), p)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "file not found"})
	}

	start, end := int64(0), size-1
	rangeHeader := c.Get("Range")
	partial := false
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") {
		if s, e, ok := parseByteRange(strings.TrimPrefix(rangeHeader, "bytes="), size); ok {
			start, end = s, e
			partial = true
		}
	}

	rc, err := backend.Open(c.Context(), p, start, end-start+1)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	defer rc.Close()

	c.Set("Accept-Ranges", "bytes")
	c.Set("Content-Type", mimeTypeByExt(p))

	if partial {
		c.Status(fiber.StatusPartialContent)
		c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	} else {
		c.Status(fiber.StatusOK)
	}
	c.Set("Content-Length", strconv.FormatInt(end-start+1, 10))

	base := path.Base(p)
	if displayName == "" {
		displayName = base
	}
	if c.Query("download") == "1" {
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, base))
	} else {
		c.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, displayName))
	}

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		if _, err := io.Copy(w, rc); err != nil {
			h.log.Debug("stream copy ended", zap.Error(err))
		}
	})
	return nil
}

// parseByteRange 解析 "start-end" / "start-" / "-suffix" 格式的 Range
func parseByteRange(spec string, size int64) (int64, int64, bool) {
	if size <= 0 {
		return 0, 0, false
	}
	if strings.Contains(spec, ",") {
		return 0, 0, false // 不支持多段
	}
	dash := strings.Index(spec, "-")
	if dash < 0 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(spec[:dash])
	endStr := strings.TrimSpace(spec[dash+1:])

	if startStr == "" {
		// suffix: 最后 N 字节
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if endStr != "" {
		if e, err := strconv.ParseInt(endStr, 10, 64); err == nil {
			end = e
		}
	}
	if end >= size {
		end = size - 1
	}
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

// mimeTypeByExt 根据扩展名推断 MIME 类型
func mimeTypeByExt(name string) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".lrc":
		return "text/plain; charset=utf-8"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
