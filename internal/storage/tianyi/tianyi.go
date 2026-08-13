// Package tianyi 天翼云盘（189 云盘）适配器
// 基于 cloud.189.cn 网页版 API，认证使用浏览器登录后的 Cookie。
package tianyi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
)

const (
	apiBase = "https://cloud.189.cn"
	rootID  = "-11"
)

// Config 天翼云盘配置
type Config struct {
	Cookie string `json:"cookie"` // 浏览器登录 cloud.189.cn 后的 Cookie
}

// TianyiStorage 天翼云盘存储后端
type TianyiStorage struct {
	id     string
	name   string
	cfg    Config
	client *http.Client
	log    *zap.Logger
	mu     sync.Mutex
}

// New 创建天翼云盘存储
func New(id, name string, cfg Config, log *zap.Logger) *TianyiStorage {
	if log == nil {
		log = zap.NewNop()
	}
	return &TianyiStorage{
		id:     id,
		name:   name,
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
		log:    log,
	}
}

func (s *TianyiStorage) ID() string               { return s.id }
func (s *TianyiStorage) Name() string              { return s.name }
func (s *TianyiStorage) Type() storage.StorageType { return storage.StorageTianyi }

// ---------- HTTP ----------

func (s *TianyiStorage) newReq(ctx context.Context, method, apiURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", s.cfg.Cookie)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	req.Header.Set("Referer", "https://cloud.189.cn/web/main/file/folder/"+rootID)
	return req, nil
}

