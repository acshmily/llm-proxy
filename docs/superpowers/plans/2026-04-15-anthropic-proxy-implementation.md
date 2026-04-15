# Anthropic 协议代理实现计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建一个 Go 语言网络代理，将 Anthropic API 协议转写为 OpenAI、Claude、Gemini 后端协议。

**Architecture:** 统一中间格式架构，包含 HTTP Server、Auth Router、Anthropic Parser、Protocol Converters、Stream Handler、Connection Pool 和 Logger 组件。

**Tech Stack:** Go 1.21+, net/http, gopkg.in/yaml.v3, sync, io, encoding/json, SSE streaming

---

## Chunk 1: 项目骨架和基础类型

### Task 1: 初始化 Go 模块和项目结构

**Files:**
- Create: `go.mod`
- Create: `pkg/types/message.go`
- Create: `internal/config/config.go`
- Create: `internal/logger/logger.go`
- Create: `config.yaml`
- Create: `cmd/proxy/main.go`

- [ ] **Step 1: 创建 go.mod**

```bash
cd /Users/r2d2/Documents/claude-projetc/proxy-gemini-go
go mod init github.com/claude-projetc/proxy-gemini-go
```

- [ ] **Step 2: 创建 pkg/types/message.go - 统一中间格式**

```go
package types

// UnifiedMessage 统一中间格式消息
type UnifiedMessage struct {
    Model    string         `json:"model"`
    Messages []MessageRole  `json:"messages"`
    Stream   bool           `json:"stream"`
    MaxTokens int           `json:"max_tokens,omitempty"`
    Temperature float64     `json:"temperature,omitempty"`
    TopP     float64        `json:"top_p,omitempty"`
    StopSequences []string  `json:"stop_sequences,omitempty"`
}

// MessageRole 单条消息角色
type MessageRole struct {
    Role    string `json:"role"`    // "user" | "assistant"
    Content string `json:"content"`
}

// UnifiedResponse 统一响应格式
type UnifiedResponse struct {
    ID      string   `json:"id"`
    Model   string   `json:"model"`
    Content []ContentBlock `json:"content"`
    Role    string   `json:"role"`
    Usage   Usage    `json:"usage"`
}

// ContentBlock 内容块
type ContentBlock struct {
    Type string `json:"type"`  // "text"
    Text string `json:"text"`
}

// Usage Token 使用统计
type Usage struct {
    InputTokens  int `json:"input_tokens"`
    OutputTokens int `json:"output_tokens"`
}

// APIError API 错误格式
type APIError struct {
    Type    string `json:"type"`
    Message string `json:"message"`
}
```

- [ ] **Step 3: 创建 internal/config/config.go - 配置结构**

```go
package config

import (
    "os"
    "time"
    "gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
    Server   ServerConfig   `yaml:"server"`
    Logging  LoggingConfig  `yaml:"logging"`
    Routes   []RouteConfig  `yaml:"routes"`
    Backends BackendsConfig `yaml:"backends"`
    Retry    RetryConfig    `yaml:"retry"`
}

type ServerConfig struct {
    Listen string `yaml:"listen"`
}

type LoggingConfig struct {
    Format string `yaml:"format"` // "json" or "text"
    Level  string `yaml:"level"`
}

type RouteConfig struct {
    APIKey    string        `yaml:"api_key"`
    Backend   string        `yaml:"backend"`
    BackendKey string       `yaml:"backend_api_key"`
    Timeout   time.Duration `yaml:"timeout"`
}

type BackendsConfig struct {
    OpenAI  BackendConfig `yaml:"openai"`
    Claude  BackendConfig `yaml:"claude"`
    Gemini  BackendConfig `yaml:"gemini"`
}

type BackendConfig struct {
    BaseURL string `yaml:"base_url"`
}

type RetryConfig struct {
    MaxAttempts int   `yaml:"max_attempts"`
    RetryErrors []int `yaml:"retry_errors"`
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }
    return &cfg, nil
}
```

