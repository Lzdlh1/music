package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/musicflow/musicflow/internal/db/models"
	"github.com/musicflow/musicflow/internal/proxy"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// TerminalAuth 实现终端认证流（这里用作被动验证码接收器）
type TerminalAuth struct {
	phone    string
	codeChan chan string
	pwdChan  chan string
}

func (a *TerminalAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a *TerminalAuth) Password(_ context.Context) (string, error) {
	select {
	case pwd := <-a.pwdChan:
		return pwd, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("password timeout")
	}
}

func (a *TerminalAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a *TerminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	select {
	case code := <-a.codeChan:
		return code, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("code timeout")
	}
}

func (a *TerminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("signup not supported")
}

// MTProtoManager 管理 MTProto 客户端会话
type MTProtoManager struct {
	db       *gorm.DB
	log      *zap.Logger
	proxyMgr *proxy.Manager
	clients  map[string]*ClientInstance
	mu       sync.RWMutex
}

// ClientInstance 单个客户端实例
type ClientInstance struct {
	Account  *models.TGAccount
	Client   *telegram.Client
	AuthFlow *TerminalAuth
	Cancel   context.CancelFunc
	Status   string
	Mu       sync.Mutex
	API      *tg.Client
}

// NewMTProtoManager 创建管理器
func NewMTProtoManager(db *gorm.DB, proxyMgr *proxy.Manager, log *zap.Logger) *MTProtoManager {
	m := &MTProtoManager{
		db:       db,
		log:      log,
		proxyMgr: proxyMgr,
		clients:  make(map[string]*ClientInstance),
	}
	// 启动时恢复所有已登录账号
	go m.restoreSessions()
	return m
}

// restoreSessions 恢复数据库中状态为 active 的会话
func (m *MTProtoManager) restoreSessions() {
	var accounts []models.TGAccount
	m.db.Where("status = ?", "active").Find(&accounts)

	for i := range accounts {
		account := &accounts[i]
		if _, err := m.StartClient(account); err != nil {
			m.log.Error("failed to restore mtproto session", zap.String("phone", account.Phone), zap.Error(err))
		}
	}
}

// StartClient 启动客户端（连接 Telegram 服务器）
func (m *MTProtoManager) StartClient(acc *models.TGAccount) (*ClientInstance, error) {
	m.mu.Lock()
	if existing, ok := m.clients[acc.ID]; ok {
		m.mu.Unlock()
		return existing, nil
	}
	m.mu.Unlock()

	// 确保 session 目录存在
	sessionDir := "./data/sessions"
	os.MkdirAll(sessionDir, 0700)

	if acc.SessionPath == "" {
		acc.SessionPath = filepath.Join(sessionDir, fmt.Sprintf("%s.session", acc.ID))
		m.db.Save(acc)
	}

	sessionStorage := &telegram.FileSessionStorage{
		Path: acc.SessionPath,
	}

	opts := telegram.Options{
		SessionStorage: sessionStorage,
		Logger:         m.log.Named("gotd").WithOptions(zap.IncreaseLevel(zap.WarnLevel)), // 减少底层日志
	}

	// 配置代理
	if m.proxyMgr != nil {
		if transport := m.proxyMgr.Transport(); transport != nil {
			// gotd 默认使用系统的 Dialer，我们可以通过设置环境变量或自定义 Dialer 来应用代理
			// 简单起见，如果配置了 HTTP 代理，我们仍然依赖系统环境变量或传入自定义 Dialer。
			// 因为 gotd 需要基于 TCP 的 Dialer (比如 golang.org/x/net/proxy)，
			// 为了保持本方案的简洁，这部分先保留默认网络连接，高级 SOCKS5 以后可在 proxy.Manager 补充。
		}
	}

	client := telegram.NewClient(acc.ApiID, acc.ApiHash, opts)
	api := client.API()

	ctx, cancel := context.WithCancel(context.Background())

	authFlow := &TerminalAuth{
		phone:    acc.Phone,
		codeChan: make(chan string, 1),
		pwdChan:  make(chan string, 1),
	}

	inst := &ClientInstance{
		Account:  acc,
		Client:   client,
		AuthFlow: authFlow,
		Cancel:   cancel,
		Status:   "connecting",
		API:      api,
	}

	m.mu.Lock()
	m.clients[acc.ID] = inst
	m.mu.Unlock()

	// 后台运行客户端
	go func() {
		err := client.Run(ctx, func(runCtx context.Context) error {
			// 检查是否已授权
			status, err := client.Auth().Status(runCtx)
			if err != nil {
				return err
			}

			if !status.Authorized {
				inst.Mu.Lock()
				inst.Status = "code_required"
				inst.Mu.Unlock()
				m.updateAccountStatus(acc.ID, "code_required")

				// 触发发送验证码，并等待流程走完
				flow := auth.NewFlow(authFlow, auth.SendCodeOptions{})
				if err := client.Auth().IfNecessary(runCtx, flow); err != nil {
					return err
				}
			}

			// 授权成功
			inst.Mu.Lock()
			inst.Status = "active"
			inst.Mu.Unlock()
			m.updateAccountStatus(acc.ID, "active")
			m.log.Info("mtproto client connected", zap.String("phone", acc.Phone))

			// 保持连接，直到 ctx 被 cancel
			<-runCtx.Done()
			return runCtx.Err()
		})

		m.log.Info("mtproto client stopped", zap.String("phone", acc.Phone), zap.Error(err))
		inst.Mu.Lock()
		inst.Status = "offline"
		inst.Mu.Unlock()
		m.updateAccountStatus(acc.ID, "offline")

		m.mu.Lock()
		delete(m.clients, acc.ID)
		m.mu.Unlock()
	}()

	return inst, nil
}

