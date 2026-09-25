// 咪咕音乐直连源。
//
// 搜索 / 封面 / 歌词走咪咕 H5 开放接口，无需登录即可拿到完整元数据；
// 播放与下载由 listenSong.do 直接返回音频字节流（该地址本身即下载直链）。
// 免费曲目免登录可用；无损（SQ/ZQ24）与 VIP 曲目需要在配置中填入
// 登录后的 token 与 userId（从咪咕客户端抓包获取）。
package source

import (
	"context"
	"encoding/base64"
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

// MiguSource 咪咕音乐源
type MiguSource struct {
	name     string
	token    string // 登录 token（可选，VIP/无损需要）
	userID   string // 账号 ID，缺省使用公共 ID
	priority int
	client   *http.Client
	log      *zap.Logger
}

// MiguConfig 咪咕音乐源配置
type MiguConfig struct {
	Name     string `json:"name"`
	Token    string `json:"token"`   // 可选：咪咕登录 token
	UserID   string `json:"user_id"` // 可选：咪咕账号 ID
	Priority int    `json:"priority"`
	Timeout  int    `json:"timeout"` // seconds
}

const (
	miguSearchAPI = "https://c.musicapp.migu.cn/v1.0/content/search_all.do"
	miguPlayAPI   = "https://app.c.nf.migu.cn/MIGUM2.0/v1.0/content/sub/listenSong.do"
	miguChannel   = "0140210"
	miguVersion   = "6.0.0"
	// 咪咕公开账号 ID：未登录时用于换取免费曲目的试听/下载地址
	miguPublicUID = "15548614588710179085069"

	miguSearchSwitch = `{"song":1,"album":0,"singer":0,"tagSong":1,"mvSong":0,"bestShow":1}`
)

// NewMiguSource 创建咪咕音乐源
func NewMiguSource(cfg MiguConfig, log *zap.Logger) *MiguSource {
	timeout := 30
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	if cfg.Name == "" {
		cfg.Name = "migu"
	}
	if strings.TrimSpace(cfg.UserID) == "" {
		cfg.UserID = miguPublicUID
	}
	return &MiguSource{
		name:     cfg.Name,
		token:    strings.TrimSpace(cfg.Token),
		userID:   strings.TrimSpace(cfg.UserID),
		priority: cfg.Priority,
		client:   &http.Client{Timeout: time.Duration(timeout) * time.Second},
		log:      log,
	}
}

func (m *MiguSource) Name() string  { return m.name }
func (m *MiguSource) Priority() int { return m.priority }

// IsAvailable 用最小搜索请求探测接口可用性
func (m *MiguSource) IsAvailable(ctx context.Context) bool {
	q := url.Values{}
	q.Set("text", "test")
	q.Set("pageNo", "1")
	q.Set("pageSize", "1")
	q.Set("isCopyright", "1")
	q.Set("sort", "1")
	q.Set("searchSwitch", miguSearchSwitch)
	body, err := m.get(ctx, miguSearchAPI+"?"+q.Encode())
	if err != nil {
		return false
	}
	var resp struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return resp.Code == "000000"
}

// ---------- 搜索 ----------

type miguRateFormat struct {
	ResourceType string   `json:"resourceType"`
	FormatType   string   `json:"formatType"`
	Format       string   `json:"format"`
	Size         string   `json:"size"`
	FileType     string   `json:"fileType"`
	ShowTag      []string `json:"showTag"`
}

type miguImgItem struct {
	ImgSizeType string `json:"imgSizeType"`
	Img         string `json:"img"`
}

type miguSong struct {
	Name        string        `json:"name"`
	CopyrightID string        `json:"copyrightId"`
	ContentID   string        `json:"contentId"`
	LyricURL    string        `json:"lyricUrl"`
	Singers     []struct {
		Name string `json:"name"`
	} `json:"singers"`
	Albums []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"albums"`
	ImgItems    []miguImgItem    `json:"imgItems"`
	RateFormats []miguRateFormat `json:"rateFormats"`
}

type miguSearchResp struct {
	Code           string `json:"code"`
	Info           string `json:"info"`
	SongResultData struct {
		Result []miguSong `json:"result"`
	} `json:"songResultData"`
}

func (m *MiguSource) Search(ctx context.Context, query SearchQuery) ([]TrackResult, error) {
	keyword := strings.TrimSpace(query.Keyword)
	if query.Artist != "" {
		keyword = strings.TrimSpace(query.Artist + " " + keyword)
	}
	if keyword == "" {
		return nil, nil
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	size := query.PageSize
	if size <= 0 {
		size = 20
	}

	q := url.Values{}
	q.Set("text", keyword)
	q.Set("pageNo", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(size))
	q.Set("isCopyright", "1")
	q.Set("sort", "1")
	q.Set("searchSwitch", miguSearchSwitch)

	body, err := m.get(ctx, miguSearchAPI+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var resp miguSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse migu search response: %w", err)
	}
	if resp.Code != "" && resp.Code != "000000" {
		return nil, fmt.Errorf("migu search error %s: %s", resp.Code, resp.Info)
	}

	results := make([]TrackResult, 0, len(resp.SongResultData.Result))
	for _, s := range resp.SongResultData.Result {
		if s.CopyrightID == "" || s.ContentID == "" || s.Name == "" {
			continue
		}
		albumID, albumName := "", ""
		if len(s.Albums) > 0 {
			albumID = s.Albums[0].ID
			albumName = s.Albums[0].Name
		}
		var artists []string
		for _, a := range s.Singers {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		cover := miguBestImg(s.ImgItems)
		quality := miguQualityFromFormats(s.RateFormats)
		score := float64(quality)
		if score == 0 {
			score = 3
		}
		results = append(results, TrackResult{
			ID:       miguTrackID(m.name, s.CopyrightID, s.ContentID, albumID, quality, cover, s.LyricURL),
			Title:    s.Name,
			Artist:   strings.Join(artists, "/"),
			Album:    albumName,
			Quality:  quality,
			FileSize: miguSizeOfFormat(s.RateFormats, quality),
			Source:   m.name,
			CoverURL: cover,
			Score:    score + float64(m.priority),
		})
	}
	return results, nil
}

// ---------- 详情 ----------

// GetTrackDetail 咪咕未提供按 ID 查询的公开详情接口，
// 这里返回可由曲目 ID 还原的最小详情（封面可用），
// 标题等元数据由前端搜索结果或其它源补齐。
func (m *MiguSource) GetTrackDetail(ctx context.Context, id string) (*TrackDetail, error) {
	copyrightID, contentID, _, _, cover, _ := parseMiguTrackID(id)
	if copyrightID == "" || contentID == "" {
		return nil, fmt.Errorf("invalid migu track id")
	}
	return &TrackDetail{
		ID:       id,
		CoverURL: cover,
		Source:   m.name,
	}, nil
}

// ---------- 下载 / 试听 ----------

// GetDownloadURL 返回可直接下载的音频地址。
// listenSong.do 本身即音频字节流地址，无需二次跳转。
func (m *MiguSource) GetDownloadURL(ctx context.Context, id string, quality Quality) (*DownloadURL, error) {
	copyrightID, contentID, albumID, bestQuality, _, _ := parseMiguTrackID(id)
	if copyrightID == "" || contentID == "" {
		return nil, fmt.Errorf("invalid migu track id")
	}

	// 按请求音质决定尝试顺序（咪咕：HQ=320K，SQ=无损，ZQ24=24bit）
	var candidates []string
	switch {
	case quality >= QualityHiRes || quality == QualityAny:
		candidates = []string{"ZQ24", "SQ", "HQ", "PQ"}
	case quality >= QualityFLAC:
		candidates = []string{"SQ", "HQ", "PQ"}
	case quality >= Quality320:
		candidates = []string{"HQ", "PQ"}
	default:
		candidates = []string{"PQ", "LQ"}
	}

	var lastErr string
	for _, flag := range candidates {
		playURL := m.playURL(flag, copyrightID, contentID, albumID)
		contentType, size, code, err := m.probe(ctx, playURL)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if code != "" {
			// 业务错误：无版权 / 需登录，换档位也无法取得
			if strings.HasPrefix(code, "2000") {
				return nil, fmt.Errorf("咪咕暂不提供该曲目地址（%s），VIP/无损曲目请在源配置中填入登录 token", code)
			}
			lastErr = "migu returned code " + code
			continue
		}
		if !miguIsAudio(contentType) {
			lastErr = "migu returned non-audio content: " + contentType
			continue
		}
		// 咪咕会自动回退到「不超过请求档位的最高可用档位」，据此推算真实音质
		actual := miguQualityFromFlag(flag)
		if bestQuality != QualityAny && bestQuality < actual {
			actual = bestQuality
		}
		if miguIsLossless(contentType) {
			if actual < QualityFLAC {
				actual = QualityFLAC
			}
		} else if actual >= QualityFLAC {
			actual = Quality320
		}
		return &DownloadURL{
			URL:      playURL,
			Quality:  actual,
			Format:   miguFormatFromContentType(contentType, flag),
			FileSize: size,
		}, nil
	}
	if lastErr != "" {
		return nil, fmt.Errorf("咪咕获取下载地址失败: %s", lastErr)
	}
	return nil, fmt.Errorf("咪咕无法获取下载地址")
}

// ---------- 歌词 / 封面 ----------

// GetLyrics 取咪咕歌词（歌词地址已随搜索结果写入曲目 ID）
func (m *MiguSource) GetLyrics(ctx context.Context, id string) (*LyricsResult, error) {
	_, _, _, _, _, lyricURL := parseMiguTrackID(id)
	if lyricURL == "" {
		return nil, fmt.Errorf("no lyrics url for migu track")
	}
	body, err := m.get(ctx, lyricURL)
	if err != nil {
		return nil, err
	}
	lrc := strings.TrimSpace(string(body))
	if lrc == "" {
		return nil, fmt.Errorf("empty lyrics")
	}
	return &LyricsResult{LRC: lrc, Source: m.name}, nil
}

// GetCover 取咪咕封面（封面地址已随搜索结果写入曲目 ID）
func (m *MiguSource) GetCover(ctx context.Context, id string) (*CoverResult, error) {
	_, _, _, _, cover, _ := parseMiguTrackID(id)
	if cover == "" {
		return nil, fmt.Errorf("no cover url for migu track")
	}
	return &CoverResult{URL: cover, Source: m.name}, nil
}

// ---------- 内部工具 ----------

// playURL 拼接 listenSong.do 地址；该地址直接返回音频字节流
func (m *MiguSource) playURL(toneFlag, copyrightID, contentID, albumID string) string {
	q := url.Values{}
	q.Set("toneFlag", toneFlag)
	q.Set("netType", "01")
	q.Set("userId", m.userID)
	q.Set("ua", "Android_migu")
	q.Set("version", miguVersion)
	q.Set("copyrightId", copyrightID)
	q.Set("contentId", contentID)
	q.Set("resourceType", "2")
	q.Set("channel", miguChannel)
	if albumID != "" {
		q.Set("albumId", albumID)
	}
	return miguPlayAPI + "?" + q.Encode()
}

// probe 探测音频地址：只读响应头（尝试 Range 以省流量），
// 返回 Content-Type、文件大小与业务错误码（非音频响应时解析 JSON 错误体）。
func (m *MiguSource) probe(ctx context.Context, rawURL string) (contentType string, size int64, code string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, "", err
	}
	m.applyHeaders(req)
	req.Header.Set("Range", "bytes=0-0")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	contentType = resp.Header.Get("Content-Type")
	if resp.StatusCode == http.StatusPartialContent {
		// Content-Range: bytes 0-0/3715450
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			if i := strings.LastIndex(cr, "/"); i >= 0 {
				size, _ = strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64)
			}
		}
		if size == 0 {
			size = resp.ContentLength
		}
	} else {
		size = resp.ContentLength
	}

	if miguIsAudio(contentType) {
		return contentType, size, "", nil
	}

	// 非音频：多半是 JSON 业务错误（如未登录 / 无版权）
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Code string `json:"code"`
		Info string `json:"info"`
	}
	if json.Unmarshal(head, &e) == nil && e.Code != "" {
		return contentType, 0, e.Code, nil
	}
	return contentType, 0, "", nil
}

// get 发起带咪咕公共请求头的 GET
func (m *MiguSource) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	m.applyHeaders(req)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("migu http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("migu read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("migu http %d", resp.StatusCode)
	}
	return body, nil
}

// applyHeaders 设置咪咕接口要求的公共请求头
func (m *MiguSource) applyHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("channel", miguChannel)
	req.Header.Set("ua", "Android_migu")
	req.Header.Set("version", miguVersion)
	req.Header.Set("appId", "music")
	if m.token != "" {
		req.Header.Set("token", m.token)
	}
}

// miguTrackID 组装聚合器格式的曲目 ID：
// migu:{copyrightId}|{contentId}|{albumId}|{最高可用音质}|{封面B64}|{歌词B64}
// 封面与歌词均用 base64url 编码，避免 URL 中出现 %2F 等被网关改写。
func miguTrackID(name, copyrightID, contentID, albumID string, quality Quality, cover, lyricURL string) string {
	return fmt.Sprintf("%s:%s|%s|%s|%d|%s|%s", name, copyrightID, contentID, albumID, int(quality),
		base64.RawURLEncoding.EncodeToString([]byte(cover)),
		base64.RawURLEncoding.EncodeToString([]byte(lyricURL)))
}

// parseMiguTrackID 解析曲目 ID，返回 (copyrightId, contentId, albumId, 最高音质, 封面, 歌词地址)
func parseMiguTrackID(id string) (copyrightID, contentID, albumID string, best Quality, cover, lyricURL string) {
	raw := id
	if idx := strings.Index(raw, ":"); idx >= 0 {
		raw = raw[idx+1:]
	}
	parts := strings.SplitN(raw, "|", 6)
	for len(parts) < 6 {
		parts = append(parts, "")
	}
	if n, err := strconv.Atoi(parts[3]); err == nil {
		best = Quality(n)
	}
	decode := func(s string) string {
		if s == "" {
			return ""
		}
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return ""
		}
		return string(b)
	}
	return parts[0], parts[1], parts[2], best, decode(parts[4]), decode(parts[5])
}

// miguQualityFromFormats 按档位列表推断最高可用音质
func miguQualityFromFormats(formats []miguRateFormat) Quality {
	best := Quality(0)
	for _, f := range formats {
		q := miguQualityFromFlag(f.FormatType)
		if q > best {
			best = q
		}
	}
	return best
}

// miguQualityFromFlag 档位标识 → 统一音质枚举
func miguQualityFromFlag(flag string) Quality {
	switch strings.ToUpper(flag) {
	case "ZQ", "ZQ24":
		return QualityHiRes
	case "SQ":
		return QualityFLAC
	case "HQ":
		return Quality320
	case "PQ", "LQ":
		return Quality128
	default:
		return QualityAny
	}
}

// miguSizeOfFormat 取指定音质对应的文件大小（字节）
func miguSizeOfFormat(formats []miguRateFormat, quality Quality) int64 {
	var best int64
	for _, f := range formats {
		if miguQualityFromFlag(f.FormatType) != quality {
			continue
		}
		if n, err := strconv.ParseInt(f.Size, 10, 64); err == nil && n > best {
			best = n
		}
	}
	return best
}

// miguBestImg 取尺寸最大的一张封面
func miguBestImg(items []miguImgItem) string {
	best, bestType := "", ""
	for _, it := range items {
		if it.Img == "" {
			continue
		}
		if best == "" || it.ImgSizeType > bestType {
			best, bestType = it.Img, it.ImgSizeType
		}
	}
	return best
}

// miguIsAudio 判断响应是否为音频流
func miguIsAudio(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "audio/") || strings.Contains(ct, "octet-stream")
}

// miguIsLossless 判断响应是否为无损音频
func miguIsLossless(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "flac") || strings.Contains(ct, "wav")
}

// miguFormatFromContentType 由响应类型推断扩展名，兜底按档位判断
func miguFormatFromContentType(contentType, flag string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "flac"):
		return "flac"
	case strings.Contains(ct, "mpeg"), strings.Contains(ct, "mp3"):
		return "mp3"
	case strings.Contains(ct, "mp4"), strings.Contains(ct, "m4a"), strings.Contains(ct, "aac"):
		return "m4a"
	}
	if q := miguQualityFromFlag(flag); q >= QualityFLAC {
		return "flac"
	}
	return "mp3"
}