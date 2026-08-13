package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/musicflow/musicflow/internal/storage"
)

// Config S3/兼容 配置
type Config struct {
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BasePath        string `json:"base_path"`
	ForcePathStyle  bool   `json:"force_path_style"`
}

// S3Storage S3/兼容对象存储后端
type S3Storage struct {
	id       string
	name     string
	cfg      Config
	basePath string
}

// New 创建 S3 存储
func New(id, name string, cfg Config) *S3Storage {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &S3Storage{
		id:       id,
		name:     name,
		cfg:      cfg,
		basePath: strings.TrimPrefix(strings.TrimSuffix(cfg.BasePath, "/"), "/"),
	}
}

func (s *S3Storage) ID() string              { return s.id }
func (s *S3Storage) Name() string            { return s.name }
func (s *S3Storage) Type() storage.StorageType { return storage.StorageS3 }

func (s *S3Storage) client(ctx context.Context) (*s3.Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(s.cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.cfg.AccessKeyID, s.cfg.SecretAccessKey, "",
		)),
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if s.cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s.cfg.Endpoint)
			o.UsePathStyle = s.cfg.ForcePathStyle
		})
	}

	return s3.NewFromConfig(cfg, s3Opts...), nil
}

func (s *S3Storage) key(remotePath string) string {
	if s.basePath == "" {
		return strings.TrimPrefix(remotePath, "/")
	}
	return s.basePath + "/" + strings.TrimPrefix(remotePath, "/")
}

func (s *S3Storage) Test(ctx context.Context) error {
	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	_, err = client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.cfg.Bucket),
	})
	if err != nil {
		return fmt.Errorf("s3 head bucket: %w", err)
	}
	return nil
}

func (s *S3Storage) Upload(ctx context.Context, localPath string, remotePath string, progress storage.ProgressCallback) error {
	client, err := s.client(ctx)
	if err != nil {
		return err
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

	var body io.Reader = f
	if progress != nil {
		body = &progressReader{reader: f, total: stat.Size(), callback: progress}
	}

	key := s.key(remotePath)
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.cfg.Bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(stat.Size()),
	})
	if err != nil {
		return fmt.Errorf("s3 put object: %w", err)
	}
	return nil
}

func (s *S3Storage) MkdirAll(_ context.Context, _ string) error {
	// S3 没有真实目录概念
	return nil
}

func (s *S3Storage) Exists(ctx context.Context, p string) (bool, error) {
	client, err := s.client(ctx)
	if err != nil {
		return false, err
	}

	_, err = client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(p)),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *S3Storage) Delete(ctx context.Context, p string) error {
	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(p)),
	})
	return err
}

func (s *S3Storage) ListDir(ctx context.Context, p string) ([]storage.FileInfo, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	prefix := s.key(p)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	resp, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.cfg.Bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 list: %w", err)
	}

	var files []storage.FileInfo

	// 子目录（CommonPrefixes）
	for _, cp := range resp.CommonPrefixes {
		name := strings.TrimPrefix(*cp.Prefix, prefix)
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		files = append(files, storage.FileInfo{
			Name:  name,
			Path:  path.Join(p, name),
			IsDir: true,
		})
	}

	// 文件
	for _, obj := range resp.Contents {
		name := strings.TrimPrefix(*obj.Key, prefix)
		if name == "" {
			continue
		}
		files = append(files, storage.FileInfo{
			Name:  name,
			Path:  path.Join(p, name),
			Size:  *obj.Size,
			IsDir: false,
		})
	}

	return files, nil
}

// Open 打开远程对象流，支持 Range（offset/length）
func (s *S3Storage) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}

	in := &s3.GetObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(p)),
	}
	if offset > 0 || length > 0 {
		if length > 0 {
			in.Range = aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
		} else {
			in.Range = aws.String(fmt.Sprintf("bytes=%d-", offset))
		}
	}

	out, err := client.GetObject(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("s3 get object: %w", err)
	}
	return out.Body, nil
}

// Size 获取远程对象大小
func (s *S3Storage) Size(ctx context.Context, p string) (int64, error) {
	client, err := s.client(ctx)
	if err != nil {
		return 0, err
	}

	h, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(s.key(p)),
	})
	if err != nil {
		return 0, fmt.Errorf("s3 head object: %w", err)
	}
	return aws.ToInt64(h.ContentLength), nil
}

// Rename 重命名/移动远程对象（S3 通过复制+删除实现）
func (s *S3Storage) Rename(ctx context.Context, oldPath, newPath string) error {
	client, err := s.client(ctx)
	if err != nil {
		return err
	}

	srcKey := s.key(oldPath)
	dstKey := s.key(newPath)

	_, err = client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.cfg.Bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(s.cfg.Bucket + "/" + encodeS3Key(srcKey)),
	})
	if err != nil {
		return fmt.Errorf("s3 copy: %w", err)
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.cfg.Bucket),
		Key:    aws.String(srcKey),
	})
	if err != nil {
		return fmt.Errorf("s3 delete after copy: %w", err)
	}
	return nil
}

// encodeS3Key 对 S3 key 做 URL 编码（保留路径分隔符）
func encodeS3Key(key string) string {
	segs := strings.Split(key, "/")
	for i, seg := range segs {
		segs[i] = url.PathEscape(seg)
	}
	return strings.Join(segs, "/")
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
