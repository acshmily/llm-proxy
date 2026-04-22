# Gemini 工具调用支持实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Gemini 后端添加 Function Calling 支持，解决 OpenClaw embedded agent 调用时返回 400 错误的问题。

**Architecture:** 在现有架构上增量扩展 — 类型层添加工具相关结构，Gemini 转换器映射到 Gemini 的 functionDeclarations/functionCall/functionResponse 格式，Server 解析层提取工具调用内容。

**Tech Stack:** Go 1.21+, 标准库 encoding/json, 现有协议转换架构

**设计文档:** `docs/superpowers/specs/2026-04-22-gemini-tool-calling-design.md`

---

## 文件结构

| 文件 | 操作 | 职责 |
|------|------|------|
| `pkg/types/message.go` | 修改 | 添加 Tool、ToolCall、FunctionDefinition、FunctionCall 类型定义 |
| `internal/protocol/gemini/converter.go` | 修改 | 处理 tools、tool_calls、tool 角色映射到 Gemini 格式 |
| `internal/protocol/gemini/converter_test.go` | 修改 | 添加工具调用的单元测试 |
| `internal/server/server.go` | 修改 | 扩展 extractContent 支持 tool_use/tool_result 内容块提取 |
| `internal/server/extract_content_test.go` | 修改 | 添加工具内容提取的测试 |

---

### Task 1: 扩展类型定义

**Files:**
- Modify: `pkg/types/message.go`

- [ ] **Step 1: 添加类型定义**

在 `pkg/types/message.go` 末尾添加以下类型（在现有代码之后，文件最后的 `}` 之前不要插入，加在最后）：

```go
// FunctionDefinition 函数定义（OpenAI 格式）
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Tool 工具声明
type Tool struct {
	Type     string             `json:"type"`     // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall 工具调用实例
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`     // "function"
	Function FunctionCall `json:"function"`
}
```

- [ ] **Step 2: 扩展 UnifiedMessage**

将现有的 `UnifiedMessage` 结构修改为：

```go
// UnifiedMessage 统一中间格式消息
type UnifiedMessage struct {
	Model         string        `json:"model"`
	Messages      []MessageRole `json:"messages"`
	Stream        bool          `json:"stream"`
	MaxTokens     int           `json:"max_tokens,omitempty"`
	Temperature   float64       `json:"temperature,omitempty"`
	TopP          float64       `json:"top_p,omitempty"`
	StopSequences []string      `json:"stop_sequences,omitempty"`
	Tools         []Tool        `json:"tools,omitempty"`
	ToolChoice    interface{}   `json:"tool_choice,omitempty"`
}
```

- [ ] **Step 3: 扩展 MessageRole**

将现有的 `MessageRole` 结构修改为：

```go
// MessageRole 单条消息角色
type MessageRole struct {
	Role       string     `json:"role"`              // "user" | "assistant" | "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
```

- [ ] **Step 4: 验证编译通过**

Run: `go build ./...`
Expected: 无编译错误

- [ ] **Step 5: 运行现有测试确保向后兼容**

Run: `go test ./... -v`
Expected: 所有测试通过

- [ ] **Step 6: 提交**

```bash
git add pkg/types/message.go
git commit -m "feat: add Tool, ToolCall types to unified message format"
```

---

### Task 2: 扩展 Gemini 转换器支持工具调用

**Files:**
- Modify: `internal/protocol/gemini/converter.go`
- Modify: `internal/protocol/gemini/converter_test.go`

- [ ] **Step 1: 编写测试 — 带 tools 的请求转换**

在 `internal/protocol/gemini/converter_test.go` 中添加：

```go
func TestConvert_WithTools(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "What's the weather in Tokyo?"},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather in a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city name",
							},
						},
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("Expected tools array in result")
	}

	tool := tools[0].(map[string]interface{})
	funcDecls, ok := tool["functionDeclarations"].([]interface{})
	if !ok || len(funcDecls) == 0 {
		t.Fatal("Expected functionDeclarations in tool")
	}

	funcDecl := funcDecls[0].(map[string]interface{})
	if funcDecl["name"] != "get_weather" {
		t.Errorf("Expected name 'get_weather', got %v", funcDecl["name"])
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/protocol/gemini/... -run TestConvert_WithTools -v`
Expected: FAIL (功能未实现)

- [ ] **Step 3: 修改 Convert 函数支持 tools**

将 `internal/protocol/gemini/converter.go` 的 `Convert` 函数替换为：

```go
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	// Gemini 使用 contents 数组
	contents := make([]map[string]interface{}, len(um.Messages))
	for i, msg := range um.Messages {
		part := buildGeminiPart(msg)
		contents[i] = map[string]interface{}{
			"role":  mapToGeminiRole(msg.Role),
			"parts": []interface{}{part},
		}
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	// 添加 tools 支持
	if len(um.Tools) > 0 {
		funcDeclarations := make([]map[string]interface{}, len(um.Tools))
		for i, tool := range um.Tools {
			funcDeclarations[i] = map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  tool.Function.Parameters,
			}
		}
		req["tools"] = []map[string]interface{}{
			{"functionDeclarations": funcDeclarations},
		}
	}

	if um.Temperature > 0 {
		req["generationConfig"] = map[string]interface{}{
			"temperature": um.Temperature,
		}
	}

	return json.Marshal(req)
}

