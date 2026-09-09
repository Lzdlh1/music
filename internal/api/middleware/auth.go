package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/musicflow/musicflow/internal/db/models"
	"gorm.io/gorm"
)

// JWTClaims JWT 声明
type JWTClaims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthMiddleware 认证中间件（数据库驱动、始终启用）：
// 校验 JWT 并加载用户到上下文；无有效登录态返回 401。
func AuthMiddleware(db *gorm.DB, secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// WebSocket 升级请求跳过（通过 query param token 验证）
		if c.Get("Upgrade") == "websocket" {
			token := c.Query("token")
			if token != "" {
				if claims, err := ValidateToken(token, secret); err == nil {
					_ = claims
					if err := c.Next(); err != nil {
						return err
					}
					return nil
				}
			}
		}

		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "unauthorized"})
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid token format"})
		}
		claims, err := ValidateToken(token, secret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid or expired token"})
		}

		var user models.User
		if err := db.First(&user, "id = ?", claims.UserID).Error; err != nil {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "user not found"})
		}
		if user.Disabled {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "account disabled"})
		}
		c.Locals("user", &user)
		return c.Next()
	}
}

// AuthQueryToken 通过 query 参数 token 鉴权（供 <audio> 无法携带 Header 的流媒体接口使用）
func AuthQueryToken(db *gorm.DB, secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Query("token")
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "unauthorized"})
		}
		claims, err := ValidateToken(token, secret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid or expired token"})
		}
		var user models.User
		if err := db.First(&user, "id = ?", claims.UserID).Error; err != nil || user.Disabled {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "user not found or disabled"})
		}
		c.Locals("user", &user)
		return c.Next()
	}
}

// StorageWriteGuard 存储目标写权限：管理员可写全部；普通用户仅可写自己创建的存储
func StorageWriteGuard(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "unauthorized"})
		}
		if u.Role == "admin" {
			return c.Next()
		}
		var target models.StorageTarget
		if err := db.First(&target, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": true, "message": "storage not found"})
		}
		if target.OwnerID != u.ID {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "共享存储只读，无法写入"})
		}
		return c.Next()
	}
}

// StorageReadGuard 存储目标读权限：管理员/自己的/共享存储可读
func StorageReadGuard(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "unauthorized"})
		}
		if u.Role == "admin" {
			return c.Next()
		}
		var target models.StorageTarget
		if err := db.First(&target, "id = ?", c.Params("id")).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": true, "message": "storage not found"})
		}
		if target.OwnerID != "" && target.OwnerID != u.ID {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "forbidden"})
		}
		return c.Next()
	}
}

// RequireAdmin 管理员权限拦截（需在 AuthMiddleware 之后使用）
func RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals("user").(*models.User)
		if !ok || u.Role != "admin" {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "admin only"})
		}
		return c.Next()
	}
}

// CurrentUser 获取当前登录用户（AuthMiddleware 之后）
func CurrentUser(c *fiber.Ctx) *models.User {
	u, _ := c.Locals("user").(*models.User)
	return u
}

// GenerateToken 生成 JWT Token
func GenerateToken(userID, username, role, secret string) (string, error) {
	claims := JWTClaims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "musicflow",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken 验证 JWT Token
func ValidateToken(tokenStr, secret string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.ErrUnauthorized
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fiber.ErrUnauthorized
	}
	return claims, nil
}