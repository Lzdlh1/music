package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// CustomAPISource 自定义 API 资源站音乐源
type CustomAPISource struct {
	name     string
	baseURL  string
	apiKey   string
	priority int
	client   *http.Client
	log      *zap.Logger
}

// CustomAPIConfig 自定义源配置
type CustomAPIConfig struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key,omitempty"`
	Priority int    `json:"priority"`
	Timeout  int    `json:"timeout"` // seconds
}

// NewCustomAPISource 创建自定义 API 源
func NewCustomAPISource(cfg CustomAPIConfig, log *zap.Logger) *CustomAPISource {
	timeout := 30
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	return &CustomAPISource{
		name:     cfg.Name,
		baseURL:  cfg.BaseURL,
		apiKey:   cfg.APIKey,
		priority: cfg.Priority,
		client:   &http.Client{Timeout: time.Duration(timeout) * time.Second},
		log:      log,
	}
}

// apiResponse 通用 API 响应结构（适配常见采集站格式）
type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type apiSearchResult struct {
	List []apiTrack `json:"list"`
}

type apiTrack struct {
	ID       interface{} `json:"id"`
	Name     string      `json:"name"`
	Artist   string      `json:"artist"`
	Album    string      `json:"album"`
	Duration int         `json:"duration"`
	Pic      string      `json:"pic"`
	URL      string      `json:"url"`
	Quality  string      `json:"quality"`
	Size     int64       `json:"size"`
	Lrc      string      `json:"lrc"`
}

func (t *apiTrack) IDString() string {
	switch v := t.ID.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s *CustomAPISource) Name() string { return s.name }
func (s *CustomAPISource) Priority() int { return s.priority }

func (s *CustomAPISource) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return false
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *CustomAPISource) doRequest(ctx context.Context, path string, params url.Values) (*apiResponse, error) {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	u.Path = u.Path + path
	if params != nil {
		u.RawQuery = params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}
	req.Header.Set("User-Agent", "MusicFlow/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api status %d: %s", resp.StatusCode, string(body))
	}

	var result apiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Code != 200 && result.Code != 0 && result.Code != 1 {
		return nil, fmt.Errorf("api error code %d: %s", result.Code, result.Msg)
	}

	return &result, nil
}

func (s *CustomAPISource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	params := url.Values{}
	params.Set("type", "search")
	params.Set("keyword", query.Keyword)
	if query.Page > 0 {
		params.Set("page", strconv.Itoa(query.Page))
	}
	if query.PageSize > 0 {
		params.Set("limit", strconv.Itoa(query.PageSize))
	}

	resp, err := s.doRequest(ctx, "", params)
	if err != nil {
		s.log.Warn("custom api search failed", zap.Error(err))
		return nil, err
	}

	var searchResp apiSearchResult
	// 尝试直接解析为列表或包装结构
	if err := json.Unmarshal(resp.Data, &searchResp); err != nil {
		var tracks []apiTrack
		if err2 := json.Unmarshal(resp.Data, &tracks); err2 != nil {
			return nil, fmt.Errorf("decode search data: %w", err)
		}
		searchResp.List = tracks
	}

	var results []TrackResult
	for _, t := range searchResp.List {
		results = append(results, TrackResult{
			ID:       s.name + ":" + t.IDString(),
			Title:    t.Name,
			Artist:   t.Artist,
			Album:    t.Album,
			Duration: t.Duration,
			Quality:  parseQuality(t.Quality),
			FileSize: t.Size,
			Source:   s.name,
			CoverURL: t.Pic,
			Score:    0.8,
		})
	}

	s.log.Info("custom api search", zap.String("keyword", query.Keyword), zap.Int("results", len(results)))
	return results, nil
}

func (s *CustomAPISource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	_, rawID := splitSourceID(id)
	params := url.Values{}
	params.Set("type", "detail")
	params.Set("id", rawID)

	resp, err := s.doRequest(ctx, "", params)
	if err != nil {
		return nil, err
	}

	var track apiTrack
	if err := json.Unmarshal(resp.Data, &track); err != nil {
		return nil, fmt.Errorf("decode track detail: %w", err)
	}

	return &TrackDetail{
		ID:       id,
		Title:    track.Name,
		Artist:   track.Artist,
		Album:    track.Album,
		Duration: track.Duration,
		CoverURL: track.Pic,
		Source:   s.name,
	}, nil
}

func (s *CustomAPISource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	_, rawID := splitSourceID(id)
	params := url.Values{}
	params.Set("type", "url")
	params.Set("id", rawID)
	if quality != QualityAny {
		params.Set("quality", quality.String())
	}

	resp, err := s.doRequest(ctx, "", params)
	if err != nil {
		return nil, err
	}

	var track apiTrack
	if err := json.Unmarshal(resp.Data, &track); err != nil {
		// 尝试直接解析为 URL 字符串
		var directURL string
		if err2 := json.Unmarshal(resp.Data, &directURL); err2 != nil {
			return nil, fmt.Errorf("decode download url: %w", err)
		}
		return &DownloadURL{
			URL:     directURL,
			Quality: quality,
			Format:  "mp3",
		}, nil
	}

	format := "mp3"
	if quality == QualityFLAC || quality == QualityHiRes {
		format = "flac"
	}

	return &DownloadURL{
		URL:      track.URL,
		Quality:  parseQuality(track.Quality),
		Format:   format,
		FileSize: track.Size,
	}, nil
}

func (s *CustomAPISource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	_, rawID := splitSourceID(id)
	params := url.Values{}
	params.Set("type", "lrc")
	params.Set("id", rawID)

	resp, err := s.doRequest(ctx, "", params)
	if err != nil {
		return nil, err
	}

	var track apiTrack
	if err := json.Unmarshal(resp.Data, &track); err != nil {
		var lrc string
		if err2 := json.Unmarshal(resp.Data, &lrc); err2 != nil {
			return nil, fmt.Errorf("decode lyrics: %w", err)
		}
		return &LyricsResult{LRC: lrc, Source: s.name}, nil
	}

	return &LyricsResult{
		LRC:    track.Lrc,
		Source: s.name,
	}, nil
}

func (s *CustomAPISource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	detail, err := s.GetTrackDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.CoverURL == "" {
		return nil, fmt.Errorf("no cover available")
	}
	return &CoverResult{
		URL:    detail.CoverURL,
		Source: s.name,
	}, nil
}

// Helper functions

func parseQuality(s string) Quality {
	switch s {
	case "128", "128k", "128K":
		return Quality128
	case "320", "320k", "320K":
		return Quality320
	case "flac", "FLAC", "lossless":
		return QualityFLAC
	case "hires", "Hi-Res", "hi-res":
		return QualityHiRes
	default:
		return Quality320
	}
}

func splitSourceID(compositeID string) (sourceName, rawID string) {
	for i, c := range compositeID {
		if c == ':' {
			return compositeID[:i], compositeID[i+1:]
		}
	}
	return "", compositeID
}