- [ ] **Step 4: 安装 YAML 依赖**

```bash
go get gopkg.in/yaml.v3
```

- [ ] **Step 5: 创建 internal/logger/logger.go - 日志组件**

```go
package logger

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "sync"
    "time"
)

type Level int

const (
    DEBUG Level = iota
    INFO
    WARN
    ERROR
)

type Format int

const (
    TEXT Format = iota
    JSON
)

type Logger struct {
    mu     sync.Mutex
    level  Level
    format Format
}

type LogEntry struct {
    Time      time.Time `json:"time"`
    Level     string    `json:"level"`
    Message   string    `json:"message"`
    RequestID string    `json:"request_id,omitempty"`
    LatencyMs int64     `json:"latency_ms,omitempty"`
    StatusCode int      `json:"status_code,omitempty"`
    InputTokens int     `json:"input_tokens,omitempty"`
    OutputTokens int    `json:"output_tokens,omitempty"`
    Backend   string    `json:"backend,omitempty"`
}

func New(format Format, level Level) *Logger {
    return &Logger{format: format, level: level}
}

func (l *Logger) Info(msg string, fields ...LogField) {
    l.log(INFO, msg, fields...)
}

func (l *Logger) Warn(msg string, fields ...LogField) {
    l.log(WARN, msg, fields...)
}

func (l *Logger) Error(msg string, fields ...LogField) {
    l.log(ERROR, msg, fields...)
}

type LogField struct {
    Key   string
    Value interface{}
}

func (l *Logger) log(level Level, msg string, fields ...LogField) {
    if level < l.level {
        return
    }
    l.mu.Lock()
    defer l.mu.Unlock()

    if l.format == JSON {
        entry := map[string]interface{}{
            "time":  time.Now().UTC().Format(time.RFC3339),
            "level": levelToString(level),
            "msg":   msg,
        }
        for _, f := range fields {
            entry[f.Key] = f.Value
        }
        data, _ := json.Marshal(entry)
        fmt.Fprintln(os.Stderr, string(data))
    } else {
        fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", 
            time.Now().Format(time.RFC3339), 
            levelToString(level), 
            msg)
    }
}

func levelToString(l Level) string {
    switch l {
    case DEBUG: return "DEBUG"
    case INFO: return "INFO"
    case WARN: return "WARN"
    case ERROR: return "ERROR"
    default: return "UNKNOWN"
    }
}
```

- [ ] **Step 6: 创建 config.yaml 配置文件模板**

```yaml
server:
  listen: :8080

logging:
  format: json
  level: info

routes:
  - api_key: "sk-client-1"
    backend: "openai"
    backend_api_key: "sk-openai-xxx"
    timeout: 60s
    
  - api_key: "sk-client-2"
    backend: "claude"
    backend_api_key: "sk-claude-xxx"
    timeout: 120s
    
  - api_key: "sk-client-3"
    backend: "gemini"
    backend_api_key: "AIzaSyD-xxx"
    timeout: 90s

backends:
  openai:
    base_url: "https://api.openai.com/v1"
    
  claude:
    base_url: "https://api.anthropic.com/v1"
    
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"

retry:
  max_attempts: 3
  retry_errors: [429, 503, 504]
```

- [ ] **Step 7: 创建 cmd/proxy/main.go - 程序入口**

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "log"
    "github.com/claude-projetc/proxy-gemini-go/internal/config"
    "github.com/claude-projetc/proxy-gemini-go/internal/logger"
)

