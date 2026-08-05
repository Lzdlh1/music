package metadata

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	"go.uber.org/zap"
)

// CoverProcessor 封面处理器
type CoverProcessor struct {
	log *zap.Logger
}

// NewCoverProcessor 创建封面处理器
func NewCoverProcessor(log *zap.Logger) *CoverProcessor {
	return &CoverProcessor{log: log}
}

// CoverSizes 封面尺寸配置
var CoverSizes = map[string]int{
	"embed":     500, // 内嵌到音频文件
	"folder":    300, // 目录 cover.jpg
	"thumbnail": 100, // 缩略图
}

// ProcessCover 处理封面：下载、生成多尺寸版本
func (cp *CoverProcessor) ProcessCover(ctx context.Context, coverURL string, outputDir string) (*CoverOutput, error) {
	// 下载原始封面
	data, err := DownloadCover(ctx, coverURL)
	if err != nil {
		return nil, fmt.Errorf("download cover: %w", err)
	}

	// 保存原图
	originPath := filepath.Join(outputDir, "cover_original.jpg")
	if err := os.WriteFile(originPath, data, 0644); err != nil {
		return nil, fmt.Errorf("save original cover: %w", err)
	}

	// 解码图片
	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode cover image: %w", err)
	}

	output := &CoverOutput{
		OriginalPath: originPath,
		OriginalData: data,
	}

	// 检查分辨率
	bounds := img.Bounds()
	if bounds.Dx() < 300 || bounds.Dy() < 300 {
		cp.log.Warn("cover resolution too low",
			zap.Int("width", bounds.Dx()),
			zap.Int("height", bounds.Dy()))
	}

	// 生成各尺寸
	for name, size := range CoverSizes {
		resized := imaging.Fit(img, size, size, imaging.Lanczos)
		path := filepath.Join(outputDir, fmt.Sprintf("cover_%s.jpg", name))
		if err := saveJPEG(resized, path, 95); err != nil {
			cp.log.Warn("save resized cover failed",
				zap.String("size", name),
				zap.Error(err))
			continue
		}

		switch name {
		case "embed":
			embedData, _ := os.ReadFile(path)
			output.EmbedData = embedData
			output.EmbedPath = path
		case "folder":
			output.FolderPath = path
		case "thumbnail":
			output.ThumbnailPath = path
		}
	}

	cp.log.Info("cover processed",
		zap.String("original", originPath),
		zap.Int("width", bounds.Dx()),
		zap.Int("height", bounds.Dy()))

	return output, nil
}

// CoverOutput 封面处理结果
type CoverOutput struct {
	OriginalPath  string
	OriginalData  []byte
	EmbedData     []byte
	EmbedPath     string
	FolderPath    string
	ThumbnailPath string
}

func saveJPEG(img image.Image, path string, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
}
