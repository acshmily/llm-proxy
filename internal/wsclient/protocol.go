package wsclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// WSRequest WebSocket 请求消息
type WSRequest struct {
	Type string    `json:"type"`
	Data WSReqData `json:"data"`
}

// WSReqData WebSocket 请求数据
type WSReqData struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
}

// WSResponse WebSocket 响应消息
type WSResponse struct {
	Type string     `json:"type"`
	Data WSRespData `json:"data"`
}

// WSRespData WebSocket 响应数据
type WSRespData struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body,omitempty"`
	Message string            `json:"message,omitempty"`
}

// ErrServerResponse 服务端返回的错误
var ErrServerResponse = errors.New("server returned error")

// EncodeRequest 将 HTTP 请求编码为 WebSocket 消息
func EncodeRequest(req *http.Request) ([]byte, error) {
	// 读取请求体
	var bodyBytes []byte
	var err error
	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		defer req.Body.Close()
	}

	// 构建请求数据
	data := WSReqData{
		Method:  req.Method,
		Path:    req.URL.RequestURI(),
		Headers: make(map[string]string),
	}

	// 只有当 body 非空时才编码
	if len(bodyBytes) > 0 {
		data.Body = base64.StdEncoding.EncodeToString(bodyBytes)
	}

	// 复制请求头（多值头部取第一个）
	for key, values := range req.Header {
		if len(values) > 0 {
			data.Headers[key] = values[0]
		}
	}

	// 构建 WebSocket 消息
	msg := WSRequest{
		Type: "request",
		Data: data,
	}

	return json.Marshal(msg)
}

// DecodeResponse 将 WebSocket 响应解码为 HTTP 响应
func DecodeResponse(data []byte) (*http.Response, error) {
	var resp WSResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	// 处理错误响应
	if resp.Type == "error" {
		return nil, ErrServerResponse
	}

	// 解码响应体
	bodyBytes := []byte{}
	var err error
	if resp.Data.Body != "" {
		bodyBytes, err = base64.StdEncoding.DecodeString(resp.Data.Body)
		if err != nil {
			return nil, err
		}
	}

	// 构建 HTTP 响应
	httpResp := &http.Response{
		Status:        http.StatusText(resp.Data.Status),
		StatusCode:    resp.Data.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(bodyBytes)),
		ContentLength: int64(len(bodyBytes)),
	}

	// 复制响应头
	for key, value := range resp.Data.Headers {
		httpResp.Header.Set(key, value)
	}

	return httpResp, nil
}