func main() {
    configPath := flag.String("config", "config.yaml", "path to config file")
    flag.Parse()

    cfg, err := config.LoadConfig(*configPath)
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    logFormat := logger.TEXT
    if cfg.Logging.Format == "json" {
        logFormat = logger.JSON
    }

    logLevel := logger.INFO
    switch cfg.Logging.Level {
    case "debug":
        logLevel = logger.DEBUG
    case "warn":
        logLevel = logger.WARN
    case "error":
        logLevel = logger.ERROR
    }

    log := logger.New(logFormat, logLevel)
    log.Info("Starting Anthropic Protocol Proxy", 
        logger.LogField{Key: "listen", Value: cfg.Server.Listen})

    // TODO: Initialize router and start server
    fmt.Println("Proxy initialized successfully")
    os.Exit(0)
}
```

- [ ] **Step 8: 编译验证**

```bash
go build -o proxy ./cmd/proxy
./proxy -config config.yaml
```

预期输出：`Proxy initialized successfully`

- [ ] **Step 9: 提交**

```bash
git add go.mod go.sum pkg/types/message.go internal/config/config.go internal/logger/logger.go config.yaml cmd/proxy/main.go docs/
git commit -m "feat: 项目骨架和基础类型定义"
```

---

## Chunk 2: 路由和认证组件

### Task 2: 实现 Auth Router

**Files:**
- Create: `internal/router/router.go`
- Test: `internal/router/router_test.go`

- [ ] **Step 1: 编写路由测试**

```go
package router

import (
    "testing"
    "time"
    "github.com/claude-projetc/proxy-gemini-go/internal/config"
)

func TestRouter_FindRoute(t *testing.T) {
    routes := []config.RouteConfig{
        {APIKey: "sk-test-1", Backend: "openai", BackendKey: "sk-openai-xxx", Timeout: 60 * time.Second},
        {APIKey: "sk-test-2", Backend: "claude", BackendKey: "sk-claude-xxx", Timeout: 120 * time.Second},
    }
    
    r := New(routes)
    
    t.Run("found route", func(t *testing.T) {
        route, found := r.FindRoute("sk-test-1")
        if !found {
            t.Fatal("Expected to find route")
        }
        if route.Backend != "openai" {
            t.Errorf("Expected backend 'openai', got '%s'", route.Backend)
        }
    })
    
    t.Run("not found route", func(t *testing.T) {
        _, found := r.FindRoute("sk-unknown")
        if found {
            t.Error("Expected not to find route")
        }
    })
}
```

- [ ] **Step 2: 实现路由组件**

```go
package router

import (
    "sync"
    "github.com/claude-projetc/proxy-gemini-go/internal/config"
)

type Route struct {
    Backend    string
    BackendKey string
    Timeout    time.Duration
}

type Router struct {
    mu     sync.RWMutex
    routes map[string]*Route
}

func New(routes []config.RouteConfig) *Router {
    r := &Router{
        routes: make(map[string]*Route),
    }
    for _, rc := range routes {
        r.routes[rc.APIKey] = &Route{
            Backend:    rc.Backend,
            BackendKey: rc.BackendKey,
            Timeout:    rc.Timeout,
        }
    }
    return r
}

func (r *Router) FindRoute(apiKey string) (*Route, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    route, found := r.routes[apiKey]
    return route, found
}
```

- [ ] **Step 3: 运行测试**

```bash
go test ./internal/router -v
```

- [ ] **Step 4: 提交**

```bash
git add internal/router/router.go internal/router/router_test.go
git commit -m "feat: 实现认证路由组件"
```

---

## Chunk 3: 协议转换核心

### Task 3: Anthropic 协议解析器

**Files:**
- Create: `internal/protocol/anthropic/parser.go`
- Test: `internal/protocol/anthropic/parser_test.go`

- [ ] **Step 1: 定义 Anthropic 请求格式**

```go
package anthropic

