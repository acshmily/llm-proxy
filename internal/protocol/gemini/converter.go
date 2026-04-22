package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// mapToGeminiRole 映射 OpenAI/Claude 角色到 Gemini 兼容角色
func mapToGeminiRole(role string) string {
	switch role {
	case "system", "developer":
		return "user"
	case "assistant":
		return "model"
	default:
		return role
	}
}

// Convert 统一格式 -> Gemini 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	// Gemini 使用 contents 数组
	contents := make([]map[string]interface{}, len(um.Messages))
	for i, msg := range um.Messages {
		parts := buildGeminiParts(msg, um.Messages)
		contents[i] = map[string]interface{}{
			"role":  mapToGeminiRole(msg.Role),
			"parts": parts,
		}
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	// 添加 tools 支持
	if len(um.Tools) > 0 {
		funcDeclarations := make([]map[string]interface{}, len(um.Tools))
		for i, tool := range um.Tools {
			funcDecl := map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
			}
			if params := sanitizeSchemaForGemini(tool.Function.Parameters); params != nil {
				funcDecl["parameters"] = params
			}
			funcDeclarations[i] = funcDecl
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

// buildGeminiParts 根据消息内容构建 Gemini parts（支持多个）
func buildGeminiParts(msg types.MessageRole, messages []types.MessageRole) []interface{} {
	// tool 角色：使用 functionResponse
	if msg.Role == "tool" {
		funcName := lookupFunctionName(msg.ToolCallID, messages)
		return []interface{}{
			map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"name": funcName,
					"response": map[string]interface{}{
						"name":    funcName,
						"content": msg.Content,
					},
				},
			},
		}
	}

	// assistant 带 tool_calls：为每个 tool_call 生成一个 functionCall part
	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		var parts []interface{}
		// 先加入文本 part（如果有）
		if msg.Content != "" {
			parts = append(parts, map[string]interface{}{
				"text": msg.Content,
			})
		}
		// 追加所有 functionCall parts
		for _, tc := range msg.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = make(map[string]interface{})
			}
			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name":      tc.Function.Name,
					"arguments": args,
				},
			})
		}
		return parts
	}

	// 默认：文本 part
	return []interface{}{
		map[string]interface{}{
			"text": msg.Content,
		},
	}
}

// lookupFunctionName 通过 tool_call_id 从 assistant 消息中查找函数名
func lookupFunctionName(toolCallID string, messages []types.MessageRole) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			for _, tc := range messages[i].ToolCalls {
				if tc.ID == toolCallID {
					return tc.Function.Name
				}
			}
		}
	}
	// 如果未找到匹配的，退回使用 tool_call_id
	return toolCallID
}

// ParseResponse 解析 Gemini 响应
func ParseResponse(data []byte, model string) (*types.UnifiedResponse, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var content []types.ContentBlock
	var toolCalls []types.ToolCall
	var finishReason string

	if len(resp.Candidates) > 0 {
		for i, part := range resp.Candidates[0].Content.Parts {
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Arguments)
				toolCalls = append(toolCalls, types.ToolCall{
					ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, i),
					Type: "function",
					Function: types.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(args),
					},
				})
			} else if part.Text != "" {
				content = append(content, types.ContentBlock{
					Type: "text",
					Text: part.Text,
				})
			}
		}
		finishReason = mapGeminiFinishReason(resp.Candidates[0].FinishReason)
	}

	return &types.UnifiedResponse{
		ID:           fmt.Sprintf("gemini-%d", len(resp.Candidates)),
		Model:        model,
		Content:      content,
		Role:         "assistant",
		FinishReason: finishReason,
		ToolCalls:    toolCalls,
	}, nil
}

// mapGeminiFinishReason 映射 Gemini finish_reason 到 OpenAI 标准值
func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION":
		return "content_filter"
	default:
		return "stop"
	}
}

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content      Content `json:"content"`
	FinishReason string  `json:"finishReason"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text         string                 `json:"text"`
	FunctionCall *FunctionCallPart      `json:"functionCall,omitempty"`
}

type FunctionCallPart struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}
