package wsclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// MockTunnel 用于测试的 Mock TunnelSender
type MockTunnel struct {
	connected bool
	response  *http.Response
	err       error
}

func (m *MockTunnel) SendRequest(req *http.Request) (*http.Response, error) {
	return m.response, m.err
}

func (m *MockTunnel) IsConnected() bool {
	return m.connected
}

// TestProxyServer_ServeHTTP 测试成功转发请求
func TestProxyServer_ServeHTTP(t *testing.T) {
	// 准备 Mock 响应
	body := `{"result": "success"}`
	mockResp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}

	// 创建 MockTunnel
	mockTunnel := &MockTunnel{
		connected: true,
		response:  mockResp,
	}

	// 创建代理服务器
	proxy := NewProxyServer(mockTunnel)

	// 创建测试请求
	req := httptest.NewRequest("GET", "http://example.com/v1/messages", nil)
	w := httptest.NewRecorder()

	// 调用 ServeHTTP
	proxy.ServeHTTP(w, req)

	// 验证响应
	res := w.Result()
	if res.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %s", contentType)
	}

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	if string(respBody) != body {
		t.Errorf("expected body %q, got %q", body, string(respBody))
	}
}

// TestProxyServer_Disconnected 测试隧道断开时返回 503
func TestProxyServer_Disconnected(t *testing.T) {
	// 创建未连接的 MockTunnel
	mockTunnel := &MockTunnel{
		connected: false,
	}

	// 创建代理服务器
	proxy := NewProxyServer(mockTunnel)

	// 创建测试请求
	req := httptest.NewRequest("GET", "http://example.com/v1/messages", nil)
	w := httptest.NewRecorder()

	// 调用 ServeHTTP
	proxy.ServeHTTP(w, req)

	// 验证返回 503
	res := w.Result()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", res.StatusCode)
	}
}

// TestProxyServer_SendError 测试发送失败时返回 502
func TestProxyServer_SendError(t *testing.T) {
	// 创建返回错误的 MockTunnel
	mockTunnel := &MockTunnel{
		connected: true,
		err:       ErrTunnelDisconnected,
	}

	// 创建代理服务器
	proxy := NewProxyServer(mockTunnel)

	// 创建测试请求
	req := httptest.NewRequest("POST", "http://example.com/v1/messages", bytes.NewReader([]byte(`{"test": "data"}`)))
	w := httptest.NewRecorder()

	// 调用 ServeHTTP
	proxy.ServeHTTP(w, req)

	// 验证返回 502
	res := w.Result()
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("expected status 502, got %d", res.StatusCode)
	}
}

// TestNewProxyServer 测试 NewProxyServer 创建
func TestNewProxyServer(t *testing.T) {
	mockTunnel := &MockTunnel{}
	proxy := NewProxyServer(mockTunnel)

	if proxy == nil {
		t.Fatal("NewProxyServer returned nil")
	}
	if proxy.tunnel != mockTunnel {
		t.Error("proxy.tunnel not set correctly")
	}
	if proxy.timeout != 60*time.Second {
		t.Errorf("expected default timeout 60s, got %v", proxy.timeout)
	}
}