import (
    "encoding/json"
    "github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

// Request Anthropic API 请求
type Request struct {
    Model         string    `json:"model"`
    Messages      []Message `json:"messages"`
    Stream        bool      `json:"stream,omitempty"`
    MaxTokens     int       `json:"max_tokens,omitempty"`
    Temperature   float64   `json:"temperature,omitempty"`
    TopP          float64   `json:"top_p,omitempty"`
    StopSequences []string  `json:"stop_sequences,omitempty"`
}

type Message struct {
    Role    string    `json:"role"`
    Content []Content `json:"content"`
}

type Content struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

// ParseRequest 解析为统一格式
func ParseRequest(data []byte) (*types.UnifiedMessage, error) {
    var req Request
    if err := json.Unmarshal(data, &req); err != nil {
        return nil, err
    }
    
    unified := &types.UnifiedMessage{
        Model:    req.Model,
        Stream:   req.Stream,
        MaxTokens: req.MaxTokens,
        Temperature: req.Temperature,
        TopP:      req.TopP,
        StopSequences: req.StopSequences,
    }
    
    for _, msg := range req.Messages {
        var content string
        for _, c := range msg.Content {
            if c.Type == "text" {
                content += c.Text
            }
        }
        unified.Messages = append(unified.Messages, types.MessageRole{
            Role:    msg.Role,
            Content: content,
        })
    }
    
    return unified, nil
}
```

- [ ] **Step 2: 编写解析测试**

```go
package anthropic

import (
    "testing"
)

func TestParseRequest(t *testing.T) {
    input := `{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`
    
    unified, err := ParseRequest([]byte(input))
    if err != nil {
        t.Fatalf("ParseRequest failed: %v", err)
    }
    
    if unified.Model != "claude-3" {
        t.Errorf("Expected model 'claude-3', got '%s'", unified.Model)
    }
    if len(unified.Messages) != 1 {
        t.Errorf("Expected 1 message, got %d", len(unified.Messages))
    }
    if unified.Messages[0].Content != "Hello" {
        t.Errorf("Expected content 'Hello', got '%s'", unified.Messages[0].Content)
    }
}
```

- [ ] **Step 3: 运行测试并提交**

```bash
go test ./internal/protocol/anthropic -v
git add internal/protocol/anthropic/parser.go internal/protocol/anthropic/parser_test.go
git commit -m "feat: 实现 Anthropic 协议解析器"
```

---

### Task 4: OpenAI 协议转换器

**Files:**
- Create: `internal/protocol/openai/converter.go`
- Test: `internal/protocol/openai/converter_test.go`

- [ ] **Step 1: 实现 OpenAI 转换器**

```go
package openai

import (
    "bytes"
    "encoding/json"
    "github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

// OpenAIRequest OpenAI API 请求格式
type OpenAIRequest struct {
    Model    string       `json:"model"`
    Messages []ChatMessage `json:"messages"`
    Stream   bool         `json:"stream,omitempty"`
    MaxTokens int         `json:"max_tokens,omitempty"`
    Temperature float64   `json:"temperature,omitempty"`
    TopP     float64      `json:"top_p,omitempty"`
    Stop     []string     `json:"stop,omitempty"`
}

type ChatMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

// Convert 统一格式 -> OpenAI 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
    req := &OpenAIRequest{
        Model:    modelOverride,
        Messages: make([]ChatMessage, len(um.Messages)),
        Stream:   um.Stream,
        MaxTokens: um.MaxTokens,
        Temperature: um.Temperature,
        TopP:     um.TopP,
        Stop:     um.StopSequences,
    }
    
    for i, msg := range um.Messages {
        req.Messages[i] = ChatMessage{
            Role:    msg.Role,
            Content: msg.Content,
        }
    }
    
    return json.Marshal(req)
}

// ParseResponse 解析 OpenAI 响应为统一格式
func ParseResponse(data []byte) (*types.UnifiedResponse, error) {
    var resp OpenAIResponse
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, err
    }
    
    unified := &types.UnifiedResponse{
        ID:    resp.ID,
        Model: resp.Model,
        Content: []types.ContentBlock{
            {Type: "text", Text: resp.Choices[0].Message.Content},
        },
        Role: "assistant",
        Usage: types.Usage{
            InputTokens:  resp.Usage.PromptTokens,
            OutputTokens: resp.Usage.CompletionTokens,
        },
    }
    
    return unified, nil
}

