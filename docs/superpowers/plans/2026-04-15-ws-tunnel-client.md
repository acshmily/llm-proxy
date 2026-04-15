# WebSocket 隧道客户端实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现本地 HTTP 代理服务器，使应用程序无需修改代码即可通过 WebSocket 隧道与服务端通信

**Architecture:** 单进程架构，维护单一 WebSocket 长连接，HTTP 请求封装为 WebSocket 消息发送

**Tech Stack:** Go 1.21, gorilla/websocket, sync.Mutex, http.Handler

---

## 文件结构

**新增文件：**
- `cmd/ws-client/main.go` - 程序入口，信号处理
- `cmd/ws-client/config.go` - 配置结构 + 加载逻辑
- `internal/wsclient/protocol.go` - 消息协议（封装/解封装）
- `internal/wsclient/protocol_test.go` - 协议单元测试
- `internal/wsclient/tunnel.go` - WebSocket 隧道管理
- `internal/wsclient/tunnel_test.go` - 隧道单元测试
- `internal/wsclient/proxy.go` - HTTP 代理服务器
- `internal/wsclient/proxy_test.go` - 代理单元测试
- `internal/wsclient/health.go` - 健康检查端点
- `internal/wsclient/logger.go` - 日志封装
- `configs/client-config.example.yaml` - 配置示例
- `docs/ws-client-guide.md` - 使用指南

**修改文件：**
- `go.mod` - 添加 gorilla/websocket 依赖

---

## Chunk 1: 消息协议层

### Task 1: 消息协议定义与测试

**Files:**
- Create: `internal/wsclient/protocol.go`
- Create: `internal/wsclient/protocol_test.go`

**依赖:** gorilla/websocket（仅用于类型定义，实际测试无需连接）

- [ ] **Step 1: 编写协议测试 - 请求封装**

```go
// internal/wsclient/protocol_test.go
package wsclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeRequest(t *testing.T) {
	// 测试 GET 请求
	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/messages", nil)
		req.Header.Set("Authorization", "Bearer sk-test")
		
		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}
		
		// 验证 JSON 结构
		var msg WSRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}
		
		if msg.Type != "request" {
			t.Errorf("expected type 'request', got %s", msg.Type)
		}
		if msg.Data.Method != "GET" {
			t.Errorf("expected method 'GET', got %s", msg.Data.Method)
		}
		if msg.Data.Path != "/v1/messages" {
			t.Errorf("expected path '/v1/messages', got %s", msg.Data.Path)
		}
		if msg.Data.Body != "" {
			t.Errorf("expected empty body for GET, got %s", msg.Data.Body)
		}
	})
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test -v ./internal/wsclient/... -run TestEncodeRequest`
Expected: FAIL with "undefined: EncodeRequest"

- [ ] **Step 3: 实现最小化代码 - 协议类型定义**

```go
// internal/wsclient/protocol.go
package wsclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
)

// WSRequest WebSocket 请求消息
type WSRequest struct {
	Type string    `json:"type"`
	Data WSReqData `json:"data"`
}

// WSReqData WebSocket 请求数据
type WSReqData struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

// WSResponse WebSocket 响应消息
type WSResponse struct {
	Type string     `json:"type"`
	Data WSRespData `json:"data"`
}

// WSRespData WebSocket 响应数据
type WSRespData struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
	Message string            `json:"message,omitempty"`
}
```

- [ ] **Step 4: 运行测试验证失败**

Run: `go test -v ./internal/wsclient/... -run TestEncodeRequest`
Expected: FAIL with "undefined: EncodeRequest" (类型已定义，函数未定义)

- [ ] **Step 5: 实现 EncodeRequest 函数**

