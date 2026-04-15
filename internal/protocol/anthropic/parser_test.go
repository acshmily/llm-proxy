package anthropic

import (
	"testing"
)

func TestParseRequest(t *testing.T) {
	input := `{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`

	unified, err := ParseRequest([]byte(input))
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if unified.Model != "claude-3" {
		t.Errorf("Expected model 'claude-3', got '%s'", unified.Model)
	}
	if len(unified.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(unified.Messages))
	}
	if unified.Messages[0].Content != "Hello" {
		t.Errorf("Expected content 'Hello', got '%s'", unified.Messages[0].Content)
	}
}
