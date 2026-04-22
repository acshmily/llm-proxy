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
		part := buildGeminiPart(msg)
		contents[i] = map[string]interface{}{
			"role":  mapToGeminiRole(msg.Role),
			"parts": []interface{}{part},
		}
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	// 添加 tools 支持
	if len(um.Tools) > 0 {
		funcDeclarations := make([]map[string]interface{}, len(um.Tools))
		for i, tool := range um.Tools {
			funcDeclarations[i] = map[string]interface{}{
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
				"parameters":  tool.Function.Parameters,
			}
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

// buildGeminiPart 根据消息内容构建 Gemini part
func buildGeminiPart(msg types.MessageRole) map[string]interface{} {
	// tool 角色：使用 functionResponse
	if msg.Role == "tool" {
		return map[string]interface{}{
			"functionResponse": map[string]interface{}{
				"name": msg.ToolCallID,
				"response": map[string]interface{}{
					"name":    msg.ToolCallID,
					"content": msg.Content,
				},
			},
		}
	}

	// assistant 带 tool_calls：使用 functionCall
	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		if msg.Content == "" && len(msg.ToolCalls) == 1 {
			var args map[string]interface{}
			json.Unmarshal([]byte(msg.ToolCalls[0].Function.Arguments), &args)
			return map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name":      msg.ToolCalls[0].Function.Name,
					"arguments": args,
				},
			}
		}
	}

	// 默认：文本 part
	return map[string]interface{}{
		"text": msg.Content,
	}
}

// ParseResponse 解析 Gemini 响应
func ParseResponse(data []byte, model string) (*types.UnifiedResponse, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var content []types.ContentBlock
	var finishReason string
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		content = append(content, types.ContentBlock{
			Type: "text",
			Text: resp.Candidates[0].Content.Parts[0].Text,
		})
		// 映射 Gemini finish_reason 到 OpenAI 标准值
		finishReason = mapGeminiFinishReason(resp.Candidates[0].FinishReason)
	}

	return &types.UnifiedResponse{
		ID:           fmt.Sprintf("gemini-%d", len(resp.Candidates)),
		Model:        model,
		Content:      content,
		Role:         "assistant",
		FinishReason: finishReason,
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
	Text string `json:"text"`
}
