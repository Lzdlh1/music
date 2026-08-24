package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/metadata"
	"github.com/musicflow/musicflow/internal/scheduler"
	"github.com/musicflow/musicflow/internal/source"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Worker 任务执行器，编排 下载→打标签→上传 流水线
type Worker struct {
	db         *gorm.DB
	aggregator *source.Aggregator
	storageMgr *storage.Manager
	mtMgr      *telegram.MTProtoManager
	tagger     *metadata.Tagger
	lyricsMgr  *metadata.LyricsManager
	coverProc  *metadata.CoverProcessor
	cfg        *config.Config
	log        *zap.Logger
}

// taskTrackInfo 从 task.TrackInfo JSON 解析的结构
type taskTrackInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	AlbumArtist string `json:"album_artist"`
	Album       string `json:"album"`
	TrackNo     int    `json:"track_no"`
	DiscNo      int    `json:"disc_no"`
	Year        int    `json:"year"`
	Genre       string `json:"genre"`
	Duration    int    `json:"duration"`
	CoverURL    string `json:"cover_url"`
	Source      string `json:"source"`
}

// taskSourceInfo 从 task.SelectedSource JSON 解析的结构
type taskSourceInfo struct {
	SourceName string `json:"source_name"`
	Quality    int    `json:"quality"`
	Format     string `json:"format"`
}

// New 创建 Worker
func New(
	db *gorm.DB,
	agg *source.Aggregator,
	sm *storage.Manager,
	mtMgr *telegram.MTProtoManager,
	cfg *config.Config,
	log *zap.Logger,
) *Worker {
	return &Worker{
		db:         db,
		aggregator: agg,
		storageMgr: sm,
		mtMgr:      mtMgr,
		tagger:     metadata.NewTagger(log),
		lyricsMgr:  metadata.NewLyricsManager(log),
		coverProc:  metadata.NewCoverProcessor(log),
		cfg:        cfg,
		log:        log,
	}
}

// loadSettings 从数据库 settings 表加载用户偏好（命名模板、下载偏好），覆盖 config 默认值
func (w *Worker) loadSettings() {
	// 命名模板
	var naming models.Setting
	if err := w.db.First(&naming, "key = ?", "naming").Error; err == nil {
		var data struct {
			Template string `json:"template"`
		}
		if err := json.Unmarshal(naming.Value, &data); err == nil && data.Template != "" {
			w.cfg.Naming.Template = data.Template
		}
	}

	// 下载偏好
	var dl models.Setting
	if err := w.db.First(&dl, "key = ?", "download").Error; err == nil {
		var data struct {
			DefaultQuality string `json:"default_quality"`
			EmbedCover     *bool  `json:"embed_cover"`
			EmbedLyrics    *bool  `json:"embed_lyrics"`
			SaveLrcFile    *bool  `json:"save_lrc_file"`
		}
		if err := json.Unmarshal(dl.Value, &data); err == nil {
			if data.DefaultQuality != "" {
				w.cfg.Download.DefaultQuality = data.DefaultQuality
			}
			if data.EmbedCover != nil {
				w.cfg.Download.EmbedCover = *data.EmbedCover
			}
			if data.EmbedLyrics != nil {
				w.cfg.Download.EmbedLyrics = *data.EmbedLyrics
			}
			if data.SaveLrcFile != nil {
				w.cfg.Download.SaveLrcFile = *data.SaveLrcFile
			}
		}
	}
}

