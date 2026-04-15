package wsclient

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTunnel_IsConnected(t *testing.T) {
	t.Run("initial state disconnected", func(t *testing.T) {
		tunnel := NewTunnel("ws://localhost:8080/ws-tunnel", 30*time.Second)
		defer tunnel.Close()

		if tunnel.IsConnected() {
			t.Error("expected disconnected in initial state")
		}
	})
}

func TestTunnel_SendRequest_Disconnected(t *testing.T) {
	tunnel := NewTunnel("ws://localhost:8080/ws-tunnel", 30*time.Second)
	defer tunnel.Close()

	req := httptest.NewRequest("GET", "/test", nil)
	_, err := tunnel.SendRequest(req)

	if err == nil {
		t.Error("expected error when sending request while disconnected")
	}
	if err != ErrTunnelDisconnected {
		t.Errorf("expected ErrTunnelDisconnected, got %v", err)
	}
}

// 集成测试 - 需要 WebSocket 服务器
func TestTunnel_ConnectAndDisconnect(t *testing.T) {
	t.Skip("requires WebSocket server mock")
}

func TestTunnel_Connect_Success(t *testing.T) {
	// 使用 httptest.Server 模拟 WebSocket 服务端
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// 服务端保持连接，等待客户端消息
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}))
	defer server.Close()

	// 转换 http:// 为 ws://
	wsURL := "ws" + server.URL[4:] + "/ws-tunnel"

	tunnel := NewTunnel(wsURL, 30*time.Second)

	err := tunnel.Connect()
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer tunnel.Close()

	if !tunnel.IsConnected() {
		t.Error("expected connected after Connect()")
	}
}

// TestTunnel_SendRequest_Integration 测试发送请求的完整流程
func TestTunnel_SendRequest_Integration(t *testing.T) {
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("WebSocket upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// 读取客户端请求
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// 解析请求
		var wsReq WSRequest
		if err := json.Unmarshal(msg, &wsReq); err != nil {
			return
		}

		// 构建响应
		wsResp := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  200,
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    base64.StdEncoding.EncodeToString([]byte(`{"result": "ok"}`)),
			},
		}
		respData, _ := json.Marshal(wsResp)
		conn.WriteMessage(websocket.TextMessage, respData)
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[4:] + "/ws-tunnel"
	tunnel := NewTunnel(wsURL, 30*time.Second)

	if err := tunnel.Connect(); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer tunnel.Close()

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := tunnel.SendRequest(req)
	if err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
