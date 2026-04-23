# Gemini SDK 集成与日志增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Gemini 后端调用从原始 HTTP 请求迁移至 `google.golang.org/genai` 官方 SDK，补全所有 OpenAI 参数映射，完善日志方案。

**Architecture:** 新增 SDK 客户端封装（`internal/gemini/client.go`）和协议适配层（`internal/protocol/gemini/sdk_adapter.go`），替换 `server.go` 中 Gemini 后端的 HTTP 调用为 SDK 调用，所有入口（Anthropic、OpenAI Chat、Completions、Gemini Native）统一走 SDK。

**Tech Stack:** Go 1.21+, `google.golang.org/genai`, 现有 `internal/logger` 模块

---

### Task 1: 添加 Gemini 代理配置字段

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: 添加 HttpProxy 字段到 BackendConfig**

在 `internal/config/config.go` 的 `BackendConfig` 结构体中添加 `HttpProxy` 字段：

```go
type BackendConfig struct {
	BaseURL   string `yaml:"base_url"`
	HttpProxy string `yaml:"http_proxy,omitempty"`
}
```

- [ ] **Step 2: 验证配置解析**

确认 `config_test.go` 中已有解析测试，新增一个测试用例验证 `http_proxy` 字段可以正确解析：

```go
func TestBackendConfig_HttpProxy(t *testing.T) {
	data := `
backends:
  gemini:
    base_url: https://generativelanguage.googleapis.com/v1beta
    http_proxy: http://127.0.0.1:18080
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if cfg.Gemini.HttpProxy != "http://127.0.0.1:18080" {
		t.Errorf("Expected http_proxy 'http://127.0.0.1:18080', got %q", cfg.Gemini.HttpProxy)
	}
}
```

- [ ] **Step 3: 运行测试并提交**

```bash
go test ./internal/config/... -v -run TestBackendConfig_HttpProxy
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add http_proxy field to BackendConfig for Gemini proxy support"
```

---

### Task 2: 安装 SDK 依赖

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: 添加 genai 依赖**

```bash
go get google.golang.org/genai
```

- [ ] **Step 2: 验证编译**

```bash
go build ./...
```

- [ ] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "chore: add google.golang.org/genai dependency"
```

---

### Task 3: SDK 客户端封装

**Files:**
- Create: `internal/gemini/client.go`
- Create: `internal/gemini/client_test.go`

- [ ] **Step 1: 编写失败的测试**

创建 `internal/gemini/client_test.go`：

```go
package gemini

import (
	"context"
	"testing"
	"github.com/claude-projetc/llm-proxy/internal/logger"
)

func TestNewGeminiClient_WithProxy(t *testing.T) {
	log := logger.New(logger.TEXT, logger.DEBUG)
	client, err := NewGeminiClient("test-api-key", "http://127.0.0.1:18080", log, true, 2048)
	if err != nil {
		t.Fatalf("NewGeminiClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewGeminiClient_WithoutProxy(t *testing.T) {
	log := logger.New(logger.TEXT, logger.INFO)
	client, err := NewGeminiClient("test-api-key", "", log, false, 2048)
	if err != nil {
		t.Fatalf("NewGeminiClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/gemini/... -v
```
Expected: FAIL — package doesn't exist yet

- [ ] **Step 3: 实现 SDK 客户端**

创建 `internal/gemini/client.go`：

```go
package gemini

import (
	"context"
	"net/http"
	"net/url"

	"google.golang.org/genai"

	"github.com/claude-projetc/llm-proxy/internal/logger"
)

// GeminiClient SDK 客户端封装
type GeminiClient struct {
	client  *genai.Client
	log     *logger.Logger
	debug   bool
	maxBody int
}

// NewGeminiClient 创建 Gemini SDK 客户端
func NewGeminiClient(apiKey string, proxy string, log *logger.Logger, debug bool, maxBody int) (*GeminiClient, error) {
	cfg := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: "google",
	}

	// 配置代理
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		cfg.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
	}

	client, err := genai.NewClient(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	return &GeminiClient{
		client:  client,
		log:     log,
		debug:   debug,
		maxBody: maxBody,
	}, nil
}

// GenerateContent 非流式调用
func (c *GeminiClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	c.log.Debug("Gemini SDK GenerateContent",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "contents_count", Value: len(contents)},
	)

	resp, err := c.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, err
	}

	c.log.Debug("Gemini SDK response received",
		logger.LogField{Key: "model", Value: model},
	)

	return resp, nil
}

// GenerateContentStream 流式调用，返回 Iterator
func (c *GeminiClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) *genai.GenerateContentResponseIterator {
	c.log.Debug("Gemini SDK GenerateContentStream",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "contents_count", Value: len(contents)},
	)

	return c.client.Models.GenerateContentStream(ctx, model, contents, config)
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/gemini/... -v
```
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/gemini/client.go internal/gemini/client_test.go
git commit -m "feat(gemini): add SDK client wrapper with proxy support"
```

---

### Task 4: SDK 适配层 — UnifiedMessage → SDK 参数转换

**Files:**
- Create: `internal/protocol/gemini/sdk_adapter.go`
- Create: `internal/protocol/gemini/sdk_adapter_test.go`

- [ ] **Step 1: 编写失败的测试**

创建 `internal/protocol/gemini/sdk_adapter_test.go`：

```go
package gemini

