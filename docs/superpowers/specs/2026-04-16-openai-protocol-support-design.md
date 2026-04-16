# OpenAI 协议支持设计文档

**日期：** 2026-04-16  
**状态：** 待实现

---

## 1. 概述

为 llm-proxy 服务端增加 OpenAI 协议入口支持，使客户端可以使用 OpenAI SDK 调用任意后端（OpenAI/Anthropic/Gemini）。

### 1.1 设计目标

- 保留现有 Anthropic 协议入口（`/v1/messages`）
- 新增 OpenAI 协议入口（`/v1/chat/completions`）
- 支持智能协议转换：OpenAI 请求 → 任意后端
- 响应格式与客户端协议一致

### 1.2 非目标

- 不支持 Embedding 端点（后续迭代）
- 不支持 Function Calling（后续迭代）
- 不支持图像输入（后续迭代）

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────┐                    ┌─────────────────┐
│  Anthropic SDK  │                    │   OpenAI SDK    │
└────────┬────────┘                    └────────┬────────┘
         │                                      │
         │ /v1/messages                         │ /v1/chat/completions
         ▼                                      ▼
┌───────────────────────────────────────────────────────────┐
│                      llm-proxy                            │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  路由层：根据请求路径选择协议解析器                   │  │
│  │  /v1/messages     → anthropic.ParseRequest()        │  │
│  │  /v1/chat/completions → openai.ParseRequest()       │  │
│  └─────────────────────────────────────────────────────┘  │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  协议转换层：客户端格式 → Unified → 后端格式          │  │
│  └─────────────────────────────────────────────────────┘  │
│  ┌─────────────────────────────────────────────────────┐  │
│  │  响应转换层：后端格式 → Unified → 客户端格式          │  │
│  └─────────────────────────────────────────────────────┘  │
└───────────────────────────────────────────────────────────┘
         │                                      │
         │ Anthropic 格式                         │ OpenAI 格式
         ▼                                      ▼
┌─────────────────┐                    ┌─────────────────┐
│  Anthropic SDK  │                    │   OpenAI SDK    │
└─────────────────┘                    └─────────────────┘
```

### 2.2 请求处理流程

```
1. 接收请求 → 根据路径选择协议解析器
2. 解析请求 → Unified 中间格式
3. 路由查找 → 根据 API Key 确定后端
4. 协议转换 → Unified → 后端格式
5. 转发后端 → 获取响应
6. 响应转换 → 后端格式 → Unified → 客户端格式
7. 返回响应 → 流式/非流式处理
```

### 2.3 数据流

```
OpenAI 请求
    ↓
openai.ParseRequest() → types.UnifiedRequest
    ↓
根据后端选择转换器
    ↓
openai.Convert()  → OpenAI 后端
claude.Convert()  → Anthropic 后端
gemini.Convert()  → Gemini 后端
    ↓
后端响应
    ↓
parseResponse() → types.UnifiedResponse
    ↓
openai.BuildResponse() → OpenAI 格式响应
```

---

## 3. 组件设计

### 3.1 Unified 中间格式定义

**文件：** `pkg/types/unified.go`

```go
// UnifiedRequest OpenAI 请求的统一中间格式
type UnifiedRequest struct {
    Model             string        `json:"model"`
    Messages          []Message     `json:"messages"`
    Stream            bool          `json:"stream"`
    Temperature       float64       `json:"temperature,omitempty"`
    MaxTokens         int           `json:"max_tokens,omitempty"`
    TopP              float64       `json:"top_p,omitempty"`
    Stop              []string      `json:"stop,omitempty"`
    PresencePenalty   float64       `json:"presence_penalty,omitempty"`
    FrequencyPenalty  float64       `json:"frequency_penalty,omitempty"`
    User              string        `json:"user,omitempty"`       // 终端用户标识
    Seed              int           `json:"seed,omitempty"`       // 可复现性
}

