package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/config"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	cfg *config.AuthConfig
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(cfg *config.AuthConfig) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

// Login 登录
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	if !middleware.CheckCredentials(req.Username, req.Password, h.cfg.Username, h.cfg.Password) {
		return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid credentials"})
	}

	token, err := middleware.GenerateToken(req.Username, h.cfg.JWTSecret)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": "generate token failed"})
	}

	return c.JSON(fiber.Map{
		"token":    token,
		"username": req.Username,
	})
}

// AuthStatus 获取当前认证状态
func (h *AuthHandler) AuthStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"auth_enabled": h.cfg.Enabled,
	})
}
