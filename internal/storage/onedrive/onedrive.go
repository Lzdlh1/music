// Package onedrive OneDrive Microsoft Graph API 适配器
package onedrive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
)

const (
	graphBase = "https://graph.microsoft.com/v1.0"
	tokenURL  = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
)

// Config OneDrive 配置
type Config struct {
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RootPath     string `json:"root_path"` // 可选，挂载子目录
}

// OneDriveStorage OneDrive 存储后端
type OneDriveStorage struct {
	id      string
	name    string
	cfg     Config
	client  *http.Client
	log     *zap.Logger
	mu      sync.Mutex
	token   string
	expires time.Time
}

// New 创建 OneDrive 存储
func New(id, name string, cfg Config, log *zap.Logger) *OneDriveStorage {
	if log == nil {
		log = zap.NewNop()
	}
	return &OneDriveStorage{
		id:     id,
		name:   name,
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
		log:    log,
	}
}

func (s *OneDriveStorage) ID() string               { return s.id }
func (s *OneDriveStorage) Name() string              { return s.name }
func (s *OneDriveStorage) Type() storage.StorageType { return storage.StorageOneDrive }

// refPath 将远程路径拼到配置的根目录下
func (s *OneDriveStorage) refPath(remotePath string) string {
	p := strings.Trim(remotePath, "/")
	if s.cfg.RootPath != "" {
		p = strings.Trim(s.cfg.RootPath, "/") + "/" + p
	}
	return p
}

// itemPath 构造 Graph 路径寻址 URL 片段
func (s *OneDriveStorage) itemPath(remotePath string) string {
	p := s.refPath(remotePath)
	if p == "" {
		return "/me/drive/root"
	}
	return "/me/drive/root:/" + url.PathEscape(p)
}

// ---------- 认证 ----------

func (s *OneDriveStorage) refreshToken(ctx context.Context) error {
	if s.cfg.RefreshToken == "" || s.cfg.ClientID == "" {
		return fmt.Errorf("refresh_token 与 client_id 均为必填")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	form.Set("refresh_token", s.cfg.RefreshToken)
	form.Set("scope", "files.readwrite offline_access")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var r struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return err
	}
	if r.AccessToken == "" {
		return fmt.Errorf("token 刷新失败: %s %s", r.Error, r.Description)
	}

	s.mu.Lock()
	s.token = r.AccessToken
	s.expires = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	s.mu.Unlock()
	return nil
}

func (s *OneDriveStorage) ensureToken(ctx context.Context) error {
	s.mu.Lock()
	valid := s.token != "" && time.Now().Before(s.expires)
	s.mu.Unlock()
	if valid {
		return nil
	}
	return s.refreshToken(ctx)
}

// ---------- HTTP ----------

// call 发送 Graph 请求；401 时刷新 token 重试一次
func (s *OneDriveStorage) call(ctx context.Context, method, apiURL string, body interface{}, out interface{}) error {
	if err := s.ensureToken(ctx); err != nil {
		return err
	}
	status, data, err := s.do(ctx, method, apiURL, body)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		s.mu.Lock()
		s.token = ""
		s.mu.Unlock()
		if rerr := s.refreshToken(ctx); rerr == nil {
			status, data, err = s.do(ctx, method, apiURL, body)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("token refresh failed: %w", rerr)
		}
	}
	if status >= 400 {
		return fmt.Errorf("graph http %d: %s", status, truncate(string(data), 300))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (s *OneDriveStorage) do(ctx context.Context, method, apiURL string, body interface{}) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, apiURL, reader)
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	s.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+s.token)
	s.mu.Unlock()

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, data, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ---------- 元数据 ----------

type driveItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Folder *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder,omitempty"`
	File *struct{} `json:"file,omitempty"`
}