// Message 统一消息格式
type Message struct {
    Role        string        `json:"role"`           // system/user/assistant
    Content     string        `json:"content"`        // 简单文本内容（兼容单字符串）
    ContentParts []ContentPart `json:"content_parts,omitempty"` // 多部分内容（预留）
    Name        string        `json:"name,omitempty"`
    Refusal     string        `json:"refusal,omitempty"`
    // 预留字段（后续 Function Calling 支持）
    ToolCalls   []ToolCall    `json:"tool_calls,omitempty"`
    ToolCallID  string        `json:"tool_call_id,omitempty"`
}

// ContentPart 多部分内容（支持图像等）
type ContentPart struct {
    Type     string    `json:"type"`      // "text" or "image_url"
    Text     string    `json:"text,omitempty"`
    ImageURL ImageURL  `json:"image_url,omitempty"`
}

// ImageURL 图像 URL 结构
type ImageURL struct {
    URL     string `json:"url"`
    Detail  string `json:"detail,omitempty"`  // "auto", "low", "high"
}

// ToolCall 工具调用结构（预留）
type ToolCall struct {
    ID       string     `json:"id"`
    Type     string     `json:"type"`  // "function"
    Function Function   `json:"function"`
}

// Function 函数调用结构
type Function struct {
    Name      string `json:"name"`
    Arguments string `json:"arguments"`
}

// UnifiedResponse 统一响应格式
type UnifiedResponse struct {
    ID        string    `json:"id"`
    Model     string    `json:"model"`
    Choices   []Choice  `json:"choices"`
    Created   int64     `json:"created"`
    Usage     Usage     `json:"usage"`
    SystemFingerprint string `json:"system_fingerprint,omitempty"`
}

// Choice 统一选择格式
type Choice struct {
    Index        int     `json:"index"`
    Message      Message `json:"message"`
    FinishReason string  `json:"finish_reason"`
}

// Usage 统一使用量格式
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}
```

**设计说明：**
- `Content` 字段兼容简单文本（单字符串）
- `ContentParts` 预留多模态支持（文本 + 图像）
- `ToolCalls` 和 `ToolCallID` 预留 Function Calling 支持
- 当前实现仅使用 `Content` 字段，其他字段为后续迭代预留

### 3.2 协议解析器（新增）

**文件：** `internal/protocol/openai/parser.go`

```go
// ParseRequest 解析 OpenAI Chat Completion 请求
// 返回错误类型：
// - ErrInvalidJSON: JSON 解析失败
// - ErrMissingField: 缺少必填字段
// - ErrInvalidValue: 字段值无效
func ParseRequest(body []byte) (*UnifiedRequest, error)

// Validate 验证请求有效性
func (r *UnifiedRequest) Validate() error
```

### 3.3 协议转换器（拆分职责）

**请求转换器：** `internal/protocol/openai/request_converter.go`

```go
// RequestConverter 请求转换器接口
type RequestConverter interface {
    // ToOpenAI 转换为 OpenAI 后端格式
    ToOpenAI(req *UnifiedRequest, model string) ([]byte, error)
    // ToAnthropic 转换为 Anthropic 后端格式
    ToAnthropic(req *UnifiedRequest, model string) ([]byte, error)
    // ToGemini 转换为 Gemini 后端格式
    ToGemini(req *UnifiedRequest, model string) ([]byte, error)
}

// 不支持的参数返回友好错误
var ErrUnsupportedParam = errors.New("unsupported parameter for this backend")
```

**响应转换器：** `internal/protocol/openai/response_converter.go`

```go
// ResponseConverter 响应转换器接口
type ResponseConverter interface {
    // FromOpenAI 从 OpenAI 响应转换
    FromOpenAI(body []byte) (*UnifiedResponse, error)
    // FromAnthropic 从 Anthropic 响应转换
    FromAnthropic(body []byte) (*UnifiedResponse, error)
    // FromGemini 从 Gemini 响应转换
    FromGemini(body []byte) (*UnifiedResponse, error)
    // BuildOpenAIResponse 构建 OpenAI 格式响应
    BuildOpenAIResponse(unified *UnifiedResponse) ([]byte, error)
}
```

### 3.4 服务器入口（修改）

**文件：** `internal/server/server.go`

**路由设计（使用 switch-case 提高可维护性）：**

```go
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
    case r.URL.Path == "/v1/messages" && r.Method == http.MethodPost:
        // 现有 Anthropic 端点
        s.serveAnthropicRequest(w, r)
    default:
        s.writeError(w, http.StatusNotFound, "Endpoint not found")
    }
}
```

### 3.5 错误处理

**统一错误格式（OpenAI 兼容）：**

```go
type APIError struct {
    Error struct {
        Message string `json:"message"`
        Type    string `json:"type"`
        Code    string `json:"code,omitempty"`
        Param   string `json:"param,omitempty"`  // 参数错误时指出字段
    } `json:"error"`
}

