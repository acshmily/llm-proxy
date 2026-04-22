package server

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertGeminiSSEToOpenAI_TextChunk(t *testing.T) {
	data := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello"}]
			}
		}]
	}`)

	result := convertGeminiSSEToOpenAI("message", data)
	assert.NotNil(t, result)
	assert.Contains(t, string(result), "data: ")

	var payload map[string]interface{}
	json.Unmarshal(bytes.TrimPrefix(result, []byte("data: ")), &payload)

	choices := payload["choices"].([]interface{})
	delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
	assert.Equal(t, "Hello", delta["content"])
	assert.Nil(t, choices[0].(map[string]interface{})["finish_reason"])
}

func TestConvertGeminiSSEToOpenAI_TextWithFinishReason(t *testing.T) {
	// 测试文本 + finish_reason 同时存在
	data := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{"text": "final chunk"}]
			},
			"finishReason": "STOP"
		}]
	}`)

	result := convertGeminiSSEToOpenAI("message", data)
	assert.NotNil(t, result)

	var payload map[string]interface{}
	json.Unmarshal(bytes.TrimPrefix(result, []byte("data: ")), &payload)

	choices := payload["choices"].([]interface{})
	delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
	assert.Equal(t, "final chunk", delta["content"])
	assert.Nil(t, choices[0].(map[string]interface{})["finish_reason"])
}

func TestConvertGeminiSSEToOpenAI_FinishReasonOnly(t *testing.T) {
	// 测试纯 finish_reason（无文本）- 当前实现不会返回任何内容
	data := []byte(`{
		"candidates": [{
			"content": {"parts": []},
			"finishReason": "STOP"
		}]
	}`)

	result := convertGeminiSSEToOpenAI("message", data)
	// 当前实现：没有 text 且没有 functionCall，返回 nil
	assert.Nil(t, result)
}

func TestConvertGeminiSSEToOpenAI_FunctionCall(t *testing.T) {
	data := []byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"arguments": {"location": "Tokyo"}
					}
				}]
			}
		}]
	}`)

	result := convertGeminiSSEToOpenAI("message", data)
	assert.NotNil(t, result)

	var payload map[string]interface{}
	json.Unmarshal(bytes.TrimPrefix(result, []byte("data: ")), &payload)

	choices := payload["choices"].([]interface{})
	delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})

	assert.Equal(t, "assistant", delta["role"])

	toolCalls := delta["tool_calls"].([]interface{})
	assert.Len(t, toolCalls, 1)

	tc := toolCalls[0].(map[string]interface{})
	assert.Equal(t, float64(0), tc["index"])
	assert.Equal(t, "function", tc["type"])
	assert.Contains(t, tc["id"].(string), "get_weather")

	fn := tc["function"].(map[string]interface{})
	assert.Equal(t, "get_weather", fn["name"])
	assert.Contains(t, fn["arguments"], "Tokyo")
}

func TestConvertGeminiSSEToOpenAI_EmptyCandidates(t *testing.T) {
	data := []byte(`{"candidates": []}`)
	result := convertGeminiSSEToOpenAI("message", data)
	assert.Nil(t, result)
}

func TestConvertGeminiSSEToOpenAI_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	result := convertGeminiSSEToOpenAI("message", data)
	assert.Nil(t, result)
}

