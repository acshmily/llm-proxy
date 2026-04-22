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

	// system(user) + user 合并为 1 条，加上 assistant → 共 2 条
	if len(contents) != 2 {
		t.Fatalf("Expected 2 contents (merged system+user + assistant), got %d", len(contents))
	}

	first := contents[0].(map[string]interface{})
	if first["role"] != "user" {
		t.Errorf("Expected system role merged to 'user', got '%v'", first["role"])
	}

	second := contents[1].(map[string]interface{})
	if second["role"] != "model" {
		t.Errorf("Expected assistant role mapped to 'model', got '%v'", second["role"])
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

func TestConvert_SanitizesToolSchema(t *testing.T) {
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash",
		Messages: []types.MessageRole{
			{Role: "user", Content: "Get weather"},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters: map[string]interface{}{
						"type":                 "object",
						"additionalProperties": false,
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":      "string",
								"default":   "unknown",
								"$ref":      "#/definitions/Location",
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

	tools := result["tools"].([]interface{})
	tool := tools[0].(map[string]interface{})
	funcDecls := tool["functionDeclarations"].([]interface{})
	params := funcDecls[0].(map[string]interface{})["parameters"].(map[string]interface{})

	// Verify unsupported fields are removed
	if _, ok := params["additionalProperties"]; ok {
		t.Error("Expected additionalProperties to be removed")
	}

	location := params["properties"].(map[string]interface{})["location"].(map[string]interface{})
	if _, ok := location["default"]; ok {
		t.Error("Expected default to be removed")
	}
	if _, ok := location["$ref"]; ok {
		t.Error("Expected $ref to be removed")
	}

	// Verify valid fields are preserved
	if params["type"] != "object" {
		t.Errorf("Expected type 'object', got %v", params["type"])
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

	// Verify response is the parsed JSON object, not wrapped in {"name": ..., "content": ...}
	respObj := funcResp["response"].(map[string]interface{})
	if respObj["temperature"] != float64(25) {
		t.Errorf("Expected response.temperature to be 25, got %v", respObj["temperature"])
	}
	if respObj["unit"] != "celsius" {
		t.Errorf("Expected response.unit to be 'celsius', got %v", respObj["unit"])
	}
	// Should NOT have the old "content" or "name" keys
	if _, ok := respObj["content"]; ok {
		t.Error("response should not have 'content' key (old format)")
	}
	if _, ok := respObj["name"]; ok {
		t.Error("response should not have 'name' key (old format)")
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

func TestConvert_MergeConsecutiveSameRoles(t *testing.T) {
	t.Run("system followed by user merges into one", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model: "gemini-2.5-flash",
			Messages: []types.MessageRole{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hello"},
			},
		}

		data, err := Convert(um, "gemini-2.5-flash")
		if err != nil {
			t.Fatal(err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}

		contents := req["contents"].([]interface{})
		// system(user) + user → should merge into 1 content
		if len(contents) != 1 {
			t.Fatalf("Expected 1 merged content (system→user + user), got %d", len(contents))
		}

		c := contents[0].(map[string]interface{})
		if c["role"] != "user" {
			t.Errorf("Expected role 'user', got %v", c["role"])
		}

		parts := c["parts"].([]interface{})
		if len(parts) != 1 {
			t.Fatalf("Expected 1 text part (merged), got %d", len(parts))
		}

		text := parts[0].(map[string]interface{})["text"]
		if text != "You are helpful.\n\nHello" {
			t.Errorf("Expected merged text 'You are helpful.\\n\\nHello', got %v", text)
		}
	})

	t.Run("multiple system messages merge into one", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model: "gemini-2.5-flash",
			Messages: []types.MessageRole{
				{Role: "system", Content: "Rule 1"},
				{Role: "system", Content: "Rule 2"},
				{Role: "user", Content: "Hello"},
			},
		}

		data, err := Convert(um, "gemini-2.5-flash")
		if err != nil {
			t.Fatal(err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}

		contents := req["contents"].([]interface{})
		// All 3 map to 'user' role (system→user, system→user, user→user) → merge into 1
		if len(contents) != 1 {
			t.Fatalf("Expected 1 merged content (all user role), got %d", len(contents))
		}

		// Content should be all 3 messages concatenated
		first := contents[0].(map[string]interface{})
		parts := first["parts"].([]interface{})
		if len(parts) != 1 {
			t.Fatalf("Expected 1 text part (merged), got %d", len(parts))
		}

		text := parts[0].(map[string]interface{})["text"]
		if text != "Rule 1\n\nRule 2\n\nHello" {
			t.Errorf("Expected merged text 'Rule 1\\n\\nRule 2\\n\\nHello', got %v", text)
		}
	})

	t.Run("proper alternation preserved", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model: "gemini-2.5-flash",
			Messages: []types.MessageRole{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
				{Role: "user", Content: "How are you?"},
			},
		}

		data, err := Convert(um, "gemini-2.5-flash")
		if err != nil {
			t.Fatal(err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}

		contents := req["contents"].([]interface{})
		if len(contents) != 3 {
			t.Fatalf("Expected 3 contents (no merge needed), got %d", len(contents))
		}
	})

	t.Run("complex alternation with tool calls preserved", func(t *testing.T) {
		um := &types.UnifiedMessage{
			Model: "gemini-2.5-flash",
			Messages: []types.MessageRole{
				{Role: "system", Content: "You are a weather assistant."},
				{Role: "developer", Content: "Use the get_weather tool."},
				{Role: "user", Content: "What's the weather in Tokyo?"},
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
					},
				},
				{Role: "tool", Content: `{"temp": 25}`, ToolCallID: "call_abc"},
				{Role: "assistant", Content: "It's 25°C in Tokyo."},
			},
		}

		data, err := Convert(um, "gemini-2.5-flash")
		if err != nil {
			t.Fatal(err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}

		contents := req["contents"].([]interface{})
		// system + developer + user → merge to 1 user
		// assistant with tool_calls → 1 model
		// tool → 1 tool
		// assistant → 1 model
		// Total: 4
		if len(contents) != 4 {
			t.Fatalf("Expected 4 contents, got %d", len(contents))
		}

		roles := make([]string, len(contents))
		for i, c := range contents {
			roles[i] = c.(map[string]interface{})["role"].(string)
		}

		expected := []string{"user", "model", "tool", "model"}
		for i, r := range roles {
			if r != expected[i] {
				t.Errorf("Content %d: expected role '%s', got '%s'", i, expected[i], r)
			}
		}
	})
}

func TestConvert_EmbeddedAgentScenario(t *testing.T) {
	// 模拟 OpenClaw embedded agent 的典型请求：system + user + assistant(tool_calls) + tool
	um := &types.UnifiedMessage{
		Model: "gemini-2.5-flash-lite",
		Messages: []types.MessageRole{
			{Role: "system", Content: "You are a helpful assistant. Use tools when appropriate."},
			{Role: "user", Content: "Hello, what can you do?"},
			{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{
						ID:   "call_abc123",
						Type: "function",
						Function: types.FunctionCall{
							Name:      "get_capabilities",
							Arguments: `{}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    `{"capabilities": ["search", "summarize"]}`,
				ToolCallID: "call_abc123",
			},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDefinition{
					Name:        "get_capabilities",
					Description: "Get assistant capabilities",
					Parameters: map[string]interface{}{
						"type":                 "object",
						"properties":           map[string]interface{}{},
						"additionalProperties": false,
					},
				},
			},
		},
	}

	data, err := Convert(um, "gemini-2.5-flash-lite")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Generated JSON:\n%s", string(data))

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	contents := result["contents"].([]interface{})
	t.Logf("Number of contents: %d", len(contents))
	for i, c := range contents {
		cm := c.(map[string]interface{})
		t.Logf("Content[%d]: role=%v, parts=%v", i, cm["role"], cm["parts"])
	}

	// 验证：
	// 1. system + user → 合并为 1 条 user
	// 2. assistant with toolCalls → 1 条 model
	// 3. tool → 1 条 tool
	// 总共应该有 3 条内容
	if len(contents) != 3 {
		t.Errorf("Expected 3 contents, got %d", len(contents))
	}

	// 验证角色交替
	expectedRoles := []string{"user", "model", "tool"}
	for i, c := range contents {
		cm := c.(map[string]interface{})
		if cm["role"] != expectedRoles[i] {
			t.Errorf("Content[%d]: expected role '%s', got '%v'", i, expectedRoles[i], cm["role"])
		}
	}

	// 验证每条都有非空 parts
	for i, c := range contents {
		cm := c.(map[string]interface{})
		parts := cm["parts"].([]interface{})
		if len(parts) == 0 {
			t.Errorf("Content[%d]: has empty parts array", i)
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