// updateAccountStatus 更新数据库状态
func (m *MTProtoManager) updateAccountStatus(id, status string) {
	m.db.Model(&models.TGAccount{}).Where("id = ?", id).Update("status", status)
}

// SubmitCode 提交验证码
func (m *MTProtoManager) SubmitCode(id, code string) error {
	m.mu.RLock()
	inst, ok := m.clients[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("client not found or not running")
	}

	inst.Mu.Lock()
	if inst.Status != "code_required" {
		inst.Mu.Unlock()
		return fmt.Errorf("not waiting for code")
	}
	inst.Mu.Unlock()

	// 将 code 传入通道
	select {
	case inst.AuthFlow.codeChan <- code:
		return nil
	default:
		return fmt.Errorf("failed to submit code")
	}
}

// SubmitPassword 提交 2FA 密码
func (m *MTProtoManager) SubmitPassword(id, password string) error {
	m.mu.RLock()
	inst, ok := m.clients[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("client not found or not running")
	}

	select {
	case inst.AuthFlow.pwdChan <- password:
		return nil
	default:
		return fmt.Errorf("failed to submit password")
	}
}

// StopClient 停止客户端
func (m *MTProtoManager) StopClient(id string) {
	m.mu.RLock()
	inst, ok := m.clients[id]
	m.mu.RUnlock()

	if ok {
		inst.Cancel()
	}
}

// GetClient 获取活动的 API 客户端
func (m *MTProtoManager) GetClient() (*ClientInstance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 查找第一个 active 的客户端
	for _, inst := range m.clients {
		inst.Mu.Lock()
		status := inst.Status
		inst.Mu.Unlock()
		if status == "active" {
			return inst, nil
		}
	}
	return nil, fmt.Errorf("no active mtproto client available")
}

// SendMessage 发送文本消息
func (inst *ClientInstance) SendMessage(ctx context.Context, peer string, message string) error {
	// 解析 peer (如 @username)
	resolved, err := inst.API.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: peer})
	if err != nil {
		return fmt.Errorf("resolve username failed: %w", err)
	}
	if len(resolved.Users) == 0 {
		return fmt.Errorf("user not found")
	}
	user := resolved.Users[0].(*tg.User)

	inputPeer := &tg.InputPeerUser{
		UserID:     user.ID,
		AccessHash: user.AccessHash,
	}

	_, err = inst.API.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  message,
		RandomID: time.Now().UnixNano(),
	})
	return err
}