// buildGeminiPart 根据消息内容构建 Gemini part
func buildGeminiPart(msg types.MessageRole) map[string]interface{} {
	// tool 角色：使用 functionResponse
	if msg.Role == "tool" {
		return map[string]interface{}{
			"functionResponse": map[string]interface{}{
				"name": msg.ToolCallID,
				"response": map[string]interface{}{
					"name":    msg.ToolCallID,
					"content": msg.Content,
				},
			},
		}
	}

	// assistant 带 tool_calls：使用 functionCall
	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		// 优先返回 tool_call，如果有文本内容则合并
		if msg.Content == "" && len(msg.ToolCalls) == 1 {
			// 解析 arguments JSON 字符串为对象
			var args map[string]interface{}
			json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args)
			return map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name":      msg.ToolCalls[0].Function.Name,
					"arguments": args,
				},
			}
		}
	}

	// 默认：文本 part
	return map[string]string{
		"text": msg.Content,
	}
}
```

- [ ] **Step 4: 添加辅助测试 — tool 角色消息转换**

在测试文件中添加：

```go
func TestConvert_ToolResult(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "What's the weather in Tokyo?"},
			{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_abc123",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "Tokyo"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"temperature": 25, "unit": "celsius"}`,
				ToolCallID: "get_weather",
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("Expected contents array")
	}

	// 第三条消息应该是 functionResponse
	thirdMsg := contents[2].(map[string]interface{})
	if thirdMsg["role"] != "tool" {
		t.Errorf("Expected role 'tool', got %v", thirdMsg["role"])
	}

	parts := thirdMsg["parts"].([]interface{})
	part := parts[0].(map[string]interface{})
	funcResp, ok := part["functionResponse"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected functionResponse part")
	}
	if funcResp["name"] != "get_weather" {
		t.Errorf("Expected functionResponse name 'get_weather', got %v", funcResp["name"])
	}
}
```

- [ ] **Step 5: 运行所有测试**

Run: `go test ./internal/protocol/gemini/... -v`
Expected: 所有测试通过（包括新增的和现有的）

- [ ] **Step 6: 提交**

```bash
git add internal/protocol/gemini/converter.go internal/protocol/gemini/converter_test.go
git commit -m "feat: add tool calling support to Gemini converter"
```

---

### Task 3: 扩展 Server 解析层提取工具调用内容

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/server/extract_content_test.go`

- [ ] **Step 1: 编写测试 — tool_use 内容块提取**

在 `internal/server/extract_content_test.go` 末尾添加：

