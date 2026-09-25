package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/musicflow/musicflow/internal/api/middleware"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/source"
	"github.com/musicflow/musicflow/internal/storage"
	"github.com/musicflow/musicflow/internal/storage/factory"
	"github.com/musicflow/musicflow/internal/storage/yun139"
	"github.com/musicflow/musicflow/internal/telegram"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// List 列出存储目标（普通用户仅可见自己的与管理员共享的）
func (h *StorageHandler) List(c *fiber.Ctx) error {
	var targets []models.StorageTarget
	q := h.db
	if me := middleware.CurrentUser(c); me != nil && me.Role == "user" {
		q = q.Where("owner_id = ? OR owner_id = ''", me.ID)
	}
	q.Order("created_at DESC").Find(&targets)
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

	ownerID := ""
	if me := middleware.CurrentUser(c); me != nil && me.Role == "user" {
		ownerID = me.ID // 普通用户创建的存储归自己
	}

	target := models.StorageTarget{
		Name:    body.Name,
		Type:    body.Type,
		Enabled: body.Enabled,
		OwnerID: ownerID,
		Config:  models.JSON(configJSON),
	}
	if err := h.db.Create(&target).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	// 创建后注册到运行时管理器，无需重启即可使用
	if target.Enabled {
		if backend, err := factory.Build(factory.TargetSpec{
			ID:     target.ID,
			Name:   target.Name,
			Type:   storage.StorageType(target.Type),
			Config: target.Config,
			Log:    h.log,
		}); err == nil {
			h.manager.Register(backend)
		}
	}
	return c.Status(201).JSON(fiber.Map{"data": target})
}

