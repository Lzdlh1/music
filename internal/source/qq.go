// Package source QQ 音乐直连源：通过腾讯 musicu.fcg 网关实现搜索/详情/下载地址/歌词/封面。
// 配置 VIP（豪华绿钻）账号的 y.qq.com Cookie 后，可直接获取 FLAC/Hi-Res 直链，
// 下载得到的为标准无加密音频文件，任意播放器均可播放。
package source

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// QQSource QQ 音乐源
type QQSource struct {
	name     string
	cookie   string
	limit    int      // 每月下载限额（本地统计参考值）
	priority int
	client   *http.Client
	guid     string
	recorder DownloadRecorder // 下载限额计数（可空）
	log      *zap.Logger
}

// QQConfig QQ 音乐源配置
type QQConfig struct {
	Name     string `json:"name"`
	Cookie   string `json:"cookie"` // 浏览器登录 y.qq.com 后的完整 Cookie（含 uin=、skey=）
	Limit    int    `json:"limit"`  // 每月下载限额（豪华绿钻默认 300）
	Priority int    `json:"priority"`
	Timeout  int    `json:"timeout"` // seconds
}

// DownloadRecorder 下载额度计数接口（由应用层提供实现）
type DownloadRecorder interface {
	RecordDownload(ctx context.Context, sourceName string, limit int) error
}

const musicuGateway = "https://u.y.qq.com/cgi-bin/musicu.fcg"

// NewQQSource 创建 QQ 音乐源
func NewQQSource(cfg QQConfig, recorder DownloadRecorder, log *zap.Logger) *QQSource {
	timeout := 30
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	if cfg.Limit == 0 {
		cfg.Limit = 300
	}
	if cfg.Name == "" {
		cfg.Name = "qq"
	}
	return &QQSource{
		name:     cfg.Name,
		cookie:   cfg.Cookie,
		limit:    cfg.Limit,
		priority: cfg.Priority,
		client:   &http.Client{Timeout: time.Duration(timeout) * time.Second},
		guid:     randomHex(32),
		recorder: recorder,
		log:      log,
	}
}

func (q *QQSource) Name() string     { return q.name }
func (q *QQSource) Priority() int    { return q.priority }

