package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/musicflow/musicflow/internal/config"
	"github.com/musicflow/musicflow/internal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TaskStatus 任务状态常量
const (
	StatusPending     = "PENDING"
	StatusFetchMeta   = "FETCHING_META"
	StatusDownloading = "DOWNLOADING"
	StatusProcessing  = "PROCESSING"
	StatusUploading   = "UPLOADING"
	StatusDone        = "DONE"
	StatusFailed      = "FAILED"
	StatusPaused      = "PAUSED"
	StatusCancelled   = "CANCELLED"
)

// TaskProgress 任务进度
type TaskProgress struct {
	Stage          string             `json:"stage"`
	Percent        float64            `json:"percent"`
	Speed          int64              `json:"speed"`
	Downloaded     int64              `json:"downloaded"`
	Total          int64              `json:"total"`
	ETA            int                `json:"eta"`
	UploadProgress map[string]float64 `json:"upload_progress,omitempty"`
}

// ProgressListener 进度监听回调
type ProgressListener func(taskID string, status string, progress TaskProgress)

// Scheduler 任务调度器
type Scheduler struct {
	db              *gorm.DB
	cfg             *config.SchedulerConfig
	log             *zap.Logger
	downloadSem     chan struct{}
	uploadSem       chan struct{}
	tasks           sync.Map
	cancelFuncs     sync.Map
	listener        ProgressListener
	workerFunc      WorkerFunc
	mu              sync.Mutex
}

// WorkerFunc 任务执行函数类型
type WorkerFunc func(ctx context.Context, task *models.Task, onProgress func(status string, progress TaskProgress)) error

// New 创建调度器
func New(db *gorm.DB, cfg *config.SchedulerConfig, log *zap.Logger) *Scheduler {
	return &Scheduler{
		db:          db,
		cfg:         cfg,
		log:         log,
		downloadSem: make(chan struct{}, cfg.MaxConcurrentDownloads),
		uploadSem:   make(chan struct{}, cfg.MaxConcurrentUploads),
	}
}

// SetWorkerFunc 设置任务工作函数
func (s *Scheduler) SetWorkerFunc(fn WorkerFunc) {
	s.workerFunc = fn
}

// SetProgressListener 设置进度监听器
func (s *Scheduler) SetProgressListener(fn ProgressListener) {
	s.listener = fn
}