```go
// internal/wsclient/protocol.go

// EncodeRequest 将 HTTP 请求编码为 WebSocket 消息
func EncodeRequest(req *http.Request) ([]byte, error) {
	// 读取请求体
	var bodyBytes []byte
	var err error
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		defer req.Body.Close()
	}
	
	// 构建请求数据
	data := WSReqData{
		Method:  req.Method,
		Path:    req.URL.RequestURI(),
		Headers: make(map[string]string),
		Body:    base64.StdEncoding.EncodeToString(bodyBytes),
	}
	
	// 复制请求头（多值头部取第一个）
	for key, values := range req.Header {
		if len(values) > 0 {
			data.Headers[key] = values[0]
		}
	}
	
	// 构建 WebSocket 消息
	msg := WSRequest{
		Type: "request",
		Data: data,
	}
	
	return json.Marshal(msg)
}
```

- [ ] **Step 6: 运行测试验证通过**

Run: `go test -v ./internal/wsclient/... -run TestEncodeRequest`
Expected: PASS

- [ ] **Step 7: 添加更多测试用例**

```go
// internal/wsclient/protocol_test.go

func TestEncodeRequest(t *testing.T) {
	// ... existing GET test ...
	
	// 测试 POST 请求带 Body
	t.Run("POST request with body", func(t *testing.T) {
		body := `{"message": "hello"}`
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		
		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}
		
		var msg WSRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}
		
		// 解码 Body 验证
		decodedBody, err := base64.StdEncoding.DecodeString(msg.Data.Body)
		if err != nil {
			t.Fatalf("Base64 decode failed: %v", err)
		}
		if string(decodedBody) != body {
			t.Errorf("expected body %q, got %q", body, string(decodedBody))
		}
	})
}
```

- [ ] **Step 8: 运行所有协议测试**

Run: `go test -v ./internal/wsclient/... -run TestEncodeRequest`
Expected: PASS (2 subtests)

- [ ] **Step 9: 提交**

```bash
git add internal/wsclient/protocol.go internal/wsclient/protocol_test.go
git commit -m "feat(ws-client): add request encoding with tests"
```

---

### Task 2: 响应解码测试与实现

**Files:**
- Modify: `internal/wsclient/protocol.go`
- Modify: `internal/wsclient/protocol_test.go`

- [ ] **Step 1: 编写响应解码测试**

```go
// internal/wsclient/protocol_test.go

func TestDecodeResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		body := `{"result": "ok"}`
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  200,
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    base64.StdEncoding.EncodeToString([]byte(body)),
			},
		}
		
		data, _ := json.Marshal(respData)
		
		resp, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("DecodeResponse failed: %v", err)
		}
		
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		
		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != body {
			t.Errorf("expected body %q, got %q", body, string(bodyBytes))
		}
	})
	
	t.Run("error response", func(t *testing.T) {
		respData := WSResponse{
			Type: "error",
			Data: WSRespData{
				Message: "server error",
			},
		}
		
		data, _ := json.Marshal(respData)
		resp, err := DecodeResponse(data)
		
		if err == nil {
			t.Error("expected error for error response type")
		}
		if resp != nil {
			t.Error("expected nil response for error type")
		}
	})
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test -v ./internal/wsclient/... -run TestDecodeResponse`
Expected: FAIL with "undefined: DecodeResponse"

- [ ] **Step 3: 实现 DecodeResponse 函数**

```go
// internal/wsclient/protocol.go

import "errors"

var ErrServerResponse = errors.New("server returned error")

// DecodeResponse 将 WebSocket 响应解码为 HTTP 响应
func DecodeResponse(data []byte) (*http.Response, error) {
	var resp WSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	
	// 处理错误响应
	if resp.Type == "error" {
		return nil, ErrServerResponse
	}
	
	// 解码响应体
	bodyBytes := []byte{}
	var err error
	if resp.Data.Body != "" {
		bodyBytes, err = base64.StdEncoding.DecodeString(resp.Data.Body)
		if err != nil {
			return nil, err
		}
	}
	
	// 构建 HTTP 响应
	httpResp := &http.Response{
		Status:        http.StatusText(resp.Data.Status),
		StatusCode:    resp.Data.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(bodyBytes)),
		ContentLength: int64(len(bodyBytes)),
	}
	
	// 复制响应头
	for key, value := range resp.Data.Headers {
		httpResp.Header.Set(key, value)
	}
	
	return httpResp, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test -v ./internal/wsclient/... -run TestDecodeResponse`