```go
func TestExtractContentWithToolUse(t *testing.T) {
	// tool_use 类型应该返回空字符串（工具调用通过 ToolCalls 字段传递）
	content := json.RawMessage(`[{"type": "tool_use", "id": "call_abc", "name": "get_weather", "input": {"location": "Tokyo"}}]`)
	result := extractContent(content)
	assert.Equal(t, "", result)
}

func TestExtractContentWithToolResult(t *testing.T) {
	// tool_result 类型应该返回 content 文本
	content := json.RawMessage(`[{"type": "tool_result", "tool_use_id": "call_abc", "content": "Temperature: 25°C"}]`)
	result := extractContent(content)
	assert.Equal(t, "Temperature: 25°C", result)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/server/... -run TestExtractContentWithTool -v`
Expected: FAIL（尚未实现）

- [ ] **Step 3: 修改 extractContent 支持工具内容块**

将 `internal/server/server.go` 中的 `extractContent` 函数替换为：

```go
// extractContent extracts text content from either a JSON string or an array of content blocks.
// OpenClaw sends array format: [{"type": "text", "text": "..."}]
// Standard clients send string format: "Hello"
func extractContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// Try string format first
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}
	// Try array format
	var blocks []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(content, &blocks); err == nil {
		var result string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				result += b.Text
			}
			if b.Type == "tool_result" && b.Content != "" {
				result += b.Content
			}
		}
		return result
	}
	return ""
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/server/... -run TestExtractContentWithTool -v`
Expected: PASS

- [ ] **Step 5: 运行全部测试确保无回归**

Run: `go test ./... -v`
Expected: 全部通过

- [ ] **Step 6: 提交**

```bash
git add internal/server/server.go internal/server/extract_content_test.go
git commit -m "feat: extract tool_result content in server parser"
```

---

### Task 4: 扩展 OpenAI 解析器传递 tools 和 tool_calls

**Files:**
- Modify: `internal/protocol/openai/parser.go`
- Modify: `internal/protocol/openai/converter.go`

> **说明：** `/v1/chat/completions` 端点使用 OpenAI 解析器，需要从中提取 tools 和 tool_calls 字段。

- [ ] **Step 1: 编写测试 — OpenAI 请求包含 tools 时的解析**

在 `internal/protocol/openai/parser_test.go` 末尾添加：

```go
func TestParseRequest_WithTools(t *testing.T) {
	data := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [{"role": "user", "content": "What's the weather?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {"type": "object"}
			}
		}]
	}`)

	unified, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(unified.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(unified.Tools))
	}
	if unified.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got %s", unified.Tools[0].Function.Name)
	}
}

func TestParseRequest_WithToolCalls(t *testing.T) {
	data := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [
			{"role": "user", "content": "What's the weather?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\": \"Tokyo\"}"
					}
				}]
			},
			{"role": "tool", "tool_call_id": "call_abc", "content": "25°C"}
		]
	}`)

	unified, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}

	// 检查 assistant 消息的 tool_calls
	if len(unified.Messages[1].ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call in assistant message")
	}
	if unified.Messages[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather'")
	}

	// 检查 tool 角色
	if unified.Messages[2].Role != "tool" {
		t.Errorf("Expected role 'tool', got %s", unified.Messages[2].Role)
	}
	if unified.Messages[2].ToolCallID != "call_abc" {
		t.Errorf("Expected tool_call_id 'call_abc', got %s", unified.Messages[2].ToolCallID)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/protocol/openai/... -run "TestParseRequest_WithTool" -v`
Expected: FAIL

- [ ] **Step 3: 修改 ChatMessage 和 ParseRequest**

在 `internal/protocol/openai/converter.go` 中，将 `ChatMessage` 结构修改为：

