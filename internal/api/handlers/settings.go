package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// StorageHandler 存储目标处理器
type StorageHandler struct {
	manager *storage.Manager
	db      *gorm.DB
	log     *zap.Logger
}

// NewStorageHandler 创建存储处理器
func NewStorageHandler(mgr *storage.Manager, db *gorm.DB, log *zap.Logger) *StorageHandler {
	return &StorageHandler{manager: mgr, db: db, log: log}
}

// List 列出存储目标
func (h *StorageHandler) List(c *fiber.Ctx) error {
	var targets []models.StorageTarget
	h.db.Find(&targets)
	return c.JSON(fiber.Map{"data": targets})
}

// Create 创建存储目标
func (h *StorageHandler) Create(c *fiber.Ctx) error {
	var body struct {
		Name    string      `json:"name"`
		Type    string      `json:"type"`
		Enabled bool        `json:"enabled"`
		Config  interface{} `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	configJSON, err := json.Marshal(body.Config)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid config"})
	}

	target := models.StorageTarget{
		Name:    body.Name,
		Type:    body.Type,
		Enabled: body.Enabled,
		Config:  models.JSON(configJSON),
	}
	if err := h.db.Create(&target).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": target})
}

// Update 更新存储目标
func (h *StorageHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var body struct {
		Name    string      `json:"name"`
		Type    string      `json:"type"`
		Enabled bool        `json:"enabled"`
		Config  interface{} `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	configJSON, _ := json.Marshal(body.Config)

	if err := h.db.Model(&models.StorageTarget{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":    body.Name,
		"type":    body.Type,
		"enabled": body.Enabled,
		"config":  models.JSON(configJSON),
	}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// Delete 删除存储目标
func (h *StorageHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.db.Delete(&models.StorageTarget{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	h.manager.Remove(id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// Test 测试存储连接
func (h *StorageHandler) Test(c *fiber.Ctx) error {
	id := c.Params("id")
	backend, ok := h.manager.Get(id)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "storage not found"})
	}
	if err := backend.Test(c.Context()); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true})
}

// Browse 浏览远程目录
func (h *StorageHandler) Browse(c *fiber.Ctx) error {
	id := c.Params("id")
	dir := c.Query("path", "/")
	backend, ok := h.manager.Get(id)
	if !ok {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "storage not found"})
	}
	files, err := backend.ListDir(c.Context(), dir)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"data": files})
}

// SourceHandler 音乐源配置处理器
type SourceHandler struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewSourceHandler 创建音乐源处理器
func NewSourceHandler(db *gorm.DB, log *zap.Logger) *SourceHandler {
	return &SourceHandler{db: db, log: log}
}

func (h *SourceHandler) List(c *fiber.Ctx) error {
	var sources []models.MusicSourceConfig
	h.db.Order("priority DESC").Find(&sources)
	return c.JSON(fiber.Map{"data": sources})
}

func (h *SourceHandler) Create(c *fiber.Ctx) error {
	var body struct {
		Name     string      `json:"name"`
		Type     string      `json:"type"`
		Priority int         `json:"priority"`
		Enabled  bool        `json:"enabled"`
		Config   interface{} `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	configJSON, err := json.Marshal(body.Config)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid config"})
	}

	src := models.MusicSourceConfig{
		Name:     body.Name,
		Type:     body.Type,
		Priority: body.Priority,
		Enabled:  body.Enabled,
		Config:   models.JSON(configJSON),
	}
	if err := h.db.Create(&src).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": src})
}

func (h *SourceHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var body struct {
		Name     string      `json:"name"`
		Type     string      `json:"type"`
		Priority int         `json:"priority"`
		Enabled  bool        `json:"enabled"`
		Config   interface{} `json:"config"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	configJSON, _ := json.Marshal(body.Config)

	h.db.Model(&models.MusicSourceConfig{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":     body.Name,
		"type":     body.Type,
		"priority": body.Priority,
		"enabled":  body.Enabled,
		"config":   models.JSON(configJSON),
	})
	return c.JSON(fiber.Map{"message": "updated"})
}

func (h *SourceHandler) Delete(c *fiber.Ctx) error {
	h.db.Delete(&models.MusicSourceConfig{}, "id = ?", c.Params("id"))
	return c.JSON(fiber.Map{"message": "deleted"})
}

func (h *SourceHandler) Test(c *fiber.Ctx) error {
	id := c.Params("id")
	var src models.MusicSourceConfig
	if err := h.db.First(&src, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "source not found"})
	}

	// 从配置中提取 base_url 进行连通性测试
	var cfg map[string]interface{}
	if err := json.Unmarshal(src.Config, &cfg); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": "invalid config JSON"})
	}

	baseURL, _ := cfg["base_url"].(string)
	if baseURL == "" {
		return c.JSON(fiber.Map{"success": false, "message": "no base_url in config"})
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": fmt.Sprintf("connection failed: %v", err)})
	}
	resp.Body.Close()

	return c.JSON(fiber.Map{"success": true, "message": fmt.Sprintf("connected, status: %d", resp.StatusCode)})
}

