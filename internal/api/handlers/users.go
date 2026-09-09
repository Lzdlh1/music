package handlers

import (
	"crypto/rand"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/db/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"go.uber.org/zap"
)

// UsersHandler 用户与邀请码管理
type UsersHandler struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewUsersHandler 创建用户处理器
func NewUsersHandler(db *gorm.DB, log *zap.Logger) *UsersHandler {
	return &UsersHandler{db: db, log: log}
}

// List 用户列表（含本月 QQ 下载量），仅管理员
func (h *UsersHandler) List(c *fiber.Ctx) error {
	var users []models.User
	h.db.Order("role ASC, created_at ASC").Find(&users)
	ym := time.Now().Format("200601")

	type item struct {
		ID        string `json:"id"`
		Username  string `json:"username"`
		Role      string `json:"role"`
		Disabled  bool   `json:"disabled"`
		QQLimit   int    `json:"qq_limit"`
		QQUsed    int    `json:"qq_used"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]item, 0, len(users))
	for _, u := range users {
		q := 0
		var quota models.QQQuota
		id := "qq_" + u.ID + "_" + ym
		if err := h.db.First(&quota, "id = ?", id).Error; err == nil {
			q = quota.Count
		}
		out = append(out, item{u.ID, u.Username, u.Role, u.Disabled, u.QQLimit, q, u.CreatedAt})
	}
	return c.JSON(fiber.Map{"data": out})
}

// Update 更新用户（角色/停用/配额/默认存储/下载目录），仅管理员
func (h *UsersHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	var body struct {
		Role           string `json:"role"`
		Disabled       *bool  `json:"disabled"`
		QQLimit        *int   `json:"qq_limit"`
		DefaultStorage string `json:"default_storage"`
		DownloadDir    string `json:"download_dir"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	updates := map[string]interface{}{}
	if body.Role == "admin" || body.Role == "user" {
		updates["role"] = body.Role
	}
	if body.Disabled != nil {
		updates["disabled"] = *body.Disabled
	}
	if body.QQLimit != nil && *body.QQLimit >= 0 {
		updates["qq_limit"] = *body.QQLimit
	}
	if body.DefaultStorage != "" {
		updates["default_storage"] = body.DefaultStorage
	}
	if body.DownloadDir != "" {
		updates["download_dir"] = body.DownloadDir
	}
	if len(updates) == 0 {
		return c.JSON(fiber.Map{"message": "nothing to update"})
	}
	// 不允许管理员停用/降级自己
	me := middleware.CurrentUser(c)
	if me != nil && me.ID == id {
		if v, ok := updates["disabled"]; ok && v == true {
			return c.Status(400).JSON(fiber.Map{"error": true, "message": "不能停用自己"})
		}
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// CreateInvite 生成一次性邀请码，仅管理员
func (h *UsersHandler) CreateInvite(c *fiber.Ctx) error {
	me := middleware.CurrentUser(c)
	key := models.InviteKey{Code: randomInviteCode(), CreatedBy: me.ID}
	if err := h.db.Create(&key).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"data": key})
}

// ListInvites 邀请码列表，仅管理员
func (h *UsersHandler) ListInvites(c *fiber.Ctx) error {
	var keys []models.InviteKey
	h.db.Order("created_at DESC").Find(&keys)
	usernames := map[string]string{}
	var users []models.User
	h.db.Find(&users)
	for _, u := range users {
		usernames[u.ID] = u.Username
	}
	type item struct {
		ID        string     `json:"id"`
		Code      string     `json:"code"`
		CreatedAt time.Time  `json:"created_at"`
		UsedBy    string     `json:"used_by"`
		Username  string     `json:"username,omitempty"`
		UsedAt    *time.Time `json:"used_at,omitempty"`
	}
	out := make([]item, 0, len(keys))
	for _, k := range keys {
		out = append(out, item{k.ID, k.Code, k.CreatedAt, k.UsedBy, usernames[k.UsedBy], k.UsedAt})
	}
	return c.JSON(fiber.Map{"data": out})
}

// DeleteInvite 撤销邀请码，仅管理员
func (h *UsersHandler) DeleteInvite(c *fiber.Ctx) error {
	id := c.Params("id")
	h.db.Delete(&models.InviteKey{}, "id = ?", id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// Me 当前用户信息
func (h *UsersHandler) Me(c *fiber.Ctx) error {
	u := middleware.CurrentUser(c)
	return c.JSON(fiber.Map{"data": u})
}

// UpdateMe 修改自己的下载目录/默认存储/密码
func (h *UsersHandler) UpdateMe(c *fiber.Ctx) error {
	u := middleware.CurrentUser(c)
	var body struct {
		DownloadDir    string `json:"download_dir"`
		DefaultStorage string `json:"default_storage"`
		Password       string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": true, "message": "invalid request"})
	}
	updates := map[string]interface{}{}
	updates["download_dir"] = body.DownloadDir
	updates["default_storage"] = body.DefaultStorage
	if body.Password != "" {
		if len(body.Password) < 6 {
			return c.Status(400).JSON(fiber.Map{"error": true, "message": "密码至少6位"})
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": true, "message": "hash password failed"})
		}
		updates["password_hash"] = string(hash)
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", u.ID).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// MyQuota 我的 QQ 源本月下载统计
func (h *UsersHandler) MyQuota(c *fiber.Ctx) error {
	u := middleware.CurrentUser(c)
	ym := time.Now().Format("200601")
	type q struct {
		SourceName string `json:"source_name"`
		Used       int    `json:"used"`
		Limit      int    `json:"limit"`
	}
	var rows []models.QQQuota
	h.db.Where("user_id = ? AND year_month = ?", u.ID, ym).Find(&rows)
	out := make([]q, 0, len(rows))
	for _, r := range rows {
		out = append(out, q{r.SourceName, r.Count, r.Limit})
	}
	// 补充用户 QQLimit 汇总（按 QQ 源）
	usedQQ := 0
	for _, r := range rows {
		if strings.Contains(r.SourceName, "QQ") || strings.Contains(r.SourceName, "qq") {
			usedQQ += r.Count
		}
	}
	limit := u.QQLimit
	if limit <= 0 {
		limit = 300
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"qq": fiber.Map{"used": usedQQ, "limit": limit},
		"items": out,
	}})
}

// randomInviteCode 生成 12 位大写邀请码
func randomInviteCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 12)
	rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}