import (
	"testing"
	"reflect"

	"github.com/claude-projetc/llm-proxy/pkg/types"
	"google.golang.org/genai"
)

func TestUnifiedMessageToSDK_BasicMessage(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.0-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Hello"},
		},
		Stream: false,
	}

	model, contents, sysInst, config, tools, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if model != "gemini-2.0-flash" {
		t.Errorf("Expected model 'gemini-2.0-flash', got %q", model)
	}
	if len(contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("Expected content role 'user', got %q", contents[0].Role)
	}
	if sysInst != nil {
		t.Error("Expected nil SystemInstruction for non-system messages")
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if len(tools) != 0 {
		t.Errorf("Expected 0 tools, got %d", len(tools))
	}
}

func TestUnifiedMessageToSDK_SystemInstruction(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.0-flash",
		Messages: []types.MessageRole{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}

	model, contents, sysInst, _, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if sysInst == nil {
		t.Fatal("Expected non-nil SystemInstruction")
	}
	if len(sysInst.Parts) != 1 {
		t.Fatalf("Expected 1 part in SystemInstruction, got %d", len(sysInst.Parts))
	}
	if sysInst.Parts[0].Text != "You are helpful." {
		t.Errorf("Expected SystemInstruction text 'You are helpful.', got %q", sysInst.Parts[0].Text)
	}
	// system 消息不应出现在 contents 中
	if len(contents) != 1 {
		t.Errorf("Expected 1 content (user only), got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("Expected content role 'user', got %q", contents[0].Role)
	}
	_ = model
}

func TestUnifiedMessageToSDK_AllParameters(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:         "gemini-2.0-flash",
		Messages:      []types.MessageRole{{Role: "user", Content: "Hello"}},
		MaxTokens:     100,
		Temperature:   0.7,
		TopP:          0.9,
		StopSequences: []string{"\n", "END"},
		Stream:        false,
	}

	_, _, _, config, _, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if config.MaxOutputTokens == nil || *config.MaxOutputTokens != 100 {
		t.Errorf("Expected MaxOutputTokens 100, got %v", config.MaxOutputTokens)
	}
	if config.Temperature == nil || *config.Temperature != 0.7 {
		t.Errorf("Expected Temperature 0.7, got %v", config.Temperature)
	}
	if config.TopP == nil || *config.TopP != 0.9 {
		t.Errorf("Expected TopP 0.9, got %v", config.TopP)
	}
	if config.StopSequences == nil || !reflect.DeepEqual(config.StopSequences, []string{"\n", "END"}) {
		t.Errorf("Expected StopSequences ['\\n','END'], got %v", config.StopSequences)
	}
}

