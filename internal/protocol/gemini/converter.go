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
func ParseResponse(data []byte) (*types.UnifiedResponse, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var content []types.ContentBlock
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		content = append(content, types.ContentBlock{
			Type: "text",
			Text: resp.Candidates[0].Content.Parts[0].Text,
		})
	}

	return &types.UnifiedResponse{
		ID:      fmt.Sprintf("gemini-%d", len(resp.Candidates)),
		Model:   "gemini-pro",
		Content: content,
		Role:    "assistant",
	}, nil
}

type GeminiResponse struct {
	Candidates []Candidate `json:"candidates"`
}

type Candidate struct {
	Content Content `json:"content"`
}

type Content struct {
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}
