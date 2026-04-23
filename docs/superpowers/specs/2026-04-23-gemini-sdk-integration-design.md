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

**三种入口的完整转换链路**:

```
/v1/messages (Anthropic)
  → anthropic.ParseRequest() → UnifiedMessage
  → sdk_adapter.ToSDKContents() → genai.Content[] + Config
  → client.Generate/GenerateStream() → genai.Response
  → sdk_adapter.FromSDKResponse() → UnifiedResponse
  → 返回 Anthropic 格式

/v1/chat/completions (OpenAI Chat)
  → openai.ParseRequest() → UnifiedMessage
  → sdk_adapter.ToSDKContents() → genai.Content[] + Config
  → client.Generate/GenerateStream() → genai.Response
  → sdk_adapter.FromSDKResponse() → UnifiedResponse
  → openai.BuildResponse() → OpenAI 格式

/v1/completions (OpenAI 旧版)
  → 解析 prompt/messages → UnifiedMessage
  → sdk_adapter.ToSDKContents() → genai.Content[] + Config
  → client.Generate/GenerateStream() → genai.Response
  → sdk_adapter.FromSDKResponse() → UnifiedResponse
  → buildCompletionsResponse() → Completions 格式
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

## 流式处理

- **非流式**: SDK `GenerateContent` → 直接转换 → 返回 JSON
- **流式**: SDK `GenerateContentStream` Iterator → 逐块转换为 SSE → 客户端

流式 SSE 转换逻辑复用现有 `convertGeminiSSEToOpenAI` 和 `convertGeminiSSEToCompletions`，但数据源从 SDK Iterator 而非 HTTP 响应体。

## 错误处理

- SDK 调用失败 → 转换为现有 `writeError` 格式
- SDK 返回安全过滤错误 → 映射为 `content_filter` finish_reason
- 超时 → 使用现有 `context.WithTimeout` 机制

## 向后兼容

- 保留原有 HTTP 调用路径作为 fallback（可通过配置切换）
- API 接口不变，客户端无感知
- 日志格式兼容现有 `journalctl` 过滤