type OpenAIResponse struct {
    ID      string   `json:"id"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   Usage    `json:"usage"`
}

type Choice struct {
    Message ChatMessage `json:"message"`
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
}
```

- [ ] **Step 2: 编写转换测试并提交**

```bash
go test ./internal/protocol/openai -v
git add internal/protocol/openai/converter.go internal/protocol/openai/converter_test.go
git commit -m "feat: 实现 OpenAI 协议转换器"
```

---

### Task 5: Claude 协议转换器

**Files:**
- Create: `internal/protocol/claude/converter.go`
- Test: `internal/protocol/claude/converter_test.go`

- [ ] **Step 1: 实现 Claude 转换器**

```go
package claude

import (
    "encoding/json"
    "github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

// Convert 统一格式 -> Claude 格式 (原生)
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
    // Claude 原生格式与 Anthropic 几乎相同
    req := map[string]interface{}{
        "model":   modelOverride,
        "messages": um.Messages,
        "stream":  um.Stream,
    }
    if um.MaxTokens > 0 {
        req["max_tokens"] = um.MaxTokens
    }
    if um.Temperature > 0 {
        req["temperature"] = um.Temperature
    }
    return json.Marshal(req)
}

// ParseResponse 解析 Claude 响应
func ParseResponse(data []byte) (*types.UnifiedResponse, error) {
    var resp map[string]interface{}
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, err
    }
    
    var content []types.ContentBlock
    if contentArr, ok := resp["content"].([]interface{}); ok {
        for _, c := range contentArr {
            if cm, ok := c.(map[string]interface{}); ok {
                if text, ok := cm["text"].(string); ok {
                    content = append(content, types.ContentBlock{Type: "text", Text: text})
                }
            }
        }
    }
    
    usage := types.Usage{}
    if usageMap, ok := resp["usage"].(map[string]interface{}); ok {
        if it, ok := usageMap["input_tokens"].(float64); ok {
            usage.InputTokens = int(it)
        }
        if ot, ok := usageMap["output_tokens"].(float64); ok {
            usage.OutputTokens = int(ot)
        }
    }
    
    return &types.UnifiedResponse{
        ID:      getString(resp, "id"),
        Model:   getString(resp, "model"),
        Content: content,
        Role:    "assistant",
        Usage:   usage,
    }, nil
}

func getString(m map[string]interface{}, key string) string {
    if v, ok := m[key].(string); ok {
        return v
    }
    return ""
}
```

- [ ] **Step 2: 提交**

```bash
git add internal/protocol/claude/converter.go internal/protocol/claude/converter_test.go
git commit -m "feat: 实现 Claude 协议转换器"
```

---

### Task 6: Gemini 协议转换器

**Files:**
- Create: `internal/protocol/gemini/converter.go`
- Test: `internal/protocol/gemini/converter_test.go`

- [ ] **Step 1: 实现 Gemini 转换器**

```go
package gemini

import (
    "encoding/json"
    "fmt"
    "github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

// Convert 统一格式 -> Gemini 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
    // Gemini 使用 contents 数组
    contents := make([]map[string]interface{}, len(um.Messages))
    for i, msg := range um.Messages {
        contents[i] = map[string]interface{}{
            "role": msg.Role,
            "parts": []map[string]string{
                {"text": msg.Content},
            },
        }
    }
    
    req := map[string]interface{}{
        "contents": contents,
    }
    
    if um.Temperature > 0 {
        req["generationConfig"] = map[string]interface{}{
            "temperature": um.Temperature,
        }
    }
    
    return json.Marshal(req)
}

// ParseResponse 解析 Gemini 响应
func ParseResponse(data []byte) (*types.UnifiedResponse, error) {
    var resp GeminiResponse
    if err := json.Unmarshal(data, &resp); err != nil {
        return nil, err
    }
    
    var content []types.ContentBlock
    if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
        content = append(content, types.ContentBlock{
            Type: "text",
            Text: resp.Candidates[0].Content.Parts[0].Text,
        })
    }
    
    return &types.UnifiedResponse{
        ID:      fmt.Sprintf("gemini-%d", len(resp.Candidates)),
        Model:   "gemini-pro",
        Content: content,
        Role:    "assistant",
    }, nil
}

type GeminiResponse struct {
    Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
    Content Content `json:"content"`
}

type Content struct {
    Parts []Part `json:"parts"`
}

type Part struct {
    Text string `json:"text"`
}
```

- [ ] **Step 2: 提交**

```bash
git add internal/protocol/gemini/converter.go internal/protocol/gemini/converter_test.go
git commit -m "feat: 实现 Gemini 协议转换器"
```

---

## Chunk 4: HTTP 服务和流式处理

### Task 7: HTTP Server 和 SSE 流式处理

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/stream/sse.go`

- [ ] **Step 1: 实现 SSE 流式处理器**

```go
package stream

import (
    "bufio"
    "bytes"
    "fmt"
    "io"
)

// SSEEvent SSE 事件
type SSEEvent struct {
    Event string
    Data  string
}

// ParseSSE 解析 SSE 流
func ParseSSE(r io.Reader, handler func(event string, data []byte)) error {
    scanner := bufio.NewScanner(r)
    var buf bytes.Buffer
    
    for scanner.Scan() {
        line := scanner.Text()
        
        if line == "" {
            // 空行表示事件结束
            if buf.Len() > 0 {
                handler("", buf.Bytes())
                buf.Reset()
            }
            continue
        }
        
        if len(line) > 6 && line[:6] == "data: " {
            buf.WriteString(line[6:])
        }
    }
    
    return scanner.Err()
}

// WriteSSEEvent 写入 SSE 事件
func WriteSSEEvent(w io.Writer, event string, data []byte) error {
    if event != "" {
        if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
            return err
        }
    }
    if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
        return err
    }
    return nil
}
```

- [ ] **Step 2: 实现 HTTP Server**

```go
package server

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/http/httputil"
    "time"
    
    "github.com/claude-projetc/proxy-gemini-go/internal/config"
    "github.com/claude-projetc/proxy-gemini-go/internal/logger"
    "github.com/claude-projetc/proxy-gemini-go/internal/protocol/anthropic"
    "github.com/claude-projetc/proxy-gemini-go/internal/protocol/openai"
    "github.com/claude-projetc/proxy-gemini-go/internal/protocol/claude"
    "github.com/claude-projetc/proxy-gemini-go/internal/protocol/gemini"
    "github.com/claude-projetc/proxy-gemini-go/internal/router"
    "github.com/claude-projetc/proxy-gemini-go/internal/stream"
    "github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

