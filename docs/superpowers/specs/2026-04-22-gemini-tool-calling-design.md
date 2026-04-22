# Gemini 工具调用支持设计

## 问题描述

OpenClaw embedded agent 通过代理调用 Gemini 后端时返回 400 Bad Request，因为代理的 Gemini 转换器不支持工具调用格式。当前所有 `tool` 角色被错误映射为 `user`，`tools` 参数被丢弃，`tool_calls` 字段未被处理。

## 目标

为 Gemini 后端添加 Function Calling 支持，使 OpenClaw embedded agent 能通过代理正常使用 Gemini 的工具调用能力。

## 范围

- **仅 Gemini 后端**，不影响已有的 OpenAI/Anthropic 逻辑
- 支持 **function calling**（`functionDeclarations` / `functionCall` / `functionResponse`）
- 支持 **流式和非流式** 两种模式
- 不涉及 code execution 或可配置 `functionCallingConfig`

## 设计

### 1. 类型层扩展

在 `pkg/types/message.go` 中新增工具相关类型：

```go
// FunctionDefinition 函数定义
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

扩展现有结构：

```go
// UnifiedMessage 新增
type UnifiedMessage struct {
    // ... 现有字段
    Tools      []Tool      `json:"tools,omitempty"`
    ToolChoice interface{} `json:"tool_choice,omitempty"`
}

// MessageRole 新增
type MessageRole struct {
    // ... 现有字段
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"`
}
```

### 2. Gemini 转换器扩展

`internal/protocol/gemini/converter.go` 需要处理：

- **tools → functionDeclarations**: 将 OpenAI 格式的 `tools` 数组转为 Gemini 的 `tools[0].functionDeclarations` 结构
- **tool_calls → functionCall**: assistant 消息中的 `tool_calls` 转为 Gemini 的 `functionCall` part
- **tool 角色 → functionResponse**: tool 角色消息（含 `tool_call_id`）转为 Gemini 的 `functionResponse` part
- **functionCallingConfig**: 当有 tools 时设为 `AUTO` 模式

### 3. Server 解析层扩展

`internal/server/server.go` 的 `extractContent` 需要扩展：

- 检测 `type: "tool_use"` 的 content block，提取为 ToolCall 信息
- 检测 `type: "tool_result"` 的 content block，提取 tool_call_id 和结果文本
- 将这些信息通过新函数附加到对应的 MessageRole

### 4. 流式响应扩展

Gemini 流式响应中可能包含 `functionCall` 类型的 part，需要在 `convertGeminiSSEToOpenAI` 中处理。

## 非目标

- Code execution 支持
- 可配置 functionCallingConfig（手动/自动切换）
- OpenAI/Anthropic 后端的工具调用改进
- 多模态内容（图片、文件）支持

## 风险评估

- Gemini 的 function calling 格式与 OpenAI 有差异（arguments 为对象而非字符串），转换器需要正确处理
- 流式模式下 tool calls 可能跨多个 SSE 事件传递