// 完整错误码映射
var errorCodeMap = map[int]string{
    400: "invalid_request_error",
    401: "authentication_error",
    403: "permission_error",
    404: "not_found_error",
    409: "conflict_error",
    422: "unprocessable_entity",
    429: "rate_limit_error",
    500: "internal_server_error",
    502: "bad_gateway_error",
    503: "service_unavailable_error",
    504: "gateway_timeout_error",
}

// 错误类型到 HTTP 状态码映射
var errorTypeToStatus = map[string]int{
    "invalid_request_error":  http.StatusBadRequest,
    "authentication_error":   http.StatusUnauthorized,
    "permission_error":       http.StatusForbidden,
    "not_found_error":        http.StatusNotFound,
    "rate_limit_error":       http.StatusTooManyRequests,
    "internal_server_error":  http.StatusInternalServerError,
    "service_unavailable_error": http.StatusServiceUnavailable,
}
```

**错误传播机制：**

```go
// 后端错误转换为统一错误
func convertBackendError(backend string, statusCode int, body []byte) *APIError {
    switch backend {
    case "openai":
        return parseOpenAIError(body)
    case "anthropic":
        return parseAnthropicError(body)
    case "gemini":
        return parseGeminiError(body)
    default:
        return &APIError{Error: struct {
            Message string `json:"message"`
            Type    string `json:"type"`
            Code    string `json:"code,omitempty"`
            Param   string `json:"param,omitempty"`
        }{
            Message: fmt.Sprintf("Backend %s error", backend),
            Type:    errorCodeMap[statusCode],
            Code:    fmt.Sprintf("BACKEND_%d", statusCode),
        }}
    }
}
```

---

## 4. API 设计

### 4.1 请求格式

**OpenAI Chat Completion 请求：**

```json
POST /v1/chat/completions
Authorization: Bearer sk-client-1
Content-Type: application/json

{
    "model": "claude-3-opus",
    "messages": [
        {"role": "system", "content": "You are helpful."},
        {"role": "user", "content": "Hello"}
    ],
    "stream": false,
    "temperature": 0.7,
    "max_tokens": 1024,
    "top_p": 1.0
}
```

### 4.2 响应格式

**非流式响应：**

```json
{
    "id": "chatcmpl-xxx",
    "object": "chat.completion",
    "created": 1234567890,
    "model": "claude-3-opus",
    "choices": [{
        "index": 0,
        "message": {
            "role": "assistant",
            "content": "Hello! How can I help you?"
        },
        "finish_reason": "stop"
    }],
    "usage": {
        "prompt_tokens": 10,
        "completion_tokens": 20,
        "total_tokens": 30
    }
}
```

**流式响应（SSE）：**

```
data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","choices":[{"delta":{"content":" world"},"finish_reason":null}]}

data: {"id":"chatcmpl-xxx","choices":[{"delta":{},"finish_reason":"stop"}]}

