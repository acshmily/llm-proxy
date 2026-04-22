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
		Model:       req.Model,
		Messages:    make([]types.MessageRole, len(req.Messages)),
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Tools:       make([]types.Tool, len(req.Tools)),
	}

	// 转换工具定义
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

		// 转换工具调用
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

	// Stop 参数映射
	if len(req.Stop) > 0 {
		unified.StopSequences = req.Stop
	}

	return unified, nil
}
