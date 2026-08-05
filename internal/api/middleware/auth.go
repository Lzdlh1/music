package middleware

import (
	"crypto/subtle"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/musicflow/musicflow/internal/config"
)

// JWTClaims JWT 声明
type JWTClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// AuthMiddleware 认证中间件
func AuthMiddleware(cfg *config.AuthConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !cfg.Enabled {
			return c.Next()
		}

		// WebSocket 升级请求跳过（通过 query param token 验证）
		if c.Get("Upgrade") == "websocket" {
			token := c.Query("token")
			if token != "" {
				if _, err := ValidateToken(token, cfg.JWTSecret); err == nil {
					return c.Next()
				}
			}
		}

		// 获取 Authorization header
		auth := c.Get("Authorization")
		if auth == "" {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "unauthorized"})
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if token == auth {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid token format"})
		}

		claims, err := ValidateToken(token, cfg.JWTSecret)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": true, "message": "invalid or expired token"})
		}

		c.Locals("username", claims.Username)
		return c.Next()
	}
}

// GenerateToken 生成 JWT Token
func GenerateToken(username, secret string) (string, error) {
	claims := JWTClaims{
		Username: username,
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

// CheckCredentials 验证用户名密码（常量时间比较）
func CheckCredentials(inputUser, inputPass, cfgUser, cfgPass string) bool {
	userMatch := subtle.ConstantTimeCompare([]byte(inputUser), []byte(cfgUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(inputPass), []byte(cfgPass)) == 1
	return userMatch && passMatch
}