data: [DONE]
```

### 4.3 参数支持范围

| 参数 | 支持 | 说明 |
|------|------|------|
| `model` | ✅ | 映射到后端模型 |
| `messages` | ✅ | system/user/assistant |
| `stream` | ✅ | SSE 流式输出 |
| `temperature` | ✅ | 0.0-2.0 |
| `max_tokens` | ✅ | 最大输出 token |
| `top_p` | ✅ | 核采样 |
| `stop` | ✅ | 停止序列（数组） |
| `presence_penalty` | ✅ | 存在惩罚 |
| `frequency_penalty` | ✅ | 频率惩罚 |

### 4.4 finish_reason 映射

**后端 finish_reason 到 OpenAI 格式映射：**

| 后端 | 原始值 | OpenAI 映射 | 说明 |
|------|--------|-------------|------|
| OpenAI | `stop` | `stop` | 自然停止 |
| OpenAI | `length` | `length` | 达到 max_tokens |
| OpenAI | `tool_calls` | `tool_calls` | 函数调用 |
| OpenAI | `content_filter` | `content_filter` | 内容过滤 |
| Anthropic | `end_turn` | `stop` | 自然停止 |
| Anthropic | `max_tokens` | `length` | 达到最大 token |
| Anthropic | `stop_sequence` | `stop` | 遇到停止序列 |
| Gemini | `STOP` | `stop` | 自然停止 |
| Gemini | `MAX_TOKENS` | `length` | 达到最大 token |
| Gemini | `SAFETY` | `content_filter` | 安全检查 |
| Gemini | `RECITATION` | `content_filter` | 版权保护 |
| Gemini | `OTHER` | `stop` | 其他原因 |

### 4.5 错误码定义

**完整错误码类型：**

```go
type APIErrorCode string

const (
    // 通用错误
    ErrCodeInvalidRequestError   APIErrorCode = "invalid_request_error"
    ErrCodeAuthenticationError   APIErrorCode = "authentication_error"
    ErrCodePermissionError       APIErrorCode = "permission_error"
    ErrCodeNotFound              APIErrorCode = "not_found_error"
    ErrCodeRateLimitError        APIErrorCode = "rate_limit_error"
    ErrCodeInternalError         APIErrorCode = "internal_server_error"
    ErrCodeServiceUnavailable    APIErrorCode = "service_unavailable_error"
    
    // 具体场景错误码
    ErrCodeContextWindowExceeded APIErrorCode = "context_window_exceeded"
    ErrCodeModelNotFound         APIErrorCode = "model_not_found"
    ErrCodeInvalidAPIKey         APIErrorCode = "invalid_api_key"
    ErrCodeContentFilter         APIErrorCode = "content_filter"
)
```

**HTTP 状态码到错误类型映射：**

```go
var errorCodeMap = map[int]APIErrorCode{
    400: ErrCodeInvalidRequestError,
    401: ErrCodeAuthenticationError,
    403: ErrCodePermissionError,
    404: ErrCodeNotFound,
    409: ErrCodeInvalidRequestError,
    422: ErrCodeInvalidRequestError,
    429: ErrCodeRateLimitError,
    500: ErrCodeInternalError,
    502: ErrCodeServiceUnavailable,
    503: ErrCodeServiceUnavailable,
    504: ErrCodeServiceUnavailable,
}
```

**错误响应示例：**

```json
{
    "error": {
        "message": "invalid_api_key: The provided API key is invalid",
        "type": "authentication_error",
        "code": "invalid_api_key"
    }
}
```

---

## 5. 协议转换映射

### 5.1 OpenAI → Anthropic

| OpenAI | Anthropic | 说明 |
|--------|-----------|------|
| `messages[].role` | `content[].type` | system 消息特殊处理 |
| `messages[].content` | `content[].text` | 简单文本直接映射 |
| `system` (第一条) | `system` 参数 | 提取为独立 system 参数 |
| `max_tokens` | `max_tokens` | 直接映射 |
| `temperature` | `temperature` | 直接映射 |
| `top_p` | `top_p` | 直接映射 |
| `stop` | `stop_sequences` | 数组映射，最多 4 个 |
| `presence_penalty` | ❌ | 不支持，返回警告 |
| `frequency_penalty` | ❌ | 不支持，返回警告 |

**System 消息处理：**
```go
// Anthropic 支持 system 参数（独立于 messages）
// 转换逻辑：
// 1. 提取第一条 system 消息作为 system 参数
// 2. 后续 system 消息转换为 user 消息（带提示）
func extractSystemMessage(messages []Message) (string, []Message) {
    var system string
    var rest []Message
    for i, msg := range messages {
        if msg.Role == "system" && i == 0 {
            system = msg.Content
            continue
        }
        rest = append(rest, msg)
    }
    return system, rest
}
```

### 5.2 OpenAI → Gemini

| OpenAI | Gemini | 说明 |
|--------|--------|------|
| `messages[]` | `contents[]` | 角色映射 |
| `system` (第一条) | `system_instruction` | Gemini 1.5+ 支持 |
| `messages[].role: user` | `role: user` | 直接映射 |
| `messages[].role: assistant` | `role: model` | 重命名为 model |
| `messages[].role: system` | 跳过/合并 | 合并到 system_instruction |
| `temperature` | `generationConfig.temperature` | 嵌套结构 |
| `max_tokens` | `generationConfig.maxOutputTokens` | 嵌套结构 |
| `top_p` | `generationConfig.topP` | 嵌套结构 |
| `stop` | `generationConfig.stopSequences` | 最多 5 个 |
| `presence_penalty` | ❌ | 不支持 |
| `frequency_penalty` | ❌ | 不支持 |

**Gemini API 认证：**
```go
// Gemini 使用 URL 参数传递 API Key，而非 Authorization Header
// 转换逻辑：
backendURL = baseURL + "/models/" + model + ":generateContent" + "?key=" + apiKey

