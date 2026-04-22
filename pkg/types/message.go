package types

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

// MessageRole 单条消息角色
type MessageRole struct {
	Role       string     `json:"role"`              // "user" | "assistant" | "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// UnifiedResponse 统一响应格式
type UnifiedResponse struct {
	ID           string         `json:"id"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	Role         string         `json:"role"`
	FinishReason string         `json:"finish_reason,omitempty"`
	Usage        Usage          `json:"usage"`
	ToolCalls    []ToolCall     `json:"tool_calls,omitempty"`
}

// ContentBlock 内容块
type ContentBlock struct {
	Type string `json:"type"` // "text"
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

// FunctionDefinition 函数定义（OpenAI 格式）
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// Tool 工具声明
type Tool struct {
	Type     string             `json:"type"`     // "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

// ToolCall 工具调用实例
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`     // "function"
	Function FunctionCall `json:"function"`
}