type Server struct {
    cfg    *config.Config
    router *router.Router
    log    *logger.Logger
    client *http.Client
}

func New(cfg *config.Config, r *router.Router, log *logger.Logger) *Server {
    return &Server{
        cfg:    cfg,
        router: r,
        log:    log,
        client: &http.Client{},
    }
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
    
    // 解析 Anthropic 请求
    unified, err := anthropic.ParseRequest(body)
    if err != nil {
        s.writeError(w, http.StatusBadRequest, "Invalid request format")
        return
    }
    
    // 选择后端转换器
    var backendURL string
    var reqBody []byte
    var modelOverride string
    
    switch route.Backend {
    case "openai":
        backendURL = s.cfg.Backends.OpenAI.BaseURL + "/chat/completions"
        reqBody, _ = openai.Convert(unified, "gpt-4")
    case "claude":
        backendURL = s.cfg.Backends.Claude.BaseURL + "/messages"
        reqBody, _ = claude.Convert(unified, "claude-3-opus-20240229")
    case "gemini":
        backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/gemini-pro:generateContent"
        reqBody, _ = gemini.Convert(unified, "")
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
    
    // 设置后端认证
    backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
    backendReq.Header.Set("Content-Type", "application/json")
    
    // 执行请求
    ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
    defer cancel()
    
    resp, err := s.client.Do(backendReq.WithContext(ctx))
    if err != nil {
        s.writeError(w, http.StatusBadGateway, "Backend request failed")
        return
    }
    defer resp.Body.Close()
    
    latency := time.Since(start).Milliseconds()
    
    // 处理响应
    if unified.Stream {
        s.handleStream(w, resp, route.Backend)
    } else {
        s.handleNonStream(w, resp, route.Backend, latency, start)
    }
}

func (s *Server) handleStream(w http.ResponseWriter, resp *http.Response, backend string) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    stream.ParseSSE(resp.Body, func(event string, data []byte) {
        w.Write(data)
        w.Write([]byte("\n\n"))
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    })
}

