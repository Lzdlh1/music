package webdav

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	gowebdav "github.com/studio-b12/gowebdav"

	"github.com/musicflow/musicflow/internal/storage"
)

// WebDAVStorage WebDAV 存储后端
type WebDAVStorage struct {
	id       string
	name     string
	client   *gowebdav.Client
	basePath string
}

// Config WebDAV 配置
type Config struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
	BasePath string `json:"base_path"`
}

// New 创建 WebDAV 存储
func New(id, name string, cfg Config) *WebDAVStorage {
	client := gowebdav.NewClient(cfg.Endpoint, cfg.Username, cfg.Password)
	return &WebDAVStorage{
		id:       id,
		name:     name,
		client:   client,
		basePath: strings.TrimSuffix(cfg.BasePath, "/"),
	}
}

func (w *WebDAVStorage) ID() string               { return w.id }
func (w *WebDAVStorage) Name() string              { return w.name }
func (w *WebDAVStorage) Type() storage.StorageType  { return storage.StorageWebDAV }

func (w *WebDAVStorage) Test(_ context.Context) error {
	_, err := w.client.ReadDir(w.basePath)
	if err != nil {
		return fmt.Errorf("webdav test: %w", err)
	}
	return nil
}

func (w *WebDAVStorage) Upload(_ context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	fullPath := path.Join(w.basePath, remotePath)

	// 确保远程目录存在
	dir := path.Dir(fullPath)
	if err := w.client.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("webdav mkdir: %w", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	var reader io.Reader = f
	if progress != nil {
		reader = &progressReader{reader: f, total: stat.Size(), callback: progress}
	}

	if err := w.client.WriteStream(fullPath, reader, 0644); err != nil {
		return fmt.Errorf("webdav upload: %w", err)
	}
	return nil
}

func (w *WebDAVStorage) MkdirAll(_ context.Context, p string) error {
	return w.client.MkdirAll(path.Join(w.basePath, p), 0755)
}

func (w *WebDAVStorage) Exists(_ context.Context, p string) (bool, error) {
	_, err := w.client.Stat(path.Join(w.basePath, p))
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (w *WebDAVStorage) Delete(_ context.Context, p string) error {
	return w.client.Remove(path.Join(w.basePath, p))
}

func (w *WebDAVStorage) ListDir(_ context.Context, p string) ([]storage.FileInfo, error) {
	entries, err := w.client.ReadDir(path.Join(w.basePath, p))
	if err != nil {
		return nil, fmt.Errorf("webdav listdir: %w", err)
	}
	var files []storage.FileInfo
	for _, e := range entries {
		files = append(files, storage.FileInfo{
			Name:  e.Name(),
			Path:  path.Join(p, e.Name()),
			Size:  e.Size(),
			IsDir: e.IsDir(),
		})
	}
	return files, nil
}

type progressReader struct {
	reader   io.Reader
	total    int64
	read     int64
	callback storage.ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)
	if pr.callback != nil {
		pr.callback(pr.read, pr.total)
	}
	return n, err
}
