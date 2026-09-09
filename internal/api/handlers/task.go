package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/scheduler"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	scheduler *scheduler.Scheduler
	db        *gorm.DB
	log       *zap.Logger
}

// NewTaskHandler 创建任务处理器
func NewTaskHandler(sched *scheduler.Scheduler, db *gorm.DB, log *zap.Logger) *TaskHandler {
	return &TaskHandler{scheduler: sched, db: db, log: log}
}

// CreateTaskRequest 创建任务请求
type CreateTaskRequest struct {
	TrackInfo      map[string]interface{} `json:"track_info"`
	SelectedSource map[string]interface{} `json:"selected_source"`
	UploadTargets  []string               `json:"upload_targets"`
	UploadDir      string                 `json:"upload_dir"`
}

// Create 创建单曲下载任务
func (h *TaskHandler) Create(c *fiber.Ctx) error {
	var req CreateTaskRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request body"})
	}

	trackJSON, _ := json.Marshal(req.TrackInfo)
	sourceJSON, _ := json.Marshal(req.SelectedSource)
	targetsJSON, _ := json.Marshal(req.UploadTargets)

	task, err := h.scheduler.CreateTask(trackJSON, sourceJSON, targetsJSON, req.UploadDir)
	if err != nil {
		h.log.Error("create task failed", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	// 绑定任务归属（普通用户任务专属）
	me := middleware.CurrentUser(c)
	ownerID := ""
	uploadDir := req.UploadDir
	if me != nil && me.Role == "user" {
		ownerID = me.ID
		if uploadDir == "" {
			uploadDir = me.DownloadDir // 用户默认下载目录
		}
	}
	h.db.Model(&models.Task{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"owner_id":   ownerID,
		"upload_dir": uploadDir,
	})

	return c.Status(201).JSON(fiber.Map{"data": task})
}

// List 获取任务列表（普通用户仅自己的）
func (h *TaskHandler) List(c *fiber.Ctx) error {
	status := c.Query("status")
	page := c.QueryInt("page", 1)
	size := c.QueryInt("size", 20)

	query := h.db.Model(&models.Task{})
	me := middleware.CurrentUser(c)
	if me != nil && me.Role == "user" {
		query = query.Where("owner_id = ?", me.ID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	query.Count(&total)

	var tasks []models.Task
	query.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&tasks)
	return c.JSON(fiber.Map{"data": tasks, "total": total, "page": page})
}

// Get 获取任务详情
func (h *TaskHandler) Get(c *fiber.Ctx) error {
	if ok, msg := h.checkOwner(c); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": msg})
	}
	task, err := h.scheduler.GetTask(c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "task not found"})
	}
	return c.JSON(fiber.Map{"data": task})
}

// checkOwner 任务归属校验（在访问各任务接口前调用）
func (h *TaskHandler) checkOwner(c *fiber.Ctx) (bool, string) {
	me := middleware.CurrentUser(c)
	if me == nil {
		return false, "unauthorized"
	}
	if me.Role == "admin" {
		return true, ""
	}
	var task models.Task
	if err := h.db.First(&task, "id = ?", c.Params("id")).Error; err != nil {
		return false, "task not found"
	}
	if task.OwnerID != me.ID {
		return false, "forbidden"
	}
	return true, ""
}

// Pause 暂停任务
func (h *TaskHandler) Pause(c *fiber.Ctx) error {
	if ok, msg := h.checkOwner(c); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": msg})
	}
	if err := h.scheduler.PauseTask(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "paused"})
}

// Resume 恢复任务
func (h *TaskHandler) Resume(c *fiber.Ctx) error {
	if ok, msg := h.checkOwner(c); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": msg})
	}
	if err := h.scheduler.ResumeTask(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "resumed"})
}

// Cancel 取消任务
func (h *TaskHandler) Cancel(c *fiber.Ctx) error {
	if ok, msg := h.checkOwner(c); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": msg})
	}
	if err := h.scheduler.CancelTask(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "cancelled"})
}

// BatchCreate 批量创建任务
func (h *TaskHandler) BatchCreate(c *fiber.Ctx) error {
	var reqs []CreateTaskRequest
	if err := c.BodyParser(&reqs); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request body"})
	}
	me := middleware.CurrentUser(c)
	count := 0
	for i := range reqs {
		req := reqs[i]
		trackJSON, _ := json.Marshal(req.TrackInfo)
		sourceJSON, _ := json.Marshal(req.SelectedSource)
		targetsJSON, _ := json.Marshal(req.UploadTargets)
		task, err := h.scheduler.CreateTask(trackJSON, sourceJSON, targetsJSON, req.UploadDir)
		if err != nil {
			continue
		}
		ownerID := ""
		if me != nil && me.Role == "user" {
			ownerID = me.ID
		}
		h.db.Model(&models.Task{}).Where("id = ?", task.ID).Update("owner_id", ownerID)
		count++
	}
	return c.Status(201).JSON(fiber.Map{"message": "batch created", "count": count})
}

// Stats 队列统计
func (h *TaskHandler) Stats(c *fiber.Ctx) error {
	stats := h.scheduler.GetStats()
	return c.JSON(fiber.Map{"data": stats})
}
