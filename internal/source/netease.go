package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// NeteaseSource 网易云音乐源（通过 NeteaseCloudMusicApi 代理）
type NeteaseSource struct {
	name     string
	baseURL  string
	priority int
	client   *http.Client
	log      *zap.Logger
}

// NeteaseConfig 网易云配置
type NeteaseConfig struct {
	Name     string `json:"name"`
	BaseURL  string `json:"base_url"` // NeteaseCloudMusicApi 服务地址
	Cookie   string `json:"cookie,omitempty"`
	Priority int    `json:"priority"`
}

// NewNeteaseSource 创建网易云源
func NewNeteaseSource(cfg NeteaseConfig, log *zap.Logger) *NeteaseSource {
	if cfg.Name == "" {
		cfg.Name = "netease"
	}
	return &NeteaseSource{
		name:     cfg.Name,
		baseURL:  strings.TrimSuffix(cfg.BaseURL, "/"),
		priority: cfg.Priority,
		client:   &http.Client{Timeout: 30 * time.Second},
		log:      log,
	}
}

func (n *NeteaseSource) Name() string     { return n.name }
func (n *NeteaseSource) Priority() int    { return n.priority }

func (n *NeteaseSource) IsAvailable(ctx context.Context) bool {
	reqURL := fmt.Sprintf("%s/", n.baseURL)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	resp, err := n.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func (n *NeteaseSource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	keyword := query.Keyword
	if query.Artist != "" {
		keyword = query.Artist + " " + keyword
	}

	limit := query.PageSize
	if limit == 0 {
		limit = 30
	}
	offset := 0
	if query.Page > 1 {
		offset = (query.Page - 1) * limit
	}

	params := url.Values{}
	params.Set("keywords", keyword)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))

	reqURL := fmt.Sprintf("%s/search?%s", n.baseURL, params.Encode())
	body, err := n.doRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Result struct {
			Songs []struct {
				ID      int    `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
				Duration int `json:"duration"`
			} `json:"songs"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	var results []TrackResult
	for _, s := range resp.Result.Songs {
		var artists []string
		for _, a := range s.Artists {
			artists = append(artists, a.Name)
		}

		results = append(results, TrackResult{
			ID:       fmt.Sprintf("netease:%d", s.ID),
			Title:    s.Name,
			Artist:   strings.Join(artists, "/"),
			Album:    s.Album.Name,
			Duration: s.Duration / 1000,
			Source:   n.name,
			Score:    float64(n.priority),
		})
	}

	return results, nil
}

func (n *NeteaseSource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	nID := n.extractID(id)

	reqURL := fmt.Sprintf("%s/song/detail?ids=%s", n.baseURL, nID)
	body, err := n.doRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Songs []struct {
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"ar"`
			Album struct {
				Name   string `json:"name"`
				PicURL string `json:"picUrl"`
			} `json:"al"`
			Duration int `json:"dt"`
		} `json:"songs"`
	}

	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Songs) == 0 {
		return nil, fmt.Errorf("track detail not found")
	}

	s := resp.Songs[0]
	var artists []string
	for _, a := range s.Artists {
		artists = append(artists, a.Name)
	}

	return &TrackDetail{
		ID:       id,
		Title:    s.Name,
		Artist:   strings.Join(artists, "/"),
		Album:    s.Album.Name,
		Duration: s.Duration / 1000,
		CoverURL: s.Album.PicURL,
		Source:   n.name,
	}, nil
}

func (n *NeteaseSource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	nID := n.extractID(id)

	level := "standard"
	switch {
	case quality >= QualityHiRes:
		level = "jymaster"
	case quality >= QualityFLAC:
		level = "lossless"
	case quality >= Quality320:
		level = "exhigh"
	}

	reqURL := fmt.Sprintf("%s/song/url/v1?id=%s&level=%s", n.baseURL, nID, level)
	body, err := n.doRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			URL  string `json:"url"`
			Size int64  `json:"size"`
			Type string `json:"type"`
			BR   int    `json:"br"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &resp); err != nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("no download url")
	}

	d := resp.Data[0]
	if d.URL == "" {
		return nil, fmt.Errorf("song url unavailable (may need VIP)")
	}

	q := parseQualityFromBR(d.BR)
	format := d.Type
	if format == "" {
		format = "mp3"
	}

	return &DownloadURL{
		URL:      d.URL,
		Quality:  q,
		Format:   format,
		FileSize: d.Size,
	}, nil
}

func (n *NeteaseSource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	nID := n.extractID(id)

	reqURL := fmt.Sprintf("%s/lyric?id=%s", n.baseURL, nID)
	body, err := n.doRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var resp struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
		TLRC struct {
			Lyric string `json:"lyric"`
		} `json:"tlyric"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	if resp.LRC.Lyric == "" {
		return nil, fmt.Errorf("no lyrics available")
	}

	return &LyricsResult{
		LRC:      resp.LRC.Lyric,
		TransLRC: resp.TLRC.Lyric,
		Source:   n.name,
	}, nil
}

func (n *NeteaseSource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	detail, err := n.GetTrackDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.CoverURL == "" {
		return nil, fmt.Errorf("no cover")
	}
	return &CoverResult{
		URL:    detail.CoverURL,
		Source: n.name,
	}, nil
}

func (n *NeteaseSource) doRequest(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
}

func (n *NeteaseSource) extractID(compositeID string) string {
	if idx := strings.Index(compositeID, ":"); idx >= 0 {
		return compositeID[idx+1:]
	}
	return compositeID
}

func parseQualityFromBR(br int) Quality {
	switch {
	case br >= 900000:
		return QualityHiRes
	case br >= 800000:
		return QualityFLAC
	case br >= 300000:
		return Quality320
	default:
		return Quality128
	}
}
