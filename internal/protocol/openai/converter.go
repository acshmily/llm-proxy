package openai

import (
	"encoding/json"
	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// OpenAIRequest OpenAI API 请求格式
type OpenAIRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

// ChatMessage OpenAI 聊天消息
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

// OpenAIResponse OpenAI API 响应格式
type OpenAIResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice OpenAI 响应选择
type Choice struct {
	Message ChatMessage `json:"message"`
}

// Usage OpenAI 使用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