// CreateTask 创建新任务
func (s *Scheduler) CreateTask(trackInfo, selectedSource, uploadTargets []byte, uploadDir string) (*models.Task, error) {
	task := &models.Task{
		ID:             uuid.New().String(),
		Type:           "SINGLE",
		Status:         StatusPending,
		Priority:       0,
		TrackInfo:      models.JSON(trackInfo),
		SelectedSource: models.JSON(selectedSource),
		UploadTargets:  models.JSON(uploadTargets),
		UploadDir:      uploadDir,
		CreatedAt:      time.Now(),
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	s.log.Info("task created", zap.String("task_id", task.ID))
	s.notifyProgress(task.ID, StatusPending, TaskProgress{Stage: "queued"})

	// 异步执行
	go s.executeTask(task)

	return task, nil
}

// CreateBatchTasks 批量创建任务
func (s *Scheduler) CreateBatchTasks(tasks []models.Task) ([]models.Task, error) {
	for i := range tasks {
		tasks[i].ID = uuid.New().String()
		tasks[i].Status = StatusPending
		tasks[i].Type = "BATCH"
		tasks[i].CreatedAt = time.Now()
	}

	if err := s.db.Create(&tasks).Error; err != nil {
		return nil, fmt.Errorf("create batch tasks: %w", err)
	}

	for i := range tasks {
		go s.executeTask(&tasks[i])
	}

	return tasks, nil
}

func (s *Scheduler) executeTask(task *models.Task) {
	// 获取下载信号量
	s.downloadSem <- struct{}{}
	defer func() { <-s.downloadSem }()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancelFuncs.Store(task.ID, cancel)
	defer func() {
		cancel()
		s.cancelFuncs.Delete(task.ID)
	}()

	now := time.Now()
	task.StartedAt = &now
	s.updateTaskStatus(task, StatusDownloading)

	if s.workerFunc == nil {
		s.log.Error("no worker function set", zap.String("task_id", task.ID))
		task.Error = "no worker function configured"
		s.updateTaskStatus(task, StatusFailed)
		return
	}

	err := s.workerFunc(ctx, task, func(status string, progress TaskProgress) {
		s.notifyProgress(task.ID, status, progress)
		// 更新数据库中的进度
		progressJSON, _ := models.MarshalTo(progress)
		s.db.Model(task).Updates(map[string]interface{}{
			"status":   status,
			"progress": progressJSON,
		})
	})

	if err != nil {
		if ctx.Err() != nil {
			s.updateTaskStatus(task, StatusCancelled)
			return
		}
		task.Error = err.Error()
		task.RetryCount++
		if task.RetryCount <= s.cfg.RetryMax {
			s.log.Warn("task failed, retrying",
				zap.String("task_id", task.ID),
				zap.Int("retry", task.RetryCount),
				zap.Error(err))
			s.updateTaskStatus(task, StatusPending)
			go s.executeTask(task)
			return
		}
		s.log.Error("task failed permanently",
			zap.String("task_id", task.ID),
			zap.Error(err))
		s.updateTaskStatus(task, StatusFailed)
		return
	}

	finished := time.Now()
	task.FinishedAt = &finished
	s.updateTaskStatus(task, StatusDone)
	s.log.Info("task completed", zap.String("task_id", task.ID))
}

func (s *Scheduler) updateTaskStatus(task *models.Task, status string) {
	task.Status = status
	updates := map[string]interface{}{
		"status":      status,
		"error":       task.Error,
		"retry_count": task.RetryCount,
	}
	if task.StartedAt != nil {
		updates["started_at"] = task.StartedAt
	}
	if task.FinishedAt != nil {
		updates["finished_at"] = task.FinishedAt
	}
	s.db.Model(task).Updates(updates)
	s.notifyProgress(task.ID, status, TaskProgress{Stage: status})
}

func (s *Scheduler) notifyProgress(taskID, status string, progress TaskProgress) {
	if s.listener != nil {
		s.listener(taskID, status, progress)
	}
}

// PauseTask 暂停任务
func (s *Scheduler) PauseTask(taskID string) error {
	if cancelFn, ok := s.cancelFuncs.Load(taskID); ok {
		cancelFn.(context.CancelFunc)()
	}
	return s.db.Model(&models.Task{}).Where("id = ?", taskID).Update("status", StatusPaused).Error
}

// ResumeTask 恢复任务
func (s *Scheduler) ResumeTask(taskID string) error {
	var task models.Task
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return fmt.Errorf("find task: %w", err)
	}
	if task.Status != StatusPaused {
		return fmt.Errorf("task is not paused")
	}
	task.Status = StatusPending
	s.db.Save(&task)
	go s.executeTask(&task)
	return nil
}

// CancelTask 取消任务
func (s *Scheduler) CancelTask(taskID string) error {
	if cancelFn, ok := s.cancelFuncs.Load(taskID); ok {
		cancelFn.(context.CancelFunc)()
	}
	return s.db.Model(&models.Task{}).Where("id = ?", taskID).Update("status", StatusCancelled).Error
}

// GetTask 获取任务详情
func (s *Scheduler) GetTask(taskID string) (*models.Task, error) {
	var task models.Task
	if err := s.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, fmt.Errorf("find task: %w", err)
	}
	return &task, nil
}

// ListTasks 获取任务列表
func (s *Scheduler) ListTasks(status string, page, pageSize int) ([]models.Task, int64, error) {
	var tasks []models.Task
	var total int64

	query := s.db.Model(&models.Task{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	query.Count(&total)
	err := query.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&tasks).Error

	return tasks, total, err
}

// GetStats 获取队列统计
func (s *Scheduler) GetStats() map[string]int64 {
	stats := make(map[string]int64)
	statuses := []string{StatusPending, StatusDownloading, StatusProcessing, StatusUploading, StatusDone, StatusFailed, StatusPaused}
	for _, st := range statuses {
		var count int64
		s.db.Model(&models.Task{}).Where("status = ?", st).Count(&count)
		stats[st] = count
	}
	return stats
}

// RecoverTasks 程序重启后恢复未完成的任务
func (s *Scheduler) RecoverTasks() {
	var tasks []models.Task
	s.db.Where("status IN ?", []string{StatusPending, StatusDownloading, StatusFetchMeta}).Find(&tasks)
	for i := range tasks {
		s.log.Info("recovering task", zap.String("task_id", tasks[i].ID))
		tasks[i].Status = StatusPending
		s.db.Save(&tasks[i])
		go s.executeTask(&tasks[i])
	}
}
