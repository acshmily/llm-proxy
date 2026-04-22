package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
	"unicode/utf8"

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
	cfg           *config.Config
	router        *router.Router
	log           *logger.Logger
	client        *http.Client
	transport     *http.Transport
	poolStats     *PoolStats
	wsTunnel      *WSTunnelMiddleware
	debugRequests bool
	debugMaxBody  int
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

	// 初始化 debug 日志配置
	debugMaxBody := cfg.Logging.DebugMaxBody
	if debugMaxBody <= 0 {
		debugMaxBody = 2048
	}

	s := &Server{
		cfg:           cfg,
		router:        r,
		log:           log,
		client:        client,
		transport:     transport,
		poolStats:     poolStats,
		wsTunnel:      wsTunnel,
		debugRequests: cfg.Logging.DebugRequests,
		debugMaxBody:  debugMaxBody,
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
	case r.URL.Path == "/v1/models" && r.Method == http.MethodGet:
		s.serveModelsList(w, r)
		return
	case r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost:
		s.serveOpenAIRequest(w, r)
		return
	case r.URL.Path == "/v1/completions" && r.Method == http.MethodPost:
		s.serveCompletionsRequest(w, r)
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

	s.logRequestBody(r.URL.Path, body)

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
		if unified.Stream {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":streamGenerateContent?alt=sse&key=" + route.BackendKey
		} else {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent?key=" + route.BackendKey
		}
		reqBody, _ = gemini.Convert(unified, model)
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
		s.handleNonStream(w, resp, route.Backend, model, latency, start, connReused)
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

	s.logRequestBody(r.URL.Path, body)

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
		if unified.Stream {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":streamGenerateContent?alt=sse&key=" + route.BackendKey
		} else {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent?key=" + route.BackendKey
		}
		reqBody, err = gemini.Convert(unified, model)
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
		s.handleOpenAINonStream(w, resp, route.Backend, model, latency, start, connReused)
	}
}

