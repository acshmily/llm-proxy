package claude

import (
	"encoding/json"
	"github.com/claude-projetc/llm-proxy/pkg/types"
)

// Convert 统一格式 -> Claude 格式（原生）
func Convert(um *types.UnifiedMessage, modelOverride string) ([]byte, error) {
	req := map[string]interface{}{
		"model":    modelOverride,
		"messages": um.Messages,
		"stream":   um.Stream,
	}
	if um.MaxTokens > 0 {
		req["max_tokens"] = um.MaxTokens
	}
	if um.Temperature > 0 {
		req["temperature"] = um.Temperature
	}
	return json.Marshal(req)
}

// ParseResponse 解析 Claude 响应
func ParseResponse(data []byte) (*types.UnifiedResponse, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var content []types.ContentBlock
	if contentArr, ok := resp["content"].([]interface{}); ok {
		for _, c := range contentArr {
			if cm, ok := c.(map[string]interface{}); ok {
				if text, ok := cm["text"].(string); ok {
					content = append(content, types.ContentBlock{Type: "text", Text: text})
				}
			}
		}
	}

	usage := types.Usage{}
	if usageMap, ok := resp["usage"].(map[string]interface{}); ok {
		if it, ok := usageMap["input_tokens"].(float64); ok {
			usage.InputTokens = int(it)
		}
		if ot, ok := usageMap["output_tokens"].(float64); ok {
			usage.OutputTokens = int(ot)
		}
	}

	return &types.UnifiedResponse{
		ID:      getString(resp, "id"),
		Model:   getString(resp, "model"),
		Content: content,
		Role:    "assistant",
		Usage:   usage,
	}, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