// 不需要设置 Authorization: Bearer header
```

### 5.3 OpenAI → OpenAI

直接透传，仅做必要验证：

```go
// 透传逻辑：
// 1. 验证 JSON 有效性
// 2. 验证必填字段（messages）
// 3. 直接转发请求体
func passthrough(req *UnifiedRequest) ([]byte, error) {
    return json.Marshal(req)
}
```

### 5.4 System 消息处理

**OpenAI 允许多条 system 消息，但 Anthropic 只支持一个 system 参数。**

**处理策略：合并所有 system 消息**

```go
// 提取并合并所有 system 消息
func extractSystemMessage(messages []Message) (string, []Message) {
    var systemParts []string
    var rest []Message
    
    for _, msg := range messages {
        if msg.Role == "system" {
            systemParts = append(systemParts, msg.Content)
            continue
        }
        rest = append(rest, msg)
    }
    
    // 合并所有 system 消息（用双换行分隔）
    system := strings.Join(systemParts, "\n\n")
    return system, rest
}
```

**各后端 System 消息处理：**

| 后端 | 处理方式 |
|------|----------|
| OpenAI | 保留在 messages 数组中（透传） |
| Anthropic | 提取为独立的 `system` 参数 |
| Gemini | 提取为 `system_instruction`（1.5+ 版本） |

**边界情况：**
- 无 system 消息：正常处理，system 为空
- 多条 system 消息：合并为一条（用 `\n\n` 分隔）
- system 消息在中间位置：提取后剩余消息保持顺序

### 5.5 不支持的参数处理

对于后端不支持的参数，返回友好警告而非错误：

```go
// 部分支持时返回警告信息
type Warning struct {
    Code    string `json:"code"`
    Message string `json:"message"`
    Param   string `json:"param"`
}

// 示例：presence_penalty 不支持时
warnings = append(warnings, Warning{
    Code:    "UNSUPPORTED_PARAM",
    Message: "presence_penalty is not supported for Anthropic backend",
    Param:   "presence_penalty",
})
// 请求继续处理，忽略不支持的参数
```

---

## 6. 测试策略

### 6.1 单元测试

| 文件 | 测试内容 | 覆盖率目标 |
|------|----------|-----------|
| `parser_test.go` | OpenAI 请求解析 | >90% |
| `request_converter_test.go` | 请求转换逻辑 | >85% |
| `response_converter_test.go` | 响应转换逻辑 | >85% |
| `error_handler_test.go` | 错误处理逻辑 | >90% |

**边界值测试：**
```go
// 空消息列表
func TestParseRequest_EmptyMessages(t *testing.T)

