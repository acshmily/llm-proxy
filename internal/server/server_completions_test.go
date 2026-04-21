package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestServer_CompletionsRequest_ToOpenAIBackend(t *testing.T) {
	// 模拟 OpenAI 后端接收转换后的请求
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求被转换为 chat 格式
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello from chat backend!"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "openai",
			BackendKey:  "test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// Completions 格式请求（旧版 OpenAI API）
	reqBody := []byte(`{
		"model": "gpt-4",
		"prompt": "Say hello",
		"max_tokens": 100,
		"temperature": 0.7
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if assert.Equal(t, http.StatusOK, rr.Code) {
		// 验证响应为 Completions 格式（choices[0].text 而非 choices[0].message.content）
		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		// 验证 choices 中有 text 字段
		choices := resp["choices"].([]interface{})
		choice := choices[0].(map[string]interface{})
		assert.Contains(t, choice, "text")
		assert.Equal(t, "Hello from chat backend!", choice["text"])
	}
}

func TestServer_CompletionsRequest_InvalidAPIKey(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "gpt-4", "prompt": "Hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer invalid-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid API key")
}

func TestServer_CompletionsRequest_WrongMethod(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// GET 方法不应匹配
	req := httptest.NewRequest(http.MethodGet, "/v1/completions", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServer_CompletionsRequest_AnthropicBackend(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/messages")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "msg_123",
			"model": "claude-3",
			"content": [{"type": "text", "text": "Hello from Claude!"}],
			"role": "assistant",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "anthropic",
			BackendKey:  "test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			Anthropic: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "claude-3", "prompt": "Hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	choices := resp["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	assert.Equal(t, "Hello from Claude!", choice["text"])
}

func TestServer_CompletionsRequest_ToGeminiBackend(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, ":generateContent")
		assert.Contains(t, r.URL.RawQuery, "key=")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "Hello from Gemini!"}]
				},
				"finishReason": "STOP"
			}],
			"usageMetadata": {
				"promptTokenCount": 10,
				"candidatesTokenCount": 5
			}
		}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "gemini",
			BackendKey:  "gemini-test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "gemini-pro", "prompt": "Hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	choices := resp["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	assert.Equal(t, "Hello from Gemini!", choice["text"])
	assert.Equal(t, "stop", choice["finish_reason"])
}

func TestServer_CompletionsRequest_EmptyPrompt(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{"message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 0, "completion_tokens": 1}
		}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "openai",
			BackendKey:  "test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "gpt-4", "prompt": ""}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// 即使 prompt 为空也应正常处理
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestServer_CompletionsRequest_BackendError(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "Backend error"}}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "openai",
			BackendKey:  "test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "gpt-4", "prompt": "Hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "Backend error")
}
