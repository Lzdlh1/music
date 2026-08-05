package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"go.uber.org/zap"
)

// LyricsProvider 歌词提供者接口
type LyricsProvider interface {
	Name() string
	GetLyrics(ctx context.Context, title, artist string) (*LyricsData, error)
}

// LyricsData 歌词数据
type LyricsData struct {
	LRC      string `json:"lrc"`
	TransLRC string `json:"trans_lrc,omitempty"`
	Source   string `json:"source"`
}

// LyricsManager 歌词管理器
type LyricsManager struct {
	providers []LyricsProvider
	log       *zap.Logger
}

// NewLyricsManager 创建歌词管理器
func NewLyricsManager(log *zap.Logger) *LyricsManager {
	lm := &LyricsManager{log: log}
	// 注册默认提供者
	lm.providers = append(lm.providers, &LRCLibProvider{})
	return lm
}

// RegisterProvider 注册歌词提供者
func (lm *LyricsManager) RegisterProvider(p LyricsProvider) {
	lm.providers = append(lm.providers, p)
}

// FetchLyrics 按优先级逐一尝试获取歌词
func (lm *LyricsManager) FetchLyrics(ctx context.Context, title, artist string) (*LyricsData, error) {
	for _, p := range lm.providers {
		lyrics, err := p.GetLyrics(ctx, title, artist)
		if err != nil {
			lm.log.Debug("lyrics provider failed",
				zap.String("provider", p.Name()),
				zap.Error(err))
			continue
		}
		if lyrics != nil && lyrics.LRC != "" {
			lm.log.Info("lyrics found",
				zap.String("provider", p.Name()),
				zap.String("title", title),
				zap.String("artist", artist))
			return lyrics, nil
		}
	}
	return nil, fmt.Errorf("no lyrics found for %s - %s", artist, title)
}

// MergeBilingualLRC 合并原文和翻译歌词为双语 LRC
func MergeBilingualLRC(original, translation string) string {
	origLines := parseLRCLines(original)
	transLines := parseLRCLines(translation)

	transMap := make(map[string]string)
	for _, line := range transLines {
		transMap[line.timestamp] = line.text
	}

	var result strings.Builder
	for _, line := range origLines {
		result.WriteString(fmt.Sprintf("%s%s\n", line.timestamp, line.text))
		if trans, ok := transMap[line.timestamp]; ok {
			result.WriteString(fmt.Sprintf("%s%s\n", line.timestamp, trans))
		}
	}
	return result.String()
}

type lrcLine struct {
	timestamp string
	text      string
}

func parseLRCLines(lrc string) []lrcLine {
	var lines []lrcLine
	for _, line := range strings.Split(lrc, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 10 || line[0] != '[' {
			continue
		}
		closeBracket := strings.Index(line, "]")
		if closeBracket < 0 {
			continue
		}
		lines = append(lines, lrcLine{
			timestamp: line[:closeBracket+1],
			text:      line[closeBracket+1:],
		})
	}
	return lines
}

// LRCLibProvider LRClib.net 歌词提供者（免费开放 API）
type LRCLibProvider struct{}

func (l *LRCLibProvider) Name() string { return "LRClib" }

func (l *LRCLibProvider) GetLyrics(ctx context.Context, title, artist string) (*LyricsData, error) {
	reqURL := fmt.Sprintf("https://lrclib.net/api/get?track_name=%s&artist_name=%s",
		neturl.QueryEscape(title), neturl.QueryEscape(artist))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create lrclib request: %w", err)
	}
	req.Header.Set("User-Agent", "MusicFlow/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lrclib request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lrclib status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read lrclib response: %w", err)
	}

	var result struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode lrclib response: %w", err)
	}

	lrc := result.SyncedLyrics
	if lrc == "" {
		lrc = result.PlainLyrics
	}

	return &LyricsData{
		LRC:    lrc,
		Source: "LRClib",
	}, nil
}
