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
}

// MessageRole 单条消息角色
type MessageRole struct {
	Role    string `json:"role"`    // "user" | "assistant"
	Content string `json:"content"`
}

// UnifiedResponse 统一响应格式
type UnifiedResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Content []ContentBlock `json:"content"`
	Role    string         `json:"role"`
	Usage   Usage          `json:"usage"`
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