// 超长内容（超过后端限制）
func TestParseRequest_ContentTooLong(t *testing.T)

// 无效角色
func TestParseRequest_InvalidRole(t *testing.T)

// 无效 JSON
func TestParseRequest_InvalidJSON(t *testing.T)

// 缺失必填字段
func TestParseRequest_MissingModel(t *testing.T)
```

**参数边界测试：**
```go
// temperature 边界
func TestValidate_TemperatureOutOfRange(t *testing.T)  // <0 or >2

// max_tokens 边界
func TestValidate_MaxTokensNegative(t *testing.T)

// stop 数组长度
func TestValidate_StopSequencesTooMany(t *testing.T)  // >4 for Anthropic
```

### 6.2 集成测试

```go
// 测试 OpenAI 请求到各后端
func TestOpenAIRequest_ToOpenAIBackend(t *testing.T)
func TestOpenAIRequest_ToAnthropicBackend(t *testing.T)
func TestOpenAIRequest_ToGeminiBackend(t *testing.T)

// 测试流式响应
func TestOpenAI_StreamResponse(t *testing.T)
func TestOpenAI_StreamResponse_EarlyDisconnect(t *testing.T)  // 客户端提前断开

// 测试错误处理
func TestOpenAI_ErrorResponse(t *testing.T)
func TestOpenAI_ErrorResponse_BackendError(t *testing.T)
func TestOpenAI_ErrorResponse_Timeout(t *testing.T)

// 测试并发请求
func TestOpenAI_ConcurrentRequests(t *testing.T)

// 测试大请求体
func TestOpenAI_LargeRequestBody(t *testing.T)
```

### 6.3 性能基准测试

```go
// 协议转换性能
func BenchmarkOpenAI_ParseRequest(b *testing.B)
func BenchmarkOpenAI_ConvertToAnthropic(b *testing.B)
func BenchmarkOpenAI_ConvertToGemini(b *testing.B)

// 端到端延迟
func BenchmarkEndToEnd_Latency(b *testing.B)

// 吞吐量测试
func BenchmarkThroughput(b *testing.B)
```

### 6.4 SSE 流式测试

```go
// 测试 SSE 格式正确性
func TestStream_ResponseFormat(t *testing.T)

// 测试流式断点续传
func TestStream_Resumable(t *testing.T)

// 测试流式连接中断
func TestStream_ConnectionInterrupt(t *testing.T)

// 测试 [DONE] 标记
func TestStream_DoneMarker(t *testing.T)
```

### 6.5 Mock 后端测试

```go
// 使用 httptest.Server 模拟后端响应
// 支持 CI/CD 无外部依赖测试

// 模拟成功响应
func TestMockBackend_Success(t *testing.T) {
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"choices": [{"message": {"content": "Hello"}}]}`))
    }))
    defer mockServer.Close()
    
    // 测试逻辑...
}

// 模拟后端错误
func TestMockBackend_BackendError(t *testing.T) {
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusUnauthorized)
        w.Write([]byte(`{"error": {"message": "Invalid API key"}}`))
    }))
    defer mockServer.Close()
    
    // 测试错误转换...
}

// 模拟超时
func TestMockBackend_Timeout(t *testing.T) {
    mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(10 * time.Second)  // 超过超时时间
    }))
    defer mockServer.Close()
    
    // 测试超时处理...
}
```

---

## 7. 审计日志和追踪

### 7.1 审计日志格式

```go
// AuditLog 审计日志结构
type AuditLog struct {
    Timestamp   time.Time `json:"timestamp"`
    ClientIP    string    `json:"client_ip"`
    APIKey      string    `json:"api_key"`      // 脱敏后（仅显示前 8 个字符）
    Protocol    string    `json:"protocol"`     // openai/anthropic
    Model       string    `json:"model"`
    Backend     string    `json:"backend"`
    Latency     int64     `json:"latency_ms"`
    StatusCode  int       `json:"status_code"`
    RequestID   string    `json:"request_id"`   // 请求追踪 ID
    InputTokens int       `json:"input_tokens"`
    OutputTokens int      `json:"output_tokens"`
}

// 示例日志输出
// {"timestamp":"2026-04-16T10:00:00Z","client_ip":"192.168.1.100",
//  "api_key":"sk-clie...","protocol":"openai","model":"claude-3-opus",
//  "backend":"anthropic","latency_ms":1250,"status_code":200,
//  "request_id":"chatcmpl-abc123","input_tokens":10,"output_tokens":20}
```