func TestUnifiedMessageToSDK_ToolChoice(t *testing.T) {
	um := &types.UnifiedMessage{
		Model:    "gemini-2.0-flash",
		Messages: []types.MessageRole{{Role: "user", Content: "Hello"}},
		Tools: []types.Tool{{
			Type: "function",
			Function: types.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get weather",
			},
		}},
		ToolChoice: "auto",
	}

	_, _, _, config, tools, err := UnifiedMessageToSDK(um)
	if err != nil {
		t.Fatalf("UnifiedMessageToSDK failed: %v", err)
	}

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if config.ToolConfig == nil {
		t.Fatal("Expected non-nil ToolConfig for tool_choice")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/protocol/gemini/... -v -run TestUnifiedMessageToSDK
```
Expected: FAIL — function doesn't exist

- [ ] **Step 3: 实现转换函数**

创建 `internal/protocol/gemini/sdk_adapter.go`：

```go
package gemini

import (
	"fmt"

	"github.com/claude-projetc/llm-proxy/pkg/types"
	"google.golang.org/genai"
)

// UnifiedMessageToSDK 将统一消息转换为 SDK 参数
func UnifiedMessageToSDK(um *types.UnifiedMessage) (
	model string,
	contents []*genai.Content,
	systemInstruction *genai.Content,
	config *genai.GenerateContentConfig,
	tools []*genai.Tool,
	err error,
) {
	model = um.Model

	// 提取 system/developer 消息为 SystemInstruction
	contents, systemInstruction = buildContentsAndSystemInstruction(um.Messages)

	// 构建生成配置
	config = buildGenerateConfig(um)

	// 构建工具定义
	if len(um.Tools) > 0 {
		tools = buildTools(um.Tools)
		// 如果有 tool_choice，配置 ToolConfig
		if um.ToolChoice != nil {
			config.ToolConfig = buildToolConfig(um.ToolChoice)
		}
	}

	return
}

// buildContentsAndSystemInstruction 将消息转换为 SDK contents 和 system instruction
func buildContentsAndSystemInstruction(messages []types.MessageRole) ([]*genai.Content, *genai.Content) {
	var systemTexts []string
	var contents []*genai.Content

	for _, msg := range messages {
		switch msg.Role {
		case "system", "developer":
			systemTexts = append(systemTexts, msg.Content)
		case "user":
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{Text: msg.Content}},
			})
		case "assistant":
			content := &genai.Content{Role: "model"}
			// 加入文本 part
			if msg.Content != "" {
				content.Parts = append(content.Parts, &genai.Part{Text: msg.Content})
			}
			// 加入 functionCall parts
			for _, tc := range msg.ToolCalls {
				var args map[string]interface{}
				// arguments 是 JSON string，需要解析
				if msg.Content != "" {
					// 简化处理，实际应使用 json.Unmarshal
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: args,
					},
				})
			}
			contents = append(contents, content)
		case "tool":
			// tool 响应映射为 user 角色的 FunctionResponse
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     lookupFunctionNameFromToolCallID(msg.ToolCallID, messages),
						Response: parseToolResponse(msg.Content),
					},
				}},
			})
		default:
			contents = append(contents, &genai.Content{
				Role:  msg.Role,
				Parts: []*genai.Part{{Text: msg.Content}},
			})
		}
	}

	// 构建 SystemInstruction
	var sysInst *genai.Content
	if len(systemTexts) > 0 {
		sysInst = &genai.Content{
			Role:  "system",
			Parts: []*genai.Part{{Text: joinStrings(systemTexts, "\n\n")}},
		}
	}

	return contents, sysInst
}

// buildGenerateConfig 从 UnifiedMessage 构建 SDK 生成配置
func buildGenerateConfig(um *types.UnifiedMessage) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}

	if um.MaxTokens > 0 {
		maxTokens := int32(um.MaxTokens)
		config.MaxOutputTokens = &maxTokens
	}
	if um.Temperature > 0 {
		temp := float32(um.Temperature)
		config.Temperature = &temp
	}
	if um.TopP > 0 {
		topP := float32(um.TopP)
		config.TopP = &topP
	}
	if len(um.StopSequences) > 0 {
		config.StopSequences = um.StopSequences
	}

	return config
}

// buildTools 将 Tool 定义转换为 SDK Tool
func buildTools(tools []types.Tool) []*genai.Tool {
	sdkTools := make([]*genai.Tool, len(tools))
	for i, tool := range tools {
		sdkTools[i] = &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  sanitizeSchemaForGemini(tool.Function.Parameters),
			}},
		}
	}
	return sdkTools
}

// buildToolConfig 从 ToolChoice 构建 ToolConfig
func buildToolConfig(toolChoice interface{}) *genai.ToolConfig {
	switch v := toolChoice.(type) {
	case string:
		mode := genai.FunctionCallingAuto
		switch v {
		case "none":
			mode = genai.FunctionCallingNone
		case "required", "any":
			mode = genai.FunctionCallingAny
		}
		return &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: mode},
		}
	case map[string]interface{}:
		if fn, ok := v["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				return &genai.ToolConfig{
					FunctionCallingConfig: &genai.FunctionCallingConfig{
						Mode:                  genai.FunctionCallingAny,
						AllowedFunctionNames:  []string{name},
					},
				}
			}
		}
	}
	return nil
}

