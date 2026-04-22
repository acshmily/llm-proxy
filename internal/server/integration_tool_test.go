package server

import (
	"encoding/json"
	"testing"

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
	if err != nil {
		t.Fatalf("OpenAI ParseRequest failed: %v", err)
	}

	// Verify tools parsed
	if len(unified.Tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(unified.Tools))
	}

	// Verify tool_calls parsed
	if len(unified.Messages[1].ToolCalls) != 1 {
		t.Fatalf("Expected tool_calls in assistant message")
	}

	// Verify tool role
	if unified.Messages[2].Role != "tool" {
		t.Fatalf("Expected role 'tool', got %s", unified.Messages[2].Role)
	}

	// Step 2: Gemini converter
	geminiReq, err := gemini.Convert(unified, "gemini-2.5-flash")
	if err != nil {
		t.Fatalf("Gemini Convert failed: %v", err)
	}

	// Step 3: Verify final Gemini request format
	var result map[string]interface{}
	if err := json.Unmarshal(geminiReq, &result); err != nil {
		t.Fatalf("Failed to unmarshal Gemini request: %v", err)
	}

	// Verify contents
	contents, ok := result["contents"].([]interface{})
	if !ok {
		t.Fatal("Expected contents array")
	}
	if len(contents) != 3 {
		t.Fatalf("Expected 3 contents, got %d", len(contents))
	}

	// Verify first message is user
	firstMsg := contents[0].(map[string]interface{})
	if firstMsg["role"] != "user" {
		t.Errorf("Expected first role 'user', got %v", firstMsg["role"])
	}

	// Verify tools
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("Expected tools array in final Gemini request")
	}
	tool := tools[0].(map[string]interface{})
	funcDeclarations := tool["functionDeclarations"].([]interface{})
	funcDecl := funcDeclarations[0].(map[string]interface{})
	if funcDecl["name"] != "get_weather" {
		t.Errorf("Expected function name 'get_weather', got %v", funcDecl["name"])
	}

	// Print final Gemini request for debugging
	t.Logf("Final Gemini request: %s", string(geminiReq))
}
