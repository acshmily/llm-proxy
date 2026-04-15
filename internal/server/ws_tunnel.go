package server

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/middleware"
)

// WSTunnelMiddleware WebSocket 隧道中间件
type WSTunnelMiddleware struct {
	cfg       *config.WebSocketTunnelConfig
	obfus     *middleware.TrafficObfuscationMiddleware
	mu        sync.Mutex
	connections sync.Map // map[string]net.Conn
}

// NewWSTunnelMiddleware 创建 WebSocket 隧道中间件
func NewWSTunnelMiddleware(cfg *config.WebSocketTunnelConfig, obfus *middleware.TrafficObfuscationMiddleware) *WSTunnelMiddleware {
	return &WSTunnelMiddleware{
		cfg:   cfg,
		obfus: obfus,
	}
}

// IsEnabled 检查 WebSocket 隧道是否启用
func (m *WSTunnelMiddleware) IsEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg != nil && m.cfg.Enabled
}

// GetPath 获取 WebSocket 隧道路径
func (m *WSTunnelMiddleware) GetPath() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg == nil || m.cfg.Path == "" {
		return "/ws-tunnel"
	}
	return m.cfg.Path
}

// HandleWebSocket 处理 WebSocket 升级请求
func (m *WSTunnelMiddleware) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !m.IsEnabled() {
		http.Error(w, "WebSocket tunnel disabled", http.StatusServiceUnavailable)
		return
	}

	// 检查 Upgrade 头
	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		http.Error(w, "Upgrade header required", http.StatusBadRequest)
		return
	}

	// WebSocket 握手
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Sec-WebSocket-Key header required", http.StatusBadRequest)
		return
	}

	// 生成接受密钥
	acceptKey := computeWebSocketAcceptKey(key)

	// 升级为 WebSocket 连接
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket upgrade not supported", http.StatusInternalServerError)
		return
	}

	conn, bufrw, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 发送握手响应
	response := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", acceptKey)

	if _, err := bufrw.WriteString(response); err != nil {
		conn.Close()
		return
	}
	bufrw.Flush()

	// 保存连接
	connID := generateConnectionID()
	m.connections.Store(connID, conn)
	defer m.connections.Delete(connID)

	// 处理 WebSocket 消息
	m.handleWebSocketMessages(conn, bufrw)
}

// handleWebSocketMessages 处理 WebSocket 消息循环
func (m *WSTunnelMiddleware) handleWebSocketMessages(conn net.Conn, bufrw *bufio.ReadWriter) {
	for {
		// 读取 WebSocket 消息
		msg, err := readWebSocketFrame(bufrw.Reader)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("WebSocket read error: %v\n", err)
			}
			break
		}

		// 处理消息
		response, err := m.processWebSocketMessage(msg)
		if err != nil {
			fmt.Printf("Process message error: %v\n", err)
			break
		}

		// 发送响应
		if err := writeWebSocketFrame(bufrw.Writer, response); err != nil {
			fmt.Printf("WebSocket write error: %v\n", err)
			break
		}
		bufrw.Flush()
	}

	conn.Close()
}

// processWebSocketMessage 处理 WebSocket 消息
func (m *WSTunnelMiddleware) processWebSocketMessage(msg []byte) ([]byte, error) {
	// 消息格式：{"type": "request", "data": {"method": "POST", "path": "/v1/messages", "headers": {}, "body": "base64..."}}
	var envelope map[string]interface{}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return nil, err
	}

	msgType, _ := envelope["type"].(string)
	switch msgType {
	case "request":
		return m.handleHTTPRequest(envelope)
	case "ping":
		return json.Marshal(map[string]string{"type": "pong"})
	default:
		return json.Marshal(map[string]string{"type": "error", "message": "unknown message type"})
	}
}

