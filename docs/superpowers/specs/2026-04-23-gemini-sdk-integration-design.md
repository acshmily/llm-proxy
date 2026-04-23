# Gemini SDK 集成与日志增强设计

> **状态**: 设计已确认，等待实现计划
> **日期**: 2026-04-23

## 目标

将 Gemini 后端调用从原始 HTTP 请求迁移至 `google.golang.org/genai` 官方 SDK，并完善日志方案，使 `journalctl -u llm-proxy -f` 可观察完整请求流程。

## 架构

```
OpenAI/Anthropic/Gemini 请求 → server.go 路由
                                    ↓
                     protocol/gemini/sdk_adapter.go
                     (UnifiedMessage ↔ SDK 双向转换)
                                    ↓
                      internal/gemini/client.go
                      (genai.Client SDK 封装)
                                    ↓
                              Gemini API
```

## 文件变更

### 新增文件

#### 1. `internal/gemini/client.go` — SDK 客户端封装

**职责**:
- 初始化和管理 `genai.Client` 单例
- 封装非流式 `GenerateContent` 和流式 `GenerateContentStream` 调用
- 支持 HTTP 代理配置（通过自定义 `http.Client` 注入 `Proxy`）
- 暴露请求/响应日志钩子

**核心结构**:
```go
type GeminiClient struct {
    client  *genai.Client
    log     *logger.Logger
    debug   bool
    maxBody int
}

func NewGeminiClient(apiKey string, proxy string, log *logger.Logger, debug bool, maxBody int) (*GeminiClient, error)
func (c *GeminiClient) Generate(ctx, model string, contents []*genai.Content, opts *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
func (c *GeminiClient) GenerateStream(ctx, model string, contents []*genai.Content, opts *genai.GenerateContentConfig) *genai.GenerateContentResponseIterator
```

**代理支持**: 通过 `http.Transport.Proxy` 注入，配置从 `config.Backends.Gemini` 读取。

#### 2. `internal/protocol/gemini/sdk_adapter.go` — SDK 适配层

**职责**:
- `UnifiedMessage` → `[]*genai.Content` + `[]*genai.Tool` + `*genai.GenerateContentConfig`
- `*genai.GenerateContentResponse` → `types.UnifiedResponse`
- 处理 `tool_calls`、`function_calls` 的双向转换
- SDK Response → REST JSON（原生端点兼容）

**关键转换映射（完整版）**:

当前 `converter.go` 已支持的参数：
| Unified 字段 | 当前状态 | Gemini SDK 映射 |
|-------------|---------|----------------|
| Messages[].Role | 映射 user/assistant/tool | Content.Role |
| Messages[].Content | 构建 text part | Content.Parts[].Text |
| Messages[].ToolCalls | 构建 functionCall part | Content.Parts[].FunctionCall |
| Tools[] | functionDeclarations | Tool.FunctionDeclarations |
| Temperature | generationConfig.temperature | GenerationConfig.Temperature |

当前 `converter.go` **缺失**需要补全的参数：
| Unified 字段 | 缺失情况 | Gemini SDK 映射 |
|-------------|---------|----------------|
| MaxTokens | ❌ 未转换 | GenerationConfig.MaxOutputTokens |
| StopSequences | ❌ 未转换 | GenerationConfig.StopSequences |
| TopP | ❌ 未转换 | GenerationConfig.TopP |
| system/developer 角色 | 映射为 user | SystemInstruction（SDK 原生支持） |
| Stream | 仅 URL 参数 | GenerateContentStream Iterator |

**三种入口的完整转换链路（含响应回写）**:

```
/v1/messages (Anthropic)
  请求: anthropic.ParseRequest() → UnifiedMessage
  转换: sdk_adapter.ToSDK() → genai.Content[] + Config
  调用: client.Generate/GenerateStream() → genai.Response
  解析: sdk_adapter.FromSDK() → UnifiedResponse
  返回: Anthropic 格式

/v1/chat/completions (OpenAI Chat API)
  请求: openai.ParseRequest() → UnifiedMessage
  转换: sdk_adapter.ToSDK() → genai.Content[] + Config
  调用: client.Generate/GenerateStream() → genai.Response
  解析: sdk_adapter.FromSDK() → UnifiedResponse
  返回: openai.BuildResponse() → OpenAI ChatCompletion JSON (含 tool_calls, usage)

/v1/completions (OpenAI Completions API)
  请求: 解析 prompt/messages → UnifiedMessage
  转换: sdk_adapter.ToSDK() → genai.Content[] + Config
  调用: client.Generate/GenerateStream() → genai.Response
  解析: sdk_adapter.FromSDK() → UnifiedResponse
  返回: buildCompletionsResponse() → OpenAI Completions JSON
```

