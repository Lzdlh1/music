package storage

import "context"

// ProgressCallback 上传进度回调
type ProgressCallback func(uploaded, total int64)

// FileInfo 远程文件信息
type FileInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// StorageType 存储类型
type StorageType string

const (
	StorageWebDAV    StorageType = "webdav"
	StorageLocal     StorageType = "local"
	StorageSFTP      StorageType = "sftp"
	StorageS3        StorageType = "s3"
	StorageOneDrive  StorageType = "onedrive"
	StorageAliyun    StorageType = "aliyun"
	StorageGDrive    StorageType = "gdrive"
)

// Backend 存储后端统一接口
type Backend interface {
	ID() string
	Name() string
	Type() StorageType
	Test(ctx context.Context) error
	Upload(ctx context.Context, localPath string, remotePath string, progress ProgressCallback) error
	MkdirAll(ctx context.Context, path string) error
	Exists(ctx context.Context, path string) (bool, error)
	Delete(ctx context.Context, path string) error
	ListDir(ctx context.Context, path string) ([]FileInfo, error)
}
