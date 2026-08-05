package sftp

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/musicflow/musicflow/internal/storage"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Config SFTP 配置
type Config struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key"`
	BasePath   string `json:"base_path"`
}

// SFTPStorage SFTP 存储后端
type SFTPStorage struct {
	id       string
	name     string
	cfg      Config
	basePath string
}

// New 创建 SFTP 存储
func New(id, name string, cfg Config) *SFTPStorage {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	return &SFTPStorage{
		id:       id,
		name:     name,
		cfg:      cfg,
		basePath: strings.TrimSuffix(cfg.BasePath, "/"),
	}
}

func (s *SFTPStorage) ID() string              { return s.id }
func (s *SFTPStorage) Name() string            { return s.name }
func (s *SFTPStorage) Type() storage.StorageType { return storage.StorageSFTP }

func (s *SFTPStorage) connect() (*sftp.Client, *ssh.Client, error) {
	var authMethods []ssh.AuthMethod

	if s.cfg.PrivateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(s.cfg.PrivateKey))
		if err != nil {
			return nil, nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if s.cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(s.cfg.Password))
	}

	sshCfg := &ssh.ClientConfig{
		User:            s.cfg.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	sshConn, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial: %w", err)
	}

	client, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return nil, nil, fmt.Errorf("sftp client: %w", err)
	}

	return client, sshConn, nil
}

func (s *SFTPStorage) Test(_ context.Context) error {
	client, sshConn, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	defer sshConn.Close()

	_, err = client.ReadDir(s.basePath)
	if err != nil {
		return fmt.Errorf("sftp readdir: %w", err)
	}
	return nil
}

func (s *SFTPStorage) Upload(_ context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	client, sshConn, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	defer sshConn.Close()

	fullPath := path.Join(s.basePath, remotePath)

	// 创建远程目录
	dir := path.Dir(fullPath)
	if err := client.MkdirAll(dir); err != nil {
		return fmt.Errorf("sftp mkdir: %w", err)
	}

	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file: %w", err)
	}
	defer src.Close()

	stat, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	dst, err := client.Create(fullPath)
	if err != nil {
		return fmt.Errorf("sftp create: %w", err)
	}
	defer dst.Close()

	var reader io.Reader = src
	if progress != nil {
		reader = &progressReader{reader: src, total: stat.Size(), callback: progress}
	}

	if _, err := io.Copy(dst, reader); err != nil {
		return fmt.Errorf("sftp copy: %w", err)
	}
	return nil
}

func (s *SFTPStorage) MkdirAll(_ context.Context, p string) error {
	client, sshConn, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	defer sshConn.Close()
	return client.MkdirAll(path.Join(s.basePath, p))
}

func (s *SFTPStorage) Exists(_ context.Context, p string) (bool, error) {
	client, sshConn, err := s.connect()
	if err != nil {
		return false, err
	}
	defer client.Close()
	defer sshConn.Close()

	_, err = client.Stat(path.Join(s.basePath, p))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *SFTPStorage) Delete(_ context.Context, p string) error {
	client, sshConn, err := s.connect()
	if err != nil {
		return err
	}
	defer client.Close()
	defer sshConn.Close()
	return client.Remove(path.Join(s.basePath, p))
}

func (s *SFTPStorage) ListDir(_ context.Context, p string) ([]storage.FileInfo, error) {
	client, sshConn, err := s.connect()
	if err != nil {
		return nil, err
	}
	defer client.Close()
	defer sshConn.Close()

	entries, err := client.ReadDir(path.Join(s.basePath, p))
	if err != nil {
		return nil, fmt.Errorf("sftp readdir: %w", err)
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
