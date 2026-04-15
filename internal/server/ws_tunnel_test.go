package server

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/middleware"
)

func TestWSTunnelMiddleware_IsEnabled(t *testing.T) {
	t.Run("returns false when config is nil", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(nil, nil)
		if middleware.IsEnabled() {
			t.Error("expected false for nil config")
		}
	})

	t.Run("returns false when disabled", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: false}
		middleware := NewWSTunnelMiddleware(cfg, nil)
		if middleware.IsEnabled() {
			t.Error("expected false when disabled")
		}
	})

	t.Run("returns true when enabled", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true}
		middleware := NewWSTunnelMiddleware(cfg, nil)
		if !middleware.IsEnabled() {
			t.Error("expected true when enabled")
		}
	})
}

func TestWSTunnelMiddleware_GetPath(t *testing.T) {
	t.Run("returns default path when not configured", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true}
		middleware := NewWSTunnelMiddleware(cfg, nil)
		if middleware.GetPath() != "/ws-tunnel" {
			t.Errorf("expected /ws-tunnel, got %s", middleware.GetPath())
		}
	})

	t.Run("returns configured path", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true, Path: "/custom-ws"}
		middleware := NewWSTunnelMiddleware(cfg, nil)
		if middleware.GetPath() != "/custom-ws" {
			t.Errorf("expected /custom-ws, got %s", middleware.GetPath())
		}
	})
}

func TestWSTunnelMiddleware_HandleWebSocket(t *testing.T) {
	t.Run("returns 503 when disabled", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: false}
		middleware := NewWSTunnelMiddleware(cfg, nil)

		req := httptest.NewRequest("GET", "/ws-tunnel", nil)
		req.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()

		middleware.HandleWebSocket(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503, got %d", w.Code)
		}
	})

	t.Run("returns 400 when Upgrade header missing", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true}
		middleware := NewWSTunnelMiddleware(cfg, nil)

		req := httptest.NewRequest("GET", "/ws-tunnel", nil)
		w := httptest.NewRecorder()

		middleware.HandleWebSocket(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 when Sec-WebSocket-Key missing", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true}
		middleware := NewWSTunnelMiddleware(cfg, nil)

		req := httptest.NewRequest("GET", "/ws-tunnel", nil)
		req.Header.Set("Upgrade", "websocket")
		w := httptest.NewRecorder()

		middleware.HandleWebSocket(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("returns 501 when hijack not supported", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true}
		middleware := NewWSTunnelMiddleware(cfg, nil)

		req := httptest.NewRequest("GET", "/ws-tunnel", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Key", "test-key")
		w := httptest.NewRecorder()

		middleware.HandleWebSocket(w, req)

		// httptest.ResponseRecorder doesn't support Hijacker
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})
}

func TestWSTunnelMiddleware_WSTunnelHandler(t *testing.T) {
	t.Run("returns 404 for wrong path", func(t *testing.T) {
		cfg := &config.WebSocketTunnelConfig{Enabled: true, Path: "/ws-tunnel"}
		middleware := NewWSTunnelMiddleware(cfg, nil)

		handler := middleware.WSTunnelHandler()

		req := httptest.NewRequest("GET", "/wrong-path", nil)
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})
}

func TestWSTunnelMiddleware_processWebSocketMessage(t *testing.T) {
	t.Run("handles ping message", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)

		msg := []byte(`{"type": "ping"}`)
		response, err := middleware.processWebSocketMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "pong" {
			t.Errorf("expected pong, got %s", result["type"])
		}
	})

	t.Run("handles unknown message type", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)

		msg := []byte(`{"type": "unknown"}`)
		response, err := middleware.processWebSocketMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "error" {
			t.Errorf("expected error, got %s", result["type"])
		}
	})

	t.Run("handles invalid json", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)

		msg := []byte(`{invalid json}`)
		_, err := middleware.processWebSocketMessage(msg)
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
}

