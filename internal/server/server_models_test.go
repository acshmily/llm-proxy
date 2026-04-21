package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/stretchr/testify/assert"
)

func TestServer_ModelsList_AllBackends(t *testing.T) {
	// 创建 OpenAI 模拟后端
	openaiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "gpt-4", "object": "model", "created": 1686935002, "owned_by": "openai"},
				{"id": "gpt-4o", "object": "model", "created": 1686935002, "owned_by": "openai"}
			]
		}`))
	}))
	defer openaiBackend.Close()

	// 创建 Anthropic 模拟后端
	anthropicBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{"id": "claude-3-opus", "object": "model", "created": 1686935002, "owned_by": "anthropic"},
				{"id": "claude-3-sonnet", "object": "model", "created": 1686935002, "owned_by": "anthropic"}
			]
		}`))
	}))
	defer anthropicBackend.Close()

	// 创建 Gemini 模拟后端
	geminiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"models": [
				{"name": "models/gemini-pro"},
				{"name": "models/gemini-2.5-flash"}
			]
		}`))
	}))
	defer geminiBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-openai", Backend: "openai", BackendKey: "key1", Timeout: 30000000000},
			{APIKey: "sk-claude", Backend: "anthropic", BackendKey: "key2", Timeout: 30000000000},
			{APIKey: "sk-gemini", Backend: "gemini", BackendKey: "key3", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			OpenAI:    config.BackendConfig{BaseURL: openaiBackend.URL},
			Anthropic: config.BackendConfig{BaseURL: anthropicBackend.URL},
			Gemini:    config.BackendConfig{BaseURL: geminiBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	assert.Equal(t, "list", resp.Object)
	assert.Len(t, resp.Data, 6) // 2 OpenAI + 2 Anthropic + 2 Gemini

	// 验证模型来源
	owners := make(map[string]int)
	for _, m := range resp.Data {
		owners[m.OwnedBy]++
	}
	assert.Equal(t, 2, owners["openai"])
	assert.Equal(t, 2, owners["anthropic"])
	assert.Equal(t, 2, owners["google"])

	// 验证 Gemini 模型名去除了 "models/" 前缀
	var geminiIDs []string
	for _, m := range resp.Data {
		if m.OwnedBy == "google" {
			geminiIDs = append(geminiIDs, m.ID)
		}
	}
	assert.Contains(t, geminiIDs, "gemini-pro")
	assert.Contains(t, geminiIDs, "gemini-2.5-flash")
}

func TestServer_ModelsList_BackendUnavailable(t *testing.T) {
	// 只有一个可用后端，另一个指向无效地址
	workingBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [{"id": "gpt-4", "object": "model"}]}`))
	}))
	defer workingBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "openai", BackendKey: "key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			OpenAI:    config.BackendConfig{BaseURL: workingBackend.URL},
			Anthropic: config.BackendConfig{BaseURL: "http://localhost:19999"}, // 不可达
			Gemini:    config.BackendConfig{BaseURL: "http://localhost:19998"}, // 不可达
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	// 仍应返回可用后端的模型
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, "gpt-4", resp.Data[0].ID)
}

func TestServer_ModelsList_WrongMethod(t *testing.T) {
	cfg := &config.Config{
		Routes:   []config.RouteConfig{},
		Backends: config.BackendsConfig{},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	// POST 方法不应匹配
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestServer_ModelsList_AuthHeaders(t *testing.T) {
	// 验证 OpenAI 后端发送 Bearer token
	openaiBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer sk-openai-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [{"id": "gpt-4", "object": "model"}]}`))
	}))
	defer openaiBackend.Close()

	// 验证 Anthropic 后端发送 x-api-key
	anthropicBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "sk-anthropic-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": [{"id": "claude-3", "object": "model"}]}`))
	}))
	defer anthropicBackend.Close()

	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-openai", Backend: "openai", BackendKey: "sk-openai-key", Timeout: 30000000000},
			{APIKey: "sk-anthropic", Backend: "anthropic", BackendKey: "sk-anthropic-key", Timeout: 30000000000},
		},
		Backends: config.BackendsConfig{
			OpenAI:    config.BackendConfig{BaseURL: openaiBackend.URL},
			Anthropic: config.BackendConfig{BaseURL: anthropicBackend.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.INFO)
	srv := New(cfg, r, log)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}