// serveCompletionsRequest 处理 OpenAI Completions 格式请求（旧版 /v1/completions）
func (s *Server) serveCompletionsRequest(w http.ResponseWriter, r *http.Request) {
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

	// 解析请求，兼容 prompt 和 messages 两种格式
	// Content 使用 json.RawMessage 兼容字符串和数组两种格式
	type rawMsg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	var rawRequest struct {
		Model       string   `json:"model"`
		Prompt      string   `json:"prompt"`
		Messages    []rawMsg `json:"messages"`
		Stream      bool     `json:"stream"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
		TopP        float64  `json:"top_p"`
		Stop        []string `json:"stop"`
	}
	if err := json.Unmarshal(body, &rawRequest); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	s.logRequestBody(r.URL.Path, body)

	// 转换为统一消息格式
	var messages []types.MessageRole
	if rawRequest.Prompt != "" {
		// prompt 格式：包装为单条 user 消息
		messages = []types.MessageRole{{Role: "user", Content: rawRequest.Prompt}}
	} else if len(rawRequest.Messages) > 0 {
		// messages 格式：提取 content 文本
		messages = make([]types.MessageRole, len(rawRequest.Messages))
		for i, msg := range rawRequest.Messages {
			messages[i] = types.MessageRole{Role: msg.Role, Content: extractContent(msg.Content)}
		}
	}

	if len(messages) == 0 {
		s.writeError(w, http.StatusBadRequest, "Either 'prompt' or 'messages' must be provided")
		return
	}

	unified := &types.UnifiedMessage{
		Model:    rawRequest.Model,
		Stream:   rawRequest.Stream,
		Messages: messages,
		MaxTokens: rawRequest.MaxTokens,
		Temperature: rawRequest.Temperature,
		TopP:     rawRequest.TopP,
		StopSequences: rawRequest.Stop,
	}

	// 复用现有后端处理逻辑
	model := unified.Model
	if model == "" {
		model = getDefaultModel(route.Backend)
	}

	var backendURL string
	var reqBody []byte

	switch route.Backend {
	case "openai":
		backendURL = s.cfg.Backends.OpenAI.BaseURL + "/chat/completions"
		reqBody, err = openai.Convert(unified, model)
	case "anthropic":
		backendURL = s.cfg.Backends.Anthropic.BaseURL + "/v1/messages"
		reqBody, err = claude.Convert(unified, model)
	case "gemini":
		if unified.Stream {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":streamGenerateContent?alt=sse&key=" + route.BackendKey
		} else {
			backendURL = s.cfg.Backends.Gemini.BaseURL + "/models/" + model + ":generateContent?key=" + route.BackendKey
		}
		reqBody, err = gemini.Convert(unified, model)
	default:
		s.writeError(w, http.StatusBadRequest, "Unknown backend")
		return
	}

	if err != nil {
		s.log.Error("Protocol conversion failed",
			logger.LogField{Key: "backend", Value: route.Backend},
			logger.LogField{Key: "error", Value: err.Error()},
		)
		s.writeError(w, http.StatusInternalServerError, "Failed to convert protocol")
		return
	}

	backendReq, err := http.NewRequest(r.Method, backendURL, bytes.NewReader(reqBody))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create backend request")
		return
	}

	if route.Backend != "gemini" {
		backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
	}
	backendReq.Header.Set("Content-Type", "application/json")

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
	s.poolStats.RecordRequest(connReused)

	// 处理响应并转换为 Completions 格式
	if unified.Stream {
		s.handleCompletionsStream(w, resp, route.Backend, connReused, r)
	} else {
		s.handleCompletionsNonStream(w, resp, route.Backend, model, latency, start, connReused)
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

func (s *Server) handleNonStream(w http.ResponseWriter, resp *http.Response, backend string, model string, latency int64, start time.Time, connReused bool) {
	body, _ := io.ReadAll(resp.Body)

	s.logResponseBody("/v1/messages", body, resp.StatusCode)

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
		unified, err = gemini.ParseResponse(body, model)
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
func (s *Server) handleOpenAINonStream(w http.ResponseWriter, resp *http.Response, backend string, model string, latency int64, start time.Time, connReused bool) {
	body, _ := io.ReadAll(resp.Body)

	s.logResponseBody("/v1/chat/completions", body, resp.StatusCode)

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
		unified, err = gemini.ParseResponse(body, model)
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

// handleCompletionsNonStream 处理 Completions 非流式响应（/v1/completions）
func (s *Server) handleCompletionsNonStream(w http.ResponseWriter, resp *http.Response, backend string, model string, latency int64, start time.Time, connReused bool) {
	body, _ := io.ReadAll(resp.Body)

	s.logResponseBody("/v1/completions", body, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		s.writeError(w, resp.StatusCode, convertBackendError(backend, body))
		return
	}

	var unified *types.UnifiedResponse
	var err error

	switch backend {
	case "openai":
		unified, err = openai.ParseResponse(body)
	case "anthropic":
		unified, err = claude.ParseResponse(body)
	case "gemini":
		unified, err = gemini.ParseResponse(body, model)
	}

	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to parse response")
		return
	}

	respBody, err := buildCompletionsResponse(unified)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to build response")
		return
	}

	s.log.Info("Completions request completed",
		logger.LogField{Key: "latency_ms", Value: latency},
		logger.LogField{Key: "status_code", Value: resp.StatusCode},
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}

// buildCompletionsResponse 将统一响应转换为 Completions 格式
func buildCompletionsResponse(unified *types.UnifiedResponse) ([]byte, error) {
	var text string
	if len(unified.Content) > 0 {
		text = unified.Content[0].Text
	}

	finishReason := unified.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	resp := map[string]interface{}{
		"id":      unified.ID,
		"object":  "text_completion",
		"created": time.Now().Unix(),
		"model":   unified.Model,
		"choices": []map[string]interface{}{
			{"text": text, "index": 0, "finish_reason": finishReason},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     unified.Usage.InputTokens,
			"completion_tokens": unified.Usage.OutputTokens,
			"total_tokens":      unified.Usage.InputTokens + unified.Usage.OutputTokens,
		},
	}

	return json.Marshal(resp)
}

// handleCompletionsStream 处理 Completions 流式响应
func (s *Server) handleCompletionsStream(w http.ResponseWriter, resp *http.Response, backend string, connReused bool, req *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.log.Info("Completions stream request completed",
		logger.LogField{Key: "backend", Value: backend},
		logger.LogField{Key: "conn_reused", Value: connReused},
	)

	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	stream.ParseSSE(resp.Body, func(event string, data []byte) {
		select {
		case <-clientDisconnected:
			return
		default:
		}

		var completionsData []byte

		switch backend {
		case "openai":
			completionsData = convertOpenAISSEToCompletions(event, data)
		case "anthropic":
			completionsData = convertAnthropicSSEToCompletions(event, data)
		case "gemini":
			completionsData = convertGeminiSSEToCompletions(event, data)
		}

		if len(completionsData) > 0 {
			w.Write(completionsData)
			w.Write([]byte("\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	})
}

// convertOpenAISSEToCompletions 转换 OpenAI SSE 为 Completions 格式
func convertOpenAISSEToCompletions(event string, data []byte) []byte {
	if bytes.Equal(data, []byte("[DONE]")) {
		return []byte("data: [DONE]")
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}
	if choices, ok := msg["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if content, ok := delta["content"]; ok && content != nil {
					if str, ok := content.(string); ok {
						choice["text"] = str
					}
					delete(choice, "delta")
				}
			}
		}
	}
	result, _ := json.Marshal(msg)
	return append([]byte("data: "), result...)
}

// convertAnthropicSSEToCompletions 转换 Anthropic SSE 为 Completions 格式
func convertAnthropicSSEToCompletions(event string, data []byte) []byte {
	if event == "message_stop" {
		return []byte(`data: {"choices":[{"text":"","finish_reason":"stop"}]}`)
	}

	if event == "content_block_delta" {
		var delta struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(data, &delta); err == nil && delta.Delta.Text != "" {
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"text": delta.Delta.Text, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	return nil
}

// convertGeminiSSEToCompletions 转换 Gemini SSE 为 Completions 格式
func convertGeminiSSEToCompletions(event string, data []byte) []byte {
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
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"text": text, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
		if finishReason != "" {
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"text": "", "finish_reason": openaiFinishReason},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	return nil
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

// serveModelsList 返回所有已配置后端的模型列表（OpenAI 兼容格式）
func (s *Server) serveModelsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type modelInfo struct {
		ID        string `json:"id"`
		Object    string `json:"object"`
		Created   int64  `json:"created"`
		OwnedBy   string `json:"owned_by"`
	}

	type modelsResponse struct {
		Object string      `json:"object"`
		Data   []modelInfo `json:"data"`
	}

	var allModels []modelInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 后端查询函数
	type backendQuery struct {
		name    string
		url     string
		ownedBy string
		auth    string // Authorization header 值，Gemini 留空
	}

	// 从路由中查找对应后端的 API Key
	findKey := func(backendName string) string {
		for _, route := range s.cfg.Routes {
			if route.Backend == backendName {
				return route.BackendKey
			}
		}
		return ""
	}

	backends := []backendQuery{}

	if s.cfg.Backends.OpenAI.BaseURL != "" {
		key := findKey("openai")
		backends = append(backends, backendQuery{
			name:    "openai",
			url:     s.cfg.Backends.OpenAI.BaseURL + "/models",
			ownedBy: "openai",
			auth:    "Bearer " + key,
		})
	}
	if s.cfg.Backends.Anthropic.BaseURL != "" {
		key := findKey("anthropic")
		backends = append(backends, backendQuery{
			name:    "anthropic",
			url:     s.cfg.Backends.Anthropic.BaseURL + "/v1/models",
			ownedBy: "anthropic",
			auth:    key,
		})
	}
	if s.cfg.Backends.Gemini.BaseURL != "" {
		for _, route := range s.cfg.Routes {
			if route.Backend == "gemini" {
				backends = append(backends, backendQuery{
					name:    "gemini",
					url:     s.cfg.Backends.Gemini.BaseURL + "/models?key=" + route.BackendKey,
					ownedBy: "google",
				})
				break
			}
		}
	}

	for _, be := range backends {
		wg.Add(1)
		go func(bq backendQuery) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, bq.url, nil)
			if err != nil {
				s.log.Error("Failed to create models request for backend",
					logger.LogField{Key: "backend", Value: bq.name},
					logger.LogField{Key: "error", Value: err.Error()},
				)
				return
			}
			if bq.auth != "" {
				if bq.name == "anthropic" {
					req.Header.Set("x-api-key", bq.auth)
				} else {
					req.Header.Set("Authorization", bq.auth)
				}
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := s.client.Do(req)
			if err != nil {
				s.log.Error("Failed to fetch models from backend",
					logger.LogField{Key: "backend", Value: bq.name},
					logger.LogField{Key: "error", Value: err.Error()},
				)
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}

			if resp.StatusCode != http.StatusOK {
				s.log.Error("Backend returned error for models endpoint",
					logger.LogField{Key: "backend", Value: bq.name},
					logger.LogField{Key: "status", Value: resp.StatusCode},
					logger.LogField{Key: "body", Value: string(body)},
				)
				return
			}

			// 解析后端响应
			var backendModels struct {
				Data []struct {
					ID       string `json:"id"`
					Object   string `json:"object"`
					Created  int64  `json:"created"`
					OwnedBy  string `json:"owned_by"`
				} `json:"data"`
			}

			// Gemini 使用 models 字段而非 data
			var geminiModels struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}

			if bq.name == "gemini" {
				if err := json.Unmarshal(body, &geminiModels); err != nil {
					s.log.Error("Failed to parse models from Gemini backend",
						logger.LogField{Key: "backend", Value: bq.name},
						logger.LogField{Key: "error", Value: err.Error()},
					)
					return
				}
				mu.Lock()
				for _, m := range geminiModels.Models {
					// Gemini 返回格式: "models/gemini-2.5-flash"
					id := m.Name
					if len(id) > 7 && id[:7] == "models/" {
						id = id[7:]
					}
					allModels = append(allModels, modelInfo{
						ID:      id,
						Object:  "model",
						Created: time.Now().Unix(),
						OwnedBy: bq.ownedBy,
					})
				}
				mu.Unlock()
			} else {
				if err := json.Unmarshal(body, &backendModels); err != nil {
					s.log.Error("Failed to parse models from backend",
						logger.LogField{Key: "backend", Value: bq.name},
						logger.LogField{Key: "error", Value: err.Error()},
					)
					return
				}
				mu.Lock()
				for _, m := range backendModels.Data {
					allModels = append(allModels, modelInfo{
						ID:      m.ID,
						Object:  "model",
						Created: m.Created,
						OwnedBy: bq.ownedBy,
					})
				}
				mu.Unlock()
			}
		}(be)
	}

	wg.Wait()

	if allModels == nil {
		allModels = []modelInfo{}
	}

	json.NewEncoder(w).Encode(modelsResponse{
		Object: "list",
		Data:   allModels,
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

// logRequestBody logs the request body if debug_requests is enabled.
func (s *Server) logRequestBody(path string, body []byte) {
	if !s.debugRequests || len(body) == 0 {
		return
	}
	truncated, total := truncateBody(body, s.debugMaxBody)
	s.log.Debug("Request body",
		logger.LogField{Key: "path", Value: path},
		logger.LogField{Key: "body", Value: truncated},
		logger.LogField{Key: "total_bytes", Value: total},
	)
}

// logResponseBody logs the response body if debug_requests is enabled.
func (s *Server) logResponseBody(path string, body []byte, statusCode int) {
	if !s.debugRequests || len(body) == 0 {
		return
	}
	truncated, total := truncateBody(body, s.debugMaxBody)
	s.log.Debug("Response body",
		logger.LogField{Key: "path", Value: path},
		logger.LogField{Key: "status", Value: statusCode},
		logger.LogField{Key: "body", Value: truncated},
		logger.LogField{Key: "total_bytes", Value: total},
	)
}

// truncateBody truncates body to max bytes, returns truncated string and total length.
// Safely handles UTF-8 multi-byte character boundaries.
func truncateBody(body []byte, max int) (string, int) {
	total := len(body)
	if total <= max {
		return string(body), total
	}
	// 回退到最后一个完整 UTF-8 字符边界
	end := max
	for end > 0 {
		_, size := utf8.DecodeLastRune(body[:end])
		if size > 0 {
			break
		}
		end--
	}
	return string(body[:end]) + fmt.Sprintf("...(truncated, %d bytes total)", total), total
}

// extractContent extracts text content from either a JSON string or an array of content blocks.
// OpenClaw sends array format: [{"type": "text", "text": "..."}]
// Standard clients send string format: "Hello"
func extractContent(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	// Try string format first
	var str string
	if err := json.Unmarshal(content, &str); err == nil {
		return str
	}
	// Try array format
	var blocks []struct {
		Type    string `json:"type"`
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(content, &blocks); err == nil {
		var result string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				result += b.Text
			}
			if b.Type == "tool_result" && b.Content != "" {
				result += b.Content
			}
		}
		return result
	}
	return ""
}