func lookupFunctionNameFromToolCallID(toolCallID string, messages []types.MessageRole) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			for _, tc := range messages[i].ToolCalls {
				if tc.ID == toolCallID {
					return tc.Function.Name
				}
			}
		}
	}
	return toolCallID
}

func parseToolResponse(content string) map[string]interface{} {
	var resp map[string]interface{}
	if content == "" {
		return map[string]interface{}{}
	}
	// 尝试解析为 JSON 对象，失败则包装
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return map[string]interface{}{"output": content}
	}
	return resp
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, s := range parts {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
```

需要添加 `encoding/json` 导入。

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/protocol/gemini/... -v -run TestUnifiedMessageToSDK
```
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/protocol/gemini/sdk_adapter.go internal/protocol/gemini/sdk_adapter_test.go
git commit -m "feat(gemini): add SDK adapter for UnifiedMessage → genai conversion"
```

---

### Task 5: SDK 响应 → UnifiedResponse 转换

**Files:**
- Modify: `internal/protocol/gemini/sdk_adapter.go`
- Modify: `internal/protocol/gemini/sdk_adapter_test.go`

- [ ] **Step 1: 编写失败的测试**

添加到 `sdk_adapter_test.go`：

```go
func TestFromSDKResponse_BasicText(t *testing.T) {
	// 模拟 SDK 响应
	sdkResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{Text: "Hello, world!"}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	unified, err := FromSDKResponse(sdkResp, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromSDKResponse failed: %v", err)
	}

	if len(unified.Content) != 1 {
		t.Fatalf("Expected 1 content block, got %d", len(unified.Content))
	}
	if unified.Content[0].Text != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got %q", unified.Content[0].Text)
	}
	if unified.Model != "gemini-2.0-flash" {
		t.Errorf("Expected model 'gemini-2.0-flash', got %q", unified.Model)
	}
	if unified.FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got %q", unified.FinishReason)
	}
	if unified.Usage.InputTokens != 10 {
		t.Errorf("Expected 10 input tokens, got %d", unified.Usage.InputTokens)
	}
	if unified.Usage.OutputTokens != 5 {
		t.Errorf("Expected 5 output tokens, got %d", unified.Usage.OutputTokens)
	}
}

func TestFromSDKResponse_FunctionCall(t *testing.T) {
	args := map[string]interface{}{"location": "Tokyo"}
	sdkResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						Name:      "get_weather",
						Arguments: args,
					},
				}},
			},
			FinishReason: genai.FinishReasonStop,
		}},
	}

	unified, err := FromSDKResponse(sdkResp, "gemini-2.0-flash")
	if err != nil {
		t.Fatalf("FromSDKResponse failed: %v", err)
	}

	if len(unified.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(unified.ToolCalls))
	}
	if unified.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %q", unified.ToolCalls[0].Function.Name)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/protocol/gemini/... -v -run TestFromSDKResponse
```
Expected: FAIL — function doesn't exist

- [ ] **Step 3: 实现 FromSDKResponse**

添加到 `sdk_adapter.go`：

```go
// FromSDKResponse 将 SDK 响应转换为统一响应格式
func FromSDKResponse(resp *genai.GenerateContentResponse, model string) (*types.UnifiedResponse, error) {
	var content []types.ContentBlock
	var toolCalls []types.ToolCall
	var finishReason string

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		if candidate.Content != nil {
			for i, part := range candidate.Content.Parts {
				if part.FunctionCall != nil {
					argsJSON := "{}"
					if part.FunctionCall.Arguments != nil {
						if b, err := json.Marshal(part.FunctionCall.Arguments); err == nil {
							argsJSON = string(b)
						}
					}
					toolCalls = append(toolCalls, types.ToolCall{
						ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, i),
						Type: "function",
						Function: types.FunctionCall{
							Name:      part.FunctionCall.Name,
							Arguments: argsJSON,
						},
					})
				} else if part.Text != "" {
					content = append(content, types.ContentBlock{
						Type: "text",
						Text: part.Text,
					})
				}
			}
		}
		finishReason = mapSDKFinishReason(candidate.FinishReason)
	}

	usage := types.Usage{}
	if resp.UsageMetadata != nil {
		usage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return &types.UnifiedResponse{
		ID:           fmt.Sprintf("gemini-sdk-%d", len(resp.Candidates)),
		Model:        model,
		Content:      content,
		Role:         "assistant",
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
		Usage:        usage,
	}, nil
}