// getJSON GET 请求并解析 JSON；未登录（res_code==-5 / -3）时报错提示
func (s *TianyiStorage) getJSON(ctx context.Context, apiURL string, out interface{}) error {
	req, err := s.newReq(ctx, http.MethodGet, apiURL)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return err
	}

	// 检测通用错误
	var errResp struct {
		ResCode   int    `json:"res_code"`
		ResMsg    string `json:"res_message"`
		ErrorCode string `json:"errorCode"`
	}
	if err := json.Unmarshal(data, &errResp); err == nil {
		if errResp.ErrorCode == "InvalidSessionKey" || errResp.ResCode == -5 || errResp.ResCode == -3 {
			return fmt.Errorf("天翼云盘 Cookie 无效或已过期，请重新登录获取")
		}
		if errResp.ResCode != 0 && errResp.ResCode != -4 && errResp.ErrorCode != "" && errResp.ErrorCode != "0" {
			if errResp.ResCode != 0 {
				return fmt.Errorf("189 api error: res_code=%d %s", errResp.ResCode, errResp.ResMsg)
			}
		}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ---------- 文件操作 ----------

type tianyiItem struct {
	ID     string
	Name   string
	Size   int64
	IsDir  bool
	Parent string
}

// listDir 获取目录下条目
func (s *TianyiStorage) listDir(ctx context.Context, folderID string) ([]tianyiItem, error) {
	u := fmt.Sprintf("%s/api/open/file/listFiles.action?pageSize=60&pageNum=1&mediaType=0&folderId=%s&iconOption=5&orderBy=lastOpTime&descending=false&noCache=%d",
		apiBase, url.QueryEscape(folderID), time.Now().UnixMilli())
	var out struct {
		ResCode int `json:"res_code"`
		Data    struct {
			FileList   []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Size int64  `json:"size"`
			} `json:"fileList"`
			FolderList []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"folderList"`
		} `json:"fileListAO"`
	}
	if err := s.getJSON(ctx, u, &out); err != nil {
		return nil, err
	}
	if out.ResCode != 0 {
		return nil, fmt.Errorf("189 list error: res_code=%d", out.ResCode)
	}
	var items []tianyiItem
	for _, f := range out.Data.FileList {
		items = append(items, tianyiItem{ID: f.ID, Name: f.Name, Size: f.Size, Parent: folderID})
	}
	for _, d := range out.Data.FolderList {
		items = append(items, tianyiItem{ID: d.ID, Name: d.Name, IsDir: true, Parent: folderID})
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

// resolvePath 逐级解析路径，返回条目
func (s *TianyiStorage) resolvePath(ctx context.Context, remotePath string) ([]tianyiItem, error) {
	cur := rootID
	for _, seg := range splitPath(remotePath) {
		items, err := s.listDir(ctx, cur)
		if err != nil {
			return nil, err
		}
		found := false
		for _, it := range items {
			if it.Name == seg {
				cur = it.ID
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("path not found: %s", remotePath)
		}
	}
	return s.listDir(ctx, cur)
}

// resolveEntry 返回远程路径对应条目（含父目录 id 用于操作）
func (s *TianyiStorage) resolveEntry(ctx context.Context, remotePath string) (*tianyiItem, error) {
	if remotePath == "/" || remotePath == "" {
		return &tianyiItem{ID: rootID, Name: "", IsDir: true, Parent: ""}, nil
	}
	parent, name := path.Split(strings.TrimSuffix(remotePath, "/"))
	if name == "" {
		parent, name = path.Split(strings.TrimSuffix(parent, "/"))
	}
	parentID, err := s.resolveDirID(ctx, parent)
	if err != nil {
		return nil, err
	}
	items, err := s.listDir(ctx, parentID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.Name == name {
			it.Parent = parentID
			return &it, nil
		}
	}
	return nil, fmt.Errorf("path not found: %s", remotePath)
}

// resolveDirID 解析目录路径的 id
func (s *TianyiStorage) resolveDirID(ctx context.Context, p string) (string, error) {
	if p == "" || p == "/" {
		return rootID, nil
	}
	cur := rootID
	for _, seg := range splitPath(p) {
		items, err := s.listDir(ctx, cur)
		if err != nil {
			return "", err
		}
		found := false
		for _, it := range items {
			if it.Name == seg {
				cur = it.ID
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("path not found: %s", p)
		}
	}
	return cur, nil
}

func (s *TianyiStorage) ListDir(ctx context.Context, p string) ([]storage.FileInfo, error) {
	items, err := s.resolvePath(ctx, p)
	if err != nil {
		return nil, err
	}
	var files []storage.FileInfo
	for _, it := range items {
		files = append(files, storage.FileInfo{
			Name:  it.Name,
			Path:  path.Join(p, it.Name),
			Size:  it.Size,
			IsDir: it.IsDir,
		})
	}
	return files, nil
}

func (s *TianyiStorage) MkdirAll(ctx context.Context, p string) error {
	cur := rootID
	for _, seg := range splitPath(p) {
		items, err := s.listDir(ctx, cur)
		if err != nil {
			return err
		}
		found := false
		for _, it := range items {
			if it.Name == seg && it.IsDir {
				cur = it.ID
				found = true
				break
			}
		}
		if found {
			continue
		}
		// 创建文件夹
		createURL := fmt.Sprintf("%s/api/open/file/createFolder.action?parentFolderId=%s&folderName=%s",
			apiBase, url.QueryEscape(cur), url.QueryEscape(seg))
		req, err := s.newReq(ctx, http.MethodGet, createURL)
		if err != nil {
			return err
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		var out struct {
			ID     string `json:"id"`
			ResCode int   `json:"res_code"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if out.ID == "" {
			return fmt.Errorf("创建文件夹失败: %s", seg)
		}
		cur = out.ID
	}
	return nil
}

func (s *TianyiStorage) Exists(ctx context.Context, p string) (bool, error) {
	_, err := s.resolveEntry(ctx, p)
	if err != nil {
		if strings.Contains(err.Error(), "path not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *TianyiStorage) Delete(ctx context.Context, p string) error {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return err
	}
	isFolder := 0
	if entry.IsDir {
		isFolder = 1
	}
	taskInfo, _ := json.Marshal([]map[string]interface{}{
		{"fileId": entry.ID, "fileName": entry.Name, "isFolder": isFolder},
	})
	u := fmt.Sprintf("%s/api/open/batch/createBatchTask.action?type=DELETE&targetFolderId=&taskInfos=%s",
		apiBase, url.QueryEscape(string(taskInfo)))
	req, err := s.newReq(ctx, http.MethodGet, u)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var out struct {
		ResCode int `json:"res_code"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if out.ResCode != 0 {
		return fmt.Errorf("删除失败: res_code=%d", out.ResCode)
	}
	return nil
}

func (s *TianyiStorage) Rename(ctx context.Context, oldPath, newPath string) error {
	entry, err := s.resolveEntry(ctx, oldPath)
	if err != nil {
		return err
	}
	_, newName := path.Split(strings.TrimSuffix(newPath, "/"))
	if newName == "" {
		return fmt.Errorf("invalid new path")
	}
	var u string
	if entry.IsDir {
		u = fmt.Sprintf("%s/api/open/file/renameFolder.action?folderId=%s&destFolderName=%s",
			apiBase, url.QueryEscape(entry.ID), url.QueryEscape(newName))
	} else {
		u = fmt.Sprintf("%s/api/open/file/renameFile.action?fileId=%s&destFileName=%s",
			apiBase, url.QueryEscape(entry.ID), url.QueryEscape(newName))
	}
	req, err := s.newReq(ctx, http.MethodGet, u)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		ResCode int `json:"res_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.ResCode != 0 {
		return fmt.Errorf("重命名失败: res_code=%d", out.ResCode)
	}
	return nil
}

func (s *TianyiStorage) Size(ctx context.Context, p string) (int64, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return 0, err
	}
	return entry.Size, nil
}

func (s *TianyiStorage) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	entry, err := s.resolveEntry(ctx, p)
	if err != nil {
		return nil, err
	}
	if entry.IsDir {
		return nil, fmt.Errorf("cannot open folder")
	}
	u := fmt.Sprintf("%s/api/portal/getFileInfo.action?fileId=%s", apiBase, url.QueryEscape(entry.ID))
	req, err := s.newReq(ctx, http.MethodGet, u)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	var out struct {
		DownloadURL string `json:"downloadUrl"`
		ResCode     int    `json:"res_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()
	if out.DownloadURL == "" {
		return nil, fmt.Errorf("获取下载链接失败")
	}

	dlReq, err := http.NewRequestWithContext(ctx, http.MethodGet, out.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 || length > 0 {
		if length > 0 {
			dlReq.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		} else {
			dlReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
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

// Upload 天翼云盘上传需要复杂的 RSA/AES 签名，暂不支持
func (s *TianyiStorage) Upload(_ context.Context, _ string, _ string, _ storage.ProgressCallback) error {
	return fmt.Errorf("天翼云盘暂不支持上传，仅支持浏览/播放/管理")
}

func (s *TianyiStorage) Test(ctx context.Context) error {
	_, err := s.listDir(ctx, rootID)
	return err
}