// Execute 任务执行主函数，由 scheduler 调用
func (w *Worker) Execute(ctx context.Context, task *models.Task, onProgress func(status string, progress scheduler.TaskProgress)) error {
	// 0. 从数据库加载用户偏好设置（覆盖 config.yaml）
	w.loadSettings()

	// 1. 解析任务信息
	var trackInfo taskTrackInfo
	if err := json.Unmarshal([]byte(task.TrackInfo), &trackInfo); err != nil {
		return fmt.Errorf("parse track info: %w", err)
	}

	var sourceInfo taskSourceInfo
	if len(task.SelectedSource) > 0 {
		if err := json.Unmarshal([]byte(task.SelectedSource), &sourceInfo); err != nil {
			w.log.Warn("parse selected source failed, using default", zap.Error(err))
		}
	}

	var uploadTargetIDs []string
	if len(task.UploadTargets) > 0 {
		json.Unmarshal([]byte(task.UploadTargets), &uploadTargetIDs)
	}

	// 用户指定的上传文件夹（相对存储根，统一为正斜杠并去掉首尾斜杠）
	uploadDir := task.UploadDir
	if uploadDir != "" {
		uploadDir = strings.Trim(strings.ReplaceAll(uploadDir, "\\", "/"), "/")
	}

	w.log.Info("worker executing task",
		zap.String("task_id", task.ID),
		zap.String("title", trackInfo.Title),
		zap.String("artist", trackInfo.Artist))

	// 2. 获取下载链接
	onProgress(scheduler.StatusFetchMeta, scheduler.TaskProgress{Stage: "fetching_meta", Percent: 5})

	quality := source.Quality(sourceInfo.Quality)
	if quality == source.QualityAny {
		quality = parseDefaultQuality(w.cfg.Download.DefaultQuality)
	}

	availSources, err := w.aggregator.GetTrackSources(ctx, trackInfo.ID, quality)
	if err != nil || len(availSources) == 0 {
		if err != nil {
			return fmt.Errorf("get download sources: %w", err)
		}
		return fmt.Errorf("no download sources available")
	}

	bestSource := availSources[0]

	// 确定文件格式
	format := sourceInfo.Format
	if format == "" {
		format = bestSource.Format
	}
	if format == "" {
		format = "mp3"
	}

	// 3. 下载音频文件到临时目录
	onProgress(scheduler.StatusDownloading, scheduler.TaskProgress{Stage: "downloading", Percent: 10})

	tempDir := w.cfg.Scheduler.TempDir
	tempFile := filepath.Join(tempDir, task.ID+"."+format)

	// 支持特殊的 mtproto 下载协议
	if strings.HasPrefix(bestSource.DownloadURL, "mtproto://") {
		err = w.downloadFromMTProto(ctx, bestSource.DownloadURL, tempFile, func(downloaded, total int64) {
			pct := 10.0
			if total > 0 {
				pct = 10.0 + 50.0*float64(downloaded)/float64(total)
			}
			onProgress(scheduler.StatusDownloading, scheduler.TaskProgress{
				Stage:      "downloading",
				Percent:    pct,
				Downloaded: downloaded,
				Total:      total,
				Speed:      0,
			})
		})
	} else {
		err = w.downloadFile(ctx, bestSource.DownloadURL, tempFile, func(downloaded, total int64) {
			pct := 10.0
			if total > 0 {
				pct = 10.0 + 50.0*float64(downloaded)/float64(total)
			}
			onProgress(scheduler.StatusDownloading, scheduler.TaskProgress{
				Stage:      "downloading",
				Percent:    pct,
				Downloaded: downloaded,
				Total:      total,
				Speed:      0,
			})
		})
	}

	// 4. 元数据处理（标签、封面、歌词）
	onProgress(scheduler.StatusProcessing, scheduler.TaskProgress{Stage: "processing", Percent: 65})

	tagInfo := &metadata.TagInfo{
		Title:       trackInfo.Title,
		Artist:      trackInfo.Artist,
		AlbumArtist: trackInfo.AlbumArtist,
		Album:       trackInfo.Album,
		TrackNo:     fmt.Sprintf("%d", trackInfo.TrackNo),
		Year:        fmt.Sprintf("%d", trackInfo.Year),
		Genre:       trackInfo.Genre,
	}

	// 下载封面（封面 URL 缺失时尝试从音乐源获取）
	if trackInfo.CoverURL == "" && trackInfo.ID != "" {
		if cv, err := w.aggregator.GetCover(ctx, trackInfo.ID); err == nil && cv != nil && cv.URL != "" {
			trackInfo.CoverURL = cv.URL
		}
	}
	var coverPath string
	if w.cfg.Download.EmbedCover && trackInfo.CoverURL != "" {
		coverOutput, err := w.coverProc.ProcessCover(ctx, trackInfo.CoverURL, tempDir)
		if err != nil {
			w.log.Warn("process cover failed", zap.Error(err))
			// 降级：直接下载原始封面
			coverData, dlErr := metadata.DownloadCover(ctx, trackInfo.CoverURL)
			if dlErr == nil {
				tagInfo.CoverData = coverData
				tagInfo.CoverMIME = "image/jpeg"
				coverPath = filepath.Join(tempDir, task.ID+"_cover.jpg")
				_ = os.WriteFile(coverPath, coverData, 0644)
			}
		} else {
			if len(coverOutput.EmbedData) > 0 {
				tagInfo.CoverData = coverOutput.EmbedData
			} else {
				tagInfo.CoverData = coverOutput.OriginalData
			}
			tagInfo.CoverMIME = "image/jpeg"
			// 优先用 300px 的目录封面（cover_folder.jpg），否则用原图
			coverPath = coverOutput.FolderPath
			if coverPath == "" {
				coverPath = coverOutput.OriginalPath
			}
		}
	}

	onProgress(scheduler.StatusProcessing, scheduler.TaskProgress{Stage: "processing_lyrics", Percent: 70})

	// 获取歌词：优先从音乐源（GD API lyric 接口），失败再回退 LRClib
	lrcContent := ""
	if w.cfg.Download.EmbedLyrics || w.cfg.Download.SaveLrcFile {
		var lyrics *metadata.LyricsData
		if trackInfo.ID != "" {
			if srcLyr, err := w.aggregator.GetLyrics(ctx, trackInfo.ID); err == nil && srcLyr != nil && srcLyr.LRC != "" {
				lyrics = &metadata.LyricsData{
					LRC:      srcLyr.LRC,
					TransLRC: srcLyr.TransLRC,
					Source:   srcLyr.Source,
				}
			}
		}
		if lyrics == nil || lyrics.LRC == "" {
			if lrcl, err := w.lyricsMgr.FetchLyrics(ctx, trackInfo.Title, trackInfo.Artist); err == nil {
				lyrics = lrcl
			} else {
				w.log.Warn("fetch lyrics failed", zap.Error(err))
			}
		}
		if lyrics != nil && lyrics.LRC != "" {
			lrcContent = lyrics.LRC
			if w.cfg.Download.EmbedLyrics {
				tagInfo.Lyrics = lyrics.LRC
			}
			if w.cfg.Download.SaveLrcFile {
				metadata.SaveLRCFile(tempFile, lyrics.LRC)
			}
		}
	}

	onProgress(scheduler.StatusProcessing, scheduler.TaskProgress{Stage: "writing_tags", Percent: 75})

	// 写入标签
	if err := w.tagger.WriteTags(tempFile, tagInfo); err != nil {
		w.log.Warn("write tags failed", zap.Error(err))
		// 标签写入失败不中断流程
	}

	// 5. 生成目标路径
	namingTpl := &storage.NamingTemplate{Template: w.cfg.Naming.Template}
	ext := format
	remotePath := namingTpl.Format(storage.TrackNamingInfo{
		Artist:      trackInfo.Artist,
		AlbumArtist: trackInfo.AlbumArtist,
		Album:       trackInfo.Album,
		Title:       trackInfo.Title,
		Year:        fmt.Sprintf("%d", trackInfo.Year),
		TrackNo:     trackInfo.TrackNo,
		DiscNo:      trackInfo.DiscNo,
		Genre:       trackInfo.Genre,
		Ext:         ext,
		Quality:     source.Quality(sourceInfo.Quality).String(),
		Source:      trackInfo.Source,
	})

	// 6. 上传到存储目标
	onProgress(scheduler.StatusUploading, scheduler.TaskProgress{Stage: "uploading", Percent: 80})

	remotePaths := make(map[string]string)

	backends := w.resolveUploadTargets(uploadTargetIDs)
	if len(backends) == 0 {
		w.log.Warn("no upload targets configured, keeping file in temp")
	}

	for i, backend := range backends {
		bid := backend.ID()

		// 每个存储使用自身配置的上传文件夹（任务级 uploadDir 作为兜底）
		bdir := uploadDir
		if p, ok := backend.(uploadDirProvider); ok {
			if d := p.UploadDir(); d != "" {
				bdir = d
			}
		}
		bdir = strings.Trim(strings.ReplaceAll(bdir, "\\", "/"), "/")
		remoteForBackend := remotePath
		if bdir != "" {
			remoteForBackend = bdir + "/" + strings.TrimPrefix(remotePath, "/")
		}

		onProgress(scheduler.StatusUploading, scheduler.TaskProgress{
			Stage:   "uploading",
			Percent: 80.0 + 18.0*float64(i)/float64(len(backends)),
			UploadProgress: map[string]float64{
				bid: 0,
			},
		})

		// 创建目录（用正斜杠，避免 Windows 反斜杠破坏云盘路径）
		dir := path.Dir(remoteForBackend)
		if dir != "" && dir != "." {
			if err := backend.MkdirAll(ctx, dir); err != nil {
				w.log.Warn("mkdir failed", zap.String("backend", bid), zap.Error(err))
			}
		}

		err := backend.Upload(ctx, tempFile, remoteForBackend, func(uploaded, total int64) {
			pct := 0.0
			if total > 0 {
				pct = float64(uploaded) / float64(total) * 100
			}
			onProgress(scheduler.StatusUploading, scheduler.TaskProgress{
				Stage:   "uploading",
				Percent: 80.0 + 18.0*float64(i)/float64(len(backends)),
				UploadProgress: map[string]float64{
					bid: pct,
				},
			})
		})
		if err != nil {
			w.log.Error("upload failed", zap.String("backend", bid), zap.Error(err))
			continue
		}
		remotePaths[bid] = remoteForBackend
		w.log.Info("uploaded", zap.String("backend", bid), zap.String("path", remoteForBackend))

		// 上传歌词 .lrc 文件（与歌曲同目录）
		if w.cfg.Download.SaveLrcFile && lrcContent != "" {
			lrcLocal := strings.TrimSuffix(tempFile, filepath.Ext(tempFile)) + ".lrc"
			lrcRemote := strings.TrimSuffix(remoteForBackend, filepath.Ext(remoteForBackend)) + ".lrc"
			if _, err := os.Stat(lrcLocal); err == nil {
				if err := backend.Upload(ctx, lrcLocal, lrcRemote, nil); err != nil {
					w.log.Warn("upload lrc failed", zap.String("backend", bid), zap.String("path", lrcRemote), zap.Error(err))
				} else {
					w.log.Info("uploaded lrc", zap.String("backend", bid), zap.String("path", lrcRemote))
				}
			}
		}

		// 上传封面文件（与歌曲同目录，命名为 cover.jpg）
		if coverPath != "" {
			if _, err := os.Stat(coverPath); err == nil {
				coverRemote := "cover.jpg"
				if dir != "" && dir != "." {
					coverRemote = dir + "/cover.jpg"
				}
				if err := backend.Upload(ctx, coverPath, coverRemote, nil); err != nil {
					w.log.Warn("upload cover failed", zap.String("backend", bid), zap.String("path", coverRemote), zap.Error(err))
				} else {
					w.log.Info("uploaded cover", zap.String("backend", bid), zap.String("path", coverRemote))
				}
			}
		}
	}

	// 7. 写入 Library 记录
	onProgress(scheduler.StatusProcessing, scheduler.TaskProgress{Stage: "saving_library", Percent: 98})

	fileInfo, _ := os.Stat(tempFile)
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	remotePathsJSON, _ := json.Marshal(remotePaths)
	library := &models.Library{
		ID:            uuid.New().String(),
		Title:         trackInfo.Title,
		Artist:        trackInfo.Artist,
		Album:         trackInfo.Album,
		Year:          trackInfo.Year,
		Genre:         trackInfo.Genre,
		Quality:       source.Quality(sourceInfo.Quality).String(),
		Format:        format,
		FileSize:      fileSize,
		Duration:      trackInfo.Duration,
		Source:        trackInfo.Source,
		SourceTrackID: trackInfo.ID,
		RemotePaths:   models.JSON(remotePathsJSON),
		CoverURL:      trackInfo.CoverURL,
		LyricsLRC:     lrcContent,
		HasLyrics:     lrcContent != "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	if err := w.db.Create(library).Error; err != nil {
		w.log.Error("save library record", zap.Error(err))
	}

	// 8. 清理临时文件
	w.cleanupTemp(tempFile)
	lrcFile := strings.TrimSuffix(tempFile, filepath.Ext(tempFile)) + ".lrc"
	w.cleanupTemp(lrcFile)
	coverFile := filepath.Join(tempDir, task.ID+"_cover.jpg")
	w.cleanupTemp(coverFile)

	onProgress(scheduler.StatusDone, scheduler.TaskProgress{Stage: "done", Percent: 100})

	return nil
}