**System Prompt 处理变更**:
- 当前: `system`/`developer` 角色映射为 `"user"`（消息合并到 contents 首部）
- 新版: 使用 `genai.SystemInstruction` 独立传递，不混入 contents
- 好处: 更准确地传达系统指令，避免角色交替违规

### 修改文件

#### 3. `internal/server/server.go`

**改动点**:
- `New()`: 若 Gemini 后端已配置，初始化 `GeminiClient`
- `serveRequest()`: `case "gemini"` 替换为 SDK 调用（非流式 + 流式）
- `serveOpenAIRequest()`: 同上
- `serveCompletionsRequest()`: 同上
- `serveGeminiRequest()`: 原生端点也改用 SDK，响应通过 `SDKResponseToREST` 转回 REST JSON
- 流式响应: 使用 SDK Iterator 逐块转换为 SSE

#### 4. `internal/protocol/gemini/converter.go`

**改动**:
- 保留 `Convert` 和 `ParseResponse` 作为 fallback
- 新增 `ToSDKContents(UnifiedMessage) ([]*genai.Content, []*genai.Tool, *genai.GenerateContentConfig)`
- 新增 `FromSDKResponse(*genai.GenerateContentResponse, string) (*types.UnifiedResponse, error)`
- 新增 `SDKResponseToREST(*genai.GenerateContentResponse) ([]byte, error)`

#### 5. `internal/config/config.go`

**新增字段**:
```yaml
backends:
  gemini:
    base_url: "https://generativelanguage.googleapis.com/v1beta"
    http_proxy: "http://127.0.0.1:18080"  # 可选代理地址
```

#### 6. `go.mod`

新增依赖:
```
google.golang.org/genai
```

## 日志方案

### 日志级别控制

通过 `config.Logging.DebugRequests` 控制 debug 级别日志输出。

### 日志输出格式

**INFO 级别（默认）**:
```
[Gemini] Request started | model=gemini-2.0-flash | stream=true
[Gemini] Request completed | model=gemini-2.0-flash | latency_ms=1234 | status=200 | input_tokens=150 | output_tokens=80
```

**DEBUG 级别（debug_requests=true）**:
```
[Gemini] Request body | model=gemini-2.0-flash | body={"contents": [...]} | total_bytes=512
[Gemini] Response body | model=gemini-2.0-flash | status=200 | body={"candidates": [...]} | total_bytes=1024
```

**DEBUG 级别（额外 SDK 内部日志）**:
```
[Gemini] SDK detail | safety_ratings=... | cached_tokens=... | finish_reason=STOP
```

### 日志截断

沿用现有 `debug_max_body` 配置，超出部分截断并标注总字节数。

## OpenAI 参数完整映射（零遗漏）

当前 `gemini.Convert()` 丢失了大量参数。SDK 适配层必须做到**参数零遗漏**。

### `/v1/chat/completions` 参数流

```
客户端请求                    ParseRequest              UnifiedMessage            SDK 适配
────────────                    ────────────              ──────────────            ──────
model: "gpt-4"              ────────────────▶   Model: "gpt-4"            ──▶  model 参数
messages[]                  ────────────────▶   Messages[]                ──▶  Contents[]
  - role: "system"           (保留原始 role)     Messages[0].Role=system    ──▶  SystemInstruction
  - role: "user"             (保留原始 role)     Messages[i].Role=user      ──▶  Content.Role="user"
  - role: "assistant"        (保留原始 role)     Messages[i].Role=assistant ──▶  Content.Role="model"
  - role: "tool"             (保留原始 role)     Messages[i].Role=tool      ──▶  Content.Parts[].FunctionResponse
  - tool_calls[]             (保留 tool_calls)   Messages[i].ToolCalls[]    ──▶  Content.Parts[].FunctionCall
  - tool_call_id             (保留 tool_call_id) Messages[i].ToolCallID     ──▶  查找 functionResponse name
max_tokens                  ────────────────▶   MaxTokens: int            ──▶  GenerationConfig.MaxOutputTokens
temperature                 ────────────────▶   Temperature: float64      ──▶  GenerationConfig.Temperature
top_p                       ────────────────▶   TopP: float64             ──▶  GenerationConfig.TopP
stop: ["\n", "END"]         ────────────────▶   StopSequences: []string   ──▶  GenerationConfig.StopSequences
tools[]                     ────────────────▶   Tools[]                   ──▶  genai.Tool{FunctionDeclarations}
tool_choice: "auto"         ────────────────▶   ToolChoice: interface{}   ──▶  ToolConfig.FunctionCallingConfig.Mode
tool_choice: {"type":"function",
  "function":{"name":"x"}}   ────────────────▶   ToolChoice: struct        ──▶  ToolConfig.FunctionCallingConfig.AllowedFunctionNames
stream: true                ────────────────▶   Stream: bool              ──▶  决定调用 Generate vs GenerateStream
```