```go
// ChatMessage OpenAI 聊天消息
type ChatMessage struct {
	Role       string       `json:"role"`
	Content    string       `json:"content"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string       `json:"tool_call_id,omitempty"`
}
```

在 `internal/protocol/openai/converter.go` 中 `ChatMessage` 定义之前添加（如果 ToolCall 尚未在此包定义）：

```go
// ToolCallRef OpenAI 格式的工具调用引用
type ToolCallRef struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
```

修改 `internal/protocol/openai/converter.go` 中的 `ChatMessage` 为：

```go
// ChatMessage OpenAI 聊天消息
type ChatMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []ToolCallRef   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}
```

在 `internal/protocol/openai/parser.go` 中，将 `ParseRequest` 函数替换为：

```go
func ParseRequest(data []byte) (*types.UnifiedMessage, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	unified := &types.UnifiedMessage{
		Model:       req.Model,
		Messages:    make([]types.MessageRole, len(req.Messages)),
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Tools:       make([]types.Tool, len(req.Tools)),
	}

	// 转换 tools
	for i, tool := range req.Tools {
		unified.Tools[i] = types.Tool{
			Type: tool.Type,
			Function: types.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		}
	}

	// 转换消息
	for i, msg := range req.Messages {
		mr := types.MessageRole{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		// 转换 tool_calls
		if len(msg.ToolCalls) > 0 {
			mr.ToolCalls = make([]types.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				mr.ToolCalls[j] = types.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: types.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		unified.Messages[i] = mr
	}

	if len(req.Stop) > 0 {
		unified.StopSequences = req.Stop
	}

	return unified, nil
}
```

- [ ] **Step 4: 扩展 OpenAIRequest 支持 tools 字段**

在 `internal/protocol/openai/converter.go` 中，将 `OpenAIRequest` 修改为：

```go
// OpenAIRequest OpenAI API 请求格式
type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Stream      bool            `json:"stream,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	TopP        float64         `json:"top_p,omitempty"`
	Stop        []string        `json:"stop,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
}

// OpenAIFunctionDef OpenAI 函数定义
type OpenAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// OpenAITool OpenAI 工具声明
type OpenAITool struct {
	Type     string            `json:"type"`
	Function OpenAIFunctionDef `json:"function"`
}

// ToolCallRef OpenAI 格式的工具调用引用
type ToolCallRef struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
```

- [ ] **Step 5: 运行所有测试**

Run: `go test ./... -v`
Expected: 全部通过

- [ ] **Step 6: 提交**

```bash
git add internal/protocol/openai/parser.go internal/protocol/openai/converter.go internal/protocol/openai/parser_test.go
git commit -m "feat: parse tools and tool_calls in OpenAI request parser"
```

---

### Task 5: 流式响应中的工具调用支持

**Files:**
- Modify: `internal/server/server.go` (convertGeminiSSEToOpenAI 函数)
- Modify: `internal/protocol/gemini/converter_test.go` (流式测试)

- [ ] **Step 1: 编写测试 — Gemini 流式响应包含 functionCall**

在 `internal/protocol/gemini/converter_test.go` 末尾添加：

```go
func TestParseResponse_WithFunctionCall(t *testing.T) {
	// 模拟 Gemini 返回 functionCall 的响应
	data := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"arguments": {"location": "Tokyo"}
					}
				}],
				"role": "model"
			},
			"finishReason": "STOP"
		}]
	}`)

	resp, err := ParseResponse(data, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	// 当前 ParseResponse 只处理 text part，functionCall 应被忽略
	// 此测试验证不会 panic 或崩溃
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/protocol/gemini/... -run TestParseResponse_WithFunctionCall -v`
Expected: PASS（当前代码已经不会崩溃，因为 Parts 数组中存在 functionCall 时 Text 为空）

- [ ] **Step 3: 修改 convertGeminiSSEToOpenAI 处理 functionCall part**

在 `internal/server/server.go` 中找到 `convertGeminiSSEToOpenAI` 函数，替换为：

```go
// convertGeminiSSEToOpenAI 转换 Gemini SSE 为 OpenAI 格式
func convertGeminiSSEToOpenAI(event string, data []byte) []byte {
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         string `json:"text"`
					FunctionCall *struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &geminiResp); err != nil {
		return nil
	}

	if len(geminiResp.Candidates) == 0 {
		return nil
	}

	candidate := geminiResp.Candidates[0]

	// 处理 functionCall part
	if len(candidate.Content.Parts) > 0 {
		part := candidate.Content.Parts[0]
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Arguments)
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]interface{}{
							"role": "assistant",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    "call_stream",
									"type":  "function",
									"function": map[string]interface{}{
										"name":      part.FunctionCall.Name,
										"arguments": string(args),
									},
								},
							},
						},
						"finish_reason": nil,
					},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	// 处理文本 part
	if len(candidate.Content.Parts) > 0 && candidate.Content.Parts[0].Text != "" {
		text := candidate.Content.Parts[0].Text
		finishReason := candidate.FinishReason

		var openaiFinishReason string
		switch finishReason {
		case "STOP":
			openaiFinishReason = "stop"
		case "MAX_TOKENS":
			openaiFinishReason = "length"
		default:
			openaiFinishReason = "stop"
		}

		if text != "" {
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{"content": text}, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
		if finishReason != "" {
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{}, "finish_reason": openaiFinishReason},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	return nil
}
```

