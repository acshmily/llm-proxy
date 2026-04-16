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

func TestServer_OpenAIRequest_ToOpenAIBackend(t *testing.T) {
	// 创建测试服务器
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello!"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: ":8080"},
		Logging: config.LoggingConfig{Format: "text", Level: "info"},
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "openai",
			BackendKey:  "test-key",
			Timeout:     30000000000, // 30s
		}},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// 创建 OpenAI 格式请求
	reqBody := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "Hello!")
}

func TestServer_OpenAIRequest_InvalidAPIKey(t *testing.T) {
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: ":8080"},
		Logging: config.LoggingConfig{Format: "text", Level: "info"},
		Routes:  []config.RouteConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{"model": "gpt-4", "messages": []}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer invalid-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid API key")
}

func TestServer_OpenAIRequest_AnthropicBackend(t *testing.T) {
	// 测试 OpenAI 请求转发到 Anthropic 后端
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

	reqBody := []byte(`{"model": "claude-3", "messages": [{"role": "user", "content": "Hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// 验证响应为 OpenAI 格式
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Contains(t, resp, "choices")
}

func TestServer_OpenAIRequest_NotFound(t *testing.T) {
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: ":8080"},
		Logging: config.LoggingConfig{Format: "text", Level: "info"},
		Routes:  []config.RouteConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// 测试不存在的路径
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	assert.Contains(t, rr.Body.String(), "Endpoint not found")
}

func TestServer_OpenAIRequest_WrongMethod(t *testing.T) {
	cfg := &config.Config{
		Server:  config.ServerConfig{Listen: ":8080"},
		Logging: config.LoggingConfig{Format: "text", Level: "info"},
		Routes:  []config.RouteConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// 测试错误的 HTTP 方法
	reqBody := []byte(`{"model": "gpt-4", "messages": []}`)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServer_OpenAIRequest_ToGeminiBackend(t *testing.T) {
	// 测试 OpenAI 请求转发到 Gemini 后端
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证 Gemini API 调用
		assert.Contains(t, r.URL.Path, ":generateContent")
		assert.Contains(t, r.URL.RawQuery, "key=") // Gemini 使用 URL 参数传递 API Key

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

	reqBody := []byte(`{"model": "gemini-pro", "messages": [{"role": "user", "content": "Hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// 验证响应为 OpenAI 格式
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Contains(t, resp, "choices")
	assert.Contains(t, resp, "usage")

	// 验证 finish_reason 映射
	choices := resp["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	assert.Equal(t, "stop", choice["finish_reason"])
}