// Update 更新存储目标
func (h *StorageHandler) Update(c *fiber.Ctx) error {
	id := c.Params("id")
	if ok := h.checkOwner(c, id); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": "forbidden"})
	}
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
	// 更新后重建并注册后端
	h.manager.Remove(id)
	if body.Enabled {
		if backend, err := factory.Build(factory.TargetSpec{
			ID:     id,
			Name:   body.Name,
			Type:   storage.StorageType(body.Type),
			Config: configJSON,
			Log:    h.log,
		}); err == nil {
			h.manager.Register(backend)
		}
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

// Delete 删除存储目标
func (h *StorageHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if ok := h.checkOwner(c, id); !ok {
		return c.Status(403).JSON(fiber.Map{"error": true, "message": "forbidden"})
	}
	if err := h.db.Delete(&models.StorageTarget{}, "id = ?", id).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	h.manager.Remove(id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// checkOwner 校验存储目标归属：管理员可操作全部；普通用户仅自己的（共享只读）
func (h *StorageHandler) checkOwner(c *fiber.Ctx, id string) bool {
	me := middleware.CurrentUser(c)
	if me == nil {
		return false
	}
	if me.Role == "admin" {
		return true
	}
	var target models.StorageTarget
	if err := h.db.First(&target, "id = ?", id).Error; err != nil {
		return false
	}
	return target.OwnerID == me.ID
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

// RefreshYun139Tokens 定时续期 139 云盘凭据。
// Authorization 自带过期时间，剩余不足 15 天时用当前 token 直接换新（无需账号密码）；
// 已过期则无法续期，只能提示用户重新登录。
func (h *StorageHandler) RefreshYun139Tokens(ctx context.Context) error {
	var targets []models.StorageTarget
	if err := h.db.Where("type = ? AND enabled = ?", "yun139", true).Find(&targets).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, t := range targets {
		var cfg yun139.Config
		if err := json.Unmarshal(t.Config, &cfg); err != nil || cfg.Token == "" {
			continue
		}
		expire, err := yun139.ParseAuthorizationExpiry(cfg.Token)
		if err != nil {
			h.log.Warn("139 凭据解析失败", zap.String("name", t.Name), zap.Error(err))
			continue
		}
		if !yun139.NeedRefresh(expire, now) {
			continue
		}
		newAuth, newExpire, err := yun139.RefreshAuthorization(ctx, cfg.Token, h.log)
		if err != nil {
			h.log.Warn("139 凭据续期失败，请到「设置 → 存储目标」重新登录该云盘",
				zap.String("name", t.Name), zap.Time("expire_at", expire), zap.Error(err))
			continue
		}
		// 写回数据库，保留其余配置字段
		var cfgMap map[string]interface{}
		if err := json.Unmarshal(t.Config, &cfgMap); err != nil {
			cfgMap = map[string]interface{}{}
		}
		cfgMap["token"] = newAuth
		newJSON, _ := json.Marshal(cfgMap)
		if err := h.db.Model(&models.StorageTarget{}).Where("id = ?", t.ID).
			Update("config", models.JSON(newJSON)).Error; err != nil {
			h.log.Warn("139 凭据写回失败", zap.String("name", t.Name), zap.Error(err))
			continue
		}
		// 热更新运行实例（旧实例持有旧 token）
		h.manager.Remove(t.ID)
		if backend, err := factory.Build(factory.TargetSpec{
			ID:     t.ID,
			Name:   t.Name,
			Type:   storage.StorageType(t.Type),
			Config: newJSON,
			Log:    h.log,
		}); err == nil {
			h.manager.Register(backend)
		}
		h.log.Info("139 凭据已自动续期", zap.String("name", t.Name), zap.Time("expire_at", newExpire))
	}
	return nil
}

// SourceHandler 音乐源配置处理器
type SourceHandler struct {
	db         *gorm.DB
	aggregator *source.Aggregator // 配置变更时热更新聚合器
	mtMgr      *telegram.MTProtoManager
	log        *zap.Logger
}

// NewSourceHandler 创建音乐源处理器
func NewSourceHandler(db *gorm.DB, aggregator *source.Aggregator, mtMgr *telegram.MTProtoManager, log *zap.Logger) *SourceHandler {
	return &SourceHandler{db: db, aggregator: aggregator, mtMgr: mtMgr, log: log}
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
	// 创建后热注册到聚合器，无需重启即可使用
	if body.Enabled {
		if ms, err := h.buildMusicSource(src); err != nil {
			h.log.Warn("register music source failed", zap.String("name", src.Name), zap.Error(err))
		} else {
			h.aggregator.Register(ms)
		}
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

	// 记录旧名称用于热更新移除
	var old models.MusicSourceConfig
	if err := h.db.First(&old, "id = ?", id).Error; err == nil {
		h.aggregator.Remove(old.Name)
	}

	configJSON, _ := json.Marshal(body.Config)

	if err := h.db.Model(&models.MusicSourceConfig{}).Where("id = ?", id).Updates(map[string]interface{}{
		"name":     body.Name,
		"type":     body.Type,
		"priority": body.Priority,
		"enabled":  body.Enabled,
		"config":   models.JSON(configJSON),
	}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": true, "message": err.Error()})
	}
	// 更新后重建并热注册
	if body.Enabled {
		ms, err := h.buildMusicSource(models.MusicSourceConfig{
			Name:     body.Name,
			Type:     body.Type,
			Priority: body.Priority,
			Config:   models.JSON(configJSON),
		})
		if err != nil {
			h.log.Warn("register music source failed", zap.String("name", body.Name), zap.Error(err))
		} else {
			h.aggregator.Register(ms)
		}
	}
	return c.JSON(fiber.Map{"message": "updated"})
}

func (h *SourceHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var src models.MusicSourceConfig
	if err := h.db.First(&src, "id = ?", id).Error; err == nil {
		h.aggregator.Remove(src.Name)
	}
	h.db.Delete(&models.MusicSourceConfig{}, "id = ?", id)
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
	if baseURL == "" && src.Type == "qq" {
		// QQ 音乐源：校验 Cookie 有效性
		var qcfg source.QQConfig
		if err := json.Unmarshal(src.Config, &qcfg); err != nil {
			return c.JSON(fiber.Map{"success": false, "message": "invalid qq config"})
		}
		qs := source.NewQQSource(qcfg, nil, h.log)
		if qs.IsAvailable(c.Context()) {
			return c.JSON(fiber.Map{"success": true, "message": "QQ 音乐 Cookie 有效"})
		}
		return c.JSON(fiber.Map{"success": false, "message": "QQ 音乐 Cookie 无效或无法访问，请重新登录 y.qq.com 获取"})
	}
	if baseURL == "" && src.Type == "migu" {
		// 咪咕音乐源：用最小搜索请求校验接口可用性
		var mcfg source.MiguConfig
		if err := json.Unmarshal(src.Config, &mcfg); err != nil {
			return c.JSON(fiber.Map{"success": false, "message": "invalid migu config"})
		}
		ms := source.NewMiguSource(mcfg, h.log)
		if ms.IsAvailable(c.Context()) {
			return c.JSON(fiber.Map{"success": true, "message": "咪咕音乐接口可用"})
		}
		return c.JSON(fiber.Map{"success": false, "message": "咪咕音乐接口不可用，请检查网络"})
	}
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

// RefreshQQCookies 遍历所有启用的 QQ 源，用 refresh_token 续期 musickey。
// 变化时写回数据库并热更新聚合器中的运行实例（无需重启即可长期有效）。
func (h *SourceHandler) RefreshQQCookies(ctx context.Context) error {
	var sources []models.MusicSourceConfig
	if err := h.db.Where("type = ? AND enabled = ?", "qq", true).Find(&sources).Error; err != nil {
		return err
	}
	for _, src := range sources {
		var cfg source.QQConfig
		if err := json.Unmarshal(src.Config, &cfg); err != nil {
			h.log.Warn("qq refresh: parse config failed", zap.String("id", src.ID), zap.Error(err))
			continue
		}
		if cfg.Name == "" {
			cfg.Name = src.Name
		}
		cfg.Priority = src.Priority
		qs := source.NewQQSource(cfg, &QuotaRecorder{db: h.db}, h.log)
		newCookie, changed, err := qs.RefreshCookie(ctx)
		if err != nil {
			h.log.Warn("qq cookie refresh failed", zap.String("name", cfg.Name), zap.Error(err))
			continue
		}
		if !changed {
			if !containsQQRefreshToken(newCookie) {
				h.log.Warn("qq cookie lacks refresh_token(psrf_qqrefresh_token), cannot auto-renew; re-copy full cookie after login",
					zap.String("name", cfg.Name))
			}
			continue
		}
		// 写回数据库，保留其余配置字段
		var cfgMap map[string]interface{}
		if err := json.Unmarshal(src.Config, &cfgMap); err != nil {
			cfgMap = map[string]interface{}{}
		}
		cfgMap["cookie"] = newCookie
		newJSON, _ := json.Marshal(cfgMap)
		if err := h.db.Model(&models.MusicSourceConfig{}).Where("id = ?", src.ID).Update("config", models.JSON(newJSON)).Error; err != nil {
			h.log.Warn("qq cookie persist failed", zap.String("name", cfg.Name), zap.Error(err))
			continue
		}
		// 热更新运行实例（旧实例含旧 musickey）
		h.aggregator.Remove(src.Name)
		if ms, err := h.buildMusicSource(models.MusicSourceConfig{
			Name:     src.Name,
			Type:     src.Type,
			Priority: src.Priority,
			Enabled:  true,
			Config:   models.JSON(newJSON),
		}); err != nil {
			h.log.Warn("qq cookie hot-reload failed", zap.String("name", src.Name), zap.Error(err))
		} else {
			h.aggregator.Register(ms)
			h.log.Info("qq cookie auto-renewed & hot-reloaded", zap.String("name", src.Name))
		}
	}
	return nil
}

// containsQQRefreshToken 判断 Cookie 是否包含刷新所需凭证（决定能否自动续期）
func containsQQRefreshToken(cookie string) bool {
	if cookie == "" {
		return false
	}
	for _, part := range strings.Split(cookie, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && strings.EqualFold(strings.TrimSpace(kv[0]), "psrf_qqrefresh_token") && kv[1] != "" {
			return true
		}
	}
	return false
}

// GetQQQuota 获取 QQ 音乐源本月下载额度使用情况（本地统计，参考值）
func (h *SourceHandler) GetQQQuota(c *fiber.Ctx) error {
	name := c.Query("name")
	ym := time.Now().Format("200601")
	userID := ""
	if me := middleware.CurrentUser(c); me != nil {
		userID = me.ID
	}
	used, limit := 0, 300
	if name != "" {
		var q models.QQQuota
		if err := h.db.First(&q, "id = ?", name+"_"+userID+"_"+ym).Error; err == nil {
			used, limit = q.Count, q.Limit
		}
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"name":   name,
		"month":  ym,
		"used":   used,
		"limit":  limit,
		"userid": userID,
	}})
}

// buildMusicSource 由数据库配置构建音乐源实例（配置变更热更新用）
func (h *SourceHandler) buildMusicSource(m models.MusicSourceConfig) (source.MusicSource, error) {
	switch m.Type {
	case "custom_api":
		var cfg source.CustomAPIConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewCustomAPISource(cfg, h.log), nil
	case "netease":
		var cfg source.NeteaseConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewNeteaseSource(cfg, h.log), nil
	case "meting":
		var cfg source.MetingConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewMetingSource(cfg, h.log), nil
	case "tgbot":
		var cfg source.TGBotSourceConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewTGBotSource(cfg, h.mtMgr, h.log), nil
	case "qq":
		var cfg source.QQConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewQQSource(cfg, &QuotaRecorder{db: h.db}, h.log), nil
	case "migu":
		var cfg source.MiguConfig
		if err := json.Unmarshal(m.Config, &cfg); err != nil {
			return nil, err
		}
		if cfg.Name == "" {
			cfg.Name = m.Name
		}
		cfg.Priority = m.Priority
		return source.NewMiguSource(cfg, h.log), nil
	default:
		return nil, fmt.Errorf("unknown music source type: %s", m.Type)
	}
}

// QuotaRecorder 实现 source.DownloadRecorder：按「源名+年月」累计下载次数（本地参考统计）
type QuotaRecorder struct{ db *gorm.DB }

// NewQuotaRecorder 创建下载额度计数器
func NewQuotaRecorder(db *gorm.DB) *QuotaRecorder { return &QuotaRecorder{db: db} }

// RecordDownload 当月下载次数 +1（跨月自动新建记录，按用户隔离）
func (r *QuotaRecorder) RecordDownload(ctx context.Context, sourceName, userID string, limit int) error {
	ym := time.Now().Format("200601")
	id := sourceName + "_" + userID + "_" + ym
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{"count": gorm.Expr("count + 1"), "updated_at": time.Now()}),
	}).Create(&models.QQQuota{ID: id, SourceName: sourceName, UserID: userID, YearMonth: ym, Count: 1, Limit: limit}).Error
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
	db      *gorm.DB
	log     *zap.Logger
	manager *storage.Manager
}

// NewLibraryHandler 创建音乐库处理器
func NewLibraryHandler(db *gorm.DB, manager *storage.Manager, log *zap.Logger) *LibraryHandler {
	return &LibraryHandler{db: db, log: log, manager: manager}
}

func (h *LibraryHandler) List(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	size := c.QueryInt("size", 50)
	search := c.Query("q")
	sort := c.Query("sort", "created_at DESC")
	kind := c.Query("kind") // favorite / download / 空=全部

	scope := func(q *gorm.DB) *gorm.DB {
		me := middleware.CurrentUser(c)
		if me != nil && me.Role == "user" {
			// 普通用户：自己的 + 管理员共享的全局下载
			q = q.Where("owner_id = ? OR owner_id = ''", me.ID)
		}
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		return q
	}

	var items []models.Library
	var total int64
	query := scope(h.db.Model(&models.Library{}))
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
	// 普通用户仅能访问自己的或全局共享条目
	if me := middleware.CurrentUser(c); me != nil && me.Role == "user" {
		if item.OwnerID != "" && item.OwnerID != me.ID {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "forbidden"})
		}
	}
	return c.JSON(fiber.Map{"data": item})
}