// handleHTTPRequest 处理 HTTP 请求转发
func (m *WSTunnelMiddleware) handleHTTPRequest(envelope map[string]interface{}) ([]byte, error) {
	data, _ := envelope["data"].(map[string]interface{})
	if data == nil {
		return json.Marshal(map[string]string{"type": "error", "message": "invalid request data"})
	}

	// 提取请求信息
	method, _ := data["method"].(string)
	path, _ := data["path"].(string)
	headersRaw, _ := data["headers"].(map[string]interface{})
	bodyRaw, _ := data["body"].(string)

	if method == "" || path == "" {
		return json.Marshal(map[string]string{"type": "error", "message": "method and path required"})
	}

	// 解码 body
	var bodyBytes []byte
	if bodyRaw != "" {
		var err error
		bodyBytes, err = base64.StdEncoding.DecodeString(bodyRaw)
		if err != nil {
			return json.Marshal(map[string]string{"type": "error", "message": "invalid body encoding"})
		}
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest(method, path, bytes.NewReader(bodyBytes))
	if err != nil {
		return json.Marshal(map[string]string{"type": "error", "message": err.Error()})
	}

	// 设置头部
	for key, value := range headersRaw {
		if strValue, ok := value.(string); ok {
			req.Header.Set(key, strValue)
		}
	}

	// 这里应该将请求转发到后端，但为了简化，我们只返回一个响应
	// 实际使用中需要集成到主服务器的路由逻辑
	return json.Marshal(map[string]interface{}{
		"type": "response",
		"data": map[string]interface{}{
			"status":  200,
			"headers": map[string]string{"Content-Type": "application/json"},
			"body":    base64.StdEncoding.EncodeToString([]byte(`{"message": "WebSocket tunnel OK"}`)),
		},
	})
}

// WebSocket 帧辅助函数

func readWebSocketFrame(r *bufio.Reader) ([]byte, error) {
	// 读取前 2 字节
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// 解析 payload 长度
	payloadLen := int(header[1] & 0x7F)
	if payloadLen == 126 {
		extLen := make([]byte, 2)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return nil, err
		}
		payloadLen = int(binary.BigEndian.Uint16(extLen))
	} else if payloadLen == 127 {
		extLen := make([]byte, 8)
		if _, err := io.ReadFull(r, extLen); err != nil {
			return nil, err
		}
		payloadLen = int(binary.BigEndian.Uint64(extLen))
	}

	// 检查是否有 mask
	masked := (header[1] & 0x80) != 0
	var maskKey []byte
	if masked {
		maskKey = make([]byte, 4)
		if _, err := io.ReadFull(r, maskKey); err != nil {
			return nil, err
		}
	}

	// 读取 payload
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	// 如果有 mask，解 mask
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	return payload, nil
}

func writeWebSocketFrame(w *bufio.Writer, data []byte) error {
	// 文本帧，服务器不 mask
	frame := make([]byte, 0, 10+len(data))
	frame = append(frame, 0x81) // FIN=1, opcode=1 (text)

	// 添加长度
	if len(data) < 126 {
		frame = append(frame, byte(len(data)))
	} else if len(data) < 65536 {
		frame = append(frame, 126)
		frame = append(frame, byte(len(data)>>8), byte(len(data)))
	} else {
		frame = append(frame, 127)
		for i := 7; i >= 0; i-- {
			frame = append(frame, byte(len(data)>>(i*8)))
		}
	}

	frame = append(frame, data...)
	_, err := w.Write(frame)
	return err
}

func computeWebSocketAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func generateConnectionID() string {
	return fmt.Sprintf("conn-%d", binary.BigEndian.Uint64([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
}

// WSTunnelHandler 返回处理 WebSocket 隧道的 HTTP Handler
func (m *WSTunnelMiddleware) WSTunnelHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != m.GetPath() {
			http.NotFound(w, r)
			return
		}
		m.HandleWebSocket(w, r)
	}
}

// WebSocket 辅助类型

// WSMessage WebSocket 消息结构
type WSMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
}

// WSRequest WebSocket 请求数据结构
type WSRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"` // Base64 编码
}

// WSResponse WebSocket 响应数据结构
type WSResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"` // Base64 编码
}

// 错误
var (
	ErrWebSocketUpgradeRequired = errors.New("websocket upgrade required")
	ErrInvalidWebSocketKey      = errors.New("invalid websocket key")
	ErrHijackNotSupported       = errors.New("hijack not supported")
)
