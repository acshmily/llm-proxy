package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRequest_ValidRequest(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [
			{"role": "system", "content": "You are helpful."},
			{"role": "user", "content": "Hello"}
		],
		"stream": false,
		"temperature": 0.7,
		"max_tokens": 1024
	}`)

	req, err := ParseRequest(body)

	assert.NoError(t, err)
	assert.Equal(t, "gpt-4", req.Model)
	assert.Equal(t, 2, len(req.Messages))
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "You are helpful.", req.Messages[0].Content)
	assert.Equal(t, "user", req.Messages[1].Role)
	assert.Equal(t, "Hello", req.Messages[1].Content)
	assert.Equal(t, false, req.Stream)
	assert.Equal(t, 0.7, req.Temperature)
	assert.Equal(t, 1024, req.MaxTokens)
}

func TestParseRequest_InvalidJSON(t *testing.T) {
	body := []byte(`{invalid json}`)
	req, err := ParseRequest(body)

	assert.Error(t, err)
	assert.Nil(t, req)
}

func TestParseRequest_EmptyMessages(t *testing.T) {
	body := []byte(`{"model": "gpt-4", "messages": []}`)
	req, err := ParseRequest(body)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(req.Messages))
}

func TestParseRequest_MissingModel(t *testing.T) {
	body := []byte(`{"messages": [{"role": "user", "content": "Hi"}]}`)
	req, err := ParseRequest(body)

	assert.NoError(t, err)
	assert.Equal(t, "", req.Model)
}

func TestParseRequest_WithTools(t *testing.T) {
	data := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [{"role": "user", "content": "What's the weather?"}],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get weather",
				"parameters": {"type": "object"}
			}
		}]
	}`)

	unified, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(unified.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(unified.Tools))
	}
	if unified.Tools[0].Function.Name != "get_weather" {
		t.Errorf("Expected tool name 'get_weather', got %s", unified.Tools[0].Function.Name)
	}
}

func TestParseRequest_WithToolCalls(t *testing.T) {
	data := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [
			{"role": "user", "content": "What's the weather?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\": \"Tokyo\"}"
					}
				}]
			},
			{"role": "tool", "tool_call_id": "call_abc", "content": "25°C"}
		]
	}`)

	unified, err := ParseRequest(data)
	if err != nil {
		t.Fatal(err)
	}

	// Check assistant message tool_calls
	if len(unified.Messages[1].ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool_call in assistant message")
	}
	if unified.Messages[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("Expected function name 'get_weather'")
	}

	// Check tool role
	if unified.Messages[2].Role != "tool" {
		t.Errorf("Expected role 'tool', got %s", unified.Messages[2].Role)
	}
	if unified.Messages[2].ToolCallID != "call_abc" {
		t.Errorf("Expected tool_call_id 'call_abc', got %s", unified.Messages[2].ToolCallID)
	}
}

func TestParseRequest_WithAllParameters(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hi"}],
		"stream": true,
		"max_tokens": 2048,
		"temperature": 0.5,
		"top_p": 0.9,
		"stop": ["\n", "END"],
		"presence_penalty": 0.1,
		"frequency_penalty": 0.2,
		"user": "user-123"
	}`)

	req, err := ParseRequest(body)

	assert.NoError(t, err)
	assert.Equal(t, "gpt-4", req.Model)
	assert.Equal(t, true, req.Stream)
	assert.Equal(t, 2048, req.MaxTokens)
	assert.Equal(t, 0.5, req.Temperature)
	assert.Equal(t, 0.9, req.TopP)
	assert.Equal(t, []string{"\n", "END"}, req.StopSequences)
	// 注意：PresencePenalty, FrequencyPenalty, User 字段当前未映射到 UnifiedMessage
	// 这些字段可以在后续扩展中添加
}
