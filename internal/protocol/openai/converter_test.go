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

func TestBuildResponse_WithToolCalls(t *testing.T) {
	unified := &types.UnifiedResponse{
		ID:      "gemini-sdk-1",
		Model:   "gemini-2.0-flash",
		Content: []types.ContentBlock{{Type: "text", Text: "Let me check..."}},
		Role:    "assistant",
		ToolCalls: []types.ToolCall{{
			ID:   "call_weather_0",
			Type: "function",
			Function: types.FunctionCall{
				Name:      "get_weather",
				Arguments: `{"location": "Tokyo"}`,
			},
		}},
		Usage: types.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}

	data, err := BuildResponse(unified)
	if err != nil {
		t.Fatalf("BuildResponse failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	choices := resp["choices"].([]interface{})
	if len(choices) != 1 {
		t.Fatalf("Expected 1 choice, got %d", len(choices))
	}
	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})

	toolCalls, ok := message["tool_calls"].([]interface{})
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %v", message["tool_calls"])
	}
	tc := toolCalls[0].(map[string]interface{})
	if tc["id"] != "call_weather_0" {
		t.Errorf("Expected tool_call id 'call_weather_0', got %q", tc["id"])
	}
	fn := tc["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("Expected function 'get_weather', got %q", fn["name"])
	}

	usage := resp["usage"].(map[string]interface{})
	if int(usage["prompt_tokens"].(float64)) != 100 {
		t.Errorf("Expected 100 prompt tokens, got %v", usage["prompt_tokens"])
	}
	if int(usage["completion_tokens"].(float64)) != 50 {
		t.Errorf("Expected 50 completion tokens, got %v", usage["completion_tokens"])
	}
}