func (h *LibraryHandler) Delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var item models.Library
	// 删除前先取出记录，拿到各存储的文件路径（记录不存在时也继续删除占位）
	_ = h.db.First(&item, "id = ?", id).Error

	// 权限：管理员可删全局/任意；普通用户只能删自己的
	if me := middleware.CurrentUser(c); me != nil && me.Role == "user" {
		if item.ID == "" || item.OwnerID != me.ID {
			return c.Status(403).JSON(fiber.Map{"error": true, "message": "forbidden"})
		}
	}

	h.deleteRemoteFiles(item)

	h.db.Delete(&models.Library{}, "id = ?", id)
	return c.JSON(fiber.Map{"message": "deleted"})
}

// deleteRemoteFiles 删除音乐库歌曲在各存储端（含本地/网盘）的文件数据及创建的空目录
func (h *LibraryHandler) deleteRemoteFiles(item models.Library) {
	if h.manager == nil || len(item.RemotePaths) == 0 {
		return
	}
	var paths map[string]string
	if err := json.Unmarshal(item.RemotePaths, &paths); err != nil {
		h.log.Warn("parse library remote_paths failed", zap.String("id", item.ID), zap.Error(err))
		return
	}
	for sid, remotePath := range paths {
		if remotePath == "" {
			continue
		}
		backend, ok := h.manager.Get(sid)
		if !ok {
			h.log.Warn("library delete: backend not found", zap.String("storage", sid))
			continue
		}
		h.deleteRemoteEntry(backend, remotePath)
	}
}

