package openai

import (
	"encoding/json"
	"time"
	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// OpenAIFunctionDef OpenAI 函数定义
type OpenAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// OpenAITool OpenAI 工具声明
type OpenAITool struct {
	Type     string            `json:"type"`
	Function OpenAIFunctionDef `json:"function"`
}

// ToolCallFunctionRef OpenAI 格式的工具调用函数引用
type ToolCallFunctionRef struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallRef OpenAI 格式的工具调用引用
type ToolCallRef struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function ToolCallFunctionRef `json:"function"`
}

// OpenAIRequest OpenAI API 请求格式
type OpenAIRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Tools       []OpenAITool  `json:"tools,omitempty"`
}

// ChatMessage OpenAI 聊天消息
type ChatMessage struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"`
	ToolCalls  []ToolCallRef     `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

// Convert 统一格式 -> OpenAI 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	req := &OpenAIRequest{
		Model:       modelOverride,
		Messages:    make([]ChatMessage, len(um.Messages)),
		Stream:      um.Stream,
		MaxTokens:   um.MaxTokens,
		Temperature: um.Temperature,
		TopP:        um.TopP,
		Stop:        um.StopSequences,
	}

	for i, msg := range um.Messages {
		contentBytes, _ := json.Marshal(msg.Content)
		req.Messages[i] = ChatMessage{
			Role:    msg.Role,
			Content: contentBytes,
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

	// 处理空 choices 数组或缺失 message 的情况
	var content string
	var finishReason string
	if len(resp.Choices) > 0 {
		content = extractContent(resp.Choices[0].Message.Content)
		finishReason = resp.Choices[0].FinishReason
	}

	unified := &types.UnifiedResponse{
		ID:    resp.ID,
		Model: resp.Model,
		Content: []types.ContentBlock{
			{Type: "text", Text: content},
		},
		Role:         "assistant",
		FinishReason: finishReason,
		Usage: types.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	return unified, nil
}

// OpenAIResponse OpenAI API 响应格式
type OpenAIResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice OpenAI 响应选择
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// Usage OpenAI 使用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// ChatCompletionResponse OpenAI Chat Completion 响应
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// BuildResponse 将统一响应转换为 OpenAI 格式
func BuildResponse(unified *types.UnifiedResponse) ([]byte, error) {
	var content string
	if len(unified.Content) > 0 {
		content = unified.Content[0].Text
	}

	// 映射 finish_reason
	finishReason := unified.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	contentBytes, _ := json.Marshal(content)
	message := ChatMessage{
		Role:    "assistant",
		Content: contentBytes,
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
