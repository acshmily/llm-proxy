package anthropic

import (
	"encoding/json"
	"github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

// Request Anthropic API 请求
type Request struct {
	Model         string    `json:"model"`
	Messages      []Message `json:"messages"`
	Stream        bool      `json:"stream,omitempty"`
	MaxTokens     int       `json:"max_tokens,omitempty"`
	Temperature   float64   `json:"temperature,omitempty"`
	TopP          float64   `json:"top_p,omitempty"`
	StopSequences []string  `json:"stop_sequences,omitempty"`
}

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ParseRequest 解析为统一格式
func ParseRequest(data []byte) (*types.UnifiedMessage, error) {
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	unified := &types.UnifiedMessage{
		Model:         req.Model,
		Stream:        req.Stream,
		MaxTokens:     req.MaxTokens,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		StopSequences: req.StopSequences,
	}

	for _, msg := range req.Messages {
		var content string
		for _, c := range msg.Content {
			if c.Type == "text" {
				content += c.Text
			}
		}
		unified.Messages = append(unified.Messages, types.MessageRole{
			Role:    msg.Role,
			Content: content,
		})
	}

	return unified, nil
}