### 7.2 请求 ID 追踪

```go
// 请求 ID 生成和追踪
type RequestContext struct {
    RequestID   string    // 唯一请求 ID
    StartTime   time.Time // 请求开始时间
    ClientIP    string    // 客户端 IP
    Protocol    string    // 协议类型
}

// 生成请求 ID
func generateRequestID() string {
    return fmt.Sprintf("chatcmpl-%s", uuid.New().String()[:8])
}

// 在响应中返回 RequestID
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
    ctx := &RequestContext{
        RequestID: generateRequestID(),
        StartTime: time.Now(),
        ClientIP:  getClientIP(r),
        Protocol:  getProtocol(r.URL.Path),
    }
    
    // 将 RequestID 添加到响应头
    w.Header().Set("X-Request-ID", ctx.RequestID)
    
    // 记录审计日志
    s.logAudit(ctx, ...)
}
```

### 7.3 日志级别

| 级别 | 内容 |
|------|------|
| INFO | 请求完成（状态码、延迟、token 数） |
| DEBUG | 详细请求参数（脱敏） |
| WARN | 触发限流、部分参数不支持 |
| ERROR | 后端错误、认证失败 |

---

## 8. 实现步骤

1. **解析器实现** - `openai/parser.go`
2. **转换器扩展** - 扩展现有 `openai/converter.go`
3. **服务器入口** - 修改 `server.go` 增加路由
4. **错误处理** - 统一错误格式
5. **测试覆盖** - 单元 + 集成测试
6. **文档更新** - README + 配置示例

---

## 9. 与现有代码兼容性

### 9.1 现有代码分析

**`internal/protocol/openai/converter.go` 已有功能：**
- `Convert()` - 将 UnifiedRequest 转换为 OpenAI 后端格式
- `ParseResponse()` - 解析 OpenAI 响应为 UnifiedResponse

**需要新增：**
- `internal/protocol/openai/parser.go` - 解析 OpenAI 请求
- `internal/protocol/openai/request_converter.go` - 请求转换（可扩展）
- `internal/protocol/openai/response_converter.go` - 响应构建

### 9.2 对现有功能的影响

| 现有功能 | 影响 | 说明 |
|----------|------|------|
| Anthropic 入口 | 无 | 保持 `/v1/messages` 路由不变 |
| WebSocket 隧道 | 无 | 独立路径，互不干扰 |
| 健康检查 | 无 | `/health` 端点保持不变 |
| 日志系统 | 小 | 增加协议类型字段 |
| 配置系统 | 小 | 路由配置无需修改 |

### 9.3 配置兼容性

**现有配置向后兼容：**

```yaml
# 现有配置保持不变
routes:
  - api_key: "sk-client-1"
    backend: "openai"
    backend_api_key: "sk-xxx"
    timeout: 60s

# 可选：添加协议类型限制（未来扩展）
# routes:
#   - api_key: "sk-client-1"
#     backend: "openai"
#     protocols: ["anthropic", "openai"]  # 允许的协议
```

---

## 10. 边缘情况处理

### 10.1 上下文长度限制

不同后端有不同的上下文长度限制：

| 后端 | 限制 | 处理策略 |
|------|------|----------|
| OpenAI | 模型相关 (4K-128K) | 截断最早的用户消息 |
| Anthropic | 200K tokens | 截断最早的用户消息 |
| Gemini | 1M tokens (1.5) | 截断最早的用户消息 |

```go
// 消息截断策略
func truncateMessages(messages []Message, maxTokens int) []Message {
    // 1. 保留第一条 system 消息
    // 2. 保留最后 N 轮对话
    // 3. 返回警告信息
}
```

