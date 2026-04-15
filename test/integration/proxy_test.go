package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/claude-projetc/llm-proxy/internal/server"
	"github.com/claude-projetc/llm-proxy/test/mock"
)

func TestProxyEndToEnd(t *testing.T) {
	// 启动 Mock 后端
	mockBackend := mock.NewMockBackend()
	defer mockBackend.Close()

	// 创建配置
	cfg := &config.Config{
		Routes: []config.RouteConfig{
			{APIKey: "sk-test", Backend: "openai", BackendKey: "sk-mock", Timeout: 5 * time.Second},
		},
		Backends: config.BackendsConfig{
			OpenAI: config.BackendConfig{BaseURL: mockBackend.Server.URL},
		},
	}

	r := router.New(cfg.Routes)
	log := logger.New(logger.TEXT, logger.ERROR)
	srv := server.New(cfg, r, log)

	// 创建测试服务器
	testServer := httptest.NewServer(srv)
	defer testServer.Close()

	// 发送请求
	reqBody := `{"model":"claude-3","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`
	req, _ := http.NewRequest("POST", testServer.URL+"/v1/messages", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var respBody map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&respBody)

	if respBody["content"] == nil {
		t.Error("Expected content in response")
	}
}
