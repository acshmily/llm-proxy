# OpenAI 协议支持实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 llm-proxy 服务端增加 OpenAI 协议入口 (`/v1/chat/completions`)，支持客户端使用 OpenAI SDK 调用任意后端。

**Architecture:** 
- 新增 OpenAI 协议解析器 (`parser.go`) 解析 OpenAI 请求为 UnifiedMessage
- 扩展现有 OpenAI 转换器支持响应转换 (`converter.go`)
- 修改 Server 入口增加 OpenAI 路由 (`server.go`)
- 统一错误处理为 OpenAI 兼容格式

**Tech Stack:** Go 1.21+, net/http, encoding/json, testify/assert (testing)

---

## Chunk 1: OpenAI 解析器和转换器

### Task 1: 创建 OpenAI 解析器 (parser.go)

**Files:**
- Create: `internal/protocol/openai/parser.go`
- Test: `internal/protocol/openai/parser_test.go`

- [ ] **Step 1: 编写测试文件**

```go
// internal/protocol/openai/parser_test.go
package openai

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestParseRequest_ValidRequest(t *testing.T) {
    body := []byte(`{
        "model": "gpt-4",
        "messages": [
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": "Hello"}
        ],
        "stream": false,
        "temperature": 0.7,
        "max_tokens": 1024
    }`)
    
    req, err := ParseRequest(body)
    
    assert.NoError(t, err)
    assert.Equal(t, "gpt-4", req.Model)
    assert.Equal(t, 2, len(req.Messages))
    assert.Equal(t, false, req.Stream)
    assert.Equal(t, 0.7, req.Temperature)
    assert.Equal(t, 1024, req.MaxTokens)
}

func TestParseRequest_InvalidJSON(t *testing.T) {
    body := []byte(`{invalid json}`)
    req, err := ParseRequest(body)
    
    assert.Error(t, err)
    assert.Nil(t, req)
}

func TestParseRequest_EmptyMessages(t *testing.T) {
    body := []byte(`{"model": "gpt-4", "messages": []}`)
    req, err := ParseRequest(body)
    
    assert.NoError(t, err)
    assert.Equal(t, 0, len(req.Messages))
}

func TestParseRequest_MissingModel(t *testing.T) {
    body := []byte(`{"messages": [{"role": "user", "content": "Hi"}]}`)
    req, err := ParseRequest(body)
    
    assert.NoError(t, err)
    assert.Equal(t, "", req.Model)
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
cd /Users/r2d2/Documents/claude-projetc/proxy-gemini-go
go test ./internal/protocol/openai/... -v -run TestParseRequest
```
Expected: FAIL with "undefined: ParseRequest"

- [ ] **Step 3: 创建解析器实现**

```go
// internal/protocol/openai/parser.go
package openai

import (
    "encoding/json"
    "github.com/claude-projetc/llm-proxy/pkg/types"
)

// OpenAIRequest OpenAI Chat Completion 请求
type OpenAIRequest struct {
    Model            string          `json:"model"`
    Messages         []ChatMessage   `json:"messages"`
    Stream           bool            `json:"stream,omitempty"`
    MaxTokens        int             `json:"max_tokens,omitempty"`
    Temperature      float64         `json:"temperature,omitempty"`
    TopP             float64         `json:"top_p,omitempty"`
    Stop             []string        `json:"stop,omitempty"`
    PresencePenalty  float64         `json:"presence_penalty,omitempty"`
    FrequencyPenalty float64         `json:"frequency_penalty,omitempty"`
    User             string          `json:"user,omitempty"`
}

// ChatMessage OpenAI 聊天消息
type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
    Name    string `json:"name,omitempty"`
}

// ParseRequest 解析 OpenAI 请求为统一格式
func ParseRequest(data []byte) (*types.UnifiedMessage, error) {
    var req OpenAIRequest
    if err := json.Unmarshal(data, &req); err != nil {
        return nil, err
    }
    
    unified := &types.UnifiedMessage{
        Model:    req.Model,
        Messages: make([]types.MessageRole, len(req.Messages)),
        Stream:   req.Stream,
        MaxTokens: req.MaxTokens,
        Temperature: req.Temperature,
        TopP:     req.TopP,
    }
    
    // 转换消息格式
    for i, msg := range req.Messages {
        unified.Messages[i] = types.MessageRole{
            Role:    msg.Role,
            Content: msg.Content,
        }
    }
    
    // Stop 参数映射
    if len(req.Stop) > 0 {
        unified.StopSequences = req.Stop
    }
    
    return unified, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/protocol/openai/... -v -run TestParseRequest
```
Expected: PASS (4 tests)

