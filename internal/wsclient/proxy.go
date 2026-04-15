package wsclient

import (
	"io"
	"log"
	"net/http"
	"time"
)

// TunnelSender 隧道发送器接口
type TunnelSender interface {
	SendRequest(*http.Request) (*http.Response, error)
	IsConnected() bool
}

// ProxyServer HTTP 代理服务器
type ProxyServer struct {
	tunnel  TunnelSender
	timeout time.Duration
}

// NewProxyServer 创建代理服务器
func NewProxyServer(tunnel TunnelSender) *ProxyServer {
	return &ProxyServer{
		tunnel:  tunnel,
		timeout: 60 * time.Second,
	}
}

// ServeHTTP 实现 http.Handler 接口
func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 检查隧道是否连接
	if !p.tunnel.IsConnected() {
		http.Error(w, "tunnel is not connected", http.StatusServiceUnavailable)
		return
	}

	// 通过隧道发送请求
	resp, err := p.tunnel.SendRequest(r)
	if err != nil {
		log.Printf("failed to send request through tunnel: %v", err)
		http.Error(w, "failed to forward request", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// 写入响应状态
	w.WriteHeader(resp.StatusCode)

	// 写入响应体
	_, err = copyBody(w, resp.Body)
	if err != nil {
		log.Printf("failed to write response body: %v", err)
	}
}

// copyBody 复制响应体
func copyBody(w http.ResponseWriter, body io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := body.Read(buf)
		if n > 0 {
			nw, errw := w.Write(buf[:n])
			total += int64(nw)
			if errw != nil {
				return total, errw
			}
		}
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
	}
}
