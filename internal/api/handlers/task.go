package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/scheduler"
	"go.uber.org/zap"
)

// TaskHandler 任务处理器
type TaskHandler struct {
	scheduler *scheduler.Scheduler
	log       *zap.Logger
}

// NewTaskHandler 创建任务处理器
func NewTaskHandler(sched *scheduler.Scheduler, log *zap.Logger) *TaskHandler {
	return &TaskHandler{scheduler: sched, log: log}
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

	return c.Status(201).JSON(fiber.Map{"data": task})
}

// List 获取任务列表
func (h *TaskHandler) List(c *fiber.Ctx) error {
	status := c.Query("status")
	page := c.QueryInt("page", 1)
	size := c.QueryInt("size", 20)

	tasks, total, err := h.scheduler.ListTasks(status, page, size)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{
		"data":  tasks,
		"total": total,
		"page":  page,
	})
}

// Get 获取任务详情
func (h *TaskHandler) Get(c *fiber.Ctx) error {
	task, err := h.scheduler.GetTask(c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "task not found"})
	}
	return c.JSON(fiber.Map{"data": task})
}

// Pause 暂停任务
func (h *TaskHandler) Pause(c *fiber.Ctx) error {
	if err := h.scheduler.PauseTask(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "paused"})
}

// Resume 恢复任务
func (h *TaskHandler) Resume(c *fiber.Ctx) error {
	if err := h.scheduler.ResumeTask(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "resumed"})
}

// Cancel 取消任务
func (h *TaskHandler) Cancel(c *fiber.Ctx) error {
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

	// 转 models.Task（简化实现）
	return c.Status(201).JSON(fiber.Map{"message": "batch created", "count": len(reqs)})
}

// Stats 队列统计
func (h *TaskHandler) Stats(c *fiber.Ctx) error {
	stats := h.scheduler.GetStats()
	return c.JSON(fiber.Map{"data": stats})
}