- [ ] **Step 5: 提交**

```bash
git add internal/protocol/openai/parser.go internal/protocol/openai/parser_test.go
git commit -m "feat: add OpenAI request parser"
```

---

### Task 2: 扩展 OpenAI 响应转换器

**Files:**
- Modify: `internal/protocol/openai/converter.go:47-74`
- Test: `internal/protocol/openai/converter_test.go` (add tests)

- [ ] **Step 1: 添加响应转换测试**

```go
// internal/protocol/openai/converter_test.go (追加)

func TestBuildResponse_ValidResponse(t *testing.T) {
    data := []byte(`{
        "id": "chatcmpl-123",
        "model": "gpt-4",
        "choices": [{
            "index": 0,
            "message": {"role": "assistant", "content": "Hello!"},
            "finish_reason": "stop"
        }],
        "usage": {"prompt_tokens": 10, "completion_tokens": 5}
    }`)
    
    body, err := BuildResponse(data)
    
    assert.NoError(t, err)
    assert.Contains(t, string(body), "chatcmpl-123")
    assert.Contains(t, string(body), "Hello!")
}

func TestParseResponse_EmptyChoices(t *testing.T) {
    data := []byte(`{
        "id": "chatcmpl-123",
        "model": "gpt-4",
        "choices": [],
        "usage": {"prompt_tokens": 10, "completion_tokens": 0}
    }`)
    
    resp, err := ParseResponse(data)
    
    assert.NoError(t, err)
    assert.Equal(t, "", resp.Content[0].Text)
}
```

- [ ] **Step 2: 添加 BuildResponse 函数**

```go
// internal/protocol/openai/converter.go (追加到文件末尾)

// ChatCompletionResponse OpenAI Chat Completion 响应
type ChatCompletionResponse struct {
    ID      string    `json:"id"`
    Object  string    `json:"object"`
    Created int64     `json:"created"`
    Model   string    `json:"model"`
    Choices []Choice  `json:"choices"`
    Usage   Usage     `json:"usage"`
}

// Choice 响应选择
type Choice struct {
    Index        int     `json:"index"`
    Message      ChatMessage `json:"message"`
    FinishReason string  `json:"finish_reason"`
}

// BuildResponse 将统一响应转换为 OpenAI 格式
func BuildResponse(unified *types.UnifiedResponse) ([]byte, error) {
    var content string
    if len(unified.Content) > 0 {
        content = unified.Content[0].Text
    }
    
    resp := ChatCompletionResponse{
        ID:      unified.ID,
        Object:  "chat.completion",
        Created: time.Now().Unix(),
        Model:   unified.Model,
        Choices: []Choice{{
            Index: 0,
            Message: ChatMessage{
                Role:    "assistant",
                Content: content,
            },
            FinishReason: "stop",
        }},
        Usage: Usage{
            PromptTokens:     unified.Usage.InputTokens,
            CompletionTokens: unified.Usage.OutputTokens,
        },
    }
    
    return json.Marshal(resp)
}
```

- [ ] **Step 3: 添加 time 导入**

```go
// internal/protocol/openai/converter.go (modify imports)
import (
    "encoding/json"
    "time"
    "github.com/claude-projetc/llm-proxy/pkg/types"
)
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/protocol/openai/... -v
```
Expected: PASS (6+ tests)

- [ ] **Step 5: 提交**

```bash
git add internal/protocol/openai/converter.go internal/protocol/openai/converter_test.go
git commit -m "feat: add OpenAI response builder"
```

---

## Chunk 2: 服务端路由和错误处理

### Task 3: 修改 Server 增加 OpenAI 路由

**Files:**
- Modify: `internal/server/server.go:119-133` (ServeHTTP method)

- [ ] **Step 1: 修改 ServeHTTP 方法**

```go
// internal/server/server.go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // WebSocket 隧道端点
    if s.wsTunnel != nil && r.URL.Path == s.wsTunnel.GetPath() {
        s.wsTunnel.WSTunnelHandler()(w, r)
        return
    }

    // 健康检查端点
    if r.URL.Path == "/health" && r.Method == http.MethodGet {
        s.HealthCheck(w, r)
        return
    }

    // 根据路径路由到不同协议处理器
    switch {
    case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
        s.serveOpenAIRequest(w, r)
        return
    case r.URL.Path == "/v1/messages" && r.Method == http.MethodPost:
        // 现有 Anthropic 端点
        s.serveRequest(w, r)
        return
    default:
        s.writeError(w, http.StatusNotFound, "Endpoint not found")
        return
    }
}
```

