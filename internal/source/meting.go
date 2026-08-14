package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MetingSource 基于 Meting API 的音乐源（兼容 GDStudio / MKOnlineMusicPlayer 等采集站）
type MetingSource struct {
	name        string
	baseURL     string
	musicSource string // netease, kuwo, tencent, joox, kugou, migu ...
	priority    int
	client      *http.Client
	log         *zap.Logger
}

// MetingConfig Meting 源配置
type MetingConfig struct {
	Name        string `json:"name"`
	BaseURL     string `json:"base_url"`     // API 地址，如 https://music-api.gdstudio.xyz/api.php
	MusicSource string `json:"music_source"` // 音乐源：netease, kuwo, tencent, joox 等
	Priority    int    `json:"priority"`
	Timeout     int    `json:"timeout"`
}

// NewMetingSource 创建 Meting 站点音乐源
func NewMetingSource(cfg MetingConfig, log *zap.Logger) *MetingSource {
	timeout := 30
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	if cfg.MusicSource == "" {
		cfg.MusicSource = "netease"
	}
	return &MetingSource{
		name:        cfg.Name,
		baseURL:     cfg.BaseURL,
		musicSource: cfg.MusicSource,
		priority:    cfg.Priority,
		client:      &http.Client{Timeout: time.Duration(timeout) * time.Second},
		log:         log,
	}
}

// Meting API 响应结构
type metingTrack struct {
	ID      interface{} `json:"id"`
	Name    string      `json:"name"`
	Artist  interface{} `json:"artist"` // 可能是字符串或数组
	Album   string      `json:"album"`
	PicID   string      `json:"pic_id"`
	URLID   string      `json:"url_id"`
	LyricID string      `json:"lyric_id"`
	Source  string      `json:"source"`
}

type metingURLResp struct {
	URL  string      `json:"url"`
	BR   interface{} `json:"br"`
	Size interface{} `json:"size"`
}

type metingLyricResp struct {
	Lyric  string `json:"lyric"`
	TLyric string `json:"tlyric"`
}

type metingPicResp struct {
	URL string `json:"url"`
}

