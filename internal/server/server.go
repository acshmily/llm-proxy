package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/middleware"
	"github.com/claude-projetc/llm-proxy/internal/protocol/anthropic"
	"github.com/claude-projetc/llm-proxy/internal/protocol/openai"
	"github.com/claude-projetc/llm-proxy/internal/protocol/claude"
	"github.com/claude-projetc/llm-proxy/internal/protocol/gemini"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/claude-projetc/llm-proxy/internal/stream"
	"github.com/claude-projetc/llm-proxy/pkg/types"
)

type Server struct {
	cfg       *config.Config
	router    *router.Router
	log       *logger.Logger
	client    *http.Client
	transport *http.Transport
	poolStats *PoolStats
	wsTunnel  *WSTunnelMiddleware
}

// PoolStats 连接池统计
type PoolStats struct {
	mu            sync.Mutex
	requests      int
	reusedCount   int
	createCount   int
}

func (p *PoolStats) RecordRequest(reused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requests++
	if reused {
		p.reusedCount++
	} else {
		p.createCount++
	}
}

func (p *PoolStats) GetStats() (int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests, p.reusedCount, p.createCount
}

func New(cfg *config.Config, r *router.Router, log *logger.Logger) *Server {
	// 配置 HTTP 连接池
	transport := &http.Transport{
		MaxIdleConns:        100,              // 最大空闲连接数
		MaxIdleConnsPerHost: 10,               // 每个主机的最大空闲连接数
		IdleConnTimeout:     90 * time.Second, // 空闲连接超时时间
		TLSHandshakeTimeout: 10 * time.Second, // TLS 握手超时
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	poolStats := &PoolStats{}

	client := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second, // 默认请求超时
	}

	// 创建 WebSocket 隧道中间件（先创建对象，但不设置回调）
	var wsTunnel *WSTunnelMiddleware
	var obfusMiddleware *middleware.TrafficObfuscationMiddleware
	if cfg.Protection.TrafficObfuscation.WebSocketTunnel.Enabled {
		obfusMiddleware = middleware.NewTrafficObfuscationMiddleware(&cfg.Protection.TrafficObfuscation)
		wsTunnel = NewWSTunnelMiddleware(&cfg.Protection.TrafficObfuscation.WebSocketTunnel, obfusMiddleware)
	}

	// 启动定期统计日志
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			// http.Transport 没有导出的 IdleConnCount 方法，用 -1 表示未知
			requests, reused, created := poolStats.GetStats()
			var reuseRate float64
			if requests > 0 {
				reuseRate = float64(reused) / float64(requests) * 100
			}
			log.LogStatsWithDetail(-1, requests, reused, created, reuseRate)
		}
	}()

	s := &Server{
		cfg:       cfg,
		router:    r,
		log:       log,
		client:    client,
		transport: transport,
		poolStats: poolStats,
		wsTunnel:  wsTunnel,
	}

	// 配置 WebSocket 隧道的请求处理回调（在 s 创建之后）
	if wsTunnel != nil {
		wsTunnel.SetRequestHandler(s.serveRequest)
	}

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// WebSocket 隧道端点
	if s.wsTunnel != nil && r.URL.Path == s.wsTunnel.GetPath() {
		s.wsTunnel.WSTunnelHandler()(w, r)
		return
	}

	// 健康检查端点
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		s.HealthCheck(w, r)
		return
	}

	// 根据路径路由到不同协议处理器
	switch {
	case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
		s.serveOpenAIRequest(w, r)
		return
	case r.URL.Path == "/v1/messages" && r.Method == http.MethodPost:
		// 现有 Anthropic 端点
		s.serveRequest(w, r)
		return
	default:
		s.writeError(w, http.StatusNotFound, "Endpoint not found")
		return
	}
}

