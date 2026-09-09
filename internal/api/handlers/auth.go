package handlers

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/db/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AuthHandler 认证处理器（数据库驱动多用户）
type AuthHandler struct {
	db     *gorm.DB
	secret string
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(db *gorm.DB, secret string) *AuthHandler {
	return &AuthHandler{db: db, secret: secret}
}

// AuthStatus 获取当前认证状态：need_setup 表示系统尚未创建管理员，需先初始化
func (h *AuthHandler) AuthStatus(c *fiber.Ctx) error {
	var count int64
	h.db.Model(&models.User{}).Count(&count)
	return c.JSON(fiber.Map{
		"auth_enabled": true,
		"need_setup":   count == 0,
	})
}

// Setup 首次使用初始化：创建超级管理员（仅 users 表为空时可调用）
func (h *AuthHandler) Setup(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 2 || len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "用户名至少2位，密码至少6位"})
	}

	var count int64
	h.db.Model(&models.User{}).Count(&count)
	if count > 0 {
		return c.Status(409).JSON(fiber.Map{"error": true, "message": "系统已初始化"})
	}
	var dup int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&dup)
	if dup > 0 {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "用户名已存在"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "hash password failed"})
	}
	user := models.User{Username: req.Username, PasswordHash: string(hash), Role: "admin", QQLimit: 300}
	if err := h.db.Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "create admin failed"})
	}
	return h.issueToken(c, &user)
}

// Login 登录（数据库账号 / bcrypt）
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	var user models.User
	if err := h.db.First(&user, "username = ?", strings.TrimSpace(req.Username)).Error; err != nil {
		return c.Status(401).JSON(fiber.Map{"error": true, "message": "用户名或密码错误"})
	}
	if user.Disabled {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": "账号已被停用"})
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return c.Status(401).JSON(fiber.Map{"error": true, "message": "用户名或密码错误"})
	}
	return h.issueToken(c, &user)
}

// Register 使用一次性邀请码注册普通用户
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req struct {
		Code     string `json:"code"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	req.Code = strings.TrimSpace(req.Code)
	req.Username = strings.TrimSpace(req.Username)
	if req.Code == "" || len(req.Username) < 2 || len(req.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "邀请码必填，用户名至少2位，密码至少6位"})
	}

	var key models.InviteKey
	if err := h.db.First(&key, "code = ?", req.Code).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "邀请码无效"})
	}
	if key.UsedAt != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "邀请码已被使用"})
	}
	var dup int64
	h.db.Model(&models.User{}).Where("username = ?", req.Username).Count(&dup)
	if dup > 0 {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "用户名已存在"})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "hash password failed"})
	}
	user := models.User{Username: req.Username, PasswordHash: string(hash), Role: "user", QQLimit: 300}
	if err := h.db.Create(&user).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "create user failed"})
	}
	// 标记邀请码已使用（一次性）
	now := time.Now()
	h.db.Model(&models.InviteKey{}).Where("id = ?", key.ID).
		Updates(map[string]interface{}{"used_by": user.ID, "used_at": &now})
	return h.issueToken(c, &user)
}

// issueToken 签发 JWT 并返回用户信息
func (h *AuthHandler) issueToken(c *fiber.Ctx, user *models.User) error {
	token, err := middleware.GenerateToken(user.ID, user.Username, user.Role, h.secret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "generate token failed"})
	}
	return c.JSON(fiber.Map{
		"token":    token,
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}