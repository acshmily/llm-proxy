package gemini

import (
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/logger"
)

func TestNewGeminiClient_WithoutProxy(t *testing.T) {
	log := logger.New(logger.TEXT, logger.INFO)
	client, err := NewGeminiClient("test-api-key", "", log, false, 2048)
	if err != nil {
		t.Fatalf("NewGeminiClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewGeminiClient_WithProxy(t *testing.T) {
	log := logger.New(logger.TEXT, logger.DEBUG)
	client, err := NewGeminiClient("test-api-key", "http://127.0.0.1:18080", log, true, 2048)
	if err != nil {
		t.Fatalf("NewGeminiClient failed: %v", err)
	}
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
}

func TestNewGeminiClient_InvalidProxy(t *testing.T) {
	log := logger.New(logger.TEXT, logger.INFO)
	_, err := NewGeminiClient("test-api-key", "://invalid", log, false, 2048)
	if err == nil {
		t.Error("expected error for invalid proxy URL, got nil")
	}
}
