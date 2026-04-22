package server

import (
	"bytes"
	"encoding/json"
	"io"
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

	// 空 prompt 应返回 400，要求提供有效输入
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Either 'prompt' or 'messages' must be provided")
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

func TestServer_CompletionsRequest_MessagesFormat(t *testing.T) {
	// 测试 messages 数组格式（OpenClaw openai-completions 模式）
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/chat/completions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello from messages!"},
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

	// messages 格式请求（替代 prompt）
	reqBody := []byte(`{
		"model": "gpt-4",
		"messages": [{"role": "user", "content": "Say hello"}],
		"max_tokens": 100
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	if assert.Equal(t, http.StatusOK, rr.Code) {
		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		choices := resp["choices"].([]interface{})
		choice := choices[0].(map[string]interface{})
		assert.Contains(t, choice, "text")
		assert.Equal(t, "Hello from messages!", choice["text"])
	}
}

func TestServer_CompletionsRequest_MessagesToGeminiBackend(t *testing.T) {
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, ":generateContent")
		assert.Contains(t, r.URL.RawQuery, "key=")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "Hello from Gemini messages!"}]
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

	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [{"role": "user", "content": "Hi"}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	choices := resp["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	assert.Equal(t, "Hello from Gemini messages!", choice["text"])
	assert.Equal(t, "stop", choice["finish_reason"])
}

func TestServer_CompletionsRequest_PromptTakesPrecedenceOverMessages(t *testing.T) {
	// 当 prompt 和 messages 同时存在时，prompt 优先
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证后端实际收到的消息是 prompt 而非 messages
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Failed to decode backend request: %v", err)
			return
		}
		msgs, ok := body["messages"].([]interface{})
		if !ok || len(msgs) == 0 {
			t.Fatal("Expected messages array in backend request")
			return
		}
		firstMsg, ok := msgs[0].(map[string]interface{})
		if !ok {
			t.Fatal("Expected first message to be a map")
			return
		}
		assert.Equal(t, "Use this prompt", firstMsg["content"], "prompt should take precedence over messages")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": "chatcmpl-123",
			"model": "gpt-4",
			"choices": [{"message": {"role": "assistant", "content": "Prompt wins!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 3}
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

	// 同时包含 prompt 和 messages，prompt 应优先
	reqBody := []byte(`{
		"model": "gpt-4",
		"prompt": "Use this prompt",
		"messages": [{"role": "user", "content": "Ignore this"}]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestServer_CompletionsRequest_MissingPromptAndMessages(t *testing.T) {
	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "openai",
			BackendKey:  "test-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: "http://localhost:9999"},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// 既无 prompt 也无 messages
	reqBody := []byte(`{"model": "gpt-4"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Either 'prompt' or 'messages' must be provided")
}

func TestServer_CompletionsRequest_GeminiStreaming(t *testing.T) {
	// 验证 Gemini 流式请求使用 :streamGenerateContent 端点且包含 alt=sse 参数
	var receivedPath string
	var receivedQuery string
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 模拟 Gemini 流式响应格式（需要空行分隔事件）
		w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{"text":" world"}]},"finishReason":"STOP"}]}

`))
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

	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [{"role": "user", "content": "Hi"}],
		"stream": true
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// 验证使用了 streamGenerateContent 端点且包含 alt=sse 参数
	assert.Contains(t, receivedPath, ":streamGenerateContent", "should use streamGenerateContent endpoint for streaming")
	assert.Contains(t, receivedQuery, "alt=sse", "should request SSE format from Gemini API")

	// 验证响应为 SSE 格式且包含文本数据
	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, "text")
	assert.Contains(t, body, "Hello")
	assert.Contains(t, body, "finish_reason")
}

func TestServer_CompletionsRequest_OpenAIStreaming(t *testing.T) {
	// 验证 OpenAI 流式响应转换
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// 模拟 OpenAI 流式响应（需要空行分隔事件）
		w.Write([]byte(`data: {"choices":[{"delta":{"content":"Hello"},"index":0}]}

data: {"choices":[{"delta":{},"index":0,"finish_reason":"stop"}]}

data: [DONE]

`))
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

	reqBody := []byte(`{"model": "gpt-4", "prompt": "Hi", "stream": true}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	// 验证 SSE 格式
	assert.Contains(t, body, "data:")
	assert.Contains(t, body, "text")
	assert.Contains(t, body, "Hello")
	// 验证 [DONE] 信号
	assert.Contains(t, body, "[DONE]")
}

func TestServer_ChatCompletionsRequest_GeminiStreaming(t *testing.T) {
	// 验证 /v1/chat/completions 的 Gemini 流式也使用正确端点
	var receivedPath string
	var receivedQuery string
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"Hi there"}]},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{}]},"finishReason":"STOP"}]}

`))
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

	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [{"role": "user", "content": "Hi"}],
		"stream": true
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Contains(t, receivedPath, ":streamGenerateContent", "chat completions should also use streamGenerateContent for streaming")
	assert.Contains(t, receivedQuery, "alt=sse", "chat completions should also request SSE format")
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "content")
}

func TestServer_MessagesRequest_GeminiStreaming(t *testing.T) {
	// 验证 /v1/messages（Anthropic 协议）的 Gemini 流式也使用正确端点
	var receivedPath string
	var receivedQuery string
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]},"finishReason":""}]}

data: {"candidates":[{"content":{"parts":[{}]},"finishReason":"STOP"}]}

`))
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

	// Anthropic 协议请求（content 是数组格式）
	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [{"role": "user", "content": [{"type": "text", "text": "Hi"}]}],
		"stream": true,
		"max_tokens": 100
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(reqBody))
	req.Header.Set("x-api-key", "sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Contains(t, receivedPath, ":streamGenerateContent", "/v1/messages should also use streamGenerateContent for streaming")
	assert.Contains(t, receivedQuery, "alt=sse", "/v1/messages should also request SSE format")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestServer_CompletionsRequest_ArrayContentFormat(t *testing.T) {
	// OpenClaw openai-completions mode sends messages with array-style content
	// messages[].content = [{"type": "text", "text": "..."}]
	var receivedBody []byte
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "Hello OpenClaw"}], "role": "model"}, "finishReason": "STOP"}]}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "gemini",
			BackendKey:  "gemini-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "Hello OpenClaw"}]}
		]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// Verify the backend received the properly formatted request
	assert.Contains(t, string(receivedBody), `"text":"Hello OpenClaw"`)
}

func TestServer_CompletionsRequest_MultiPartContent(t *testing.T) {
	// Multi-part content: multiple text blocks should be concatenated
	var receivedBody []byte
	mockBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"candidates": [{"content": {"parts": [{"text": "Combined response"}], "role": "model"}, "finishReason": "STOP"}]}`))
	}))
	defer mockBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{{
			APIKey:      "sk-test-key",
			Backend:     "gemini",
			BackendKey:  "gemini-key",
			Timeout:     30000000000,
		}},
		Backends: config.BackendsConfig{
			Gemini: config.BackendConfig{BaseURL: mockBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	reqBody := []byte(`{
		"model": "gemini-pro",
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "Part one. "},
				{"type": "text", "text": "Part two. "},
				{"type": "text", "text": "Part three."}
			]}
		]
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-key")
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	// Verify backend received concatenated content (JSON marshal removes trailing spaces)
	assert.Contains(t, string(receivedBody), `"text":"Part one. Part two. Part three."`)
}