// SettingsHandler 系统设置处理器
type SettingsHandler struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewSettingsHandler 创建设置处理器
func NewSettingsHandler(db *gorm.DB, log *zap.Logger) *SettingsHandler {
	return &SettingsHandler{db: db, log: log}
}

func (h *SettingsHandler) Get(c *fiber.Ctx) error {
	var settings []models.Setting
	h.db.Find(&settings)
	result := make(map[string]json.RawMessage)
	for _, s := range settings {
		result[s.Key] = json.RawMessage(s.Value)
	}
	return c.JSON(fiber.Map{"data": result})
}

func (h *SettingsHandler) Update(c *fiber.Ctx) error {
	var updates map[string]json.RawMessage
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	for key, val := range updates {
		h.db.Where(models.Setting{Key: key}).Assign(models.Setting{Value: models.JSON(val)}).FirstOrCreate(&models.Setting{})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

func (h *SettingsHandler) GetDownload(c *fiber.Ctx) error {
	var s models.Setting
	h.db.First(&s, "key = ?", "download")
	return c.JSON(fiber.Map{"data": json.RawMessage(s.Value)})
}

func (h *SettingsHandler) UpdateDownload(c *fiber.Ctx) error {
	body := c.Body()
	h.db.Where(models.Setting{Key: "download"}).Assign(models.Setting{Value: models.JSON(body)}).FirstOrCreate(&models.Setting{})
	return c.JSON(fiber.Map{"message": "updated"})
}

func (h *SettingsHandler) GetNaming(c *fiber.Ctx) error {
	var s models.Setting
	h.db.First(&s, "key = ?", "naming")
	return c.JSON(fiber.Map{"data": json.RawMessage(s.Value)})
}

func (h *SettingsHandler) UpdateNaming(c *fiber.Ctx) error {
	body := c.Body()
	h.db.Where(models.Setting{Key: "naming"}).Assign(models.Setting{Value: models.JSON(body)}).FirstOrCreate(&models.Setting{})
	return c.JSON(fiber.Map{"message": "updated"})
}

// LibraryHandler 音乐库处理器
type LibraryHandler struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewLibraryHandler 创建音乐库处理器
func NewLibraryHandler(db *gorm.DB, log *zap.Logger) *LibraryHandler {
	return &LibraryHandler{db: db, log: log}
}

func (h *LibraryHandler) List(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	size := c.QueryInt("size", 50)
	search := c.Query("q")
	sort := c.Query("sort", "created_at DESC")

	var items []models.Library
	var total int64

	query := h.db.Model(&models.Library{})
	if search != "" {
		query = query.Where("title LIKE ? OR artist LIKE ? OR album LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}
	query.Count(&total)
	query.Order(sort).Offset((page - 1) * size).Limit(size).Find(&items)

	return c.JSON(fiber.Map{"data": items, "total": total, "page": page})
}

func (h *LibraryHandler) Get(c *fiber.Ctx) error {
	var item models.Library
	if err := h.db.First(&item, "id = ?", c.Params("id")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "not found"})
	}
	return c.JSON(fiber.Map{"data": item})
}

func (h *LibraryHandler) Delete(c *fiber.Ctx) error {
	h.db.Delete(&models.Library{}, "id = ?", c.Params("id"))
	return c.JSON(fiber.Map{"message": "deleted"})
}

// SystemHandler 系统信息处理器
type SystemHandler struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewSystemHandler 创建系统处理器  
func NewSystemHandler(db *gorm.DB, log *zap.Logger) *SystemHandler {
	return &SystemHandler{db: db, log: log}
}

func (h *SystemHandler) Info(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"version": "1.0.0",
		"name":    "MusicFlow",
	})
}

func (h *SystemHandler) Logs(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": []string{}, "message": "log endpoint ready"})
}

func (h *SystemHandler) StorageUsage(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"temp_dir_size": 0})
}

func (h *SystemHandler) Cleanup(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"message": "cleanup done"})
}
