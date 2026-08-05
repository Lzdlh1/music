package handlers

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
	"github.com/musicflow/musicflow/internal/scheduler"
	"go.uber.org/zap"
)

// WSMessage WebSocket 消息格式
type WSMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp string      `json:"timestamp"`
}

// WSHub WebSocket 连接管理中心
type WSHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
	log     *zap.Logger
}

// NewWSHub 创建 WebSocket Hub
func NewWSHub(log *zap.Logger) *WSHub {
	hub := &WSHub{
		clients: make(map[*websocket.Conn]bool),
		log:     log,
	}
	go hub.heartbeatLoop()
	return hub
}

// heartbeatLoop 定期发送 ping 消息
func (h *WSHub) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.RLock()
		for conn := range h.clients {
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				h.log.Debug("ws ping failed", zap.Error(err))
			}
		}
		h.mu.RUnlock()
	}
}

// HandleTasksWS 处理任务进度 WebSocket 连接
func (h *WSHub) HandleTasksWS(c *websocket.Conn) {
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()

	h.log.Info("ws client connected", zap.String("addr", c.RemoteAddr().String()))

	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		c.Close()
		h.log.Info("ws client disconnected", zap.String("addr", c.RemoteAddr().String()))
	}()

	// 设置 pong 处理器
	c.SetPongHandler(func(string) error {
		return c.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))

	// 读取客户端消息（保持连接活跃）
	for {
		_, _, err := c.ReadMessage()
		if err != nil {
			break
		}
	}
}

// BroadcastQueueStats 广播队列统计信息
func (h *WSHub) BroadcastQueueStats(stats map[string]interface{}) {
	msg := WSMessage{
		Type:      "queue_stats",
		Data:      stats,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcast(msg)
}

// BroadcastTaskUpdate 广播任务状态更新
func (h *WSHub) BroadcastTaskUpdate(taskID, status string, progress scheduler.TaskProgress) {
	msg := WSMessage{
		Type: "task_update",
		Data: map[string]interface{}{
			"task_id":  taskID,
			"status":   status,
			"progress": progress,
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	h.broadcast(msg)
}

// Broadcast 广播消息给所有连接的客户端
func (h *WSHub) broadcast(msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("marshal ws message", zap.Error(err))
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			h.log.Debug("ws write failed", zap.Error(err))
		}
	}
}