### `/v1/completions` 参数流

```
客户端请求                    serveCompletionsRequest     UnifiedMessage            SDK 适配
────────────                    ──────────────────────      ──────────────            ──────
prompt: "Hello"             ────────────────▶   Messages=[Role:user,   ──▶  Content.Parts[].Text
                                               Content:"Hello"]
messages[]                  ────────────────▶   Messages[]              ──▶  (同上 chat/completions)
max_tokens                  ────────────────▶   MaxTokens: int          ──▶  GenerationConfig.MaxOutputTokens
temperature                 ────────────────▶   Temperature: float64    ──▶  GenerationConfig.Temperature
top_p                       ────────────────▶   TopP: float64           ──▶  GenerationConfig.TopP
stop: ["\n"]                ────────────────▶   StopSequences: []string ──▶  GenerationConfig.StopSequences
tools[]                     ────────────────▶   Tools[]                 ──▶  genai.Tool{FunctionDeclarations}
tool_choice                 ────────────────▶   ToolChoice              ──▶  ToolConfig.FunctionCallingConfig
stream                      ────────────────▶   Stream: bool            ──▶  决定调用 Generate vs GenerateStream
```

### 参数映射核对表

| OpenAI 参数 | UnifiedMessage 字段 | 当前 gemini.Convert | SDK 适配层 | 影响 |
|------------|-------------------|-------------------|-----------|-----|
| model | Model | ✅ URL 使用 | ✅ | - |
| messages | Messages | ✅ 转换 | ✅ 完整保留 | - |
| max_tokens | MaxTokens | ❌ **丢失** | ✅ MaxOutputTokens | 限制输出长度 |
| temperature | Temperature | ✅ | ✅ | - |
| top_p | TopP | ❌ **丢失** | ✅ TopP | 采样控制 |
| stop | StopSequences | ❌ **丢失** | ✅ StopSequences | 停止词 |
| tools | Tools | ✅ | ✅ 完整保留 | - |
| tool_choice | ToolChoice | ❌ **丢失** | ✅ ToolConfig | 工具调用策略 |
| stream | Stream | ✅ URL 使用 | ✅ 方法选择 | - |
| system role | Messages[0].Role | 映射为 user | ✅ SystemInstruction | 系统指令隔离 |

### SDK 适配层签名

```go
// UnifiedMessageToSDK 将统一消息转换为 SDK 参数
func UnifiedMessageToSDK(um *types.UnifiedMessage) (
    model string,                          // 模型名
    contents []*genai.Content,             // 对话内容
    systemInstruction *genai.Content,      // 系统指令（可选）
    config *genai.GenerateContentConfig,   // 生成配置
    tools []*genai.Tool,                   // 工具定义
    err error,
)
```

---

## OpenAI 兼容响应链路

SDK 调用完成后，需要将 Gemini SDK 响应正确转换为 OpenAI 格式返回。

### Chat Completions API (`/v1/chat/completions`)

**请求链路**:
```
openai.ParseRequest()    → UnifiedMessage (已含 max_tokens, stop, top_p, tools)
sdk_adapter.ToSDK()      → genai.Content[] + GenerateContentConfig + Tools
client.Generate()        → genai.GenerateContentResponse
sdk_adapter.FromSDK()    → UnifiedResponse (含 content, tool_calls, usage, finish_reason)
openai.BuildResponse()   → OpenAI ChatCompletion JSON
```

