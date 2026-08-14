package handlers

import (
	"net/url"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/metadata"
	"github.com/musicflow/musicflow/internal/source"
	"go.uber.org/zap"
)

// SearchHandler 搜索处理器
type SearchHandler struct {
	aggregator *source.Aggregator
	lyricsMgr  *metadata.LyricsManager
	log        *zap.Logger
}

// NewSearchHandler 创建搜索处理器
func NewSearchHandler(agg *source.Aggregator, log *zap.Logger) *SearchHandler {
	return &SearchHandler{
		aggregator: agg,
		lyricsMgr:  metadata.NewLyricsManager(log),
		log:        log,
	}
}

// Search 搜索音乐
func (h *SearchHandler) Search(c *fiber.Ctx) error {
	query := source.SearchQuery{
		Keyword:  c.Query("q"),
		Artist:   c.Query("artist"),
		Album:    c.Query("album"),
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("size", 20),
	}

	if query.Keyword == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "query parameter 'q' is required"})
	}

	qualityStr := c.Query("quality")
	switch qualityStr {
	case "128":
		query.Quality = source.Quality128
	case "320":
		query.Quality = source.Quality320
	case "flac":
		query.Quality = source.QualityFLAC
	case "hires":
		query.Quality = source.QualityHiRes
	default:
		query.Quality = source.QualityAny
	}

	results, err := h.aggregator.Search(c.Context(), query)
	if err != nil {
		h.log.Error("search failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "search failed"})
	}

	return c.JSON(fiber.Map{
		"data":  results,
		"total": len(results),
		"page":  query.Page,
	})
}

// GetTrackSources 获取曲目所有可用源
func (h *SearchHandler) GetTrackSources(c *fiber.Ctx) error {
	id := pathParam(c, "id")
	sources, err := h.aggregator.GetTrackSources(c.Context(), id, source.QualityAny)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"data": sources})
}

// pathParam 获取路径参数并做 URL 解码（Fiber 的 Params 不解码 URL 编码）
func pathParam(c *fiber.Ctx, key string) string {
	raw := c.Params(key)
	if decoded, err := url.PathUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

// GetLyrics 获取歌词预览
func (h *SearchHandler) GetLyrics(c *fiber.Ctx) error {
	id := pathParam(c, "id")

	// 先尝试从音乐源获取
	sources := h.aggregator.Sources()
	for _, src := range sources {
		lyrics, err := src.GetLyrics(c.Context(), id)
		if err == nil && lyrics != nil && lyrics.LRC != "" {
			return c.JSON(fiber.Map{"data": lyrics})
		}
	}

	// 降级：通过 LRClib 搜索（需要歌曲信息）
	// 由前端传 query params
	title := c.Query("title")
	artist := c.Query("artist")
	if title != "" {
		lyrics, err := h.lyricsMgr.FetchLyrics(c.Context(), title, artist)
		if err == nil && lyrics != nil {
			return c.JSON(fiber.Map{"data": fiber.Map{
				"lrc":    lyrics.LRC,
				"source": lyrics.Source,
			}})
		}
	}

	return c.JSON(fiber.Map{"data": nil, "message": "lyrics not found"})
}

// GetCover 获取封面
func (h *SearchHandler) GetCover(c *fiber.Ctx) error {
	id := pathParam(c, "id")

	sources := h.aggregator.Sources()
	for _, src := range sources {
		cover, err := src.GetCover(c.Context(), id)
		if err == nil && cover != nil && cover.URL != "" {
			return c.JSON(fiber.Map{"data": fiber.Map{
				"url":    cover.URL,
				"source": cover.Source,
			}})
		}
	}

	return c.JSON(fiber.Map{"data": nil, "message": "cover not found"})
}
