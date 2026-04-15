package openai

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

func TestConvert(t *testing.T) {
	t.Run("basic conversion", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:  "gpt-4",
			Stream: true,
			Messages: []types.MessageRole{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
			},
			MaxTokens:   100,
			Temperature: 0.7,
			TopP:        0.9,
		}

		data, err := Convert(um, "gpt-4-turbo")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req OpenAIRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if req.Model != "gpt-4-turbo" {
			t.Errorf("Expected model 'gpt-4-turbo', got '%s'", req.Model)
		}
		if !req.Stream {
			t.Error("Expected stream to be true")
		}
		if req.MaxTokens != 100 {
			t.Errorf("Expected max_tokens 100, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(req.Messages))
		}
	})

	t.Run("minimal conversion", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:    "gpt-3.5-turbo",
			Messages: []types.MessageRole{{Role: "user", Content: "Test"}},
		}

		data, err := Convert(um, "gpt-3.5-turbo")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req OpenAIRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if req.Stream {
			t.Error("Expected stream to be false (default)")
		}
		if req.MaxTokens != 0 {
			t.Errorf("Expected max_tokens 0, got %d", req.MaxTokens)
		}
	})
}

func TestParseResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		input := `{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Hello, I am Claude!"
				}
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 20
			}
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if resp.ID != "chatcmpl-123" {
			t.Errorf("Expected id 'chatcmpl-123', got '%s'", resp.ID)
		}
		if resp.Model != "gpt-4" {
			t.Errorf("Expected model 'gpt-4', got '%s'", resp.Model)
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

	t.Run("empty response", func(t *testing.T) {
		input := `{
			"id": "chatcmpl-empty",
			"model": "gpt-4",
			"choices": [{}],
			"usage": {"prompt_tokens": 0, "completion_tokens": 0}
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(resp.Content))
		}
		if resp.Content[0].Text != "" {
			t.Errorf("Expected empty content, got '%s'", resp.Content[0].Text)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("convert then parse", func(t *testing.T) {
		original := &types.UnifiedMessage{
			Model:  "gpt-4",
			Stream: false,
			Messages: []types.MessageRole{
				{Role: "user", Content: "Test message"},
			},
			MaxTokens:   50,
			Temperature: 0.5,
		}

		// Convert to OpenAI format
		data, err := Convert(original, "gpt-4")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		// Parse back
		var req OpenAIRequest
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		// Verify round-trip
		if req.Model != "gpt-4" {
			t.Errorf("Model mismatch: expected 'gpt-4', got '%s'", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Errorf("Message count mismatch: expected 1, got %d", len(req.Messages))
		}
		if req.Messages[0].Content != "Test message" {
			t.Errorf("Content mismatch: expected 'Test message', got '%s'", req.Messages[0].Content)
		}
	})
}
