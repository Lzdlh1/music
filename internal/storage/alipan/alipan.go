// Package alipan 阿里云盘开放平台 API 适配器
package alipan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
)

const (
	baseURL      = "https://openapi.alipan.com"
	oauthURL     = "https://openapi.alipan.com/oauth/access_token"
	uploadPartSz = 20 * 1024 * 1024 // 单分片 20MB
)

// Config 阿里云盘配置
type Config struct {
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RootFolderID string `json:"root_folder_id"` // 可选，默认 root
	DriveID      string `json:"drive_id"`       // 可选，留空自动获取
}

// AlipanStorage 阿里云盘存储后端
type AlipanStorage struct {
	id      string
	name    string
	cfg     Config
	client  *http.Client
	log     *zap.Logger
	mu      sync.Mutex
	token   string
	driveID string
	expires time.Time
}

// New 创建阿里云盘存储
func New(id, name string, cfg Config, log *zap.Logger) *AlipanStorage {
	if log == nil {
		log = zap.NewNop()
	}
	return &AlipanStorage{
		id:     id,
		name:   name,
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
		log:    log,
	}
}

func (s *AlipanStorage) ID() string               { return s.id }
func (s *AlipanStorage) Name() string              { return s.name }
func (s *AlipanStorage) Type() storage.StorageType { return storage.StorageAlipan }

// root 根目录 file_id
func (s *AlipanStorage) root() string {
	if s.cfg.RootFolderID != "" {
		return s.cfg.RootFolderID
	}
	return "root"
}

// ---------- 认证 ----------

