package storage

import (
	"context"
	"io"
)

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
	StorageAlipan    StorageType = "alipan"
	StorageYun139    StorageType = "yun139"
	StorageTianyi    StorageType = "tianyi"
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
	// Open 打开远程文件流（用于下载/播放）。offset 为起始字节偏移，length<=0 表示读取至文件末尾。
	Open(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error)
	// Size 获取远程文件大小（字节）
	Size(ctx context.Context, path string) (int64, error)
	// Rename 重命名/移动远程文件或目录
	Rename(ctx context.Context, oldPath, newPath string) error
}