// downloadFile 下载文件到本地路径
func (w *Worker) downloadFile(ctx context.Context, fileURL, destPath string, onProgress func(downloaded, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	req.Header.Set("User-Agent", "MusicFlow/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status: %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write temp file: %w", writeErr)
			}
			downloaded += int64(n)
			if onProgress != nil {
				onProgress(downloaded, total)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read download stream: %w", readErr)
		}
	}

	return nil
}

// downloadFromMTProto 从 Telegram MTProto 下载文件
func (w *Worker) downloadFromMTProto(ctx context.Context, url string, destPath string, onProgress func(downloaded, total int64)) error {
	// url 格式: mtproto://botUsername/docID/accessHash
	parts := strings.Split(strings.TrimPrefix(url, "mtproto://"), "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid mtproto url: %s", url)
	}

	// 这里未来实现具体的 MTProto 大文件下载逻辑
	// 暂作为待实现桩
	w.log.Info("downloading from mtproto", zap.String("url", url))
	return fmt.Errorf("mtproto download not implemented yet")
}

// uploadDirProvider 可选：存储后端提供自身“上传文件夹”（相对该存储根）
type uploadDirProvider interface {
	UploadDir() string
}

// resolveUploadTargets 解析上传目标
func (w *Worker) resolveUploadTargets(targetIDs []string) []storage.Backend {
	var backends []storage.Backend

	if len(targetIDs) > 0 {
		for _, id := range targetIDs {
			if b, ok := w.storageMgr.Get(id); ok {
				backends = append(backends, b)
			}
		}
	} else {
		// 没有指定目标时，使用所有已注册的存储
		backends = w.storageMgr.List()
	}

	return backends
}

// cleanupTemp 清理临时文件
func (w *Worker) cleanupTemp(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		w.log.Warn("cleanup temp file", zap.String("path", path), zap.Error(err))
	}
}

func parseDefaultQuality(s string) source.Quality {
	switch strings.ToUpper(s) {
	case "128", "128K":
		return source.Quality128
	case "320", "320K":
		return source.Quality320
	case "FLAC", "LOSSLESS":
		return source.QualityFLAC
	case "HIRES", "HI-RES":
		return source.QualityHiRes
	default:
		return source.QualityFLAC
	}
}