// deleteRemoteEntry 删除远端文件、同名 .lrc，并清理目录（含独占封面）
func (h *LibraryHandler) deleteRemoteEntry(backend storage.Backend, remotePath string) {
	ctx := context.Background()
	// 删除音频本体
	if err := backend.Delete(ctx, remotePath); err != nil {
		h.log.Warn("library delete: remove file failed", zap.String("path", remotePath), zap.Error(err))
	}
	// 删除同名 .lrc（若存在）
	base := strings.TrimSuffix(remotePath, path.Ext(remotePath))
	_ = backend.Delete(ctx, base+".lrc")

	dir := path.Dir(remotePath)
	if dir != "" && dir != "." {
		// 若目录只剩封面图（无其它音频/子目录），一并删除封面，便于彻底清空该歌曲目录
		if files, err := backend.ListDir(ctx, dir); err == nil && len(files) > 0 {
			onlyCover := true
			for _, f := range files {
				if f.IsDir || !isCoverImage(f.Name) {
					onlyCover = false
					break
				}
			}
			if onlyCover {
				for _, f := range files {
					_ = backend.Delete(ctx, path.Join(dir, f.Name))
				}
			}
		}
	}
	// 从文件所在目录向上清理空目录（遇到非空/出错即停止，避免误删用户其它文件）
	h.cleanupEmptyDirs(backend, dir)
}

// isCoverImage 判断是否为封面图片名（cover.jpg/jpeg/png/webp）
func isCoverImage(name string) bool {
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "cover.") {
		return false
	}
	return strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") ||
		strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".webp")
}

func (h *LibraryHandler) cleanupEmptyDirs(backend storage.Backend, start string) {
	ctx := context.Background()
	dir := start
	for dir != "" && dir != "/" && dir != "." {
		files, err := backend.ListDir(ctx, dir)
		if err != nil || len(files) > 0 {
			// 目录非空或无法读取则停止（保留，不误删）
			return
		}
		if err := backend.Delete(ctx, dir); err != nil {
			return
		}
		parent := path.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
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
		"name":    "商角",
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