**关键要求**: `openai.BuildResponse()` 需要补全，当前缺失:
- `tool_calls` 字段回写（assistant 调用的函数）
- `usage` 字段填充（prompt_tokens, completion_tokens, total_tokens）
- 流式 SSE delta 格式中的 `tool_calls` 增量

**当前 `openai/converter.go` 状态**:
| 字段 | BuildResponse | 流式 delta |
|------|--------------|-----------|
| content text | ✅ 有 | ✅ 有 (via convertGeminiSSEToOpenAI) |
| tool_calls | ❌ 缺失 | ❌ 缺失 |
| usage | ❌ 缺失 | ❌ 缺失 |
| finish_reason | ✅ 有 | ✅ 有 |

**流式 OpenAI SSE 格式**:
```
data: {"id":"chat-1","choices":[{"delta":{"role":"assistant","content":""},"finish_reason":null}]}
data: {"id":"chat-1","choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}
data: {"id":"chat-1","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_xxx","function":{"name":"search","arguments":"{\"q\":\"test\"}"}}]},"finish_reason":null}]}
data: {"id":"chat-1","choices":[{"delta":{},"finish_reason":"stop"}]}
data: [DONE]
```

### Completions API (`/v1/completions`)

**请求链路**:
```
serveCompletionsRequest  → 解析 prompt/messages → UnifiedMessage
sdk_adapter.ToSDK()      → genai.Content[] + Config
client.Generate()        → genai.GenerateContentResponse
sdk_adapter.FromSDK()    → UnifiedResponse
buildCompletionsResponse() → {"id","object":"text_completion","choices":[{"text":"...","finish_reason":"stop"}]}
```

**当前 `buildCompletionsResponse` 状态**: 已实现基础文本转换

**流式 Completions SSE 格式**:
```
data: {"choices":[{"text":"Hello","finish_reason":null}]}
data: {"choices":[{"text":" world","finish_reason":null}]}
data: {"choices":[{"text":"","finish_reason":"stop"}]}
data: [DONE]
```

### 响应格式映射表 (Gemini SDK → OpenAI)

| Gemini SDK 字段 | OpenAI Chat 字段 | Completions 字段 |
|----------------|-----------------|-----------------|
| candidates[0].content.parts[0].text | choices[0].message.content | choices[0].text |
| candidates[0].content.parts[0].functionCall | choices[0].message.tool_calls | (不支持) |
| candidates[0].finishReason | choices[0].finish_reason | choices[0].finish_reason |
| usageMetadata.promptTokenCount | usage.prompt_tokens | usage.prompt_tokens |
| usageMetadata.candidatesTokenCount | usage.completion_tokens | usage.completion_tokens |
| candidates[0].safetyRatings | (不暴露) | (不暴露) |

### SDK 流式转 OpenAI SSE 实现方案

当前 `convertGeminiSSEToOpenAI` 从 HTTP 响应体解析 SSE，改为从 SDK Iterator 直接构建:

```go
// server.go 新增
func (s *Server) handleOpenAIStreamFromSDK(w http.ResponseWriter, iter *genai.GenerateContentResponseIterator, backend string, connReused bool) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    
    for {
        resp, err := iter.Next()
        if err == io.EOF {
            w.Write([]byte("data: [DONE]\n\n"))
            break
        }
        
        // 转换为 OpenAI delta SSE chunk
        chunk := buildOpenAIStreamChunk(resp)
        w.Write(chunk)
        if f, ok := w.(http.Flusher); ok {
            f.Flush()
        }
    }
}
```

### 向后兼容承诺

- `/v1/chat/completions` 返回的 JSON 结构必须与 OpenAI API 完全兼容
- `/v1/completions` 返回的 JSON 结构必须与 OpenAI 旧版 API 完全兼容
- 流式 SSE 格式必须与 OpenAI 格式一致
- 客户端无需修改代码即可切换后端为 Gemini

## 错误处理

- SDK 调用失败 → 转换为现有 `writeError` 格式
- SDK 返回安全过滤错误 → 映射为 `content_filter` finish_reason
- 超时 → 使用现有 `context.WithTimeout` 机制

## 向后兼容

- 保留原有 HTTP 调用路径作为 fallback（可通过配置切换）
- API 接口不变，客户端无感知
- 日志格式兼容现有 `journalctl` 过滤