func (s *Server) handleNonStream(w http.ResponseWriter, resp *http.Response, backend string, latency int64, start time.Time) {
    body, _ := io.ReadAll(resp.Body)
    
    if resp.StatusCode != http.StatusOK {
        s.writeError(w, resp.StatusCode, string(body))
        return
    }
    
    // 转换响应为 Anthropic 格式
    var unified *types.UnifiedResponse
    var err error
    
    switch backend {
    case "openai":
        unified, err = openai.ParseResponse(body)
    case "claude":
        unified, err = claude.ParseResponse(body)
    case "gemini":
        unified, err = gemini.ParseResponse(body)
    }
    
    if err != nil {
        s.writeError(w, http.StatusInternalServerError, "Failed to parse response")
        return
    }
    
    // 记录日志
    s.log.Info("Request completed",
        logger.LogField{Key: "latency_ms", Value: latency},
        logger.LogField{Key: "status_code", Value: resp.StatusCode},
        logger.LogField{Key: "input_tokens", Value: unified.Usage.InputTokens},
        logger.LogField{Key: "output_tokens", Value: unified.Usage.OutputTokens},
        logger.LogField{Key: "backend", Value: backend},
    )
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(unified)
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(types.APIError{
        Type:    "error",
        Message: msg,
    })
}

func extractBearerToken(auth string) string {
    if len(auth) > 7 && auth[:7] == "Bearer " {
        return auth[7:]
    }
    return ""
}
```

- [ ] **Step 3: 更新 main.go 启动服务器**

```go
// 在 cmd/proxy/main.go 中替换 TODO 部分
import (
    "net/http"
    "github.com/claude-projetc/proxy-gemini-go/internal/router"
    "github.com/claude-projetc/proxy-gemini-go/internal/server"
)

// ... 在加载配置和日志之后

r := router.New(cfg.Routes)
srv := server.New(cfg, r, log)

log.Info("Listening", logger.LogField{Key: "address", Value: cfg.Server.Listen})
if err := http.ListenAndServe(cfg.Server.Listen, srv); err != nil {
    log.Error("Server failed", logger.LogField{Key: "error", Value: err.Error()})
    os.Exit(1)
}
```

- [ ] **Step 4: 编译并测试**

```bash
go build -o proxy ./cmd/proxy
./proxy -config config.yaml
```

- [ ] **Step 5: 提交**

```bash
git add internal/server/server.go internal/stream/sse.go cmd/proxy/main.go
git commit -m "feat: 实现 HTTP 服务器和 SSE 流式处理"
```

---

## Chunk 5: 测试和文档

### Task 8: 集成测试

**Files:**
- Create: `test/integration/proxy_test.go`
- Create: `test/mock/backend.go`

- [ ] **Step 1: 创建 Mock 后端**

```go
package mock

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
)