// IsAvailable 校验 Cookie 存在且腾讯音乐域名可访问
func (q *QQSource) IsAvailable(ctx context.Context) bool {
	if q.cookie == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://y.qq.com/", nil)
	if err != nil {
		return false
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// ---------- 搜索 ----------

type qqSong struct {
	Mid      string `json:"mid"`
	SongID   int    `json:"id"`
	Name     string `json:"name"`
	Singer   []struct {
		Name string `json:"name"`
	} `json:"singer"`
	Album struct {
		Mid  string `json:"mid"`
		Name string `json:"name"`
	} `json:"album"`
	AlbumName string `json:"albumname"`
	Interval  int   `json:"interval"`
	File      struct {
		Mid      string `json:"media_mid"`
		Size128  int64  `json:"size_128mp3"`
		Size320  int64  `json:"size_320mp3"`
		SizeFlac int64  `json:"size_flac"`
		SizeHires int64 `json:"size_hires"`
	} `json:"file"`
}

func (q *QQSource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	keyword := query.Keyword
	if query.Artist != "" {
		keyword = query.Artist + " " + keyword
	}
	limit := query.PageSize
	if limit == 0 {
		limit = 20
	}
	page := query.Page
	if page < 1 {
		page = 1
	}

	body, err := q.musicu(ctx, map[string]interface{}{
		"module": "music.search.SearchCgiService",
		"method": "DoSearchForQQMusicDesktop",
		"param": map[string]interface{}{
			"grp":          1,
			"num_per_page": limit,
			"page_num":     page,
			"query":        keyword,
			"search_type":  0,
		},
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Req0 struct {
			Code int `json:"code"`
			Data struct {
				Body struct {
					Song struct {
						List []qqSong `json:"list"`
					} `json:"song"`
				} `json:"body"`
			} `json:"data"`
		} `json:"req_0"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse qq search response: %w", err)
	}

	var results []TrackResult
	for _, s := range resp.Req0.Data.Body.Song.List {
		if s.Mid == "" {
			continue
		}
		var artists []string
		for _, a := range s.Singer {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		album := s.Album.Name
		if album == "" {
			album = s.AlbumName
		}
		quality := qqHighestQuality(s.File.SizeHires, s.File.SizeFlac, s.File.Size320, s.File.Size128)
		score := float64(quality)
		if score == 0 {
			score = 3
		}
		results = append(results, TrackResult{
			ID:       qqTrackID(q.name, s.SongID, s.Mid, s.File.Mid),
			Title:    s.Name,
			Artist:   strings.Join(artists, "/"),
			Album:    album,
			Duration: s.Interval,
			Quality:  quality,
			Source:   q.name,
			Score:    score + float64(q.priority),
		})
	}
	return results, nil
}

// ---------- 详情 ----------

func (q *QQSource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	_, mid, _ := parseQQTrackID(id)
	if mid == "" {
		return nil, fmt.Errorf("invalid qq track id")
	}

	// 老单曲接口：无需登录即可返回名称/歌手/专辑/时长，并可直接拼封面图
	reqURL := fmt.Sprintf(
		"https://c.y.qq.com/v8/fcg-bin/fcg_play_single_song.fcg?songmid=%s&format=json&utf8=1&outCharset=utf-8&loginUin=0&platform=yqq&needNewCode=1",
		mid)
	body, err := q.get(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	body = stripJSONP(body)

	var resp struct {
		Code int `json:"code"`
		Data []struct {
			Mid       string `json:"mid"`
			Name      string `json:"name"`
			Singer    []struct {
				Name string `json:"name"`
			} `json:"singer"`
			AlbumName string `json:"albumname"`
			Album     struct {
				Mid  string `json:"mid"`
				Name string `json:"name"`
			} `json:"album"`
			AlbumMid string `json:"albummid"`
			Interval int `json:"interval"`
			NewStatus int `json:"newstatus"` // 兼容字段占位
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Code != 0 || len(resp.Data) == 0 {
		// 兜底：至少给出可由 ID 推导的最小详情，避免任务流程中断
		return &TrackDetail{
			ID:     id,
			Title:  mid,
			Source: q.name,
		}, nil
	}
	d := resp.Data[0]
	var artists []string
	for _, a := range d.Singer {
		if a.Name != "" {
			artists = append(artists, a.Name)
		}
	}
	album := d.Album.Name
	if album == "" {
		album = d.AlbumName
	}
	albumMid := d.Album.Mid
	if albumMid == "" {
		albumMid = d.AlbumMid
	}
	coverURL := ""
	if albumMid != "" {
		coverURL = fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T002R800x800M000%s.jpg", albumMid)
	}
	return &TrackDetail{
		ID:       id,
		Title:    d.Name,
		Artist:   strings.Join(artists, "/"),
		Album:    album,
		Duration: d.Interval,
		CoverURL: coverURL,
		Source:   q.name,
	}, nil
}

// ---------- 下载地址 ----------

func (q *QQSource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	_, mid, mediaMid := parseQQTrackID(id)
	if mid == "" {
		return nil, fmt.Errorf("invalid qq track id")
	}

	// 按请求音质决定尝试顺序（腾讯 filename 规则：F000=FLAC，M500=320K，C400=128K）
	var candidates []string
	switch {
	case quality >= QualityFLAC || quality == QualityAny:
		candidates = []string{"F000", "M500", "C400"}
	case quality >= Quality320:
		candidates = []string{"M500", "C400"}
	default:
		candidates = []string{"C400"}
	}
	var lastErr string
	for _, prefix := range candidates {
		fname := qqFilename(prefix, mediaMid, mid)
		body, err := q.musicu(ctx, map[string]interface{}{
			"module": "vkey.GetVkeyServer",
			"method": "CgiGetVkey",
			"param": map[string]interface{}{
				"guid":        q.guid,
				"filename":    []string{fname},
				"songmid":     []string{mid},
				"songtype":    []int{0},
				"uin":         "0", // 登录态由 Cookie/authst 判定
				"loginflag":   1,
				"platform":    "20",
				"needNewCode": 1,
				"req_type":    1,
			},
		})
		if err != nil {
			lastErr = err.Error()
			continue
		}
		var resp struct {
			Req0 struct {
				Code int `json:"code"`
				Data struct {
					Sip []string `json:"sip"`
					MidURLInfo []struct {
						Purl string `json:"purl"`
					} `json:"midurlinfo"`
				} `json:"data"`
			} `json:"req_0"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			lastErr = err.Error()
			continue
		}
		if len(resp.Req0.Data.MidURLInfo) == 0 || resp.Req0.Data.MidURLInfo[0].Purl == "" {
			lastErr = "当前账号无此音质权限（需 VIP 或 未登录）"
			continue
		}
		purl := resp.Req0.Data.MidURLInfo[0].Purl
		prefixHost := ""
		for _, s := range resp.Req0.Data.Sip {
			if s != "" {
				prefixHost = s
				break
			}
		}

		return &DownloadURL{
			URL:      prefixHost + purl,
			Quality:  qqQualityFromPurl(purl),
			Format:   qqFormatFromPurl(purl),
			FileSize: 0, // 腾讯接口不返回大小，由下载后文件决定
		}, nil
	}
	if lastErr != "" {
		return nil, fmt.Errorf("qq 获取下载地址失败: %s", lastErr)
	}
	return nil, fmt.Errorf("qq 无法获取下载地址")
}

// RecordDownload 记录一次成功下载（由 worker 在任务下载成功后调用，计入每月限额）
func (q *QQSource) RecordDownload(ctx context.Context) error {
	if q.recorder == nil {
		return nil
	}
	return q.recorder.RecordDownload(ctx, q.name, q.limit)
}

// ---------- 歌词 ----------

func (q *QQSource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	songid, mid, _ := parseQQTrackID(id)
	if mid == "" {
		return nil, fmt.Errorf("invalid qq track id")
	}

	body, err := q.musicu(ctx, map[string]interface{}{
		"module": "music.musichallSong.PlayLyricInfo",
		"method": "GetPlayLyricInfo",
		"param": map[string]interface{}{
			"songMID": mid,
			"songID":  songid,
		},
	})
	if err != nil {
		return nil, err
	}

	var resp struct {
		Req0 struct {
			Code int `json:"code"`
			Data struct {
				Lyric  string `json:"lyric"`
				TLyric string `json:"tlyric"`
			} `json:"data"`
		} `json:"req_0"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if resp.Req0.Data.Lyric == "" {
		return nil, fmt.Errorf("no lyrics available")
	}
	return &LyricsResult{
		LRC:      resp.Req0.Data.Lyric,
		TransLRC: resp.Req0.Data.TLyric,
		Source:   q.name,
	}, nil
}

// ---------- 封面 ----------

func (q *QQSource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	detail, err := q.GetTrackDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	if detail.CoverURL == "" {
		return nil, fmt.Errorf("no cover")
	}
	return &CoverResult{URL: detail.CoverURL, Source: q.name}, nil
}

// ---------- 内部工具 ----------

// musicu 调用腾讯音乐统一网关，带公共 comm 与 Cookie
func (q *QQSource) musicu(ctx context.Context, req map[string]interface{}) ([]byte, error) {
	comm := map[string]interface{}{
		"ct":     19,
		"cv":     1843,
		"uin":    qqUin(q.cookie),
		"format": "json",
	}
	// 新版登录态：VPN 音质需要 comm.authst（musicKey，来自浏览器 qm_keyst/qqmusic_key）
	if key := qqMusicKey(q.cookie); key != "" {
		comm["authst"] = key
	}
	payload := map[string]interface{}{
		"comm":  comm,
		"req_0": req,
	}
	js, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, musicuGateway, strings.NewReader(string(js)))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json;charset=UTF-8")
	hreq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	hreq.Header.Set("Referer", "https://y.qq.com/")
	hreq.Header.Set("Origin", "https://y.qq.com")
	if q.cookie != "" {
		hreq.Header.Set("Cookie", q.cookie)
	}

	resp, err := q.client.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("qq musicu http: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, err
	}

	// 网关公共错误
	var wrapper struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Code != 0 {
		return nil, fmt.Errorf("qq musicu error %d: %s", wrapper.Code, wrapper.Message)
	}
	// 请求块级错误
	var reqCheck struct {
		Req0 struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"req_0"`
	}
	if err := json.Unmarshal(data, &reqCheck); err == nil && reqCheck.Req0.Code != 0 {
		return nil, fmt.Errorf("qq musicu req error %d: %s", reqCheck.Req0.Code, reqCheck.Req0.Msg)
	}
	return data, nil
}

// get 发送 GET 请求
func (q *QQSource) get(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qq http: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
}

// qqTrackID 构造聚合器格式的曲目 ID：源名:songid|songmid|media_mid
func qqTrackID(name string, songid int, mid, mediaMid string) string {
	return fmt.Sprintf("%s:%d|%s|%s", name, songid, mid, mediaMid)
}

// parseQQTrackID 解析聚合器格式的曲目 ID，返回 (songid, songmid, media_mid)
func parseQQTrackID(id string) (songid int, mid, mediaMid string) {
	raw := id
	if idx := strings.Index(raw, ":"); idx >= 0 {
		raw = raw[idx+1:]
	}
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) >= 2 {
		songid, _ = strconv.Atoi(parts[0])
		mid = parts[1]
	}
	if len(parts) == 3 {
		mediaMid = parts[2]
	}
	return
}

// qqFilename 拼接腾讯下载档位文件名（F000=FLAC，M500=320K，C400=128K）
func qqFilename(prefix, mediaMid, fallback string) string {
	id := mediaMid
	if id == "" {
		id = fallback
	}
	switch prefix {
	case "F000":
		return "F000" + id + ".flac"
	case "M500":
		return "M500" + id + ".mp3"
	default:
		return "C400" + id + ".m4a"
	}
}

// qqUin 从 Cookie 中提取 uin（去掉 o 前缀），无则返回空
func qqUin(cookie string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "uin") {
			v := strings.TrimSpace(kv[1])
			if strings.HasPrefix(strings.ToLower(v), "o") {
				v = v[1:]
			}
			return v
		}
	}
	return ""
}

