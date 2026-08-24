package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/musicflow/musicflow/internal/storage"
)

// LocalStorage 本地目录存储后端
type LocalStorage struct {
	id        string
	name      string
	basePath  string
	uploadDir string
}

// New 创建本地存储
func New(id, name, basePath, uploadDir string) (*LocalStorage, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	return &LocalStorage{id: id, name: name, basePath: abs, uploadDir: uploadDir}, nil
}

// UploadDir 返回该存储配置的上传文件夹（相对存储根）
func (l *LocalStorage) UploadDir() string { return l.uploadDir }

// fullPath 将远程路径（正斜杠）转换为本地文件系统路径
func (l *LocalStorage) fullPath(remote string) string {
	rel := filepath.FromSlash(remote)
	return filepath.Join(l.basePath, rel)
}

func (l *LocalStorage) ID() string               { return l.id }
func (l *LocalStorage) Name() string              { return l.name }
func (l *LocalStorage) Type() storage.StorageType { return storage.StorageLocal }

func (l *LocalStorage) Test(_ context.Context) error {
	return os.MkdirAll(l.basePath, 0750)
}

func (l *LocalStorage) Upload(_ context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	dest := l.fullPath(remotePath)

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

func (l *LocalStorage) MkdirAll(_ context.Context, p string) error {
	return os.MkdirAll(l.fullPath(p), 0750)
}

func (l *LocalStorage) Exists(_ context.Context, p string) (bool, error) {
	_, err := os.Stat(l.fullPath(p))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func (l *LocalStorage) Delete(_ context.Context, p string) error {
	// 支持删除文件或目录（递归）
	return os.RemoveAll(l.fullPath(p))
}

func (l *LocalStorage) ListDir(_ context.Context, p string) ([]storage.FileInfo, error) {
	entries, err := os.ReadDir(l.fullPath(p))
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
			Path:  path.Join(p, e.Name()),
			Size:  size,
			IsDir: e.IsDir(),
		})
	}
	return files, nil
}

// Open 打开本地文件流，支持 Range（offset/length）
func (l *LocalStorage) Open(_ context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	f, err := os.Open(l.fullPath(p))
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	if length > 0 {
		return &limitedReadCloser{ReadCloser: f, remaining: length}, nil
	}
	return f, nil
}

// Size 获取本地文件大小
func (l *LocalStorage) Size(_ context.Context, p string) (int64, error) {
	info, err := os.Stat(l.fullPath(p))
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// Rename 重命名/移动本地文件或目录
func (l *LocalStorage) Rename(_ context.Context, oldPath, newPath string) error {
	return os.Rename(l.fullPath(oldPath), l.fullPath(newPath))
}

// limitedReadCloser 限制读取字节数的 ReadCloser
type limitedReadCloser struct {
	io.ReadCloser
	remaining int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.ReadCloser.Read(p)
	l.remaining -= int64(n)
	return n, err
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
