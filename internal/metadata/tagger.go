package metadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bogem/id3v2/v2"
	"go.uber.org/zap"
)

// Tagger 音频标签写入器
type Tagger struct {
	log *zap.Logger
}

// TagInfo 标签信息
type TagInfo struct {
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"album_artist"`
	Album       string `json:"album"`
	TrackNo     string `json:"track_no"`
	Year        string `json:"year"`
	Genre       string `json:"genre"`
	Comment     string `json:"comment"`
	CoverData   []byte `json:"-"`
	CoverMIME   string `json:"-"`
	Lyrics      string `json:"lyrics"`
}

// NewTagger 创建标签写入器
func NewTagger(log *zap.Logger) *Tagger {
	return &Tagger{log: log}
}

// WriteMP3Tags 写入 MP3 ID3v2 标签
func (t *Tagger) WriteMP3Tags(filePath string, info *TagInfo) error {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return fmt.Errorf("open mp3 for tagging: %w", err)
	}
	defer tag.Close()

	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	tag.SetTitle(info.Title)
	tag.SetArtist(info.Artist)
	tag.SetAlbum(info.Album)
	tag.SetYear(info.Year)
	tag.SetGenre(info.Genre)

	if info.AlbumArtist != "" {
		tag.AddTextFrame("TPE2", id3v2.EncodingUTF8, info.AlbumArtist)
	}
	if info.TrackNo != "" {
		tag.AddTextFrame("TRCK", id3v2.EncodingUTF8, info.TrackNo)
	}
	if info.Comment != "" {
		tag.AddCommentFrame(id3v2.CommentFrame{
			Encoding: id3v2.EncodingUTF8,
			Language: "eng",
			Text:     info.Comment,
		})
	}

	// 内嵌封面
	if len(info.CoverData) > 0 {
		mime := info.CoverMIME
		if mime == "" {
			mime = "image/jpeg"
		}
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    mime,
			PictureType: id3v2.PTFrontCover,
			Picture:     info.CoverData,
		})
	}

	// 内嵌歌词
	if info.Lyrics != "" {
		tag.AddUnsynchronisedLyricsFrame(id3v2.UnsynchronisedLyricsFrame{
			Encoding: id3v2.EncodingUTF8,
			Language: "eng",
			Lyrics:   info.Lyrics,
		})
	}

	if err := tag.Save(); err != nil {
		return fmt.Errorf("save mp3 tags: %w", err)
	}

	t.log.Info("mp3 tags written",
		zap.String("file", filePath),
		zap.String("title", info.Title),
		zap.String("artist", info.Artist))
	return nil
}

// WriteTags 根据文件扩展名自动选择标签写入方式
func (t *Tagger) WriteTags(filePath string, info *TagInfo) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp3":
		return t.WriteMP3Tags(filePath, info)
	case ".flac":
		// FLAC 标签写入通过 ffmpeg 完成
		return t.WriteFLACTagsViaFFmpeg(filePath, info)
	default:
		return fmt.Errorf("unsupported format for tagging: %s", ext)
	}
}

// WriteFLACTagsViaFFmpeg 通过 ffmpeg 写入 FLAC Vorbis Comment 标签
func (t *Tagger) WriteFLACTagsViaFFmpeg(filePath string, info *TagInfo) error {
	// 构建 ffmpeg metadata 参数
	args := []string{"-i", filePath, "-y"}

	// 添加封面图
	hasCover := len(info.CoverData) > 0
	var coverTmpPath string
	if hasCover {
		coverTmpPath = filePath + ".cover.jpg"
		if err := os.WriteFile(coverTmpPath, info.CoverData, 0644); err != nil {
			t.log.Warn("write cover tmp for ffmpeg", zap.Error(err))
			hasCover = false
		} else {
			defer os.Remove(coverTmpPath)
			args = append(args, "-i", coverTmpPath)
		}
	}

	// metadata
	args = append(args,
		"-map", "0:a",
	)
	if hasCover {
		args = append(args,
			"-map", "1",
			"-disposition:v:0", "attached_pic",
		)
	}

	args = append(args, "-codec", "copy")

	// Vorbis Comment 标签
	metadataArgs := []struct{ key, val string }{
		{"TITLE", info.Title},
		{"ARTIST", info.Artist},
		{"ALBUM", info.Album},
		{"ALBUMARTIST", info.AlbumArtist},
		{"DATE", info.Year},
		{"GENRE", info.Genre},
		{"TRACKNUMBER", info.TrackNo},
		{"COMMENT", info.Comment},
	}
	for _, m := range metadataArgs {
		if m.val != "" {
			args = append(args, "-metadata", m.key+"="+m.val)
		}
	}

	// 如需内嵌歌词（LYRICS tag）
	if info.Lyrics != "" {
		args = append(args, "-metadata", "LYRICS="+info.Lyrics)
	}

	outputPath := filePath + ".tmp.flac"
	args = append(args, outputPath)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.log.Error("ffmpeg flac tagging failed",
			zap.String("output", string(output)),
			zap.Error(err))
		os.Remove(outputPath)
		return fmt.Errorf("ffmpeg flac tag: %w", err)
	}

	// 替换原文件
	if err := os.Rename(outputPath, filePath); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("replace flac file: %w", err)
	}

	t.log.Info("flac tags written via ffmpeg",
		zap.String("file", filePath),
		zap.String("title", info.Title),
		zap.String("artist", info.Artist))
	return nil
}

// DownloadCover 从 URL 下载封面图片
func DownloadCover(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create cover request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download cover: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024)) // 最大 20MB
	if err != nil {
		return nil, fmt.Errorf("read cover data: %w", err)
	}
	return data, nil
}

// SaveLRCFile 保存歌词为 .lrc 文件
func SaveLRCFile(audioPath, lrcContent string) error {
	ext := filepath.Ext(audioPath)
	lrcPath := strings.TrimSuffix(audioPath, ext) + ".lrc"
	return os.WriteFile(lrcPath, []byte(lrcContent), 0644)
}
