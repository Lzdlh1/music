package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/musicflow/musicflow/internal/storage"
)

// LocalStorage 本地目录存储后端
type LocalStorage struct {
	id      string
	name    string
	basePath string
}

// New 创建本地存储
func New(id, name, basePath string) (*LocalStorage, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	return &LocalStorage{id: id, name: name, basePath: abs}, nil
}

func (l *LocalStorage) ID() string               { return l.id }
func (l *LocalStorage) Name() string              { return l.name }
func (l *LocalStorage) Type() storage.StorageType  { return storage.StorageLocal }

func (l *LocalStorage) Test(_ context.Context) error {
	return os.MkdirAll(l.basePath, 0750)
}

func (l *LocalStorage) Upload(_ context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	dest := filepath.Join(l.basePath, remotePath)

	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	dst, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dst.Close()

	if progress != nil {
		pr := &progressReader{reader: src, total: stat.Size(), callback: progress}
		_, err = io.Copy(dst, pr)
	} else {
		_, err = io.Copy(dst, src)
	}

	if err != nil {
		return fmt.Errorf("copy file: %w", err)
	}
	return nil
}

func (l *LocalStorage) MkdirAll(_ context.Context, path string) error {
	return os.MkdirAll(filepath.Join(l.basePath, path), 0750)
}

func (l *LocalStorage) Exists(_ context.Context, path string) (bool, error) {
	_, err := os.Stat(filepath.Join(l.basePath, path))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (l *LocalStorage) Delete(_ context.Context, path string) error {
	return os.Remove(filepath.Join(l.basePath, path))
}

func (l *LocalStorage) ListDir(_ context.Context, path string) ([]storage.FileInfo, error) {
	entries, err := os.ReadDir(filepath.Join(l.basePath, path))
	if err != nil {
		return nil, err
	}
	var files []storage.FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		files = append(files, storage.FileInfo{
			Name:  e.Name(),
			Path:  filepath.Join(path, e.Name()),
			Size:  size,
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
