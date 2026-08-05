package handlers

import (
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/playlist"
	"go.uber.org/zap"
)

// PlaylistHandler 歌单导入处理器
type PlaylistHandler struct {
	parser *playlist.Parser
	log    *zap.Logger
}

// NewPlaylistHandler 创建歌单处理器
func NewPlaylistHandler(log *zap.Logger) *PlaylistHandler {
	return &PlaylistHandler{
		parser: playlist.NewParser(log),
		log:    log,
	}
}

// ParseURL 通过 URL 导入歌单
func (h *PlaylistHandler) ParseURL(c *fiber.Ctx) error {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if req.URL == "" {
		return fiber.NewError(fiber.StatusBadRequest, "url is required")
	}

	tracks, err := h.parser.ParseURL(req.URL)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(fiber.Map{
		"data":  tracks,
		"total": len(tracks),
	})
}

// ParseText 通过文本导入歌单
func (h *PlaylistHandler) ParseText(c *fiber.Ctx) error {
	var req struct {
		Text   string `json:"text"`
		Format string `json:"format"` // text, m3u, csv
	}
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	var tracks []playlist.Track
	var err error

	switch req.Format {
	case "m3u", "m3u8":
		tracks, err = h.parser.ParseM3U(req.Text)
	case "csv":
		tracks, err = h.parser.ParseCSV(req.Text)
	default:
		tracks, err = h.parser.ParseText(req.Text)
	}

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(fiber.Map{
		"data":  tracks,
		"total": len(tracks),
	})
}

// ParseFile 通过文件导入歌单
func (h *PlaylistHandler) ParseFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "file is required")
	}

	f, err := file.Open()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "open file failed")
	}
	defer f.Close()

	content, err := io.ReadAll(io.LimitReader(f, 10*1024*1024)) // 10MB max
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "read file failed")
	}

	text := string(content)
	var tracks []playlist.Track

	filename := file.Filename
	switch {
	case hasSuffix(filename, ".m3u", ".m3u8"):
		tracks, err = h.parser.ParseM3U(text)
	case hasSuffix(filename, ".csv"):
		tracks, err = h.parser.ParseCSV(text)
	default:
		tracks, err = h.parser.ParseText(text)
	}

	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.JSON(fiber.Map{
		"data":  tracks,
		"total": len(tracks),
	})
}

func hasSuffix(s string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