- [ ] **Step 2: 添加 serveOpenAIRequest 方法**

```go
// internal/server/server.go (添加新方法)

// serveOpenAIRequest 处理 OpenAI 协议请求
func (s *Server) serveOpenAIRequest(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // 获取 API Key
    apiKey := r.Header.Get("x-api-key")
    if apiKey == "" {
        apiKey = extractBearerToken(r.Header.Get("Authorization"))
    }

    route, found := s.router.FindRoute(apiKey)
    if !found {
        s.writeError(w, http.StatusUnauthorized, "Invalid API key")
        return
    }

    // 读取请求体
    body, err := io.ReadAll(r.Body)
    if err != nil {
        s.writeError(w, http.StatusBadRequest, "Failed to read request body")
        return
    }

    // 解析 OpenAI 请求
    unified, err := openai.ParseRequest(body)
    if err != nil {
        s.writeError(w, http.StatusBadRequest, "Invalid request format")
        return
    }

    // 选择后端转换器
    var backendURL string
    var reqBody []byte

    model := unified.Model
    if model == "" {
        model = getDefaultModel(route.Backend)
    }

    switch route.Backend {
    case "openai":
        backendURL = s.cfg.Backends.OpenAI.BaseURL + "/chat/completions"
        reqBody, _ = openai.Convert(unified, model)
    case "anthropic":
        backendURL = s.cfg.Backends.Anthropic.BaseURL + "/v1/messages"
        reqBody, _ = claude.Convert(unified, model)
    case "gemini":
        backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent"
        reqBody, _ = gemini.Convert(unified, model)
        backendURL = backendURL + "?key=" + route.BackendKey
    default:
        s.writeError(w, http.StatusBadRequest, "Unknown backend")
        return
    }

    // 创建后端请求
    backendReq, err := http.NewRequest(r.Method, backendURL, bytes.NewReader(reqBody))
    if err != nil {
        s.writeError(w, http.StatusInternalServerError, "Failed to create backend request")
        return
    }

    // 设置后端认证（Gemini 不需要 Authorization header）
    if route.Backend != "gemini" {
        backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
    }
    backendReq.Header.Set("Content-Type", "application/json")

    // 执行请求
    ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
    defer cancel()

    var connReused bool
    connReused = true

    trace := &httptrace.ClientTrace{
        GotConn: func(info httptrace.GotConnInfo) {
            connReused = info.Reused
        },
    }
    ctx = httptrace.WithClientTrace(ctx, trace)

    resp, err := s.client.Do(backendReq.WithContext(ctx))
    if err != nil {
        s.writeError(w, http.StatusBadGateway, "Backend request failed")
        return
    }
    defer resp.Body.Close()

    latency := time.Since(start).Milliseconds()

    // 记录连接复用统计
    s.poolStats.RecordRequest(connReused)

    // 处理响应
    if unified.Stream {
        s.handleOpenAIStream(w, resp, route.Backend, connReused)
    } else {
        s.handleOpenAINonStream(w, resp, route.Backend, latency, start, connReused)
    }
}
```

- [ ] **Step 3: 添加 handleOpenAINonStream 方法**

```go
// internal/server/server.go (添加新方法)

func (s *Server) handleOpenAINonStream(w http.ResponseWriter, resp *http.Response, backend string, latency int64, start time.Time, connReused bool) {
    body, _ := io.ReadAll(resp.Body)

    if resp.StatusCode != http.StatusOK {
        // 转换后端错误为 OpenAI 格式
        s.writeOpenAIError(w, resp.StatusCode, convertBackendError(backend, body))
        return
    }

    // 转换响应为统一格式
    var unified *types.UnifiedResponse
    var err error

    switch backend {
    case "openai":
        unified, err = openai.ParseResponse(body)
    case "anthropic":
        unified, err = claude.ParseResponse(body)
    case "gemini":
        unified, err = gemini.ParseResponse(body)
    }

    if err != nil {
        s.writeOpenAIError(w, http.StatusInternalServerError, "Failed to parse response")
        return
    }

    // 构建 OpenAI 格式响应
    respBody, err := openai.BuildResponse(unified)
    if err != nil {
        s.writeOpenAIError(w, http.StatusInternalServerError, "Failed to build response")
        return
    }

    // 记录日志
    requests, reused, created := s.poolStats.GetStats()
    s.log.Info("OpenAI request completed",
        logger.LogField{Key: "latency_ms", Value: latency},
        logger.LogField{Key: "status_code", Value: resp.StatusCode},
        logger.LogField{Key: "backend", Value: backend},
        logger.LogField{Key: "conn_reused", Value: connReused},
    )

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write(respBody)
}
```