// serveRequest 处理实际请求（供 WebSocket 隧道调用）
func (s *Server) serveRequest(w http.ResponseWriter, r *http.Request) {

	start := time.Now()

	// 获取 API Key
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		apiKey = extractBearerToken(r.Header.Get("Authorization"))
	}

	route, found := s.router.FindRoute(apiKey)
	if !found {
		s.writeError(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 解析 Anthropic 请求
	unified, err := anthropic.ParseRequest(body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// 选择后端转换器
	var backendURL string
	var reqBody []byte

	// 使用客户端请求中的 model，如果没有则使用默认值
	model := unified.Model
	if model == "" {
		model = getDefaultModel(route.Backend)
	}

	switch route.Backend {
	case "openai":
		backendURL = s.cfg.Backends.OpenAI.BaseURL + "/chat/completions"
		reqBody, _ = openai.Convert(unified, model)
	case "anthropic":
		backendURL = s.cfg.Backends.Anthropic.BaseURL + "/v1/messages"
		reqBody, _ = claude.Convert(unified, model)
	case "gemini":
		backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent"
		reqBody, _ = gemini.Convert(unified, model)
		// Gemini 使用 URL 参数 key= 而非 Bearer Token
		backendURL = backendURL + "?key=" + route.BackendKey
	default:
		s.writeError(w, http.StatusBadRequest, "Unknown backend")
		return
	}

	// 创建后端请求
	backendReq, err := http.NewRequest(r.Method, backendURL, bytes.NewReader(reqBody))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create backend request")
		return
	}

	// 设置后端认证（Gemini 不需要 Authorization header）
	if route.Backend != "gemini" {
		backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
	}
	backendReq.Header.Set("Content-Type", "application/json")

	// 执行请求
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	// 使用 httptrace 追踪连接是否复用
	var connReused bool
	connReused = true // 默认假设复用，除非明确触发拨号

	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			connReused = info.Reused
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	resp, err := s.client.Do(backendReq.WithContext(ctx))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Backend request failed")
		return
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	// 记录连接复用统计
	s.poolStats.RecordRequest(connReused)

	// 处理响应
	if unified.Stream {
		s.handleStream(w, resp, route.Backend, connReused)
	} else {
		s.handleNonStream(w, resp, route.Backend, latency, start, connReused)
	}
}

// serveOpenAIRequest 处理 OpenAI 协议请求
func (s *Server) serveOpenAIRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// 获取 API Key
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		apiKey = extractBearerToken(r.Header.Get("Authorization"))
	}

	route, found := s.router.FindRoute(apiKey)
	if !found {
		s.writeError(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	// 解析 OpenAI 请求
	unified, err := openai.ParseRequest(body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// 选择后端转换器
	var backendURL string
	var reqBody []byte

	model := unified.Model
	if model == "" {
		model = getDefaultModel(route.Backend)
	}

	switch route.Backend {
	case "openai":
		backendURL = s.cfg.Backends.OpenAI.BaseURL + "/chat/completions"
		reqBody, err = openai.Convert(unified, model)
	case "anthropic":
		backendURL = s.cfg.Backends.Anthropic.BaseURL + "/v1/messages"
		reqBody, err = claude.Convert(unified, model)
	case "gemini":
		backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent"
		reqBody, err = gemini.Convert(unified, model)
		if err == nil {
			backendURL = backendURL + "?key=" + route.BackendKey
		}
	default:
		s.writeError(w, http.StatusBadRequest, "Unknown backend")
		return
	}

	if err != nil {
		s.log.Error("Protocol conversion failed",
			logger.LogField{Key: "backend", Value: route.Backend},
			logger.LogField{Key: "error", Value: err.Error()},
		)
		s.writeOpenAIError(w, http.StatusInternalServerError, "Failed to convert protocol")
		return
	}

	// 创建后端请求
	backendReq, err := http.NewRequest(r.Method, backendURL, bytes.NewReader(reqBody))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create backend request")
		return
	}

	// 设置后端认证（Gemini 不需要 Authorization header）
	if route.Backend != "gemini" {
		backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
	}
	backendReq.Header.Set("Content-Type", "application/json")

	// 执行请求
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	var connReused bool
	connReused = true

	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			connReused = info.Reused
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	resp, err := s.client.Do(backendReq.WithContext(ctx))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Backend request failed")
		return
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	// 记录连接复用统计
	s.poolStats.RecordRequest(connReused)

	// 处理响应
	if unified.Stream {
		s.handleOpenAIStream(w, resp, route.Backend, connReused, r)
	} else {
		s.handleOpenAINonStream(w, resp, route.Backend, latency, start, connReused)
	}
}

func (s *Server) handleStream(w http.ResponseWriter, resp *http.Response, backend string, connReused bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 记录连接复用状态
	s.log.Info("Stream request completed",
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
	)

	stream.ParseSSE(resp.Body, func(event string, data []byte) {
		w.Write(data)
		w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
}

func (s *Server) handleNonStream(w http.ResponseWriter, resp *http.Response, backend string, latency int64, start time.Time, connReused bool) {
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		s.writeError(w, resp.StatusCode, string(body))
		return
	}

	// 转换响应为 Anthropic 格式
	var unified *types.UnifiedResponse
	var err error

	switch backend {
	case "openai":
		unified, err = openai.ParseResponse(body)
	case "anthropic":
		unified, err = claude.ParseResponse(body)
	case "gemini":
		unified, err = gemini.ParseResponse(body)
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to parse response")
		return
	}

	// 记录日志
	requests, reused, created := s.poolStats.GetStats()
	s.log.Info("Request completed",
		logger.LogField{Key: "latency_ms", Value: latency},
		logger.LogField{Key: "status_code", Value: resp.StatusCode},
		logger.LogField{Key: "input_tokens", Value: unified.Usage.InputTokens},
		logger.LogField{Key: "output_tokens", Value: unified.Usage.OutputTokens},
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
		logger.LogField{Key: "pool_requests", Value: requests},
		logger.LogField{Key: "pool_reused", Value: reused},
		logger.LogField{Key: "pool_created", Value: created},
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(unified)
}

// handleOpenAINonStream 处理 OpenAI 非流式响应
func (s *Server) handleOpenAINonStream(w http.ResponseWriter, resp *http.Response, backend string, latency int64, start time.Time, connReused bool) {
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// 转换后端错误为 OpenAI 格式
		s.writeOpenAIError(w, resp.StatusCode, convertBackendError(backend, body))
		return
	}

	// 转换响应为统一格式
	var unified *types.UnifiedResponse
	var err error

	switch backend {
	case "openai":
		unified, err = openai.ParseResponse(body)
	case "anthropic":
		unified, err = claude.ParseResponse(body)
	case "gemini":
		unified, err = gemini.ParseResponse(body)
	}

	if err != nil {
		s.writeOpenAIError(w, http.StatusInternalServerError, "Failed to parse response")
		return
	}

	// 构建 OpenAI 格式响应
	respBody, err := openai.BuildResponse(unified)
	if err != nil {
		s.writeOpenAIError(w, http.StatusInternalServerError, "Failed to build response")
		return
	}

	// 记录日志
	s.log.Info("OpenAI request completed",
		logger.LogField{Key: "latency_ms", Value: latency},
		logger.LogField{Key: "status_code", Value: resp.StatusCode},
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

// handleOpenAIStream 处理 OpenAI 流式响应
func (s *Server) handleOpenAIStream(w http.ResponseWriter, resp *http.Response, backend string, connReused bool, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// 记录连接复用状态
	s.log.Info("OpenAI stream request completed",
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
	)

	// 检测客户端断开连接（使用 context 替代已弃用的 http.CloseNotifier）
	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	// 流式解析并转换为 OpenAI SSE 格式
	stream.ParseSSE(resp.Body, func(event string, data []byte) {
		select {
		case <-clientDisconnected:
			return
		default:
		}

		var openaiData []byte

		switch backend {
		case "openai":
			// OpenAI 后端直接透传
			openaiData = data
		case "anthropic":
			// 转换 Anthropic SSE 为 OpenAI SSE 格式
			openaiData = convertAnthropicSSEToOpenAI(event, data)
		case "gemini":
			// 转换 Gemini SSE 为 OpenAI SSE 格式
			openaiData = convertGeminiSSEToOpenAI(event, data)
		}

		if len(openaiData) > 0 {
			w.Write(openaiData)
			w.Write([]byte("\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	})
}

// convertAnthropicSSEToOpenAI 转换 Anthropic SSE 为 OpenAI 格式
func convertAnthropicSSEToOpenAI(event string, data []byte) []byte {
	if event == "message_stop" {
		return []byte(`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	}

	if event == "content_block_delta" {
		var delta struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &delta); err == nil && delta.Delta.Text != "" {
			// 使用 json.Marshal 避免 JSON 注入（文本中包含引号、换行等特殊字符）
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{"content": delta.Delta.Text}, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	return nil
}

// convertGeminiSSEToOpenAI 转换 Gemini SSE 为 OpenAI 格式
func convertGeminiSSEToOpenAI(event string, data []byte) []byte {
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &geminiResp); err != nil {
		return nil
	}

	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text := geminiResp.Candidates[0].Content.Parts[0].Text
		finishReason := geminiResp.Candidates[0].FinishReason

		var openaiFinishReason string
		if finishReason == "STOP" {
			openaiFinishReason = "stop"
		} else if finishReason == "MAX_TOKENS" {
			openaiFinishReason = "length"
		} else {
			openaiFinishReason = "stop"
		}

		if text != "" {
			// 使用 json.Marshal 避免 JSON 注入
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{"content": text}, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
		if finishReason != "" {
			// 使用 json.Marshal 避免 JSON 注入
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{}, "finish_reason": openaiFinishReason},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	return nil
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.APIError{
		Type:    "error",
		Message: msg,
	})
}

// writeOpenAIError 写入 OpenAI 格式错误
func (s *Server) writeOpenAIError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	type OpenAIError struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code,omitempty"`
		} `json:"error"`
	}

	errType := errorCodeMap[code]
	if errType == "" {
		errType = "internal_server_error"
	}

	err := OpenAIError{}
	err.Error.Message = message
	err.Error.Type = errType
	err.Error.Code = errType

	json.NewEncoder(w).Encode(err)
}

// errorCodeMap HTTP 状态码到错误类型映射
var errorCodeMap = map[int]string{
	400: "invalid_request_error",
	401: "authentication_error",
	403: "permission_error",
	404: "not_found_error",
	429: "rate_limit_error",
	500: "internal_server_error",
	502: "service_unavailable_error",
	503: "service_unavailable_error",
}

// convertBackendError 转换后端错误为 OpenAI 格式
func convertBackendError(backend string, body []byte) string {
	// 尝试解析后端错误
	var backendErr map[string]interface{}
	if err := json.Unmarshal(body, &backendErr); err != nil {
		return string(body)
	}

	// OpenAI 格式：{"error": {"message": "...", "type": "..."}}
	// Anthropic 格式：{"error": {"message": "...", "type": "..."}}
	// Gemini 格式：{"error": {"code": ..., "message": "..."}}

	if backend == "openai" || backend == "anthropic" {
		if errMap, ok := backendErr["error"].(map[string]interface{}); ok {
			if msg, ok := errMap["message"].(string); ok {
				return msg
			}
		}
	}

	if backend == "gemini" {
		if errMap, ok := backendErr["error"].(map[string]interface{}); ok {
			if msg, ok := errMap["message"].(string); ok {
				return msg
			}
		}
	}

	return string(body)
}

// HealthCheck 健康检查端点
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func extractBearerToken(auth string) string {
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

func getDefaultModel(backend string) string {
	switch backend {
	case "openai":
		return "gpt-4"
	case "anthropic":
		return "claude-3-opus-20240229"
	case "gemini":
		return "gemini-pro"
	default:
		return ""
	}
}