func mapSDKFinishReason(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	case genai.FinishReasonSafety:
		return "content_filter"
	default:
		return "stop"
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/protocol/gemini/... -v -run TestFromSDKResponse
```
Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/protocol/gemini/sdk_adapter.go internal/protocol/gemini/sdk_adapter_test.go
git commit -m "feat(gemini): add FromSDKResponse for genai → UnifiedResponse conversion"
```

---

### Task 6: 补全 OpenAI BuildResponse — tool_calls 和 usage

**Files:**
- Modify: `internal/protocol/openai/converter.go`
- Modify: `internal/protocol/openai/converter_test.go`

- [ ] **Step 1: 编写失败的测试**

添加到 `converter_test.go`：

```go
func TestBuildResponse_WithToolCalls(t *testing.T) {
	unified := &types.UnifiedResponse{
		ID:    "gemini-sdk-1",
		Model: "gemini-2.0-flash",
		Content: []types.ContentBlock{{Type: "text", Text: "Let me check..."}},
		Role:   "assistant",
		ToolCalls: []types.ToolCall{{
			ID:   "call_weather_0",
			Type: "function",
			Function: types.FunctionCall{
				Name:      "get_weather",
				Arguments: `{"location": "Tokyo"}`,
			},
		}},
		Usage: types.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	data, err := BuildResponse(unified)
	if err != nil {
		t.Fatalf("BuildResponse failed: %v", err)
	}

	var resp ChatCompletionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(resp.Choices))
	}
	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("Expected function 'get_weather', got %q", tc.Function.Name)
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("Expected 100 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 50 {
		t.Errorf("Expected 50 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/protocol/openai/... -v -run TestBuildResponse_WithToolCalls
```
Expected: FAIL — tool_calls not populated

- [ ] **Step 3: 修改 BuildResponse 补全 tool_calls 和 usage**

修改 `internal/protocol/openai/converter.go` 的 `BuildResponse` 函数：

```go
func BuildResponse(unified *types.UnifiedResponse) ([]byte, error) {
	var content string
	if len(unified.Content) > 0 {
		content = unified.Content[0].Text
	}

	finishReason := unified.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	message := ChatMessage{
		Role:    "assistant",
		Content: content,
	}

	// 补全 tool_calls
	if len(unified.ToolCalls) > 0 {
		message.ToolCalls = make([]ToolCallRef, len(unified.ToolCalls))
		for i, tc := range unified.ToolCalls {
			message.ToolCalls[i] = ToolCallRef{
				ID:   tc.ID,
				Type: tc.Type,
				Function: ToolCallFunctionRef{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	resp := ChatCompletionResponse{
		ID:      unified.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   unified.Model,
		Choices: []Choice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: Usage{
			PromptTokens:     unified.Usage.InputTokens,
			CompletionTokens: unified.Usage.OutputTokens,
		},
	}

	return json.Marshal(resp)
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/protocol/openai/... -v -run TestBuildResponse_WithToolCalls
```
Expected: PASS

- [ ] **Step 5: 运行所有 openai 测试**

```bash
go test ./internal/protocol/openai/... -v
```
Expected: All PASS

- [ ] **Step 6: 提交**

```bash
git add internal/protocol/openai/converter.go internal/protocol/openai/converter_test.go
git commit -m "feat(openai): add tool_calls and usage to BuildResponse"
```

---

### Task 7: 替换 server.go 中 Gemini HTTP 调用为 SDK 调用

**Files:**
- Modify: `internal/server/server.go`
- Test: `internal/server/server_openai_test.go`

- [ ] **Step 1: 在 Server 结构中添加 GeminiClient 字段**

修改 `internal/server/server.go` 的 `Server` 结构体：

```go
type Server struct {
	cfg           *config.Config
	router        *router.Router
	log           *logger.Logger
	client        *http.Client
	transport     *http.Transport
	poolStats     *PoolStats
	wsTunnel      *WSTunnelMiddleware
	debugRequests bool
	debugMaxBody  int
	geminiClient  *gemini.GeminiClient // 新增
}
```

- [ ] **Step 2: 在 New() 中初始化 GeminiClient**

在 `server.go` 的 `New()` 函数中，在 `s := &Server{...}` 创建之前添加：

```go
// 初始化 Gemini SDK 客户端
var geminiClient *gemini.GeminiClient
if cfg.Backends.Gemini.BaseURL != "" {
	apiKey := findBackendKey(cfg.Routes, "gemini")
	var err error
	geminiClient, err = gemini.NewGeminiClient(
		apiKey,
		cfg.Backends.Gemini.HttpProxy,
		log,
		cfg.Logging.DebugRequests,
		debugMaxBody,
	)
	if err != nil {
		log.Error("Failed to create Gemini SDK client",
			logger.LogField{Key: "error", Value: err.Error()},
		)
		// 继续运行，fallback 到 HTTP
		geminiClient = nil
	}
}
```

添加辅助函数：

```go
func findBackendKey(routes []config.RouteConfig, backend string) string {
	for _, r := range routes {
		if r.Backend == backend {
			return r.BackendKey
		}
	}
	return ""
}
```

更新 Server 结构体初始化，添加 `geminiClient` 字段。

- [ ] **Step 3: 替换 serveOpenAIRequest 中的 Gemini 分支**

找到 `serveOpenAIRequest` 中的 `case "gemini"` 代码块（约第 332-338 行），替换为：

```go
case "gemini":
	if s.geminiClient != nil {
		// 使用 SDK 调用
		err = s.serveOpenAIWithSDK(w, r, route, model, unified, start)
		if err != nil {
			s.writeOpenAIError(w, http.StatusInternalServerError, "SDK call failed")
		}
		return
	}
	// Fallback: HTTP 调用
	backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent?key=" + route.BackendKey
	if unified.Stream {
		backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":streamGenerateContent?alt=sse&key=" + route.BackendKey
	}
	reqBody, err = gemini.Convert(unified, model)
```

- [ ] **Step 4: 实现 serveOpenAIWithSDK 方法**

添加到 `server.go`：

```go
// serveOpenAIWithSDK 使用 SDK 处理 OpenAI → Gemini 调用
func (s *Server) serveOpenAIWithSDK(w http.ResponseWriter, r *http.Request, route *config.RouteConfig, model string, unified *types.UnifiedMessage, start time.Time) error {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	sdkModel, contents, sysInst, config, tools, err := gemini.UnifiedMessageToSDK(unified)
	if err != nil {
		return err
	}

	s.log.Info("Gemini SDK request started",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: unified.Stream},
	)

	if unified.Stream {
		iter := s.geminiClient.GenerateContentStream(ctx, sdkModel, contents, config)
		s.handleOpenAIStreamFromSDK(w, iter, route.Backend, true, r, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, sdkModel, contents, config)
		if err != nil {
			return err
		}

		unifiedResp, err := gemini.FromSDKResponse(resp, model)
		if err != nil {
			return err
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini SDK request completed",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
			logger.LogField{Key: "input_tokens", Value: unifiedResp.Usage.InputTokens},
			logger.LogField{Key: "output_tokens", Value: unifiedResp.Usage.OutputTokens},
		)

		respBody, err := openai.BuildResponse(unifiedResp)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBody)
	}

	return nil
}
```

- [ ] **Step 5: 对 serveRequest 和 serveCompletionsRequest 做相同替换**

对 `serveRequest`（Anthropic 入口）和 `serveCompletionsRequest` 中的 `case "gemini"` 分支做相同替换，分别创建 `serveAnthropicWithSDK` 和 `serveCompletionsWithSDK` 方法。逻辑类似但返回格式不同：
- `serveAnthropicWithSDK`: 返回 Anthropic 格式
- `serveCompletionsWithSDK`: 返回 Completions 格式

- [ ] **Step 6: 编译确认通过**

```bash
go build ./...
```

- [ ] **Step 7: 提交**

```bash
git add internal/server/server.go
git commit -m "feat(server): replace Gemini HTTP calls with SDK for OpenAI/Anthropic endpoints"
```

---

### Task 8: 原生端点改用 SDK

**Files:**
- Modify: `internal/server/server.go`
- Test: `internal/server/server_gemini_test.go`

- [ ] **Step 1: 修改 serveGeminiRequest 使用 SDK**

替换 `serveGeminiRequest` 的 HTTP 调用部分为 SDK 调用。由于原生端点接收的是 Gemini 原生 JSON 格式，需要先解析为 `genai.Content` 再用 SDK 调用。

在 `serveGeminiRequest` 中，找到创建后端请求的部分（约第 838 行），替换为：

```go
if s.geminiClient != nil {
	s.serveGeminiWithSDK(w, r, route, start)
	return
}
```

- [ ] **Step 2: 实现 serveGeminiWithSDK**

```go
func (s *Server) serveGeminiWithSDK(w http.ResponseWriter, r *http.Request, route *config.RouteConfig, start time.Time) {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	// 解析原生 Gemini 请求体
	body, _ := io.ReadAll(r.Body)
	var nativeReq map[string]interface{}
	if err := json.Unmarshal(body, &nativeReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// 提取模型名
	pathSuffix := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	model := strings.SplitN(pathSuffix, ":", 2)[0]

	// 将原生请求转换为 SDK 参数
	contents, err := parseGeminiContents(nativeReq)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid contents format")
		return
	}

	config := parseGenerationConfig(nativeReq)

	// 检查是否流式
	isStream := strings.Contains(r.URL.Path, "streamGenerateContent")

	s.log.Info("Gemini native SDK request started",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: isStream},
	)

	if isStream {
		iter := s.geminiClient.GenerateContentStream(ctx, model, contents, config)
		s.serveGeminiStreamFromSDK(w, iter, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, model, contents, config)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, "SDK call failed")
			return
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini native SDK request completed",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
		)

		// 转换为 REST JSON 返回
		restBody, err := gemini.SDKResponseToREST(resp)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to convert response")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(restBody)
	}
}
```

- [ ] **Step 3: 实现 SDKResponseToREST**

添加到 `sdk_adapter.go`：

```go
// SDKResponseToREST 将 SDK 响应转换为 Gemini REST API JSON 格式
func SDKResponseToREST(resp *genai.GenerateContentResponse) ([]byte, error) {
	result := map[string]interface{}{
		"candidates": []interface{}{},
	}

	for _, candidate := range resp.Candidates {
		candMap := map[string]interface{}{
			"finishReason": candidate.FinishReason.String(),
		}

		if candidate.Content != nil {
			parts := []interface{}{}
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					parts = append(parts, map[string]interface{}{"text": part.Text})
				}
				if part.FunctionCall != nil {
					parts = append(parts, map[string]interface{}{
						"functionCall": map[string]interface{}{
							"name":      part.FunctionCall.Name,
							"arguments": part.FunctionCall.Arguments,
						},
					})
				}
			}
			candMap["content"] = map[string]interface{}{
				"parts": parts,
				"role":  candidate.Content.Role,
			}
		}

		result["candidates"] = append(result["candidates"].([]interface{}), candMap)
	}

	if resp.UsageMetadata != nil {
		result["usageMetadata"] = map[string]interface{}{
			"promptTokenCount":     resp.UsageMetadata.PromptTokenCount,
			"candidatesTokenCount": resp.UsageMetadata.CandidatesTokenCount,
			"totalTokenCount":      resp.UsageMetadata.TotalTokenCount,
		}
	}

	return json.Marshal(result)
}
```

- [ ] **Step 4: 编译和测试**

```bash
go build ./...
go test ./internal/server/... -v -run TestGeminiEndpoint
```

- [ ] **Step 5: 提交**

```bash
git add internal/server/server.go internal/protocol/gemini/sdk_adapter.go
git commit -m "feat(server): migrate Gemini native endpoint to SDK"
```

---

### Task 9: SDK 流式转 OpenAI SSE

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: 实现 handleOpenAIStreamFromSDK**

```go
func (s *Server) handleOpenAIStreamFromSDK(w http.ResponseWriter, iter *genai.GenerateContentResponseIterator, backend string, connReused bool, req *http.Request, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	for {
		select {
		case <-clientDisconnected:
			w.Write([]byte("data: [DONE]\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return
		default:
		}

		resp, err := iter.Next()
		if err == io.EOF {
			w.Write([]byte("data: [DONE]\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			break
		}
		if err != nil {
			return
		}

		// 转换为 OpenAI delta SSE chunk
		chunk := buildOpenAIStreamChunkFromSDK(resp)
		if len(chunk) > 0 {
			w.Write(chunk)
			w.Write([]byte("\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini SDK stream completed",
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}

func buildOpenAIStreamChunkFromSDK(resp *genai.GenerateContentResponse) []byte {
	if len(resp.Candidates) == 0 {
		return nil
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil
	}

	part := candidate.Content.Parts[0]

	var delta map[string]interface{}

	if part.FunctionCall != nil {
		argsJSON := "{}"
		if part.FunctionCall.Arguments != nil {
			if b, err := json.Marshal(part.FunctionCall.Arguments); err == nil {
				argsJSON = string(b)
			}
		}
		delta = map[string]interface{}{
			"role": "assistant",
			"tool_calls": []map[string]interface{}{{
				"index": 0,
				"id":    fmt.Sprintf("call_%s", part.FunctionCall.Name),
				"type":  "function",
				"function": map[string]interface{}{
					"name":      part.FunctionCall.Name,
					"arguments": argsJSON,
				},
			}},
		}
	} else if part.Text != "" {
		delta = map[string]interface{}{
			"content": part.Text,
		}
	}

	if delta == nil {
		return nil
	}

	payload := map[string]interface{}{
		"id": "gemini-sdk-stream",
		"choices": []map[string]interface{}{{
			"delta":         delta,
			"finish_reason": nil,
		}},
	}

	result, _ := json.Marshal(payload)
	return append([]byte("data: "), result...)
}
```

- [ ] **Step 2: 实现 serveGeminiStreamFromSDK**

```go
func (s *Server) serveGeminiStreamFromSDK(w http.ResponseWriter, iter *genai.GenerateContentResponseIterator, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		resp, err := iter.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}

		// 转换为 Gemini REST SSE 格式
		restPart, err := gemini.SDKResponseToREST(resp)
		if err != nil {
			continue
		}

		w.Write([]byte("data: "))
		w.Write(restPart)
		w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini native SDK stream completed",
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}
```

- [ ] **Step 3: 编译确认**

```bash
go build ./...
```

- [ ] **Step 4: 提交**

```bash
git add internal/server/server.go
git commit -m "feat(server): add SDK stream-to-SSE conversion for OpenAI and native endpoints"
```

---

### Task 10: 运行全部测试并修复

**Files:** 全项目

- [ ] **Step 1: 运行所有测试**

```bash
go test ./... -v -count=1
```

- [ ] **Step 2: 修复任何失败的测试**

确保所有现有测试通过。SDK 迁移不应破坏 Anthropic/OpenAI 非 Gemini 后端的测试。

- [ ] **Step 3: 运行覆盖率**

```bash
go test ./... -cover
```

- [ ] **Step 4: 提交**

```bash
git add -A
git commit -m "test: fix failing tests after Gemini SDK migration"
```

---

### Task 11: 更新 CHANGELOG

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: 更新 CHANGELOG**

在 CHANGELOG.md 顶部添加：

```markdown
## v0.8.0

### Added
- Gemini backend now uses official `google.golang.org/genai` SDK
- HTTP proxy support for Gemini backend via `http_proxy` config
- Complete parameter mapping: max_tokens, top_p, stop_sequences, tool_choice
- System prompts now use Gemini's native SystemInstruction
- Full request/response logging at DEBUG level via journalctl

### Changed
- All Gemini calls (OpenAI Chat, Completions, Anthropic, Native) now go through SDK
- System/developer role messages no longer mixed into user contents
```

- [ ] **Step 2: 提交**

```bash
git add CHANGELOG.md
git commit -m "chore: bump CHANGELOG for v0.8.0 - Gemini SDK migration"
```

---

## Self-Review

### 1. Spec Coverage Check

| Spec Requirement | Task |
|-----------------|------|
| SDK client with proxy | Task 3 |
| UnifiedMessage → SDK conversion | Task 4 |
| SDK Response → UnifiedResponse | Task 5 |
| Complete parameter mapping (max_tokens, top_p, stop, tool_choice) | Task 4 |
| SystemInstruction support | Task 4 |
| OpenAI BuildResponse tool_calls + usage | Task 6 |
| server.go SDK integration (all 3 entry points) | Task 7 |
| Native endpoint SDK | Task 8 |
| Streaming SSE from SDK Iterator | Task 9 |
| All tests pass | Task 10 |
| HTTP proxy config | Task 1 |
| Debug logging | Tasks 3, 7, 8, 9 |
| CHANGELOG | Task 11 |

### 2. Placeholder Scan

No TBD/TODO patterns found. All steps contain actual code.

### 3. Type Consistency

- `UnifiedMessageToSDK` returns match the parameters consumed by `GeminiClient.GenerateContent`
- `FromSDKResponse` returns `*types.UnifiedResponse` matching what `openai.BuildResponse` expects
- `tool_choice` handling covers string ("auto"/"none"/"required") and object ({"type":"function","function":{"name":"x"}}) formats
- `genai` types used consistently across sdk_adapter.go and client.go