func (t *metingTrack) IDString() string {
	switch v := t.ID.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// stripPicSuffix 剥离 ID 末尾附加的 #picID 部分，返回真实 track id
func stripPicSuffix(rawID string) string {
	if i := strings.Index(rawID, "#"); i >= 0 {
		return rawID[:i]
	}
	return rawID
}

// picIDFrom 从 ID 中提取封面 ID（# 之后的部分），无则返回整个 rawID
func picIDFrom(rawID string) string {
	if i := strings.Index(rawID, "#"); i >= 0 {
		return rawID[i+1:]
	}
	return rawID
}

func (t *metingTrack) ArtistString() string {
	switch v := t.Artist.(type) {
	case string:
		return v
	case []interface{}:
		var names []string
		for _, a := range v {
			if s, ok := a.(string); ok {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			result := names[0]
			for _, n := range names[1:] {
				result += " / " + n
			}
			return result
		}
		return "Unknown"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (s *MetingSource) Name() string     { return s.name }
func (s *MetingSource) Priority() int    { return s.priority }

func (s *MetingSource) IsAvailable(ctx context.Context) bool {
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

func (s *MetingSource) doGet(ctx context.Context, params url.Values) ([]byte, error) {
	u, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	// 合并已有的 query 参数
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "MusicFlow/1.0")
	req.Header.Set("Referer", s.baseURL)

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

	return body, nil
}

func (s *MetingSource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	params := url.Values{}
	params.Set("types", "search")
	params.Set("source", s.musicSource)
	params.Set("name", query.Keyword)
	if query.PageSize > 0 {
		params.Set("count", strconv.Itoa(query.PageSize))
	}
	if query.Page > 0 {
		params.Set("pages", strconv.Itoa(query.Page))
	}

	body, err := s.doGet(ctx, params)
	if err != nil {
		s.log.Warn("meting search failed", zap.Error(err))
		return nil, err
	}

	var tracks []metingTrack
	if err := json.Unmarshal(body, &tracks); err != nil {
		// 有些 Meting API 包一层 {code, data}
		var wrapped struct {
			Code int               `json:"code"`
			Data json.RawMessage   `json:"data"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 == nil && len(wrapped.Data) > 0 {
			_ = json.Unmarshal(wrapped.Data, &tracks)
		}
		if tracks == nil {
			return nil, fmt.Errorf("decode search result: %w (body: %s)", err, string(body[:min(len(body), 200)]))
		}
	}

	var results []TrackResult
	// GD API 的 url 接口 br=999 时大部分音乐源（netease/joox 等）返回无损，
	// 而 bilibili 等视频源只返回流媒体音质，按源类型标记搜索结果音质
	searchQuality := QualityFLAC
	if s.musicSource == "bilibili" {
		searchQuality = Quality320
	}
	for _, t := range tracks {
		// 封面 ID 一并编码进曲目 ID，便于后续 GetCover 直接使用（pic_id 与 track_id 可能不同）
		compositeID := s.name + ":" + t.IDString()
		if t.PicID != "" {
			compositeID += "#" + t.PicID
		}
		results = append(results, TrackResult{
			ID:       compositeID,
			Title:    t.Name,
			Artist:   t.ArtistString(),
			Album:    t.Album,
			Quality:  searchQuality,
			Source:   s.name,
			CoverURL: "", // Meting 搜索不直接返回封面 URL，需要额外请求
			Score:    0.8,
		})
	}

	s.log.Info("meting search", zap.String("keyword", query.Keyword), zap.Int("results", len(results)))
	return results, nil
}

func (s *MetingSource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	// Meting API 没有单独的 detail 接口，用搜索的缓存数据
	// 返回基本信息，封面通过 GetCover 获取
	_, rawID := splitSourceID(id)
	return &TrackDetail{
		ID:     id,
		Title:  "Track " + stripPicSuffix(rawID),
		Source: s.name,
	}, nil
}

func (s *MetingSource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	_, rawID := splitSourceID(id)
	rawID = stripPicSuffix(rawID)
	params := url.Values{}
	params.Set("types", "url")
	params.Set("source", s.musicSource)
	params.Set("id", rawID)

	// 映射音质参数
	br := "999"
	switch quality {
	case Quality128:
		br = "128"
	case Quality320:
		br = "320"
	case QualityFLAC, QualityHiRes:
		br = "999"
	}
	params.Set("br", br)

	body, err := s.doGet(ctx, params)
	if err != nil {
		return nil, err
	}

	var resp metingURLResp
	if err := json.Unmarshal(body, &resp); err != nil {
		// 可能直接返回 URL 字符串
		var directURL string
		if err2 := json.Unmarshal(body, &directURL); err2 == nil && directURL != "" {
			return &DownloadURL{URL: directURL, Quality: quality, Format: "mp3"}, nil
		}
		return nil, fmt.Errorf("decode url response: %w", err)
	}

	if resp.URL == "" {
		return nil, fmt.Errorf("empty download url")
	}

	format := "mp3"
	// 解析返回的实际码率
	actualBR := 0
	switch v := resp.BR.(type) {
	case float64:
		actualBR = int(v)
	case string:
		actualBR, _ = strconv.Atoi(v)
	}
	actualQuality := quality
	if actualBR >= 700 {
		format = "flac"
		actualQuality = QualityFLAC
	} else if actualBR >= 320 {
		actualQuality = Quality320
	} else {
		actualQuality = Quality128
	}

	var fileSize int64
	switch v := resp.Size.(type) {
	case float64:
		fileSize = int64(v) * 1024 // API返回单位是KB
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			fileSize = n * 1024
		}
	}

	return &DownloadURL{
		URL:      resp.URL,
		Quality:  actualQuality,
		Format:   format,
		FileSize: fileSize,
	}, nil
}

func (s *MetingSource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	_, rawID := splitSourceID(id)
	rawID = stripPicSuffix(rawID)
	params := url.Values{}
	params.Set("types", "lyric")
	params.Set("source", s.musicSource)
	params.Set("id", rawID)

	body, err := s.doGet(ctx, params)
	if err != nil {
		return nil, err
	}

	var resp metingLyricResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode lyric: %w", err)
	}

	return &LyricsResult{
		LRC:      resp.Lyric,
		TransLRC: resp.TLyric,
		Source:   s.name,
	}, nil
}

func (s *MetingSource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	_, rawID := splitSourceID(id)
	// 优先使用编码进 ID 的 pic_id；无 # 后缀时即 track id
	picID := picIDFrom(rawID)

	// 部分源（如 bilibili）的 pic_id 本身就是完整 URL（可能以 // 开头）
	if strings.HasPrefix(picID, "http://") || strings.HasPrefix(picID, "https://") {
		return &CoverResult{URL: picID, Source: s.name}, nil
	}
	if strings.HasPrefix(picID, "//") {
		return &CoverResult{URL: "https:" + picID, Source: s.name}, nil
	}

	params := url.Values{}
	params.Set("types", "pic")
	params.Set("source", s.musicSource)
	params.Set("id", picID)
	params.Set("size", "500")

	body, err := s.doGet(ctx, params)
	if err != nil {
		return nil, err
	}

	var resp metingPicResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode pic: %w", err)
	}
	if resp.URL == "" {
		return nil, fmt.Errorf("no cover url")
	}

	return &CoverResult{
		URL:    resp.URL,
		Source: s.name,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
