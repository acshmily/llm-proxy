package gemini

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

func TestConvert(t *testing.T) {
	t.Run("basic conversion", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:  "gemini-pro",
			Stream: true,
			Messages: []types.MessageRole{
				{Role: "user", Content: "Hello"},
				{Role: "model", Content: "Hi there"},
				{Role: "user", Content: "How are you?"},
			},
			Temperature: 0.7,
		}

		data, err := Convert(um, "gemini-pro")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		contents, ok := req["contents"].([]interface{})
		if !ok {
			t.Fatal("Expected contents array")
		}
		if len(contents) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(contents))
		}

		genConfig, ok := req["generationConfig"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected generationConfig")
		}
		if temp, ok := genConfig["temperature"].(float64); !ok || temp != 0.7 {
			t.Errorf("Expected temperature 0.7, got %v", genConfig["temperature"])
		}
	})

	t.Run("no temperature", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model:    "gemini-pro",
			Messages: []types.MessageRole{{Role: "user", Content: "Test"}},
		}

		data, err := Convert(um, "gemini-pro")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		json.Unmarshal(data, &req)

		if _, ok := req["generationConfig"]; ok {
			t.Error("Expected no generationConfig when temperature is 0")
		}
	})
}

func TestParseResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		input := `{
			"candidates": [{
				"content": {
					"parts": [{
						"text": "Hello, I am Gemini!"
					}]
				}
			}]
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 1 {
			t.Errorf("Expected 1 content block, got %d", len(resp.Content))
		}
		if resp.Content[0].Text != "Hello, I am Gemini!" {
			t.Errorf("Expected content 'Hello, I am Gemini!', got '%s'", resp.Content[0].Text)
		}
		if resp.Model != "gemini-pro" {
			t.Errorf("Expected model 'gemini-pro', got '%s'", resp.Model)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		input := `{"candidates": []}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 0 {
			t.Errorf("Expected 0 content blocks, got %d", len(resp.Content))
		}
	})

	t.Run("empty parts", func(t *testing.T) {
		input := `{
			"candidates": [{
				"content": {
					"parts": []
				}
			}]
		}`

		resp, err := ParseResponse([]byte(input))
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 0 {
			t.Errorf("Expected 0 content blocks, got %d", len(resp.Content))
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

func TestGeminiRoundTrip(t *testing.T) {
	t.Run("convert then verify", func(t *testing.T) {
		original := &types.UnifiedMessage{
			Model:  "gemini-pro",
			Stream: false,
			Messages: []types.MessageRole{
				{Role: "user", Content: "Test message"},
			},
			Temperature: 0.8,
		}

		data, err := Convert(original, "gemini-pro")
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		contents, ok := req["contents"].([]interface{})
		if !ok {
			t.Fatal("Expected contents array")
		}
		if len(contents) != 1 {
			t.Errorf("Expected 1 message, got %d", len(contents))
		}
	})
}