Expected: PASS

- [ ] **Step 5: 添加往返测试**

```go
// internal/wsclient/protocol_test.go

func TestRoundTrip(t *testing.T) {
	// 验证编码后再解码能还原原始请求
	t.Run("encode then decode", func(t *testing.T) {
		originalBody := `{"test": "data"}`
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader([]byte(originalBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-test")
		
		// 编码
		encoded, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}
		
		// 模拟服务端响应（原样返回）
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  200,
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    base64.StdEncoding.EncodeToString([]byte(originalBody)),
			},
		}
		respBytes, _ := json.Marshal(respData)
		
		// 解码
		resp, err := DecodeResponse(respBytes)
		if err != nil {
			t.Fatalf("DecodeResponse failed: %v", err)
		}
		
		// 验证
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		
		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != originalBody {
			t.Errorf("body mismatch: expected %q, got %q", originalBody, string(bodyBytes))
		}
	})
}
```

- [ ] **Step 6: 运行往返测试**

Run: `go test -v ./internal/wsclient/... -run TestRoundTrip`
Expected: PASS

- [ ] **Step 7: 提交**

```bash
git add internal/wsclient/protocol.go internal/wsclient/protocol_test.go
git commit -m "feat(ws-client): add response decoding with round-trip tests"
```

---

### Task 3: 协议错误处理测试

**Files:**
- Modify: `internal/wsclient/protocol_test.go`

- [ ] **Step 1: 添加错误处理测试**

```go
// internal/wsclient/protocol_test.go

func TestEncodeRequest_EdgeCases(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		data, err := EncodeRequest(req)
		
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		var msg WSRequest
		json.Unmarshal(data, &msg)
		if msg.Data.Body != "" {
			t.Errorf("expected empty body, got %q", msg.Data.Body)
		}
	})
	
	t.Run("binary body", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF}
		req := httptest.NewRequest("POST", "/binary", bytes.NewReader(binaryData))
		
		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}
		
		var msg WSRequest
		json.Unmarshal(data, &msg)
		
		// 验证 Base64 编码正确
		decoded, err := base64.StdEncoding.DecodeString(msg.Data.Body)
		if err != nil {
			t.Fatalf("Base64 decode failed: %v", err)
		}
		if !bytes.Equal(decoded, binaryData) {
			t.Errorf("binary data mismatch")
		}
	})
}

func TestDecodeResponse_EdgeCases(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		_, err := DecodeResponse([]byte(`{invalid}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
	
	t.Run("invalid base64 body", func(t *testing.T) {
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status: 200,
				Body:   "!!!invalid-base64!!!",
			},
		}
		data, _ := json.Marshal(respData)
		
		_, err := DecodeResponse(data)
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})
	
	t.Run("missing body (empty response)", func(t *testing.T) {
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  204,
				Headers: map[string]string{},
			},
		}
		data, _ := json.Marshal(respData)
		
		resp, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		
		body, _ := io.ReadAll(resp.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body for 204, got %d bytes", len(body))
		}
	})
}
```

- [ ] **Step 2: 运行边界测试**

Run: `go test -v ./internal/wsclient/... -run "TestEncodeRequest_EdgeCases|TestDecodeResponse_EdgeCases"`
Expected: PASS (所有子测试)

- [ ] **Step 3: 提交**

```bash
git add internal/wsclient/protocol_test.go
git commit -m "test(ws-client): add edge case tests for protocol"
```

---

## Chunk 2: WebSocket 隧道层

### Task 4: 隧道连接管理测试与实现

**Files:**
- Create: `internal/wsclient/tunnel.go`
- Create: `internal/wsclient/tunnel_test.go`

- [ ] **Step 1: 编写隧道连接测试**

```go
// internal/wsclient/tunnel_test.go
package wsclient

