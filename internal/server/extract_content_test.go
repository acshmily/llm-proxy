package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractContent_StringFormat(t *testing.T) {
	content := json.RawMessage(`"Hello World"`)
	result := extractContent(content)
	assert.Equal(t, "Hello World", result)
}

func TestExtractContent_ArrayFormat(t *testing.T) {
	content := json.RawMessage(`[{"type": "text", "text": "Hello OpenClaw"}]`)
	result := extractContent(content)
	assert.Equal(t, "Hello OpenClaw", result)
}

func TestExtractContent_MultiPartArray(t *testing.T) {
	content := json.RawMessage(`[
		{"type": "text", "text": "Part one. "},
		{"type": "text", "text": "Part two."}
	]`)
	result := extractContent(content)
	assert.Equal(t, "Part one. Part two.", result)
}

func TestExtractContent_Empty(t *testing.T) {
	assert.Equal(t, "", extractContent(nil))
	assert.Equal(t, "", extractContent(json.RawMessage{}))
}

func TestExtractContent_EmptyString(t *testing.T) {
	content := json.RawMessage(`""`)
	assert.Equal(t, "", extractContent(content))
}

func TestExtractContent_EmptyArray(t *testing.T) {
	content := json.RawMessage(`[]`)
	assert.Equal(t, "", extractContent(content))
}

func TestExtractContent_MixedTypes(t *testing.T) {
	// Non-text blocks should be skipped
	content := json.RawMessage(`[
		{"type": "text", "text": "Hello"},
		{"type": "image_url", "image_url": {"url": "http://example.com"}},
		{"type": "text", "text": " World"}
	]`)
	result := extractContent(content)
	assert.Equal(t, "Hello World", result)
}

func TestExtractContent_InvalidJSON(t *testing.T) {
	content := json.RawMessage(`{invalid json}`)
	assert.Equal(t, "", extractContent(content))
}
