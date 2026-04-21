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
