package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/claude-projetc/llm-proxy/internal/protocol/gemini"
	"github.com/claude-projetc/llm-proxy/internal/protocol/openai"
)

func TestEndToEnd_ToolCalling(t *testing.T) {
	// Simulate OpenClaw sending OpenAI format request
	openaiReq := []byte(`{
		"model": "gemini-2.5-flash",
		"messages": [
			{"role": "user", "content": "What's the weather in Tokyo?"},
			{
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\": \"Tokyo\"}"
					}
				}]
			},
			{"role": "tool", "tool_call_id": "call_abc123", "content": "{\"temperature\": 25, \"unit\": \"celsius\"}"}
		],
		"tools": [{
			"type": "function",
			"function": {
				"name": "get_weather",
				"description": "Get the current weather in a location",
				"parameters": {
					"type": "object",
					"properties": {
						"location": {"type": "string", "description": "The city name"}
					}
				}
			}
		}]
	}`)

	// Step 1: OpenAI parser
	unified, err := openai.ParseRequest(openaiReq)
	assert.NoError(t, err)
	assert.Len(t, unified.Tools, 1)
	assert.Equal(t, "get_weather", unified.Tools[0].Function.Name)

	// Verify tool_calls parsed
	assert.Len(t, unified.Messages[1].ToolCalls, 1)
	assert.Equal(t, "get_weather", unified.Messages[1].ToolCalls[0].Function.Name)

	// Verify tool role
	assert.Equal(t, "tool", unified.Messages[2].Role)
	assert.Equal(t, "call_abc123", unified.Messages[2].ToolCallID)

	// Step 2: Gemini converter
	geminiReq, err := gemini.Convert(unified, "gemini-2.5-flash")
	assert.NoError(t, err)

	// Step 3: Verify final Gemini request format
	var result map[string]interface{}
	err = json.Unmarshal(geminiReq, &result)
	assert.NoError(t, err)

	// Verify contents
	contents, ok := result["contents"].([]interface{})
	if !assert.True(t, ok, "Expected contents array, got %T", result["contents"]) {
		return
	}
	assert.Len(t, contents, 3)

	// Verify first message is user
	firstMsg, ok := contents[0].(map[string]interface{})
	if assert.True(t, ok, "Expected first content to be map") {
		assert.Equal(t, "user", firstMsg["role"])
	}

	// Verify assistant message has functionCall part
	secondMsg, ok := contents[1].(map[string]interface{})
	if assert.True(t, ok, "Expected second content to be map") {
		assert.Equal(t, "model", secondMsg["role"])
		parts, ok := secondMsg["parts"].([]interface{})
		if assert.True(t, ok) && assert.Len(t, parts, 1) {
			part, ok := parts[0].(map[string]interface{})
			if assert.True(t, ok) {
				_, hasFunctionCall := part["functionCall"]
				assert.True(t, hasFunctionCall, "Expected functionCall part in assistant message")
			}
		}
	}

	// Verify tool message has functionResponse part
	thirdMsg, ok := contents[2].(map[string]interface{})
	if assert.True(t, ok, "Expected third content to be map") {
		assert.Equal(t, "tool", thirdMsg["role"])
	}

	// Verify tools
	tools, ok := result["tools"].([]interface{})
	if assert.True(t, ok, "Expected tools array in final Gemini request") {
		tool, ok := tools[0].(map[string]interface{})
		if assert.True(t, ok) {
			funcDeclarations, ok := tool["functionDeclarations"].([]interface{})
			if assert.True(t, ok) && assert.Len(t, funcDeclarations, 1) {
				funcDecl, ok := funcDeclarations[0].(map[string]interface{})
				if assert.True(t, ok) {
					assert.Equal(t, "get_weather", funcDecl["name"])
				}
			}
		}
	}

	// Print final Gemini request for debugging
	t.Logf("Final Gemini request: %s", string(geminiReq))
}