// qqMusicKey 从 Cookie 中提取新版登录态 musicKey（qm_keyst 或 qqmusic_key）
func qqMusicKey(cookie string) string {
	key := cookiesGet(cookie, "qm_keyst")
	if key == "" {
		key = cookiesGet(cookie, "qqmusic_key")
	}
	return key
}

// cookiesGet 从 Cookie 串中取指定键的值
func cookiesGet(cookie, name string) string {
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), name) {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// qqHighestQuality 由歌曲各音质大小字段推断可用最高音质
func qqHighestQuality(hires, flac, q320, q128 int64) Quality {
	switch {
	case hires > 0:
		return QualityHiRes
	case flac > 0:
		return QualityFLAC
	case q320 > 0:
		return Quality320
	case q128 > 0:
		return Quality128
	default:
		return QualityAny
	}
}

// qqQualityFromPurl 由下载文件名推断音质（C400=128K，M500=320K，F000=FLAC，A000=Hi-Res）
func qqQualityFromPurl(purl string) Quality {
	name := path.Base(purl)
	switch {
	case strings.Contains(name, "A000"):
		return QualityHiRes
	case strings.Contains(name, "F000"), strings.HasSuffix(strings.ToLower(purl), ".flac"):
		return QualityFLAC
	case strings.Contains(name, "M500"), strings.Contains(name, "M800"):
		return Quality320
	default:
		return Quality128
	}
}

// qqFormatFromPurl 由下载文件名推断格式（URL 可能带 query，需先截断）
func qqFormatFromPurl(purl string) string {
	clean := purl
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i]
	}
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(clean)), ".")
	switch ext {
	case "mp3", "flac", "m4a", "aac", "ogg", "ape", "wav":
		return ext
	default:
		return "mp3"
	}
}

// randomHex 生成 n 位随机十六进制字符串（设备指纹/请求标识）
func randomHex(n int) string {
	b := make([]byte, (n+1)/2)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}

// stripJSONP 剥除 JSONP 回调包裹（老 fcg 接口可能带 callback(...) 或前导字符串）
func stripJSONP(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if i := strings.Index(s, "("); i >= 0 && strings.HasSuffix(s, ")") {
		return []byte(s[i+1 : len(s)-1])
	}
	return b
}