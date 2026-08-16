package handlers

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/storage/factory"
	"github.com/musicflow/musicflow/internal/storage/yun139"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Yun139Handler 移动云盘（139）登录处理器
type Yun139Handler struct {
	login   *yun139.LoginClient
	db      *gorm.DB
	manager *storage.Manager
	log     *zap.Logger
}

// NewYun139Handler 创建移动云盘登录处理器
func NewYun139Handler(db *gorm.DB, mgr *storage.Manager, log *zap.Logger) *Yun139Handler {
	return &Yun139Handler{
		login:   yun139.NewLoginClient(log),
		db:      db,
		manager: mgr,
		log:     log,
	}
}

// SendSms 发送短信验证码
func (h *Yun139Handler) SendSms(c *fiber.Ctx) error {
	var body struct {
		Account string `json:"account"`
	}
	if err := c.BodyParser(&body); err != nil || body.Account == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "account is required"})
	}
	random, err := h.login.GetSmsCode(c.Context(), body.Account)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"random": random}})
}

// SmsLogin 短信验证码登录
func (h *Yun139Handler) SmsLogin(c *fiber.Ctx) error {
	var body struct {
		Account   string `json:"account"`
		Code      string `json:"code"`
		StorageID string `json:"storage_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.Account == "" || body.Code == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "account and code are required"})
	}
	result, err := h.login.SmsLogin(c.Context(), body.Account, body.Code)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	if err := h.saveToken(body.StorageID, result); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}

// PasswordLogin 账号密码登录
func (h *Yun139Handler) PasswordLogin(c *fiber.Ctx) error {
	var body struct {
		Account   string `json:"account"`
		Password  string `json:"password"`
		StorageID string `json:"storage_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.Account == "" || body.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "account and password are required"})
	}
	result, err := h.login.PasswordLogin(c.Context(), body.Account, body.Password)
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	if err := h.saveToken(body.StorageID, result); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}

// StartQR 开始扫码登录
func (h *Yun139Handler) StartQR(c *fiber.Ctx) error {
	info, err := h.login.StartQRLogin()
	if err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": info})
}

// PollQR 轮询扫码登录状态
func (h *Yun139Handler) PollQR(c *fiber.Ctx) error {
	var body struct {
		SID       string `json:"sid"`
		StorageID string `json:"storage_id"`
	}
	if err := c.BodyParser(&body); err != nil || body.SID == "" {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "sid is required"})
	}
	result, err := h.login.PollQRLogin(c.Context(), body.SID)
	if err != nil {
		msg := err.Error()
		code := ""
		if le, ok := err.(*yun139.LoginError); ok {
			code = le.Code
			if t := yun139.QRStatusText(le.Code); t != "" {
				msg = t
			}
		}
		return c.JSON(fiber.Map{"success": false, "code": code, "message": msg})
	}
	if err := h.saveToken(body.StorageID, result); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"success": true, "data": result})
}

// saveToken 登录成功后把 authorization 写入指定存储配置（storage_id 为空时跳过）
func (h *Yun139Handler) saveToken(storageID string, result *yun139.LoginResult) error {
	if storageID == "" {
		return nil
	}
	var target models.StorageTarget
	if err := h.db.First(&target, "id = ?", storageID).Error; err != nil {
		return nil // 存储不存在则仅返回登录结果
	}
	if target.Type != "yun139" {
		return nil
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(target.Config, &cfg); err != nil {
		cfg = map[string]interface{}{}
	}
	cfg["token"] = result.Authorization
	if result.Account != "" {
		cfg["account"] = result.Account
	}
	if result.UserDomainID != "" {
		cfg["user_domain_id"] = result.UserDomainID
	}
	if result.PersonalHost != "" {
		cfg["personal_host"] = result.PersonalHost
	}
	configJSON, _ := json.Marshal(cfg)

	if err := h.db.Model(&models.StorageTarget{}).Where("id = ?", storageID).Update("config", models.JSON(configJSON)).Error; err != nil {
		return err
	}
	// 重建并注册后端，无需重启即可使用
	backend, err := factory.Build(factory.TargetSpec{
		ID:     target.ID,
		Name:   target.Name,
		Type:   storage.StorageYun139,
		Config: configJSON,
		Log:    h.log,
	})
	if err == nil && h.manager != nil {
		h.manager.Remove(storageID)
		h.manager.Register(backend)
	}
	return nil
}
