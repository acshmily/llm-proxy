package claude

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

func TestConvert(t *testing.T) {
	t.Run("basic conversion", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:  "claude-3",
			Stream: true,
			Messages: []types.MessageRole{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
			},
			MaxTokens:   100,
			Temperature: 0.7,
		}

		data, err := Convert(um, "claude-3-opus")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if req["model"] != "claude-3-opus" {
			t.Errorf("Expected model 'claude-3-opus', got '%v'", req["model"])
		}
		if req["stream"] != true {
			t.Error("Expected stream to be true")
		}
		if req["max_tokens"] != float64(100) {
			t.Errorf("Expected max_tokens 100, got %v", req["max_tokens"])
		}
		if req["temperature"] != 0.7 {
			t.Errorf("Expected temperature 0.7, got %v", req["temperature"])
		}
	})

	t.Run("minimal conversion", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:    "claude-3",
			Messages: []types.MessageRole{{Role: "user", Content: "Test"}},
		}

		data, err := Convert(um, "claude-3-sonnet")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		json.Unmarshal(data, &req)

		if _, ok := req["max_tokens"]; ok {
			t.Error("Expected no max_tokens when 0")
		}
		if _, ok := req["temperature"]; ok {
			t.Error("Expected no temperature when 0")
		}
	})
}

func TestParseResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		input := `{
			"id": "msg_123",
			"model": "claude-3-opus",
			"content": [
				{"type": "text", "text": "Hello, I am Claude!"}
			],
			"usage": {
				"input_tokens": 10,
				"output_tokens": 20
			}
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if resp.ID != "msg_123" {
			t.Errorf("Expected id 'msg_123', got '%s'", resp.ID)
		}
		if resp.Model != "claude-3-opus" {
			t.Errorf("Expected model 'claude-3-opus', got '%s'", resp.Model)
		}
		if len(resp.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(resp.Content))
		}
		if resp.Content[0].Text != "Hello, I am Claude!" {
			t.Errorf("Expected content 'Hello, I am Claude!', got '%s'", resp.Content[0].Text)
		}
		if resp.Usage.InputTokens != 10 {
			t.Errorf("Expected input tokens 10, got %d", resp.Usage.InputTokens)
		}
		if resp.Usage.OutputTokens != 20 {
			t.Errorf("Expected output tokens 20, got %d", resp.Usage.OutputTokens)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		input := `{
			"id": "msg_empty",
			"model": "claude-3",
			"content": [],
			"usage": {"input_tokens": 0, "output_tokens": 0}
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 0 {
			t.Errorf("Expected 0 content blocks, got %d", len(resp.Content))
		}
	})

	t.Run("no usage", func(t *testing.T) {
		input := `{
			"id": "msg_nousage",
			"model": "claude-3",
			"content": [{"type": "text", "text": "Hi"}]
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if resp.Usage.InputTokens != 0 {
			t.Errorf("Expected 0 input tokens, got %d", resp.Usage.InputTokens)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// Important: 测试无效 JSON 输入
		_, err := ParseResponse([]byte(`{invalid json}`))
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

func TestGetString(t *testing.T) {
	t.Run("existing key", func(t *testing.T) {
		m := map[string]interface{}{"key": "value"}
		result := getString(m, "key")
		if result != "value" {
			t.Errorf("Expected 'value', got '%s'", result)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		m := map[string]interface{}{"other": "value"}
		result := getString(m, "key")
		if result != "" {
			t.Errorf("Expected empty string, got '%s'", result)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		m := map[string]interface{}{"key": 123}
		result := getString(m, "key")
		if result != "" {
			t.Errorf("Expected empty string for non-string type, got '%s'", result)
		}
	})
}

func TestClaudeRoundTrip(t *testing.T) {
	t.Run("convert then verify", func(t *testing.T) {
		original := &types.UnifiedMessage{
			Model:  "claude-3",
			Stream: false,
			Messages: []types.MessageRole{
				{Role: "user", Content: "Test message"},
			},
			MaxTokens: 50,
		}

		data, err := Convert(original, "claude-3-opus")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if req["model"] != "claude-3-opus" {
			t.Errorf("Model mismatch: expected 'claude-3-opus', got '%v'", req["model"])
		}
	})
}
