package wsclient

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	tunnel    TunnelSender
	startTime time.Time
	server    string
}

// HealthStatus 健康状态响应
type HealthStatus struct {
	Status      string `json:"status"`
	Server      string `json:"server"`
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(tunnel TunnelSender, server string) *HealthChecker {
	return &HealthChecker{
		tunnel:    tunnel,
		startTime: time.Now(),
		server:    server,
	}
}

// ServeHTTP 实现 http.Handler 接口
func (h *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 只处理 GET /health
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 获取连接状态
	status := "disconnected"
	if h.tunnel.IsConnected() {
		status = "connected"
	}

	// 计算运行时间
	uptime := int64(time.Since(h.startTime).Seconds())

	// 构建响应
	resp := HealthStatus{
		Status:        status,
		Server:        h.server,
		UptimeSeconds: uptime,
	}

	// 写入响应
	data, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
