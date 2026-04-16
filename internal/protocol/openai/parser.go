package openai

import (
	"encoding/json"
	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// ParseRequest 解析 OpenAI 请求为统一格式
func ParseRequest(data []byte) (*types.UnifiedMessage, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	unified := &types.UnifiedMessage{
		Model:    req.Model,
		Messages: make([]types.MessageRole, len(req.Messages)),
		Stream:   req.Stream,
		MaxTokens: req.MaxTokens,
		Temperature: req.Temperature,
		TopP:     req.TopP,
	}

	// 转换消息格式
	for i, msg := range req.Messages {
		unified.Messages[i] = types.MessageRole{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	// Stop 参数映射
	if len(req.Stop) > 0 {
		unified.StopSequences = req.Stop
	}

	return unified, nil
}
