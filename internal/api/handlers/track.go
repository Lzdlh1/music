package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/source"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TrackHandler 在线试听与收藏
type TrackHandler struct {
	aggregator *source.Aggregator
	db         *gorm.DB
	client     *http.Client
	log        *zap.Logger
}

// NewTrackHandler 创建曲目处理器
func NewTrackHandler(agg *source.Aggregator, db *gorm.DB, log *zap.Logger) *TrackHandler {
	return &TrackHandler{
		aggregator: agg,
		db:         db,
		client:     &http.Client{},
		log:        log,
	}
}

// Stream 在线试听（不落盘）：按最低音质(128K)取直链并代理转发，避免暴露高优直链
func (h *TrackHandler) Stream(c *fiber.Ctx) error {
	id := pathParam(c, "id")
	sources, err := h.aggregator.GetTrackSources(c.Context(), id, source.Quality128)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	var direct string
	for _, s := range sources {
		if strings.HasPrefix(s.DownloadURL, "http://") || strings.HasPrefix(s.DownloadURL, "https://") {
			direct = s.DownloadURL
			break
		}
	}
	if direct == "" {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "该曲目暂无可在线试听资源"})
	}

	req, err := http.NewRequestWithContext(c.Context(), http.MethodGet, direct, nil)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://y.qq.com/")

	resp, err := h.client.Do(req)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": true, "message": "上游资源获取失败"})
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.Status(502).JSON(fiber.Map{"error": true, "message": "上游资源不可用"})
	}

	c.Set("Content-Type", resp.Header.Get("Content-Type"))
	if resp.Header.Get("Content-Length") != "" {
		c.Set("Content-Length", resp.Header.Get("Content-Length"))
	}
	c.Set("Cache-Control", "no-store")
	c.Set("Accept-Ranges", "bytes")
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c, resp.Body)
	return nil
}

// Favorite 收藏歌曲（写入音乐库 kind=favorite，在线试听不下载）
func (h *TrackHandler) Favorite(c *fiber.Ctx) error {
	var req struct {
		TrackID  string `json:"track_id"`
		Title    string `json:"title"`
		Artist   string `json:"artist"`
		Album    string `json:"album"`
		Duration int    `json:"duration"`
		CoverURL string `json:"cover_url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	req.TrackID = strings.TrimSpace(req.TrackID)
	if req.TrackID == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "track_id required"})
	}

	me := middleware.CurrentUser(c)
	var exist models.Library
	if err := h.db.First(&exist, "owner_id = ? AND source_track_id = ? AND kind = ?",
		me.ID, req.TrackID, "favorite").Error; err == nil {
		return c.JSON(fiber.Map{"data": exist, "already": true})
	}

	item := models.Library{
		OwnerID:       me.ID,
		Kind:          "favorite",
		Title:         req.Title,
		Artist:        req.Artist,
		Album:         req.Album,
		Duration:      req.Duration,
		CoverURL:      req.CoverURL,
		SourceTrackID: req.TrackID,
		HasLyrics:     false,
	}

	// 优先从源站补齐曲目详情（封面/时长等）；失败时降级使用前端传入信息
	detail, err := h.aggregator.GetTrackDetail(c.Context(), req.TrackID)
	if err == nil && detail != nil {
		if detail.Title != "" {
			item.Title = detail.Title
		}
		if detail.Artist != "" {
			item.Artist = detail.Artist
		}
		if detail.Album != "" {
			item.Album = detail.Album
		}
		if detail.Duration > 0 {
			item.Duration = detail.Duration
		}
		if detail.CoverURL != "" {
			item.CoverURL = detail.CoverURL
		}
		if detail.Source != "" {
			item.Source = detail.Source
		}
	}

	if item.Title == "" {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "获取歌曲信息失败，请重试"})
	}

	if err := h.db.Create(&item).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": item})
}

// Unfavorite 取消收藏（仅自己）
func (h *TrackHandler) Unfavorite(c *fiber.Ctx) error {
	id := c.Params("id")
	me := middleware.CurrentUser(c)
	var item models.Library
	if err := h.db.First(&item, "id = ? AND owner_id = ?", id, me.ID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "not found"})
	}
	if item.Kind != "favorite" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "非收藏条目，请用删除操作"})
	}
	h.db.Delete(&models.Library{}, "id = ?", id)
	return c.JSON(fiber.Map{"message": "unfavorited"})
}