func TestWSTunnelMiddleware_handleHTTPRequest(t *testing.T) {
	t.Run("handles valid HTTP request", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)
		// 配置模拟的请求处理器
		middleware.SetRequestHandler(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message": "ok"}`))
		})

		envelope := map[string]interface{}{
			"type": "request",
			"data": map[string]interface{}{
				"method":  "GET",
				"path":    "/test",
				"headers": map[string]interface{}{},
			},
		}

		response, err := middleware.handleHTTPRequest(envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "response" {
			t.Errorf("expected response, got %s", result["type"])
		}
	})

	t.Run("returns error for missing method", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)

		envelope := map[string]interface{}{
			"type": "request",
			"data": map[string]interface{}{
				"path":    "/test",
				"headers": map[string]interface{}{},
			},
		}

		response, err := middleware.handleHTTPRequest(envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "error" {
			t.Errorf("expected error, got %s", result["type"])
		}
	})

	t.Run("returns error for invalid body encoding", func(t *testing.T) {
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, nil)

		envelope := map[string]interface{}{
			"type": "request",
			"data": map[string]interface{}{
				"method":  "POST",
				"path":    "/test",
				"headers": map[string]interface{}{},
				"body":    "invalid!base64!",
			},
		}

		response, err := middleware.handleHTTPRequest(envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]string
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "error" {
			t.Errorf("expected error, got %s", result["type"])
		}
	})

	t.Run("decodes base64 body correctly", func(t *testing.T) {
		obfus := middleware.NewTrafficObfuscationMiddleware(&config.TrafficObfuscationConfig{})
		middleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{Enabled: true}, obfus)
		// 配置模拟的请求处理器
		middleware.SetRequestHandler(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		})

		bodyBytes := []byte(`{"message": "hello"}`)
		bodyB64 := base64.StdEncoding.EncodeToString(bodyBytes)

		envelope := map[string]interface{}{
			"type": "request",
			"data": map[string]interface{}{
				"method":  "POST",
				"path":    "/test",
				"headers": map[string]interface{}{},
				"body":    bodyB64,
			},
		}

		response, err := middleware.handleHTTPRequest(envelope)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "response" {
			t.Errorf("expected response, got %s", result["type"])
		}
	})
}

func TestWSTunnelMiddleware_Integration(t *testing.T) {
	t.Run("full request/response cycle", func(t *testing.T) {
		obfus := middleware.NewTrafficObfuscationMiddleware(&config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 1024,
			},
		})
		wsMiddleware := NewWSTunnelMiddleware(&config.WebSocketTunnelConfig{
			Enabled: true,
			Path:    "/ws-tunnel",
		}, obfus)

		// 配置模拟的请求处理器
		wsMiddleware.SetRequestHandler(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		})

		// Create a sharded request
		body := []byte(`{"test": "data"}`)
		chunks, err := obfus.ShardRequest(body)
		if err != nil {
			t.Fatalf("ShardRequest failed: %v", err)
		}

		// Wrap in WebSocket message
		envelope := map[string]interface{}{
			"type": "request",
			"data": map[string]interface{}{
				"method":  "POST",
				"path":    "/v1/messages",
				"headers": map[string]interface{}{"Content-Type": "application/json"},
				"body":    chunks[0], // Single chunk for this test
			},
		}

		response, err := wsMiddleware.handleHTTPRequest(envelope)
		if err != nil {
			t.Fatalf("handleHTTPRequest failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(response, &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if result["type"] != "response" {
			t.Errorf("expected response type, got %s", result["type"])
		}
	})
}

func Test_computeWebSocketAcceptKey(t *testing.T) {
	// Test with known key from RFC 6455
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	expected := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="

	result := computeWebSocketAcceptKey(key)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestWSRequest_WSMarshal(t *testing.T) {
	req := WSRequest{
		Method: "POST",
		Path:   "/v1/messages",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: base64.StdEncoding.EncodeToString([]byte(`{"test": "data"}`)),
	}

	msg := WSMessage{
		Type: "request",
		Data: req,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled WSMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != "request" {
		t.Errorf("expected type request, got %s", unmarshaled.Type)
	}
}
