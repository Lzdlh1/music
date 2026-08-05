package playlist

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Track 歌单中的一条曲目
type Track struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
}

// Parser 歌单解析器
type Parser struct {
	log    *zap.Logger
	client *http.Client
}

// NewParser 创建歌单解析器
func NewParser(log *zap.Logger) *Parser {
	return &Parser{
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ParseURL 解析在线歌单 URL
func (p *Parser) ParseURL(url string) ([]Track, error) {
	url = strings.TrimSpace(url)

	switch {
	case strings.Contains(url, "music.163.com"):
		return p.parseNeteasePlaylist(url)
	case strings.Contains(url, "y.qq.com"):
		return p.parseQQMusicPlaylist(url)
	case strings.Contains(url, "open.spotify.com"):
		return p.parseSpotifyPlaylist(url)
	default:
		return nil, fmt.Errorf("不支持的歌单链接: %s", url)
	}
}

// ParseText 解析文本格式歌单（每行: 歌手 - 歌名 或 歌名 - 歌手）
func (p *Parser) ParseText(text string) ([]Track, error) {
	var tracks []Track
	scanner := bufio.NewScanner(strings.NewReader(text))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		track := parseLine(line)
		if track.Title != "" {
			tracks = append(tracks, track)
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("未解析到任何曲目")
	}

	p.log.Info("parsed text playlist", zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// ParseM3U 解析 M3U/M3U8 文件内容
func (p *Parser) ParseM3U(content string) ([]Track, error) {
	var tracks []Track
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentTrack *Track
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#EXTINF:") {
			// #EXTINF:duration,Artist - Title
			info := strings.TrimPrefix(line, "#EXTINF:")
			parts := strings.SplitN(info, ",", 2)
			if len(parts) == 2 {
				duration := 0
				fmt.Sscanf(parts[0], "%d", &duration)
				track := parseLine(parts[1])
				track.Duration = duration
				currentTrack = &track
			}
		} else if !strings.HasPrefix(line, "#") && line != "" && currentTrack != nil {
			tracks = append(tracks, *currentTrack)
			currentTrack = nil
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("未解析到 M3U 曲目")
	}

	p.log.Info("parsed m3u playlist", zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// ParseCSV 解析 CSV 格式（Title,Artist,Album）
func (p *Parser) ParseCSV(content string) ([]Track, error) {
	var tracks []Track
	scanner := bufio.NewScanner(strings.NewReader(content))

	lineNo := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNo++

		if line == "" || lineNo == 1 && (strings.Contains(strings.ToLower(line), "title") || strings.Contains(strings.ToLower(line), "歌名")) {
			continue // skip header
		}

		parts := strings.Split(line, ",")
		track := Track{}
		if len(parts) >= 1 {
			track.Title = strings.TrimSpace(parts[0])
		}
		if len(parts) >= 2 {
			track.Artist = strings.TrimSpace(parts[1])
		}
		if len(parts) >= 3 {
			track.Album = strings.TrimSpace(parts[2])
		}

		if track.Title != "" {
			tracks = append(tracks, track)
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("CSV 中未解析到曲目")
	}

	p.log.Info("parsed csv playlist", zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// parseLine 解析 "Artist - Title" 或 "Title - Artist" 格式
func parseLine(line string) Track {
	separators := []string{" - ", " – ", " — ", "\t"}
	for _, sep := range separators {
		if idx := strings.Index(line, sep); idx > 0 {
			left := strings.TrimSpace(line[:idx])
			right := strings.TrimSpace(line[idx+len(sep):])
			// 启发式判断：如果左侧看起来像歌手名（较短），则 left=artist, right=title
			if len(left) < len(right) {
				return Track{Artist: left, Title: right}
			}
			return Track{Title: left, Artist: right}
		}
	}
	return Track{Title: line}
}

// parseNeteasePlaylist 解析网易云音乐歌单
func (p *Parser) parseNeteasePlaylist(url string) ([]Track, error) {
	// 提取歌单 ID
	id := extractID(url, "playlist?id=", "/playlist/")
	if id == "" {
		return nil, fmt.Errorf("无法提取网易云歌单 ID")
	}

	apiURL := fmt.Sprintf("https://music.163.com/api/playlist/detail?id=%s", id)
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://music.163.com")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch netease playlist: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))

	var result struct {
		Result struct {
			Tracks []struct {
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
				Duration int `json:"duration"`
			} `json:"tracks"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse netease response: %w", err)
	}

	var tracks []Track
	for _, t := range result.Result.Tracks {
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		tracks = append(tracks, Track{
			Title:    t.Name,
			Artist:   strings.Join(artists, "/"),
			Album:    t.Album.Name,
			Duration: t.Duration / 1000,
		})
	}

	p.log.Info("parsed netease playlist", zap.String("id", id), zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// parseQQMusicPlaylist QQ 音乐歌单解析
func (p *Parser) parseQQMusicPlaylist(url string) ([]Track, error) {
	id := extractID(url, "id=", "/playlist/")
	if id == "" {
		return nil, fmt.Errorf("无法提取 QQ 音乐歌单 ID")
	}

	apiURL := fmt.Sprintf("https://c.y.qq.com/v8/fcg-bin/fcg_v8_playlist_cp.fcg?id=%s&format=json&inCharset=utf8&outCharset=utf-8", id)
	req, _ := http.NewRequest(http.MethodGet, apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://y.qq.com")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch qq playlist: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))

	var result struct {
		Data struct {
			CDList []struct {
				SongList []struct {
					SongName string `json:"songname"`
					Singer   []struct {
						Name string `json:"name"`
					} `json:"singer"`
					AlbumName string `json:"albumname"`
					Interval  int    `json:"interval"`
				} `json:"songlist"`
			} `json:"cdlist"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse qq response: %w", err)
	}

	var tracks []Track
	for _, cd := range result.Data.CDList {
		for _, s := range cd.SongList {
			var singers []string
			for _, sg := range s.Singer {
				singers = append(singers, sg.Name)
			}
			tracks = append(tracks, Track{
				Title:    s.SongName,
				Artist:   strings.Join(singers, "/"),
				Album:    s.AlbumName,
				Duration: s.Interval,
			})
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("QQ 音乐歌单为空或无法解析")
	}

	p.log.Info("parsed qq music playlist", zap.String("id", id), zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// parseSpotifyPlaylist Spotify 歌单解析（通过页面抓取基本信息）
func (p *Parser) parseSpotifyPlaylist(url string) ([]Track, error) {
	// Spotify 嵌入页面可获取基本信息
	id := ""
	if idx := strings.Index(url, "/playlist/"); idx >= 0 {
		rest := url[idx+len("/playlist/"):]
		if qIdx := strings.IndexAny(rest, "?#"); qIdx >= 0 {
			id = rest[:qIdx]
		} else {
			id = rest
		}
	}
	if id == "" {
		return nil, fmt.Errorf("无法提取 Spotify 歌单 ID")
	}

	embedURL := fmt.Sprintf("https://open.spotify.com/embed/playlist/%s", id)
	req, _ := http.NewRequest(http.MethodGet, embedURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch spotify embed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	html := string(body)

	// 从 embed 页面提取 resource JSON
	var tracks []Track
	searchStr := `<script id="__NEXT_DATA__" type="application/json">`
	if idx := strings.Index(html, searchStr); idx >= 0 {
		jsonStart := idx + len(searchStr)
		if endIdx := strings.Index(html[jsonStart:], "</script>"); endIdx >= 0 {
			jsonData := html[jsonStart : jsonStart+endIdx]
			tracks = parseSpotifyJSON(jsonData)
		}
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("Spotify 歌单解析失败，可能需要登录或歌单为私有")
	}

	p.log.Info("parsed spotify playlist", zap.String("id", id), zap.Int("tracks", len(tracks)))
	return tracks, nil
}

// parseSpotifyJSON 从 Spotify 嵌入页面 JSON 提取曲目
func parseSpotifyJSON(data string) []Track {
	var nextData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &nextData); err != nil {
		return nil
	}

	var tracks []Track
	// 递归查找 trackList
	findTracks(nextData, &tracks)
	return tracks
}

// findTracks 在 JSON 中递归查找曲目数据
func findTracks(data interface{}, tracks *[]Track) {
	switch v := data.(type) {
	case map[string]interface{}:
		if name, ok := v["name"].(string); ok {
			if artists, ok := v["artists"]; ok {
				// 这可能是一个 track 对象
				track := Track{Title: name}
				if artistList, ok := artists.([]interface{}); ok {
					var names []string
					for _, a := range artistList {
						if am, ok := a.(map[string]interface{}); ok {
							if n, ok := am["name"].(string); ok {
								names = append(names, n)
							}
						}
					}
					track.Artist = strings.Join(names, "/")
				}
				if album, ok := v["album"].(map[string]interface{}); ok {
					if n, ok := album["name"].(string); ok {
						track.Album = n
					}
				}
				if dur, ok := v["duration_ms"].(float64); ok {
					track.Duration = int(dur / 1000)
				}
				if track.Artist != "" {
					*tracks = append(*tracks, track)
					return
				}
			}
		}
		for _, val := range v {
			findTracks(val, tracks)
		}
	case []interface{}:
		for _, item := range v {
			findTracks(item, tracks)
		}
	}
}

// extractID 从 URL 提取 ID
func extractID(url string, patterns ...string) string {
	for _, p := range patterns {
		if idx := strings.Index(url, p); idx >= 0 {
			rest := url[idx+len(p):]
			// 取到下一个非数字字符
			end := 0
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if end > 0 {
				return rest[:end]
			}
		}
	}
	return ""
}