// getItem 通过路径获取条目
func (s *OneDriveStorage) getItem(ctx context.Context, remotePath string) (*driveItem, error) {
	var item driveItem
	if err := s.call(ctx, http.MethodGet, graphBase+s.itemPath(remotePath), nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// ---------- 目录操作 ----------

func (s *OneDriveStorage) ListDir(ctx context.Context, p string) ([]storage.FileInfo, error) {
	listURL := graphBase + s.itemPath(p) + ":/children?$top=999"
	var files []storage.FileInfo
	for listURL != "" {
		var out struct {
			Value      []driveItem `json:"value"`
			NextLink   string      `json:"@odata.nextLink"`
			Error      *struct {
				Code string `json:"code"`
			} `json:"error,omitempty"`
		}
		if err := s.call(ctx, http.MethodGet, listURL, nil, &out); err != nil {
			return nil, err
		}
		if out.Error != nil {
			if out.Error.Code == "itemNotFound" {
				return nil, fmt.Errorf("path not found: %s", p)
			}
			return nil, fmt.Errorf("graph error: %s", out.Error.Code)
		}
		for _, it := range out.Value {
			files = append(files, storage.FileInfo{
				Name:  it.Name,
				Path:  path.Join(p, it.Name),
				Size:  it.Size,
				IsDir: it.Folder != nil,
			})
		}
		listURL = out.NextLink
	}
	return files, nil
}

func (s *OneDriveStorage) MkdirAll(ctx context.Context, p string) error {
	segs := strings.Split(strings.Trim(p, "/"), "/")
	cur := ""
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		child := path.Join(cur, seg)
		if _, err := s.getItem(ctx, child); err == nil {
			cur = child
			continue
		}
		parentRef := s.itemPath(cur)
		if cur == "" {
			parentRef = "/me/drive/root/children"
		} else {
			parentRef = s.itemPath(cur) + ":/children"
		}
		var created driveItem
		if err := s.call(ctx, http.MethodPost, graphBase+parentRef, map[string]interface{}{
			"name":                              seg,
			"folder":                            map[string]interface{}{},
			"@microsoft.graph.conflictBehavior": "rename",
		}, &created); err != nil {
			return err
		}
		cur = child
	}
	return nil
}

func (s *OneDriveStorage) Exists(ctx context.Context, p string) (bool, error) {
	_, err := s.getItem(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "http 404") || strings.Contains(err.Error(), "itemNotFound") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *OneDriveStorage) Delete(ctx context.Context, p string) error {
	item, err := s.getItem(ctx, p)
	if err != nil {
		return err
	}
	return s.call(ctx, http.MethodDelete, graphBase+"/me/drive/items/"+item.ID, nil, nil)
}

func (s *OneDriveStorage) Rename(ctx context.Context, oldPath, newPath string) error {
	item, err := s.getItem(ctx, oldPath)
	if err != nil {
		return err
	}
	_, newName := path.Split(strings.TrimSuffix(newPath, "/"))
	if newName == "" {
		return fmt.Errorf("invalid new path")
	}
	return s.call(ctx, http.MethodPatch, graphBase+"/me/drive/items/"+item.ID, map[string]interface{}{
		"name": newName,
	}, nil)
}

// ---------- 文件流 ----------

func (s *OneDriveStorage) Size(ctx context.Context, p string) (int64, error) {
	item, err := s.getItem(ctx, p)
	if err != nil {
		return 0, err
	}
	return item.Size, nil
}

// Open 跟随 /content 的 302 重定向获取实际下载 URL，再请求 Range
func (s *OneDriveStorage) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	if err := s.ensureToken(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, graphBase+s.itemPath(p)+":/content", nil)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+s.token)
	s.mu.Unlock()

	// 不跟随重定向，拿到 Location
	redirectClient := &http.Client{
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := redirectClient.Do(req)
	if err != nil {
		return nil, err
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && loc == "" {
		return nil, fmt.Errorf("redirect without location")
	}
	if resp.StatusCode == http.StatusOK {
		// 直接返回内容（未重定向）
		req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, graphBase+s.itemPath(p)+":/content", nil)
		s.mu.Lock()
		req2.Header.Set("Authorization", "Bearer "+s.token)
		s.mu.Unlock()
		if offset > 0 || length > 0 {
			setRange(req2, offset, length)
		}
		r2, err := s.client.Do(req2)
		if err != nil {
			return nil, err
		}
		if r2.StatusCode >= 400 {
			r2.Body.Close()
			return nil, fmt.Errorf("download http %d", r2.StatusCode)
		}
		return r2.Body, nil
	}
	if loc == "" {
		return nil, fmt.Errorf("download failed: http %d", resp.StatusCode)
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 || length > 0 {
		setRange(dlReq, offset, length)
	}
	dlResp, err := s.client.Do(dlReq)
	if err != nil {
		return nil, err
	}
	if dlResp.StatusCode >= 400 {
		dlResp.Body.Close()
		return nil, fmt.Errorf("download http %d", dlResp.StatusCode)
	}
	return dlResp.Body, nil
}

func setRange(req *http.Request, offset, length int64) {
	if length > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
}

// Upload 简单上传（≤250MB）
func (s *OneDriveStorage) Upload(ctx context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if stat.Size() > 250*1024*1024 {
		return fmt.Errorf("文件超过 250MB，OneDrive 简单上传不支持大文件")
	}

	parent, name := path.Split(strings.TrimSuffix(remotePath, "/"))
	if name == "" {
		return fmt.Errorf("invalid remote path")
	}

	putURL := graphBase + s.itemPath(parent) + ":/" + url.PathEscape(name) + ":/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL, f)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.ContentLength = stat.Size()
	s.mu.Lock()
	req.Header.Set("Authorization", "Bearer "+s.token)
	s.mu.Unlock()

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload http %d: %s", resp.StatusCode, truncate(string(data), 300))
	}
	if progress != nil {
		progress(stat.Size(), stat.Size())
	}
	return nil
}

func (s *OneDriveStorage) Test(ctx context.Context) error {
	var root driveItem
	return s.call(ctx, http.MethodGet, graphBase+"/me/drive/root", nil, &root)
}
