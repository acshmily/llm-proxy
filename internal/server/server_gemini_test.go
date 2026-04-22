package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/router"
)

func TestGeminiEndpoint_NotFound(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "gemini", BackendKey: "gemini-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
		},
	}
	srv := New(cfg, router.New(cfg.Routes), logger.New(logger.TEXT, logger.INFO))

	// GET on Gemini endpoint should 404 (only POST supported)
	req := httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-pro:generateContent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for GET, got %d", w.Code)
	}
}

func TestGeminiEndpoint_ForwardsRequest(t *testing.T) {
	// Mock Gemini backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header not forwarded from client
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Client Authorization header should not be forwarded, got: %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "gemini", BackendKey: "gemini-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: backend.URL},
		},
	}
	srv := New(cfg, router.New(cfg.Routes), logger.New(logger.TEXT, logger.INFO))

	body := `{
		"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-pro:generateContent", nil)
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(body))

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, "candidates") {
		t.Errorf("Expected Gemini response body, got: %s", respBody)
	}
}

func TestGeminiEndpoint_InvalidApiKey(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-valid", Backend: "gemini", BackendKey: "gemini-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
		},
	}
	srv := New(cfg, router.New(cfg.Routes), logger.New(logger.TEXT, logger.INFO))

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-pro:generateContent", nil)
	req.Header.Set("Authorization", "Bearer sk-invalid")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401 for invalid API key, got %d", w.Code)
	}
}

func TestGeminiEndpoint_NonGeminiBackend(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "openai", BackendKey: "sk-openai-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: "https://api.openai.com"},
		},
	}
	srv := New(cfg, router.New(cfg.Routes), logger.New(logger.TEXT, logger.INFO))

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-pro:generateContent", nil)
	req.Header.Set("Authorization", "Bearer sk-test")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for non-Gemini backend, got %d", w.Code)
	}
}

func TestGeminiEndpoint_ForwardsStreamingResponse(t *testing.T) {
	// Mock Gemini backend returning SSE
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}

`))
		w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":" world"}]}}]}

`))
		w.Write([]byte(`data: {"candidates":[{"finishReason":"STOP"}]}

`))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "gemini", BackendKey: "gemini-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: backend.URL},
		},
	}
	srv := New(cfg, router.New(cfg.Routes), logger.New(logger.TEXT, logger.INFO))

	body := `{
		"contents": [{"role": "user", "parts": [{"text": "hello"}]}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-pro:streamGenerateContent?alt=sse", nil)
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(body))

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Should contain SSE events
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "data:") {
		t.Errorf("Expected SSE response, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "hello") {
		t.Errorf("Expected response body to contain 'hello', got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, " world") {
		t.Errorf("Expected response body to contain ' world', got: %s", bodyStr)
	}

	// Should have SSE content type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Expected Content-Type text/event-stream, got: %s", ct)
	}
}
