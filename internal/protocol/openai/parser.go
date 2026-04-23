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

	// 转换消息，提取 content 为纯字符串
	for i, msg := range req.Messages {
		mr := types.MessageRole{
			Role:       msg.Role,
			Content:    extractContent(msg.Content),
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

// extractContent 从 JSON 字符串或内容数组中提取纯文本
func extractContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// 先尝试字符串格式
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}
	// 再尝试数组格式
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
