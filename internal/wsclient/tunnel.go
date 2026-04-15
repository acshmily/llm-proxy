package wsclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ErrTunnelDisconnected 隧道未连接的错误
var ErrTunnelDisconnected = errors.New("tunnel is disconnected")

// Tunnel WebSocket 隧道连接管理
type Tunnel struct {
	mu           sync.Mutex
	conn         *websocket.Conn
	server       string
	pingInterval time.Duration
	done         chan struct{}
}

// NewTunnel 创建 WebSocket 隧道
func NewTunnel(server string, pingInterval time.Duration) *Tunnel {
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}

	return &Tunnel{
		server:       server,
		pingInterval: pingInterval,
		done:         make(chan struct{}),
	}
}

// IsConnected 检查是否已连接
func (t *Tunnel) IsConnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.conn != nil
}

// Connect 建立 WebSocket 连接
func (t *Tunnel) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 关闭旧连接
	if t.conn != nil {
		t.conn.Close()
	}

	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(t.server, nil)
	if err != nil {
		return fmt.Errorf("failed to dial WebSocket: %w", err)
	}

	t.conn = conn

	// 启动心跳循环
	go t.pingLoop()

	return nil
}

// pingLoop 心跳循环
func (t *Tunnel) pingLoop() {
	ticker := time.NewTicker(t.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.mu.Lock()
			if t.conn != nil {
				pingMsg := WSRequest{Type: "ping"}
				data, _ := json.Marshal(pingMsg)
				t.conn.WriteMessage(websocket.TextMessage, data)
			}
			t.mu.Unlock()
		case <-t.done:
			return
		}
	}
}

// SendRequest 发送 HTTP 请求
func (t *Tunnel) SendRequest(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		return nil, ErrTunnelDisconnected
	}

	// 编码请求
	data, err := EncodeRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	// 发送 WebSocket 消息
	if err := t.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, fmt.Errorf("failed to write message: %w", err)
	}

	// 读取响应
	_, respData, err := t.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// 解码响应
	return DecodeResponse(respData)
}

// Close 关闭隧道
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	close(t.done)

	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}
