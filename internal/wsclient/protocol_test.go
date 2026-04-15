package wsclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
)

func TestEncodeRequest(t *testing.T) {
	// 测试 GET 请求
	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/messages", nil)
		req.Header.Set("Authorization", "Bearer sk-test")

		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}

		// 验证 JSON 结构
		var msg WSRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if msg.Type != "request" {
			t.Errorf("expected type 'request', got %s", msg.Type)
		}
		if msg.Data.Method != "GET" {
			t.Errorf("expected method 'GET', got %s", msg.Data.Method)
		}
		if msg.Data.Path != "/v1/messages" {
			t.Errorf("expected path '/v1/messages', got %s", msg.Data.Path)
		}
		if msg.Data.Body != "" {
			t.Errorf("expected empty body for GET, got %s", msg.Data.Body)
		}
	})

	// 测试 POST 请求带 Body
	t.Run("POST request with body", func(t *testing.T) {
		body := `{"message": "hello"}`
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")

		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}

		var msg WSRequest
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		// 验证 method
		if msg.Data.Method != "POST" {
			t.Errorf("expected method 'POST', got %s", msg.Data.Method)
		}

		// 验证 Content-Type 头部
		if msg.Data.Headers["Content-Type"] != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %s", msg.Data.Headers["Content-Type"])
		}

		// 解码 Body 验证
		decodedBody, err := base64.StdEncoding.DecodeString(msg.Data.Body)
		if err != nil {
			t.Fatalf("Base64 decode failed: %v", err)
		}
		if string(decodedBody) != body {
			t.Errorf("expected body %q, got %q", body, string(decodedBody))
		}
	})
}

func TestDecodeResponse(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		body := `{"result": "ok"}`
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  200,
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    base64.StdEncoding.EncodeToString([]byte(body)),
			},
		}

		data, _ := json.Marshal(respData)

		resp, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("DecodeResponse failed: %v", err)
		}

		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != body {
			t.Errorf("expected body %q, got %q", body, string(bodyBytes))
		}
	})

	t.Run("error response", func(t *testing.T) {
		respData := WSResponse{
			Type: "error",
			Data: WSRespData{
				Message: "server error",
			},
		}

		data, _ := json.Marshal(respData)
		resp, err := DecodeResponse(data)

		if err == nil {
			t.Error("expected error for error response type")
		}
		if resp != nil {
			t.Error("expected nil response for error type")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	// 验证编码后再解码能还原原始请求
	t.Run("encode then decode", func(t *testing.T) {
		originalBody := `{"test": "data"}`
		req := httptest.NewRequest("POST", "/v1/messages", bytes.NewReader([]byte(originalBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-test")

		// 编码
		encoded, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}

		// 验证编码结果
		var wsReq WSRequest
		if err := json.Unmarshal(encoded, &wsReq); err != nil {
			t.Fatalf("JSON unmarshal failed: %v", err)
		}

		if wsReq.Type != "request" {
			t.Errorf("expected type 'request', got %s", wsReq.Type)
		}
		if wsReq.Data.Method != "POST" {
			t.Errorf("expected method 'POST', got %s", wsReq.Data.Method)
		}

		// 模拟服务端响应（原样返回）
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  200,
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    base64.StdEncoding.EncodeToString([]byte(originalBody)),
			},
		}
		respBytes, _ := json.Marshal(respData)

		// 解码
		resp, err := DecodeResponse(respBytes)
		if err != nil {
			t.Fatalf("DecodeResponse failed: %v", err)
		}

		// 验证
		if resp.StatusCode != 200 {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		if string(bodyBytes) != originalBody {
			t.Errorf("body mismatch: expected %q, got %q", originalBody, string(bodyBytes))
		}
	})
}

func TestEncodeRequest_EdgeCases(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		data, err := EncodeRequest(req)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var msg WSRequest
		json.Unmarshal(data, &msg)
		if msg.Data.Body != "" {
			t.Errorf("expected empty body, got %q", msg.Data.Body)
		}
	})

	t.Run("binary body", func(t *testing.T) {
		binaryData := []byte{0x00, 0x01, 0x02, 0xFF}
		req := httptest.NewRequest("POST", "/binary", bytes.NewReader(binaryData))

		data, err := EncodeRequest(req)
		if err != nil {
			t.Fatalf("EncodeRequest failed: %v", err)
		}

		var msg WSRequest
		json.Unmarshal(data, &msg)

		// 验证 Base64 编码正确
		decoded, err := base64.StdEncoding.DecodeString(msg.Data.Body)
		if err != nil {
			t.Fatalf("Base64 decode failed: %v", err)
		}
		if !bytes.Equal(decoded, binaryData) {
			t.Errorf("binary data mismatch")
		}
	})
}

func TestDecodeResponse_EdgeCases(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		_, err := DecodeResponse([]byte(`{invalid}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("invalid base64 body", func(t *testing.T) {
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status: 200,
				Body:   "!!!invalid-base64!!!",
			},
		}
		data, _ := json.Marshal(respData)

		_, err := DecodeResponse(data)
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})

	t.Run("missing body (empty response)", func(t *testing.T) {
		respData := WSResponse{
			Type: "response",
			Data: WSRespData{
				Status:  204,
				Headers: map[string]string{},
			},
		}
		data, _ := json.Marshal(respData)

		resp, err := DecodeResponse(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		body, _ := io.ReadAll(resp.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body for 204, got %d bytes", len(body))
		}
	})
}
