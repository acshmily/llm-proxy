package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// mapToGeminiRole 映射 OpenAI/Claude 角色到 Gemini 兼容角色
func mapToGeminiRole(role string) string {
	switch role {
	case "system", "developer":
		return "user"
	case "assistant":
		return "model"
	case "tool", "tool_result":
		// Gemini 不支持 tool 角色，函数响应必须使用 user 角色
		return "user"
	default:
		return role
	}
}

// Convert 统一格式 -> Gemini 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	// 先合并连续相同角色的消息，避免 Gemini API 角色交替违规
	messages := mergeConsecutiveSameRoles(um.Messages)

	// Gemini 使用 contents 数组
	contents := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		parts := buildGeminiParts(msg, messages)
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

// mergeConsecutiveSameRoles 合并连续相同 Gemini 角色的消息。
// Gemini API 要求角色严格交替（user → model → user → model），
// 连续的 user 或 model 角色会导致 400 错误。
func mergeConsecutiveSameRoles(messages []types.MessageRole) []types.MessageRole {
	if len(messages) == 0 {
		return messages
	}

	var result []types.MessageRole
	for i := 0; i < len(messages); i++ {
		geminiRole := mapToGeminiRole(messages[i].Role)

		// 查找从 i 开始有多少条连续的同角色消息
		end := i + 1
		for end < len(messages) && mapToGeminiRole(messages[end].Role) == geminiRole {
			end++
		}

		if end-i == 1 {
			// 只有一条，不需要合并
			result = append(result, messages[i])
		} else {
			// 合并连续的同角色消息
			merged := mergeMessages(messages[i:end], geminiRole)
			result = append(result, merged)
		}

		i = end - 1 // 循环会 i++
	}

	return result
}

// mergeMessages 将多条同角色消息合并为一条
func mergeMessages(msgs []types.MessageRole, geminiRole string) types.MessageRole {
	merged := types.MessageRole{Role: msgs[0].Role} // 保留原始 role

	switch geminiRole {
	case "user":
		// user 消息：将所有文本内容拼接
		var parts []string
		for _, m := range msgs {
			if m.Content != "" {
				parts = append(parts, m.Content)
			}
		}
		merged.Content = strings.Join(parts, "\n\n")
	case "model":
		// model 消息：合并文本，保留所有 tool_calls
		var texts []string
		for _, m := range msgs {
			if m.Content != "" {
				texts = append(texts, m.Content)
			}
			merged.ToolCalls = append(merged.ToolCalls, m.ToolCalls...)
		}
		merged.Content = strings.Join(texts, "\n\n")
	default:
		// tool 等其他角色：不合并，只取第一条
		merged = msgs[0]
	}

	return merged
}

// buildGeminiParts 根据消息内容构建 Gemini parts（支持多个）
func buildGeminiParts(msg types.MessageRole, messages []types.MessageRole) []interface{} {
	// tool 角色：使用 functionResponse
	if msg.Role == "tool" {
		funcName := lookupFunctionName(msg.ToolCallID, messages)
		// Gemini API 要求 response 字段必须是 JSON 对象。
		// 如果工具返回的内容是 JSON 字符串，尝试解析为对象；
		// 否则包装在 "output" 字段下。
		var responseObj map[string]interface{}
		if msg.Content != "" {
			if err := json.Unmarshal([]byte(msg.Content), &responseObj); err == nil {
				// 已经是有效的 JSON 对象
			} else {
				// 不是 JSON 对象，包装起来
				responseObj = map[string]interface{}{"output": msg.Content}
			}
		} else {
			responseObj = map[string]interface{}{}
		}
		return []interface{}{
			map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"name":     funcName,
					"response": responseObj,
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
