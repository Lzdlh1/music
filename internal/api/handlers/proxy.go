package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/proxy"
	"go.uber.org/zap"
)

// ProxyHandler 代理配置处理器
type ProxyHandler struct {
	mgr *proxy.Manager
	log *zap.Logger
}

// NewProxyHandler 创建代理处理器
func NewProxyHandler(mgr *proxy.Manager, log *zap.Logger) *ProxyHandler {
	return &ProxyHandler{mgr: mgr, log: log}
}

// GetConfig 获取当前代理配置
func (h *ProxyHandler) GetConfig(c *fiber.Ctx) error {
	rawURL, enabled := h.mgr.GetConfig()
	return c.JSON(fiber.Map{
		"url":     rawURL,
		"enabled": enabled,
	})
}

// SetConfig 设置代理
func (h *ProxyHandler) SetConfig(c *fiber.Ctx) error {
	var req struct {
		URL     string `json:"url"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	if err := h.mgr.SetProxy(req.URL, req.Enabled); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	h.log.Info("proxy config updated", zap.String("url", req.URL), zap.Bool("enabled", req.Enabled))
	return c.JSON(fiber.Map{"success": true, "message": "代理配置已保存"})
}

// Test 测试代理连通性
func (h *ProxyHandler) Test(c *fiber.Ctx) error {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}

	if req.URL == "" {
		return c.JSON(fiber.Map{"success": false, "message": "代理地址不能为空"})
	}

	if err := h.mgr.TestProxy(req.URL); err != nil {
		return c.JSON(fiber.Map{"success": false, "message": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "代理连接成功"})
}
