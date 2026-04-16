package openai

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestParseResponse_ValidResponse(t *testing.T) {
	data := []byte(`{
		"id": "chatcmpl-123",
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": "Hello!"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5}
	}`)

	resp, err := ParseResponse(data)

	assert.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, "gpt-4", resp.Model)
	assert.Equal(t, 1, len(resp.Content))
	assert.Equal(t, "Hello!", resp.Content[0].Text)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestParseResponse_EmptyChoices(t *testing.T) {
	data := []byte(`{
		"id": "chatcmpl-123",
		"model": "gpt-4",
		"choices": [],
		"usage": {"prompt_tokens": 10, "completion_tokens": 0}
	}`)

	resp, err := ParseResponse(data)

	assert.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, "", resp.Content[0].Text)
}

func TestBuildResponse_ValidResponse(t *testing.T) {
	unified := &types.UnifiedResponse{
		ID:    "chatcmpl-456",
		Model: "gpt-4",
		Content: []types.ContentBlock{
			{Type: "text", Text: "Hello from assistant!"},
		},
		Role: "assistant",
		Usage: types.Usage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	}

	body, err := BuildResponse(unified)

	assert.NoError(t, err)
	assert.Contains(t, string(body), "chatcmpl-456")
	assert.Contains(t, string(body), "Hello from assistant!")
	assert.Contains(t, string(body), "gpt-4")
}

func TestBuildResponse_EmptyContent(t *testing.T) {
	unified := &types.UnifiedResponse{
		ID:    "chatcmpl-789",
		Model: "gpt-4",
		Content: []types.ContentBlock{},
		Role:  "assistant",
		Usage: types.Usage{
			InputTokens:  10,
			OutputTokens: 0,
		},
	}

	body, err := BuildResponse(unified)

	assert.NoError(t, err)
	assert.Contains(t, string(body), "chatcmpl-789")

	// 解析响应验证结构
	var resp map[string]interface{}
	err = json.Unmarshal(body, &resp)
	assert.NoError(t, err)
	assert.Equal(t, "chat.completion", resp["object"])
}
