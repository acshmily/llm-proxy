package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// Convert 统一格式 -> Gemini 格式
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	// Gemini 使用 contents 数组
	contents := make([]map[string]interface{}, len(um.Messages))
	for i, msg := range um.Messages {
		contents[i] = map[string]interface{}{
			"role": msg.Role,
			"parts": []map[string]string{
				{"text": msg.Content},
			},
		}
	}

	req := map[string]interface{}{
		"contents": contents,
	}

	if um.Temperature > 0 {
		req["generationConfig"] = map[string]interface{}{
			"temperature": um.Temperature,
		}
	}

	return json.Marshal(req)
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