- [ ] **Step 4: 添加 handleOpenAIStream 方法（流式处理）】**

```go
// internal/server/server.go (添加新方法)

func (s *Server) handleOpenAIStream(w http.ResponseWriter, resp *http.Response, backend string, connReused bool) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // 记录连接复用状态
    s.log.Info("OpenAI stream request completed",
        logger.LogField{Key: "backend", Value: backend},
        logger.LogField{Key: "conn_reused", Value: connReused},
    )

    // 流式解析并转发
    stream.ParseSSE(resp.Body, func(event string, data []byte) {
        w.Write(data)
        w.Write([]byte("\n\n"))
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    })
}
```

- [ ] **Step 5: 添加辅助函数**

```go
// internal/server/server.go (添加辅助函数)

// convertBackendError 转换后端错误为 OpenAI 格式
func convertBackendError(backend string, body []byte) string {
    // 简单实现：直接返回后端错误消息
    // 可以进一步解析并格式化
    return string(body)
}

// writeOpenAIError 写入 OpenAI 格式错误
func (s *Server) writeOpenAIError(w http.ResponseWriter, code int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    
    type OpenAIError struct {
        Error struct {
            Message string `json:"message"`
            Type    string `json:"type"`
            Code    string `json:"code,omitempty"`
        } `json:"error"`
    }
    
    err := OpenAIError{}
    err.Error.Message = message
    err.Error.Type = errorCodeMap[code]
    err.Error.Code = errorCodeMap[code]
    
    json.NewEncoder(w).Encode(err)
}

// errorCodeMap HTTP 状态码到错误类型映射
var errorCodeMap = map[int]string{
    400: "invalid_request_error",
    401: "authentication_error",
    403: "permission_error",
    404: "not_found_error",
    429: "rate_limit_error",
    500: "internal_server_error",
    502: "service_unavailable_error",
    503: "service_unavailable_error",
}
```

- [ ] **Step 6: 运行编译检查**

```bash
go build ./...
```
Expected: 编译成功

- [ ] **Step 7: 提交**

```bash
git add internal/server/server.go
git commit -m "feat: add OpenAI protocol route handler"
```

---

## Chunk 3: 集成测试

### Task 4: 创建端到端集成测试

**Files:**
- Create: `internal/server/server_openai_test.go`

- [ ] **Step 1: 创建测试文件**

```go
// internal/server/server_openai_test.go
package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/claude-projetc/llm-proxy/internal/config"
    "github.com/claude-projetc/llm-proxy/internal/logger"
    "github.com/claude-projetc/llm-proxy/internal/router"
)

func TestServer_OpenAIRequest_ToOpenAIBackend(t *testing.T) {
    // 创建测试服务器
    mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{
            "id": "chatcmpl-123",
            "model": "gpt-4",
            "choices": [{
                "message": {"role": "assistant", "content": "Hello!"}
            }],
            "usage": {"prompt_tokens": 10, "completion_tokens": 5}
        }`))
    }))
    defer mockBackend.Close()

    cfg := &config.Config{
        Server: config.ServerConfig{Listen: ":8080"},
        Logging: config.LoggingConfig{Format: "text", Level: "info"},
        Routes: []config.RouteConfig{{
            APIKey:      "sk-test-key",
            Backend:     "openai",
            BackendKey:  "test-key",
            Timeout:     30000000000, // 30s
        }},
        Backends: config.BackendsConfig{
            OpenAI: config.BackendConfig{BaseURL: mockBackend.URL},
        },
    }

    r := router.New(cfg.Routes)
    log := logger.New(logger.TEXT, logger.INFO)
    srv := New(cfg, r, log)

    // 创建 OpenAI 格式请求
    reqBody := []byte(`{
        "model": "gpt-4",
        "messages": [{"role": "user", "content": "Hi"}]
    }`)

    req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
    req.Header.Set("Authorization", "Bearer sk-test-key")
    req.Header.Set("Content-Type", "application/json")

    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
    assert.Contains(t, rr.Body.String(), "Hello!")
}