- [ ] **Step 4: 运行全部测试**

Run: `go test ./... -v`
Expected: 全部通过

- [ ] **Step 5: 提交**

```bash
git add internal/server/server.go internal/protocol/gemini/converter_test.go
git commit -m "feat: handle functionCall in Gemini streaming response"
```

---

### Task 6: 端到端集成测试

**Files:**
- Create: `internal/server/integration_tool_test.go`

> **说明：** 验证完整链路：OpenAI 解析 → 统一格式 → Gemini 转换

- [ ] **Step 1: 编写端到端测试**

创建 `internal/server/integration_tool_test.go`：

```go
package server

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/protocol/gemini"
	"github.com/claude-projetc/llm-proxy/internal/protocol/openai"
)

func TestEndToEnd_ToolCalling(t *testing.T) {
	// 模拟 OpenClaw 发送的 OpenAI 格式请求
	openaiReq := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [
			{"role": "user", "content": "What's the weather in Tokyo?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\": \"Tokyo\"}"
					}
				}]
			},
			{"role": "tool", "tool_call_id": "call_abc123", "content": "{\"temperature\": 25, \"unit\": \"celsius\"}"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get the current weather in a location",
				"parameters": {
					"type": "object",
					"properties": {
						"location": {"type": "string", "description": "The city name"}
					}
				}
			}
		}]
	}`)

	// Step 1: OpenAI 解析器解析
	unified, err := openai.ParseRequest(openaiReq)
	if err != nil {
		t.Fatalf("OpenAI ParseRequest failed: %v", err)
	}

	// 验证 tools 被正确解析
	if len(unified.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(unified.Tools))
	}

	// 验证 tool_calls 被正确解析
	if len(unified.Messages[1].ToolCalls) != 1 {
		t.Fatalf("Expected tool_calls in assistant message")
	}

	// 验证 tool 角色
	if unified.Messages[2].Role != "tool" {
		t.Fatalf("Expected role 'tool', got %s", unified.Messages[2].Role)
	}

	// Step 2: Gemini 转换器转换
	geminiReq, err := gemini.Convert(unified, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("Gemini Convert failed: %v", err)
	}

	// Step 3: 验证最终 Gemini 请求格式
	var result map[string]interface{}
	if err := json.Unmarshal(geminiReq, &result); err != nil {
		t.Fatalf("Failed to unmarshal Gemini request: %v", err)
	}

	// 验证 contents
	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("Expected contents array")
	}
	if len(contents) != 3 {
		t.Fatalf("Expected 3 contents, got %d", len(contents))
	}

	// 验证第一条消息是 user
	firstMsg := contents[0].(map[string]interface{})
	if firstMsg["role"] != "user" {
		t.Errorf("Expected first role 'user', got %v", firstMsg["role"])
	}

	// 验证 tools
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("Expected tools array in final Gemini request")
	}
	tool := tools[0].(map[string]interface{})
	funcDecls := tool["functionDeclarations"].([]interface{})
	funcDecl := funcDecls[0].(map[string]interface{})
	if funcDecl["name"] != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %v", funcDecl["name"])
	}

	// 打印最终的 Gemini 请求用于调试
	t.Logf("Final Gemini request: %s", string(geminiReq))
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/server/... -run TestEndToEnd_ToolCalling -v`
Expected: PASS

- [ ] **Step 3: 运行全部测试最终确认**

Run: `go test ./... -v`
Expected: 全部通过，零失败

- [ ] **Step 4: 提交**

```bash
git add internal/server/integration_tool_test.go
git commit -m "test: add end-to-end tool calling integration test"
```
