package handlers

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TelegramHandler Telegram 配置处理器
type TelegramHandler struct {
	bot   *telegram.Bot
	mtMgr *telegram.MTProtoManager
	db    *gorm.DB
	log   *zap.Logger
}

// NewTelegramHandler 创建 Telegram 处理器
func NewTelegramHandler(bot *telegram.Bot, mtMgr *telegram.MTProtoManager, db *gorm.DB, log *zap.Logger) *TelegramHandler {
	return &TelegramHandler{bot: bot, mtMgr: mtMgr, db: db, log: log}
}

// ListBots 列出所有 Bot
func (h *TelegramHandler) ListBots(c *fiber.Ctx) error {
	var bots []models.TGBot
	h.db.Order("priority DESC").Find(&bots)
	return c.JSON(fiber.Map{"data": bots})
}

// CreateBot 创建 Bot 配置
func (h *TelegramHandler) CreateBot(c *fiber.Ctx) error {
	var req struct {
		Name     string          `json:"name"`
		Username string          `json:"username"`
		Config   json.RawMessage `json:"config"`
		Priority int             `json:"priority"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	bot := models.TGBot{
		ID:       uuid.New().String(),
		Name:     req.Name,
		Username: req.Username,
		Config:   models.JSON(req.Config),
		Priority: req.Priority,
		Enabled:  true,
	}

	if err := h.db.Create(&bot).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": bot})
}

// UpdateBot 更新 Bot 配置
func (h *TelegramHandler) UpdateBot(c *fiber.Ctx) error {
	id := c.Params("id")
	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	if err := h.db.Model(&models.TGBot{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// DeleteBot 删除 Bot 配置
func (h *TelegramHandler) DeleteBot(c *fiber.Ctx) error {
	h.db.Delete(&models.TGBot{}, "id = ?", c.Params("id"))
	return c.JSON(fiber.Map{"message": "deleted"})
}

// TestBot 测试 Bot Token
func (h *TelegramHandler) TestBot(c *fiber.Ctx) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	info, err := h.bot.TestBot(req.Token)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	// 更新测试时间（如果是已保存的 Bot）
	id := c.Params("id")
	if id != "" {
		now := time.Now()
		h.db.Model(&models.TGBot{}).Where("id = ?", id).Updates(map[string]interface{}{
			"last_tested":  &now,
			"success_rate": 1.0,
			"username":     info.Username,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    info,
	})
}

// ListAccounts 列出所有 Telegram 账号
func (h *TelegramHandler) ListAccounts(c *fiber.Ctx) error {
	var accounts []models.TGAccount
	h.db.Find(&accounts)
	return c.JSON(fiber.Map{"data": accounts})
}

// CreateAccount 创建 Telegram 账号
func (h *TelegramHandler) CreateAccount(c *fiber.Ctx) error {
	var account models.TGAccount
	if err := c.BodyParser(&account); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	account.ID = uuid.New().String()
	account.Status = "pending"
	if err := h.db.Create(&account).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": account})
}

// DeleteAccount 删除 Telegram 账号
func (h *TelegramHandler) DeleteAccount(c *fiber.Ctx) error {
	id := c.Params("id")
	h.mtMgr.StopClient(id)
	h.db.Delete(&models.TGAccount{}, "id = ?", id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// StartAccount 登录 Telegram 账号（发送验证码）
func (h *TelegramHandler) StartAccount(c *fiber.Ctx) error {
	id := c.Params("id")
	var account models.TGAccount
	if err := h.db.First(&account, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": true, "message": "account not found"})
	}

	// 启动客户端并触发验证码流
	inst, err := h.mtMgr.StartClient(&account)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	inst.Mu.Lock()
	status := inst.Status
	inst.Mu.Unlock()

	return c.JSON(fiber.Map{"message": "started", "status": status})
}

// SubmitCode 提交验证码
func (h *TelegramHandler) SubmitCode(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	if err := h.mtMgr.SubmitCode(id, req.Code); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "code submitted"})
}

// SubmitPassword 提交两步验证密码
func (h *TelegramHandler) SubmitPassword(c *fiber.Ctx) error {
	id := c.Params("id")
	var req struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	if err := h.mtMgr.SubmitPassword(id, req.Password); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "password submitted"})
}
