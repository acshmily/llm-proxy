package gemini

import (
	"encoding/json"
	"testing"

	"github.com/claude-projetc/llm-proxy/pkg/types"
)

func TestMapToGeminiRole(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"system", "user"},
		{"developer", "user"},
		{"tool", "tool"},
		{"assistant", "model"},
		{"user", "user"},
		{"model", "model"},
		{"tool_result", "tool_result"}, // unknown role passthrough
	}

	for _, tt := range tests {
		if got := mapToGeminiRole(tt.input); got != tt.expected {
			t.Errorf("mapToGeminiRole(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConvert_RoleMapping(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-pro",
		Messages: []types.MessageRole{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
		},
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

	first := contents[0].(map[string]interface{})
	if first["role"] != "user" {
		t.Errorf("Expected system role mapped to 'user', got '%v'", first["role"])
	}

	third := contents[2].(map[string]interface{})
	if third["role"] != "model" {
		t.Errorf("Expected assistant role mapped to 'model', got '%v'", third["role"])
	}
}

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

		resp, err := ParseResponse([]byte(input), "gemini-pro")
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

		resp, err := ParseResponse([]byte(input), "gemini-pro")
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

		resp, err := ParseResponse([]byte(input), "gemini-pro")
		if err != nil {
			t.Fatalf("ParseResponse failed: %v", err)
		}

		if len(resp.Content) != 0 {
			t.Errorf("Expected 0 content blocks, got %d", len(resp.Content))
		}
	})

	t.Run("model parameter propagation", func(t *testing.T) {
		input := `{
			"candidates": [{
				"content": {
					"parts": [{"text": "response"}]
				}
			}]
		}`

		for _, model := range []string{"gemini-2.5-flash", "gemini-2.0-pro", "gemini-pro"} {
			resp, err := ParseResponse([]byte(input), model)
			if err != nil {
				t.Fatalf("ParseResponse failed for %s: %v", model, err)
			}
			if resp.Model != model {
				t.Errorf("Expected model '%s', got '%s'", model, resp.Model)
			}
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		// Important: 测试无效 JSON 输入
		_, err := ParseResponse([]byte(`{invalid json}`), "gemini-pro")
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

func TestParseResponse_WithFunctionCall(t *testing.T) {
	// Simulate Gemini returning functionCall response
	data := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"arguments": {"location": "Tokyo"}
					}
				}],
				"role": "model"
			},
			"finishReason": "STOP"
		}]
	}`)

	resp, err := ParseResponse(data, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call, got %d", len(resp.ToolCalls))
	}

	tc := resp.ToolCalls[0]
	if tc.ID == "" {
		t.Error("Expected non-empty tool_call ID")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got '%s'", tc.Function.Name)
	}

	expectedArgs := `{"location":"Tokyo"}`
	if tc.Function.Arguments != expectedArgs {
		t.Errorf("Expected arguments '%s', got '%s'", expectedArgs, tc.Function.Arguments)
	}

	// Should have no text content
	if len(resp.Content) != 0 {
		t.Errorf("Expected 0 text content blocks, got %d", len(resp.Content))
	}
}

func TestConvert_WithTools(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "What's the weather in Tokyo?"},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather in a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city name",
							},
						},
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("Expected tools array in result")
	}

	tool := tools[0].(map[string]interface{})
	funcDecls, ok := tool["functionDeclarations"].([]interface{})
	if !ok || len(funcDecls) == 0 {
		t.Fatal("Expected functionDeclarations in tool")
	}

	funcDecl := funcDecls[0].(map[string]interface{})
	if funcDecl["name"] != "get_weather" {
		t.Errorf("Expected name 'get_weather', got %v", funcDecl["name"])
	}
}

func TestConvert_ToolResult(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "What's the weather in Tokyo?"},
			{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_abc123",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "Tokyo"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"temperature": 25, "unit": "celsius"}`,
				ToolCallID: "call_abc123", // matches the assistant's tool_call ID, not the function name
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("Expected contents array")
	}

	thirdMsg := contents[2].(map[string]interface{})
	if thirdMsg["role"] != "tool" {
		t.Errorf("Expected role 'tool', got %v", thirdMsg["role"])
	}

	parts := thirdMsg["parts"].([]interface{})
	part := parts[0].(map[string]interface{})
	funcResp, ok := part["functionResponse"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected functionResponse part")
	}
	if funcResp["name"] != "get_weather" {
		t.Errorf("Expected functionResponse name 'get_weather' (looked up from tool_call ID), got %v", funcResp["name"])
	}
}

func TestConvert_MixedTextAndToolCalls(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Get weather for Tokyo"},
			{
				Role:    "assistant",
				Content: "Let me check the weather for you.",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_abc",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "Tokyo"}`,
						},
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	contents := result["contents"].([]interface{})
	secondMsg := contents[1].(map[string]interface{})
	parts := secondMsg["parts"].([]interface{})

	// Should have 2 parts: text + functionCall
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts (text + functionCall), got %d", len(parts))
	}

	// First part should be text
	textPart := parts[0].(map[string]interface{})
	if textPart["text"] != "Let me check the weather for you." {
		t.Errorf("Expected text part with content, got %v", textPart["text"])
	}

	// Second part should be functionCall
	funcPart := parts[1].(map[string]interface{})
	if _, ok := funcPart["functionCall"]; !ok {
		t.Error("Expected functionCall part as second part")
	}
}

func TestConvert_MultipleToolCalls(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Get weather for Tokyo and Paris"},
			{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_abc",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "Tokyo"}`,
						},
					},
					{
						ID:   "call_def",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "Paris"}`,
						},
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash")
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("Expected contents array")
	}
	if len(contents) != 2 {
		t.Fatalf("Expected 2 contents, got %d", len(contents))
	}

	// 第二条消息应该有 2 个 functionCall parts
	secondMsg := contents[1].(map[string]interface{})
	if secondMsg["role"] != "model" {
		t.Errorf("Expected role 'model', got %v", secondMsg["role"])
	}

	parts := secondMsg["parts"].([]interface{})
	if len(parts) != 2 {
		t.Fatalf("Expected 2 parts for multiple tool_calls, got %d", len(parts))
	}

	// 验证两个 parts 都是 functionCall
	for i, part := range parts {
		p := part.(map[string]interface{})
		if _, ok := p["functionCall"]; !ok {
			t.Errorf("Expected functionCall at index %d", i)
		}
	}
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
