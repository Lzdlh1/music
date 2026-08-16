// Package yun139 中国移动云盘（139 云盘）适配器
// 基于新版个人云（hcy）接口，认证使用网页端获取的 token（Base64 编码）。
package yun139

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
)

const (
	routeURL = "https://user-njs.yun.139.com/user/route/qryRoutePolicy"
)

// Config 139 云盘配置
type Config struct {
	Token        string `json:"token"`          // 网页端登录获取的 Authorization（Base64，不含 Basic 前缀）
	UserDomainID string `json:"user_domain_id"` // 用户域 ID
	PersonalHost string `json:"personal_host"`  // 个人云动态 host（登录时从 routerInfo 获取，缓存避免每次查询）
}

// Yun139Storage 139 云盘存储后端
type Yun139Storage struct {
	id        string
	name      string
	cfg       Config
	client    *http.Client
	log       *zap.Logger
	mu        sync.Mutex
	host      string
	account   string
	visitorID string // 设备指纹，部分接口签名校验需要
	skeyOnce  sync.Once
	skeyErr   error
	secretKey string // mcloud-skey：RSA 加密的随机 AESKey
}

// New 创建 139 云盘存储
func New(id, name string, cfg Config, log *zap.Logger) *Yun139Storage {
	if log == nil {
		log = zap.NewNop()
	}
	return &Yun139Storage{
		id:        id,
		name:      name,
		cfg:       cfg,
		client:    &http.Client{Timeout: 60 * time.Second},
		log:       log,
		visitorID: randomHex(32),
	}
}

func (s *Yun139Storage) ID() string               { return s.id }
func (s *Yun139Storage) Name() string              { return s.name }
func (s *Yun139Storage) Type() storage.StorageType { return storage.StorageYun139 }