// refreshToken 用 refresh_token 换取 access_token
func (s *AlipanStorage) refreshToken(ctx context.Context) error {
	if s.cfg.RefreshToken == "" || s.cfg.ClientID == "" {
		return fmt.Errorf("refresh_token 与 client_id 均为必填")
	}
	body, _ := json.Marshal(map[string]interface{}{
		"client_id":     s.cfg.ClientID,
		"client_secret": s.cfg.ClientSecret,
		"grant_type":    "refresh_token",
		"refresh_token": s.cfg.RefreshToken,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpireTime   string `json:"expire_time"`
		ExpiresIn    int64  `json:"expires_in"`
		Error        string `json:"error"`
		ErrorMsg     string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.AccessToken == "" {
		return fmt.Errorf("token 刷新失败: %s %s", r.Error, r.ErrorMsg)
	}

	s.mu.Lock()
	s.token = r.AccessToken
	if r.ExpiresIn > 0 {
		s.expires = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	} else {
		s.expires = time.Now().Add(2 * time.Hour)
	}
	s.mu.Unlock()
	return nil
}

// ensureToken 确保 access_token 有效
func (s *AlipanStorage) ensureToken(ctx context.Context) error {
	s.mu.Lock()
	valid := s.token != "" && time.Now().Before(s.expires)
	s.mu.Unlock()
	if valid {
		return nil
	}
	return s.refreshToken(ctx)
}

// ensureDriveID 获取默认网盘 drive_id
func (s *AlipanStorage) ensureDriveID(ctx context.Context) error {
	if s.driveID != "" {
		return nil
	}
	var out struct {
		DefaultDriveID string `json:"default_drive_id"`
		BackupDriveID  string `json:"backup_drive_id"`
		ResourceDrive  string `json:"resource_drive_id"`
	}
	if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/user/getDriveInfo", map[string]interface{}{}, &out); err != nil {
		return err
	}
	id := out.DefaultDriveID
	if id == "" {
		id = out.BackupDriveID
	}
	if id == "" {
		id = out.ResourceDrive
	}
	if id == "" {
		return fmt.Errorf("无法获取 drive_id")
	}
	s.mu.Lock()
	s.driveID = id
	s.mu.Unlock()
	return nil
}

// ---------- HTTP ----------

// call 发送 API 请求并解析 JSON；令牌失效时自动刷新重试一次
func (s *AlipanStorage) call(ctx context.Context, method, apiPath string, body interface{}, out interface{}) error {
	if err := s.ensureToken(ctx); err != nil {
		return err
	}
	err := s.do(ctx, method, apiPath, body, out)
	if err != nil && isTokenError(err) {
		s.mu.Lock()
		s.token = ""
		s.mu.Unlock()
		if rerr := s.refreshToken(ctx); rerr == nil {
			return s.do(ctx, method, apiPath, body, out)
		}
	}
	return err
}

func (s *AlipanStorage) do(ctx context.Context, method, apiPath string, body interface{}, out interface{}) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+apiPath, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+s.token)
	s.mu.Unlock()

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return err
	}

	// 检测 API 错误
	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Name    string `json:"name"`
	}
	if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Code != "" && resp.StatusCode >= 400 {
		if isTokenError(fmt.Errorf("%s", apiErr.Code)) {
			return fmt.Errorf("token_error: %s %s", apiErr.Code, apiErr.Message)
		}
		return fmt.Errorf("alipan api error: %s %s", apiErr.Code, apiErr.Message)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("alipan http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func isTokenError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "AccessTokenInvalid") ||
		strings.Contains(msg, "AccessTokenExpired") ||
		strings.Contains(msg, "token_error")
}

// ---------- 路径解析 ----------

type alipanFile struct {
	FileID   string `json:"file_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	ParentID string `json:"parent_file_id"`
}

// resolvePath 将远程路径解析为 file_id（逐级查找），返回目录的 file_id
func (s *AlipanStorage) resolvePath(ctx context.Context, remotePath string) (string, error) {
	cur := s.root()
	for _, seg := range splitPath(remotePath) {
		found := false
		marker := ""
		for {
			var out struct {
				Items      []alipanFile `json:"items"`
				NextMarker string       `json:"next_marker"`
			}
			if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/list", map[string]interface{}{
				"drive_id":        s.driveID,
				"parent_file_id":  cur,
				"limit":           200,
				"marker":          marker,
				"order_by":        "name",
				"order_direction": "ASC",
			}, &out); err != nil {
				return "", err
			}
			for _, f := range out.Items {
				if f.Name == seg {
					cur = f.FileID
					found = true
					break
				}
			}
			if found || out.NextMarker == "" {
				break
			}
			marker = out.NextMarker
		}
		if !found {
			return "", fmt.Errorf("path not found: %s", remotePath)
		}
	}
	return cur, nil
}

// resolveEntry 解析路径，返回文件条目信息
func (s *AlipanStorage) resolveEntry(ctx context.Context, remotePath string) (*alipanFile, error) {
	if remotePath == "/" || remotePath == "" {
		return &alipanFile{FileID: s.root(), Name: "", Type: "folder"}, nil
	}
	parent, name := path.Split(strings.TrimSuffix(remotePath, "/"))
	if name == "" {
		parent, name = path.Split(strings.TrimSuffix(parent, "/"))
	}
	parentID, err := s.resolvePath(ctx, parent)
	if err != nil {
		return nil, err
	}
	marker := ""
	for {
		var out struct {
			Items      []alipanFile `json:"items"`
			NextMarker string       `json:"next_marker"`
		}
		if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/list", map[string]interface{}{
			"drive_id":        s.driveID,
			"parent_file_id":  parentID,
			"limit":           200,
			"marker":          marker,
			"order_by":        "name",
			"order_direction": "ASC",
		}, &out); err != nil {
			return nil, err
		}
		for _, f := range out.Items {
			if f.Name == name {
				f.ParentID = parentID
				return &f, nil
			}
		}
		if out.NextMarker == "" {
			break
		}
		marker = out.NextMarker
	}
	return nil, fmt.Errorf("path not found: %s", remotePath)
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

// ---------- 目录操作 ----------

func (s *AlipanStorage) ListDir(ctx context.Context, p string) ([]storage.FileInfo, error) {
	if err := s.ensureDriveID(ctx); err != nil {
		return nil, err
	}
	folderID, err := s.resolvePath(ctx, p)
	if err != nil {
		return nil, err
	}
	marker := ""
	var files []storage.FileInfo
	for {
		var out struct {
			Items      []alipanFile `json:"items"`
			NextMarker string       `json:"next_marker"`
		}
		if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/list", map[string]interface{}{
			"drive_id":        s.driveID,
			"parent_file_id":  folderID,
			"limit":           200,
			"marker":          marker,
			"order_by":        "name",
			"order_direction": "ASC",
		}, &out); err != nil {
			return nil, err
		}
		for _, f := range out.Items {
			files = append(files, storage.FileInfo{
				Name:  f.Name,
				Path:  path.Join(p, f.Name),
				Size:  f.Size,
				IsDir: f.Type == "folder",
			})
		}
		if out.NextMarker == "" {
			break
		}
		marker = out.NextMarker
	}
	return files, nil
}

func (s *AlipanStorage) MkdirAll(ctx context.Context, p string) error {
	if err := s.ensureDriveID(ctx); err != nil {
		return err
	}
	cur := s.root()
	for _, seg := range splitPath(p) {
		// 在当前层查找是否已有同名文件夹
		found := false
		var out struct {
			Items []alipanFile `json:"items"`
		}
		if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/list", map[string]interface{}{
			"drive_id":       s.driveID,
			"parent_file_id": cur,
			"limit":          200,
			"order_by":       "name",
		}, &out); err != nil {
			return err
		}
		for _, f := range out.Items {
			if f.Name == seg && f.Type == "folder" {
				cur = f.FileID
				found = true
				break
			}
		}
		if found {
			continue
		}
		var created alipanFile
		if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/create", map[string]interface{}{
			"drive_id":        s.driveID,
			"parent_file_id":  cur,
			"name":            seg,
			"type":            "folder",
			"check_name_mode": "auto_rename",
		}, &created); err != nil {
			return err
		}
		cur = created.FileID
	}
	return nil
}

func (s *AlipanStorage) Exists(ctx context.Context, p string) (bool, error) {
	_, err := s.resolveEntry(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "path not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *AlipanStorage) Delete(ctx context.Context, p string) error {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return err
	}
	return s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/recyclebin/trash", map[string]interface{}{
		"drive_id": s.driveID,
		"file_id":  entry.FileID,
	}, nil)
}

func (s *AlipanStorage) Rename(ctx context.Context, oldPath, newPath string) error {
	entry, err := s.resolveEntry(ctx, oldPath)
	if err != nil {
		return err
	}
	_, newName := path.Split(strings.TrimSuffix(newPath, "/"))
	if newName == "" {
		return fmt.Errorf("invalid new path")
	}
	return s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/update", map[string]interface{}{
		"drive_id": s.driveID,
		"file_id":  entry.FileID,
		"name":     newName,
	}, nil)
}

// ---------- 文件流 ----------

func (s *AlipanStorage) Size(ctx context.Context, p string) (int64, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return 0, err
	}
	var out struct {
		Size int64 `json:"size"`
	}
	if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/get", map[string]interface{}{
		"drive_id": s.driveID,
		"file_id":  entry.FileID,
	}, &out); err != nil {
		return 0, err
	}
	if out.Size == 0 {
		out.Size = entry.Size
	}
	return out.Size, nil
}

// Open 获取下载直链并请求 Range 范围
func (s *AlipanStorage) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return nil, err
	}
	if entry.Type == "folder" {
		return nil, fmt.Errorf("cannot open folder")
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/getDownloadUrl", map[string]interface{}{
		"drive_id":   s.driveID,
		"file_id":    entry.FileID,
		"expire_sec": 14400,
	}, &out); err != nil {
		return nil, err
	}
	if out.URL == "" {
		return nil, fmt.Errorf("no download url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, out.URL, nil)
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

// Upload 分片上传
func (s *AlipanStorage) Upload(ctx context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	if err := s.ensureDriveID(ctx); err != nil {
		return err
	}
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

	// 计算分片数量
	partCount := int64(1)
	if size > uploadPartSz {
		partCount = (size + uploadPartSz - 1) / uploadPartSz
	}
	var parts []map[string]interface{}
	for i := int64(1); i <= partCount; i++ {
		parts = append(parts, map[string]interface{}{"part_number": i})
	}

	// 创建文件（预创建 + 分片信息）
	var created struct {
		FileID        string `json:"file_id"`
		UploadID      string `json:"upload_id"`
		RapidUpload   bool   `json:"rapid_upload"`
		Exist         bool   `json:"exist"`
		PartInfoList  []struct {
			PartNumber int    `json:"part_number"`
			UploadURL  string `json:"upload_url"`
		} `json:"part_info_list"`
	}
	if err := s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/create", map[string]interface{}{
		"drive_id":        s.driveID,
		"parent_file_id":  parentID,
		"name":            name,
		"type":            "file",
		"check_name_mode": "auto_rename",
		"size":            size,
		"part_info_list":  parts,
	}, &created); err != nil {
		return err
	}

	// 秒传成功或文件已存在
	if created.Exist || created.RapidUpload {
		if progress != nil {
			progress(size, size)
		}
		return nil
	}

	// 上传各分片
	for _, part := range created.PartInfoList {
		if part.UploadURL == "" {
			continue
		}
		offset := (int64(part.PartNumber) - 1) * uploadPartSz
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return err
		}
		n := uploadPartSz
		if offset+int64(n) > size {
			n = int(size - offset)
		}
		if n == 0 {
			continue
		}
		pr := io.LimitReader(f, int64(n))
		if progress != nil {
			pr = &progressReader{reader: pr, offset: offset, total: size, callback: progress}
		}
		if err := s.putURL(ctx, part.UploadURL, pr, int64(n)); err != nil {
			return fmt.Errorf("upload part %d: %w", part.PartNumber, err)
		}
	}

	// 完成上传
	return s.call(ctx, http.MethodPost, "/adrive/v1.0/openFile/complete", map[string]interface{}{
		"drive_id":  s.driveID,
		"file_id":   created.FileID,
		"upload_id": created.UploadID,
	}, nil)
}

func (s *AlipanStorage) putURL(ctx context.Context, url string, body io.Reader, length int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = length
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload url http %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func (s *AlipanStorage) Test(ctx context.Context) error {
	if err := s.ensureToken(ctx); err != nil {
		return err
	}
	var out map[string]interface{}
	return s.call(ctx, http.MethodPost, "/adrive/v1.0/user/getDriveInfo", map[string]interface{}{}, &out)
}

type progressReader struct {
	reader   io.Reader
	offset   int64
	total    int64
	callback storage.ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.offset += int64(n)
		pr.callback(pr.offset, pr.total)
	}
	return n, err
}