type MockBackend struct {
    Server *httptest.Server
}

func NewMockBackend() *MockBackend {
    mb := &MockBackend{}
    mb.Server = httptest.NewServer(mb)
    return mb
}

func (mb *MockBackend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "id":      "test-123",
        "model":   "test-model",
        "choices": []map[string]interface{}{
            {"message": map[string]string{"role": "assistant", "content": "Hello from mock"}},
        },
        "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20},
    })
}

func (mb *MockBackend) Close() {
    mb.Server.Close()
}
```

- [ ] **Step 2: 创建集成测试**

```go
package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/claude-projetc/proxy-gemini-go/internal/config"
    "github.com/claude-projetc/proxy-gemini-go/internal/logger"
    "github.com/claude-projetc/proxy-gemini-go/internal/router"
    "github.com/claude-projetc/proxy-gemini-go/internal/server"
    "github.com/claude-projetc/proxy-gemini-go/test/mock"
)

func TestProxyEndToEnd(t *testing.T) {
    // 启动 Mock 后端
    mockBackend := mock.NewMockBackend()
    defer mockBackend.Close()
    
    // 创建配置
    cfg := &config.Config{
        Routes: []config.RouteConfig{
            {APIKey: "sk-test", Backend: "openai", BackendKey: "sk-mock", Timeout: 5 * time.Second},
        },
        Backends: config.BackendsConfig{
            OpenAI: config.BackendConfig{BaseURL: mockBackend.Server.URL},
        },
    }
    
    r := router.New(cfg.Routes)
    log := logger.New(logger.TEXT, logger.ERROR)
    srv := server.New(cfg, r, log)
    
    // 创建测试服务器
    testServer := httptest.NewServer(srv)
    defer testServer.Close()
    
    // 发送请求
    reqBody := `{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`
    req, _ := http.NewRequest("POST", testServer.URL+"/v1/messages", bytes.NewReader([]byte(reqBody)))
    req.Header.Set("Authorization", "Bearer sk-test")
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("Request failed: %v", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected status 200, got %d", resp.StatusCode)
    }
    
    var respBody map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&respBody)
    
    if respBody["content"] == nil {
        t.Error("Expected content in response")
    }
}
```

- [ ] **Step 3: 运行集成测试**

```bash
go test ./test/integration -v
```

- [ ] **Step 4: 提交**

```bash
git add test/integration/proxy_test.go test/mock/backend.go
git commit -m "test: 添加集成测试"
```

---

### Task 9: README 文档

**Files:**
- Create: `README.md`

- [ ] **Step 1: 创建 README**

```markdown
# Anthropic Protocol Proxy

将 Anthropic API 协议转写为多种后端协议（OpenAI、Claude、Gemini）的网络代理。

## 快速开始

### 安装

```bash
go build -o proxy ./cmd/proxy
```

### 配置

编辑 `config.yaml`：

```yaml
server:
  listen: :8080

routes:
  - api_key: "sk-client-1"
    backend: "openai"
    backend_api_key: "sk-openai-xxx"
    timeout: 60s

backends:
  openai:
    base_url: "https://api.openai.com/v1"
```

### 运行

```bash
./proxy -config config.yaml
```

### 使用示例

```bash
curl http://localhost:8080/v1/messages \
  -H "Authorization: Bearer sk-client-1" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

## 支持的端点

- `POST /v1/messages` - 聊天完成
- `GET /v1/models` - 获取模型列表
- `POST /v1/messages/count_tokens` - Token 计数

## 许可证

MIT
```

- [ ] **Step 2: 提交**

```bash
git add README.md
git commit -m "docs: 添加 README 文档"
```

---

## 完成检查

- [ ] 所有测试通过
- [ ] 代码已提交
- [ ] README 已更新
- [ ] 配置示例完整

---

**计划完成。准备执行？**