// firstBytes 取字节切片前 n 字节
func firstBytes(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// ---------- 认证与路由 ----------

// accountName 从 token 中解析账号（Base64(uid:account:token|v2|v3|expireMs)）
func (s *Yun139Storage) accountName() string {
	raw, err := base64.StdEncoding.DecodeString(s.cfg.Token)
	if err != nil {
		return ""
	}
	parts := strings.SplitN(string(raw), ":", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// ensureSecretKey 获取 RSA 公钥并生成 mcloud-skey（懒加载，只执行一次）
func (s *Yun139Storage) ensureSecretKey(ctx context.Context) {
	s.skeyOnce.Do(func() {
		plainJSON := []byte(`{"clientCode":"10701","type":"1"}`)
		enc, err := encryptPayload(plainJSON)
		if err != nil {
			s.skeyErr = err
			return
		}
		raw, err := s.postRawInternal(ctx, "https://yun.139.com/orchestration/auth-rebuild/key/v1.0/getRsaPublicKey", enc, calSign(string(plainJSON)), "")
		if err != nil {
			s.skeyErr = err
			return
		}
		respBytes := []byte(strings.TrimSpace(string(raw)))
		// 响应可能为 JSON 字符串包裹的 base64 密文
		if strings.HasPrefix(string(respBytes), `"`) {
			var s string
			if err := json.Unmarshal(respBytes, &s); err == nil {
				respBytes = []byte(s)
			}
		}
		if !strings.HasPrefix(string(respBytes), "{") {
			if pt, derr := decryptPayload(string(respBytes)); derr == nil {
				respBytes = pt
			}
		}
		var out struct {
			Success bool `json:"success"`
			Data    struct {
				PublicKey string `json:"publicKey"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respBytes, &out); err != nil || out.Data.PublicKey == "" {
			s.skeyErr = fmt.Errorf("获取 RSA 公钥失败: %s", string(respBytes))
			return
		}
		aesKey := randomString(16)
		skey, err := rsaEncryptPKCS1(aesKey, out.Data.PublicKey)
		if err != nil {
			s.skeyErr = err
			return
		}
		s.secretKey = skey
	})
}

// normalizePersonalHost 处理路由 host：去除尾部斜杠与重复的 /hcy 后缀
func normalizePersonalHost(h string) string {
	h = strings.TrimSuffix(strings.TrimSuffix(h, "/"), "/hcy")
	return h
}

// ensureHost 获取个人云动态 host：优先使用登录时缓存的路由，缺失时实时查询
func (s *Yun139Storage) ensureHost(ctx context.Context) error {
	if s.host != "" {
		return nil
	}
	if s.cfg.PersonalHost != "" {
		s.mu.Lock()
		s.host = normalizePersonalHost(s.cfg.PersonalHost)
		s.mu.Unlock()
		s.log.Info("139 use cached personal host", zap.String("host", s.host))
		return nil
	}
	s.log.Warn("139 no personal host in config",
		zap.Bool("hasUserDomainID", s.cfg.UserDomainID != ""),
		zap.Bool("hasToken", s.cfg.Token != ""))
	if s.cfg.UserDomainID == "" {
		return fmt.Errorf("路由错误：缺少个人云路由信息（personal_host/user_domain_id 未保存），请重新登录")
	}
	body := map[string]interface{}{
		"userInfo":    map[string]interface{}{"userDomainId": s.cfg.UserDomainID},
		"modAddrType": 1,
	}
	plainJSON, _ := json.Marshal(body)
	s.ensureSecretKey(ctx)
	if s.skeyErr != nil {
		return s.skeyErr
	}
	// qryRoutePolicy 与登录接口一致：请求体需 AES 加密封装（服务端按密文解密），签名基于明文计算
	enc, err := encryptPayload(plainJSON)
	if err != nil {
		return err
	}
	raw, err := s.postRawInternal(ctx, routeURL, enc, calSign(string(plainJSON)), s.cfg.Token)
	if err != nil {
		return err
	}
	respBytes, derr := decryptRespPayload(raw)
	if derr != nil {
		s.log.Warn("139 qryRoutePolicy decrypt failed",
			zap.Error(derr),
			zap.ByteString("raw_prefix", firstBytes(raw, 200)))
		return derr
	}
	s.log.Debug("139 qryRoutePolicy response", zap.ByteString("body", respBytes))
	var out struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			RoutePolicyList []struct {
				ModName string `json:"modName"`
				HTTPS   string `json:"httpsUrl"`
			} `json:"routePolicyList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		s.log.Warn("139 qryRoutePolicy unmarshal failed",
			zap.Error(err),
			zap.ByteString("resp_bytes", respBytes))
		return err
	}
	if !out.Success {
		return fmt.Errorf("139 查询路由失败: %s %s", out.Code, out.Message)
	}
	for _, p := range out.Data.RoutePolicyList {
		if p.ModName == "personal" && p.HTTPS != "" {
			s.mu.Lock()
			s.host = normalizePersonalHost(p.HTTPS)
			s.mu.Unlock()
			s.log.Info("139 qryRoutePolicy got personal host", zap.String("host", s.host))
			return nil
		}
	}
	s.log.Warn("139 qryRoutePolicy response", zap.ByteString("body", respBytes))
	return fmt.Errorf("未找到个人云路由")
}

// postRawInternal 执行 POST（带完整签名头，可选 Authorization）
func (s *Yun139Storage) postRawInternal(ctx context.Context, url, body, sign, authToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("caller", "web")
	req.Header.Set("hcy-cool-flag", "1")
	req.Header.Set("CMS-DEVICE", "default")
	req.Header.Set("x-yun-api-version", "v1")
	req.Header.Set("x-yun-svc-type", "1")
	req.Header.Set("x-SvcType", "1")
	req.Header.Set("x-yun-module-type", "100")
	req.Header.Set("x-yun-app-channel", "10000034")
	req.Header.Set("x-yun-channel-source", "10000034")
	req.Header.Set("x-m4c-caller", "PC")
	req.Header.Set("x-m4c-src", "10002")
	req.Header.Set("x-inner-ntwk", "2")
	req.Header.Set("mcloud-route", "001")
	req.Header.Set("mcloud-channel", "1000101")
	req.Header.Set("mcloud-client", "10701")
	req.Header.Set("mcloud-version", "7.17.9")
	req.Header.Set("x-huawei-channelSrc", "10000034")
	if authToken != "" {
		req.Header.Set("Authorization", "Basic "+authToken)
	}
	// 设备指纹头（签名校验/会话关联需要）
	if s.visitorID != "" {
		req.Header.Set("X-Deviceinfo", "||9|7.17.9|chrome|142.0.7444.235|"+s.visitorID+"||windows 10||zh-CN|||")
		req.Header.Set("x-yun-client-info", "||9|7.17.9|chrome|142.0.7444.235|"+s.visitorID+"||windows 10||zh-CN|||undefined||")
	}
	if sign == "" {
		sign = calSign(body)
	}
	req.Header.Set("mcloud-sign", sign)
	if s.secretKey != "" {
		req.Header.Set("mcloud-skey", s.secretKey)
	}
	req.Header.Set("INNER-HCY-ROUTER-HTTPS", "1")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/w/")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ---------- 签名 ----------

// jsEncodeURIComponent 实现 JS encodeURIComponent 语义
func jsEncodeURIComponent(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '!' || c == '~' || c == '*' || c == '\'' || c == '(' || c == ')' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// calSign 生成 mcloud-sign 头
func calSign(body string) string {
	ts := time.Now().Format("2006-01-02 15:04:05")
	randStr := randomString(16)

	encoded := jsEncodeURIComponent(body)
	chars := strings.Split(encoded, "")
	sort.Strings(chars)
	sorted := strings.Join(chars, "")
	base64Str := base64.StdEncoding.EncodeToString([]byte(sorted))

	part1 := md5Hex(base64Str)
	part2 := md5Hex(ts + ":" + randStr)
	sign := md5Hex(part1 + part2)

	return fmt.Sprintf("%s,%s,%s", ts, randStr, strings.ToUpper(sign))
}

func randomString(n int) string {
	const letters = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// ---------- HTTP ----------

func (s *Yun139Storage) post(ctx context.Context, apiPath string, body interface{}, out interface{}) error {
	if err := s.ensureHost(ctx); err != nil {
		return err
	}
	s.ensureSecretKey(ctx)
	if s.skeyErr != nil {
		return s.skeyErr
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}
	// hcy 接口请求体同样需 AES 加密封装（服务端按密文解密），签名基于明文计算
	enc, err := encryptPayload(bodyBytes)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.host+apiPath, strings.NewReader(enc))
	if err != nil {
		return err
	}
	s.setHeaders(req, string(bodyBytes))

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return err
	}

	// hcy 接口响应同样为 AES 封装（JSON 字符串包裹的 base64 密文），统一解密
	if plain, derr := decryptRespPayload(data); derr == nil {
		data = plain
	} else {
		s.log.Warn("139 hcy response decrypt failed",
			zap.String("path", apiPath),
			zap.Error(derr),
			zap.ByteString("raw_prefix", firstBytes(data, 200)))
	}

	var result struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &result)
	if !result.Success {
		s.log.Warn("139 hcy api error",
			zap.String("path", apiPath),
			zap.String("code", result.Code),
			zap.String("message", result.Message),
			zap.ByteString("raw_prefix", firstBytes(data, 300)))
		return fmt.Errorf("139 api error: %s %s", result.Code, result.Message)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (s *Yun139Storage) setHeaders(req *http.Request, body string) {
	sign := calSign(body)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("Authorization", "Basic "+s.cfg.Token)
	req.Header.Set("caller", "web")
	req.Header.Set("hcy-cool-flag", "1")
	req.Header.Set("CMS-DEVICE", "default")
	req.Header.Set("x-yun-api-version", "v1")
	req.Header.Set("x-yun-svc-type", "1")
	req.Header.Set("x-SvcType", "1")
	req.Header.Set("x-yun-module-type", "100")
	req.Header.Set("x-yun-app-channel", "10000034")
	req.Header.Set("x-yun-channel-source", "10000034")
	req.Header.Set("x-m4c-caller", "PC")
	req.Header.Set("x-m4c-src", "10002")
	req.Header.Set("x-inner-ntwk", "2")
	req.Header.Set("mcloud-route", "001")
	req.Header.Set("mcloud-channel", "1000101")
	req.Header.Set("mcloud-client", "10701")
	req.Header.Set("mcloud-version", "7.17.9")
	req.Header.Set("x-huawei-channelSrc", "10000034")
	if s.visitorID != "" {
		req.Header.Set("X-Deviceinfo", "||9|7.17.9|chrome|142.0.7444.235|"+s.visitorID+"||windows 10||zh-CN|||")
		req.Header.Set("x-yun-client-info", "||9|7.17.9|chrome|142.0.7444.235|"+s.visitorID+"||windows 10||zh-CN|||undefined||")
	}
	req.Header.Set("mcloud-sign", sign)
	if s.secretKey != "" {
		req.Header.Set("mcloud-skey", s.secretKey)
	}
	req.Header.Set("INNER-HCY-ROUTER-HTTPS", "1")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/w/")
}

// ---------- 文件操作 ----------

type yun139Item struct {
	FileID   string `json:"fileId"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
	ParentID string `json:"parentFileId"`
}

// listPage 获取一层目录
func (s *Yun139Storage) listDir(ctx context.Context, parentFileID string) ([]yun139Item, error) {
	var items []yun139Item
	cursor := ""
	for {
		pageInfo := map[string]interface{}{"pageSize": 100}
		if cursor != "" {
			pageInfo["pageCursor"] = cursor
		}
		body := map[string]interface{}{
			"pageInfo":              pageInfo,
			"orderBy":               "name",
			"orderDirection":        "ASC",
			"parentFileId":          parentFileID,
			"imageThumbnailStyleList": []string{"Small", "Large"},
		}
		var out struct {
			Data struct {
				Items      []yun139Item `json:"items"`
				NextCursor string       `json:"nextPageCursor"`
			} `json:"data"`
		}
		if err := s.post(ctx, "/hcy/file/list", body, &out); err != nil {
			return nil, err
		}
		items = append(items, out.Data.Items...)
		if out.Data.NextCursor == "" {
			break
		}
		cursor = out.Data.NextCursor
	}
	return items, nil
}

func splitPath(p string) []string {
	var segs []string
	for _, s := range strings.Split(strings.Trim(p, "/"), "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return segs
}

// resolvePath 逐级解析路径，返回 fileId
func (s *Yun139Storage) resolvePath(ctx context.Context, remotePath string) (string, error) {
	cur := "/"
	for _, seg := range splitPath(remotePath) {
		items, err := s.listDir(ctx, cur)
		if err != nil {
			return "", err
		}
		found := false
		for _, it := range items {
			if it.Name == seg {
				cur = it.FileID
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("path not found: %s", remotePath)
		}
	}
	return cur, nil
}

// resolveEntry 解析路径并返回条目
func (s *Yun139Storage) resolveEntry(ctx context.Context, remotePath string) (*yun139Item, error) {
	parent, name := path.Split(strings.TrimSuffix(remotePath, "/"))
	if name == "" {
		parent, name = path.Split(strings.TrimSuffix(parent, "/"))
	}
	parentID, err := s.resolvePath(ctx, parent)
	if err != nil {
		return nil, err
	}
	items, err := s.listDir(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Name == name {
			it.ParentID = parentID
			return &it, nil
		}
	}
	return nil, fmt.Errorf("path not found: %s", remotePath)
}

func (s *Yun139Storage) ListDir(ctx context.Context, p string) ([]storage.FileInfo, error) {
	folderID, err := s.resolvePath(ctx, p)
	if err != nil {
		return nil, err
	}
	items, err := s.listDir(ctx, folderID)
	if err != nil {
		return nil, err
	}
	var files []storage.FileInfo
	for _, it := range items {
		files = append(files, storage.FileInfo{
			Name:  it.Name,
			Path:  path.Join(p, it.Name),
			Size:  it.Size,
			IsDir: it.Type == "folder",
		})
	}
	return files, nil
}

func (s *Yun139Storage) MkdirAll(ctx context.Context, p string) error {
	cur := "/"
	for _, seg := range splitPath(p) {
		items, err := s.listDir(ctx, cur)
		if err != nil {
			return err
		}
		found := false
		for _, it := range items {
			if it.Name == seg && it.Type == "folder" {
				cur = it.FileID
				found = true
				break
			}
		}
		if found {
			continue
		}
		var out struct {
			Data struct {
				FileID string `json:"fileId"`
			} `json:"data"`
		}
		if err := s.post(ctx, "/hcy/file/create", map[string]interface{}{
			"parentFileId":  cur,
			"name":          seg,
			"description":   "",
			"type":          "folder",
			"fileRenameMode": "force_rename",
		}, &out); err != nil {
			return err
		}
		cur = out.Data.FileID
	}
	return nil
}

func (s *Yun139Storage) Exists(ctx context.Context, p string) (bool, error) {
	_, err := s.resolveEntry(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "path not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Yun139Storage) Delete(ctx context.Context, p string) error {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return err
	}
	return s.post(ctx, "/hcy/recyclebin/batchTrash", map[string]interface{}{
		"fileIds": []string{entry.FileID},
	}, nil)
}

func (s *Yun139Storage) Rename(ctx context.Context, oldPath, newPath string) error {
	entry, err := s.resolveEntry(ctx, oldPath)
	if err != nil {
		return err
	}
	_, newName := path.Split(strings.TrimSuffix(newPath, "/"))
	if newName == "" {
		return fmt.Errorf("invalid new path")
	}
	return s.post(ctx, "/hcy/file/update", map[string]interface{}{
		"fileId":      entry.FileID,
		"name":        newName,
		"description": "",
	}, nil)
}

func (s *Yun139Storage) Size(ctx context.Context, p string) (int64, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return 0, err
	}
	return entry.Size, nil
}

func (s *Yun139Storage) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return nil, err
	}
	if entry.Type == "folder" {
		return nil, fmt.Errorf("cannot open folder")
	}
	var out struct {
		Data struct {
			CdnURL string `json:"cdnUrl"`
		} `json:"data"`
	}
	if err := s.post(ctx, "/hcy/file/getDownloadUrl", map[string]interface{}{
		"fileId": entry.FileID,
	}, &out); err != nil {
		return nil, err
	}
	if out.Data.CdnURL == "" {
		return nil, fmt.Errorf("no download url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.Data.CdnURL, nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 || length > 0 {
		if length > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		} else {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("download http %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// Upload 单分片上传（≤5GB）
func (s *Yun139Storage) Upload(ctx context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()

	parent, name := path.Split(strings.TrimSuffix(remotePath, "/"))
	if name == "" {
		return fmt.Errorf("invalid remote path")
	}
	parentID, err := s.resolvePath(ctx, parent)
	if err != nil {
		return err
	}

	contentHash, err := sha256Hex(f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	partInfo := map[string]interface{}{
		"parallelHashCtx": map[string]interface{}{"partOffset": 0},
		"partNumber":      1,
		"partSize":        size,
	}

	var created struct {
		Data struct {
			FileID       string `json:"fileId"`
			UploadID     string `json:"uploadId"`
			Exist        bool   `json:"exist"`
			RapidUpload  bool   `json:"rapidUpload"`
			PartInfos    []struct {
				PartNumber int    `json:"partNumber"`
				UploadURL  string `json:"uploadUrl"`
			} `json:"partInfos"`
		} `json:"data"`
	}
	if err := s.post(ctx, "/hcy/file/create", map[string]interface{}{
		"parentFileId":       parentID,
		"name":               name,
		"type":               "file",
		"size":               size,
		"fileRenameMode":     "auto_rename",
		"contentHash":        contentHash,
		"contentHashAlgorithm": "SHA256",
		"contentType":        "application/oct-stream",
		"parallelUpload":     false,
		"partInfos":          []interface{}{partInfo},
	}, &created); err != nil {
		return err
	}
	if created.Data.Exist || created.Data.RapidUpload {
		if progress != nil {
			progress(size, size)
		}
		return nil
	}
	if len(created.Data.PartInfos) == 0 || created.Data.PartInfos[0].UploadURL == "" {
		return fmt.Errorf("上传分片地址为空")
	}

	// PUT 上传
	uploadURL := created.Data.PartInfos[0].UploadURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, f)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/oct-stream")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/")
	req.ContentLength = size

	pr := io.Reader(f)
	if progress != nil {
		pr = &progressReader{reader: f, done: 0, total: size, callback: progress}
	}
	req.Body = io.NopCloser(pr)
	req.ContentLength = size

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	// complete
	return s.post(ctx, "/hcy/file/complete", map[string]interface{}{
		"fileId":              created.Data.FileID,
		"uploadId":            created.Data.UploadID,
		"contentHash":         contentHash,
		"contentHashAlgorithm": "SHA256",
	}, nil)
}

func (s *Yun139Storage) Test(ctx context.Context) error {
	_, err := s.listDir(ctx, "/")
	return err
}

func sha256Hex(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressReader struct {
	reader   io.Reader
	done     int64
	total    int64
	callback storage.ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.done += int64(n)
		pr.callback(pr.done, pr.total)
	}
	return n, err
}