### 10.2 Unicode 和特殊字符

```go
// 处理策略：
// 1. emoji: 正常处理（UTF-8 编码）
// 2. 控制字符：过滤掉 0x00-0x1F (除了\n\t)
// 3. BOM 标记：移除
// 4. 代理对：正常处理

func sanitizeContent(content string) string {
    // 过滤控制字符
    return strings.Map(func(r rune) rune {
        if r < 0x20 && r != '\n' && r != '\t' {
            return -1
        }
        return r
    }, content)
}
```

### 10.3 流式连接中断

```go
// 客户端提前断开连接处理：
// 1. 检测连接断开 (http.CloseNotifier)
// 2. 停止向后端发送请求
// 3. 清理资源
// 4. 记录日志（不返回错误）

func (s *Server) handleStream(w http.ResponseWriter, resp *http.Response) {
    flusher, _ := w.(http.Flusher)
    notifier, _ := w.(http.CloseNotifier)
    <-notifier.CloseNotify()
    // 客户端已断开，停止流式输出
}
```

### 10.4 请求体大小限制

```go
// 默认限制：10MB
const MaxRequestBodySize = 10 * 1024 * 1024

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
    // 超过限制返回 413 Payload Too Large
}
```

### 10.5 超时设置

```go
// 各后端超时配置
var backendTimeouts = map[string]time.Duration{
    "openai":    120 * time.Second,
    "anthropic": 120 * time.Second,
    "gemini":    90 * time.Second,  // Gemini 官方建议
}

// 路由配置可覆盖默认值
// config.yaml:
// routes:
//   - api_key: "xxx"
//     backend: "gemini"
//     timeout: 180s  # 自定义超时
```

### 10.6 认证和授权

```go
// API Key 验证流程：
// 1. 从 X-API-Key header 或 Authorization: Bearer 提取
// 2. 在路由表中查找
// 3. 验证后端权限
// 4. 记录审计日志

func extractAPIKey(r *http.Request) string {
    // 优先 X-API-Key
    if key := r.Header.Get("x-api-key"); key != "" {
        return key
    }
    // 回退到 Bearer Token
    auth := r.Header.Get("Authorization")
    if strings.HasPrefix(auth, "Bearer ") {
        return strings.TrimPrefix(auth, "Bearer ")
    }
    return ""
}
```

---

## 11. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 参数映射不完整 | 部分功能不可用 | 记录不支持的参数，返回警告而非错误 |
| 流式响应格式错误 | 客户端解析失败 | 严格按照 OpenAI SSE 格式输出，使用测试用例验证 |
| 错误码映射错误 | 客户端错误处理异常 | 参考 OpenAI 官方文档，编写完整映射表 |
| 上下文超长 | 后端拒绝请求 | 实现消息截断策略，提前返回警告 |
| 特殊字符处理 | 后端解析失败 | 实现内容清洗函数，过滤控制字符 |
| 连接中断 | 资源泄漏 | 使用 CloseNotifier 检测断开，清理资源 |
| 请求体过大 | 内存耗尽 | 设置 MaxBytesReader 限制 |
| 超时不一致 | 请求挂起 | 各后端配置独立超时，使用 context.WithTimeout |

---

## 12. 验收标准

### 功能验收

- [ ] OpenAI SDK 可直接调用 `/v1/chat/completions`
- [ ] Anthropic SDK 可继续调用 `/v1/messages`
- [ ] 支持流式和非流式响应
- [ ] 支持配置的后端（OpenAI/Anthropic/Gemini）
- [ ] 错误响应符合 OpenAI 格式
- [ ] 不支持的参数返回友好警告

### 质量验收

- [ ] 单元测试覆盖率 > 85%
- [ ] 集成测试通过
- [ ] 边界值测试通过
- [ ] 性能基准测试建立基线
- [ ] SSE 流式格式验证通过

### 文档验收

- [ ] README.md 更新 OpenAI 入口说明
- [ ] 配置示例更新
- [ ] API 文档更新
- [ ] 变更日志更新