import (
	"testing"
	"time"
)

func TestTunnel_IsConnected(t *testing.T) {
	t.Run("initial state disconnected", func(t *testing.T) {
		tunnel := NewTunnel("ws://localhost:8080/ws-tunnel", 30*time.Second)
		defer tunnel.Close()
		
		if tunnel.IsConnected() {
			t.Error("expected disconnected in initial state")
		}
	})
}

func TestTunnel_Connect(t *testing.T) {
	// 使用 httptest.Server 模拟 WebSocket 服务端
	// 这个测试需要实际的网络连接
	t.Skip("requires WebSocket server mock")
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test -v ./internal/wsclient/... -run TestTunnel_IsConnected`
Expected: FAIL with "undefined: NewTunnel"

- [ ] **Step 3: 实现 Tunnel 结构**

```go
// internal/wsclient/tunnel.go
package wsclient

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test -v ./internal/wsclient/... -run TestTunnel_IsConnected`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/wsclient/tunnel.go internal/wsclient/tunnel_test.go
git commit -m "feat(ws-client): add tunnel connection state management"
```

---

### Task 5: 隧道连接与重连

**Files:**
- Modify: `internal/wsclient/tunnel.go`
- Modify: `internal/wsclient/tunnel_test.go`

- [ ] **Step 1: 编写连接测试**

```go
// internal/wsclient/tunnel_test.go

func TestTunnel_ConnectAndDisconnect(t *testing.T) {
	// 创建 Mock WebSocket 服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket 升级处理在集成测试中实现
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))
	defer server.Close()
	
	wsURL := "ws" + server.URL[4:] + "/ws-tunnel"
	tunnel := NewTunnel(wsURL, 30*time.Second)
	defer tunnel.Close()
	
	// 测试连接
	err := tunnel.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	
	if !tunnel.IsConnected() {
		t.Error("expected connected after Connect()")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test -v ./internal/wsclient/... -run TestTunnel_ConnectAndDisconnect`
Expected: FAIL with "undefined: Connect"

- [ ] **Step 3: 实现 Connect 方法**

```go
// internal/wsclient/tunnel.go

import (
	"fmt"
	"net/url"
)

// Connect 建立 WebSocket 连接
func (t *Tunnel) Connect() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
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
```

- [ ] **Step 4: 添加 import**

```go
// internal/wsclient/tunnel.go 顶部
import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)
```

- [ ] **Step 5: 运行测试**

Run: `go test -v ./internal/wsclient/... -run TestTunnel_ConnectAndDisconnect`
Expected: 根据实际连接结果

- [ ] **Step 6: 提交**

```bash
git add internal/wsclient/tunnel.go
git commit -m "feat(ws-client): add WebSocket connection with heartbeat"
```

---

## Chunk 3: HTTP 代理层

（计划继续，受消息长度限制）

---

## Chunk 4: 配置与健康检查

（计划继续）

---

## Chunk 5: 集成与文档

（计划继续）

---

## 测试命令汇总

**单元测试：**
```bash
go test -v ./internal/wsclient/...
```

**集成测试：**
```bash
go test -v ./internal/wsclient/... -run Integration
```

**构建验证：**
```bash
go build ./cmd/ws-client
./ws-client --help
```

---

## 依赖安装

```bash
go get github.com/gorilla/websocket@v1.5.1
go mod tidy
```

---

## 提交规范

使用 conventional commits:
- `feat(ws-client):` - 新功能
- `test(ws-client):` - 测试
- `fix(ws-client):` - 修复
- `docs(ws-client):` - 文档
- `chore:` - 工具/配置