func TestServer_OpenAIRequest_InvalidAPIKey(t *testing.T) {
    cfg := &config.Config{
        Server: config.ServerConfig{Listen: ":8080"},
        Logging: config.LoggingConfig{Format: "text", Level: "info"},
        Routes: []config.RouteConfig{},
    }

    r := router.New(cfg.Routes)
    log := logger.New(logger.TEXT, logger.INFO)
    srv := New(cfg, r, log)

    reqBody := []byte(`{"model": "gpt-4", "messages": []}`)
    req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
    req.Header.Set("Authorization", "Bearer invalid-key")

    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusUnauthorized, rr.Code)
    assert.Contains(t, rr.Body.String(), "Invalid API key")
}

func TestServer_OpenAIRequest_AnthropicBackend(t *testing.T) {
    // 测试 OpenAI 请求转发到 Anthropic 后端
    mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Contains(t, r.URL.Path, "/v1/messages")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{
            "id": "msg_123",
            "model": "claude-3",
            "content": [{"type": "text", "text": "Hello from Claude!"}],
            "usage": {"input_tokens": 10, "output_tokens": 5}
        }`))
    }))
    defer mockBackend.Close()

    cfg := &config.Config{
        Routes: []config.RouteConfig{{
            APIKey:      "sk-test-key",
            Backend:     "anthropic",
            BackendKey:  "test-key",
            Timeout:     30000000000,
        }},
        Backends: config.BackendsConfig{
            Anthropic: config.BackendConfig{BaseURL: mockBackend.URL},
        },
    }

    r := router.New(cfg.Routes)
    log := logger.New(logger.TEXT, logger.INFO)
    srv := New(cfg, r, log)

    reqBody := []byte(`{"model": "claude-3", "messages": [{"role": "user", "content": "Hi"}]}`)
    req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
    req.Header.Set("Authorization", "Bearer sk-test-key")

    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)

    assert.Equal(t, http.StatusOK, rr.Code)
    // 验证响应为 OpenAI 格式
    var resp map[string]interface{}
    json.Unmarshal(rr.Body.Bytes(), &resp)
    assert.Contains(t, resp, "choices")
}
```

- [ ] **Step 2: 运行集成测试**

```bash
go test ./internal/server/... -v -run TestServer_OpenAI
```
Expected: PASS (3 tests)

- [ ] **Step 3: 提交**

```bash
git add internal/server/server_openai_test.go
git commit -m "test: add OpenAI protocol integration tests"
```

---

## Chunk 4: 文档更新

### Task 5: 更新 README 和配置示例

**Files:**
- Modify: `README.md`
- Modify: `config.example.yaml`

- [ ] **Step 1: 更新 README.md - 添加 OpenAI 入口说明**

找到 "## 使用示例" 或类似章节，添加:

```markdown
### OpenAI 协议兼容

服务端同时支持 OpenAI 协议 (`/v1/chat/completions`) 和 Anthropic 协议 (`/v1/messages`)。

**OpenAI SDK 调用示例：**

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080",
    api_key="sk-client-1"  # 配置的客户端 API Key
)

response = client.chat.completions.create(
    model="claude-3-opus",  # 模型名透传到后端
    messages=[
        {"role": "user", "content": "Hello!"}
    ]
)
print(response.choices[0].message.content)
```

**curl 调用示例：**

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-client-1" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-opus",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```
```

- [ ] **Step 2: 更新 config.example.yaml**

在文件末尾添加:

```yaml
# ======================
# 协议说明
# ======================
# 服务端同时支持两种协议入口：
# - Anthropic 协议：POST /v1/messages
# - OpenAI 协议：POST /v1/chat/completions
#
# 两种入口可以混合使用，客户端可根据 SDK 选择协议：
# - 使用 Anthropic SDK 时调用 /v1/messages
# - 使用 OpenAI SDK 时调用 /v1/chat/completions
#
# 模型名直接透传到后端，无需映射配置。
# 例如：客户端发送 "claude-3-opus" -> 后端接收 "claude-3-opus"
```

- [ ] **Step 3: 提交**

```bash
git add README.md config.example.yaml
git commit -m "docs: add OpenAI protocol usage examples"
```

---

## 验收标准

完成所有任务后验证：

- [ ] OpenAI SDK 可直接调用 `/v1/chat/completions`
- [ ] 支持转发到 OpenAI/Anthropic/Gemini 后端
- [ ] 错误响应符合 OpenAI 格式
- [ ] 单元测试通过率 100%
- [ ] 集成测试通过率 100%
- [ ] README 包含 OpenAI 调用示例

---

## 依赖和前置条件

**Go 版本：** 1.21+

**测试依赖：**
```bash
go get github.com/stretchr/testify/assert
```

**编译检查：**
```bash
go build ./...
go test ./...
```
