package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/claude-projetc/llm-proxy/internal/config"
	"github.com/claude-projetc/llm-proxy/internal/gemini"
	"github.com/claude-projetc/llm-proxy/internal/logger"
	"github.com/claude-projetc/llm-proxy/internal/middleware"
	"github.com/claude-projetc/llm-proxy/internal/protocol/anthropic"
	"github.com/claude-projetc/llm-proxy/internal/protocol/claude"
	protocolgemini "github.com/claude-projetc/llm-proxy/internal/protocol/gemini"
	"github.com/claude-projetc/llm-proxy/internal/protocol/openai"
	"github.com/claude-projetc/llm-proxy/internal/router"
	"github.com/claude-projetc/llm-proxy/internal/stream"
	"github.com/claude-projetc/llm-proxy/pkg/types"
	"google.golang.org/genai"
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
	geminiClient  *gemini.GeminiClient // SDK 客户端（nil 时 fallback 到 HTTP）
}

// PoolStats 连接池统计
type PoolStats struct {
	mu          sync.Mutex
	requests    int
	reusedCount int
	createCount int
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
		MaxIdleConns:          100,              // 最大空闲连接数
		MaxIdleConnsPerHost:   10,               // 每个主机的最大空闲连接数
		IdleConnTimeout:       90 * time.Second, // 空闲连接超时时间
		TLSHandshakeTimeout:   10 * time.Second, // TLS 握手超时
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

	// 初始化 Gemini SDK 客户端
	// 仅当使用标准 Gemini API URL 时才创建 SDK（非测试 mock 地址）
	var geminiClient *gemini.GeminiClient
	if cfg.Backends.Gemini.BaseURL != "" && isStandardGeminiAPI(cfg.Backends.Gemini.BaseURL) {
		apiKey := findGeminiBackendKey(cfg.Routes)
		gc, err := gemini.NewGeminiClient(
			apiKey,
			cfg.Backends.Gemini.HttpProxy,
			log,
			cfg.Logging.DebugRequests,
			debugMaxBody,
		)
		if err != nil {
			log.Error("Failed to create Gemini SDK client, fallback to HTTP",
				logger.LogField{Key: "error", Value: err.Error()},
			)
		} else {
			geminiClient = gc
		}
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
		geminiClient:  geminiClient,
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
	case strings.HasPrefix(r.URL.Path, "/v1beta/models/") && r.Method == http.MethodPost:
		s.serveGeminiRequest(w, r)
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

	// Gemini SDK 调用优先
	if route.Backend == "gemini" && s.geminiClient != nil {
		if err := s.serveRequestWithSDK(w, r, route, model, unified, start); err != nil {
			s.log.Error("Gemini SDK call failed",
				logger.LogField{Key: "error", Value: err.Error()},
			)
			s.writeError(w, http.StatusInternalServerError, "SDK call failed")
		}
		return
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
		reqBody, _ = protocolgemini.Convert(unified, model)
	default:
		s.writeError(w, http.StatusBadRequest, "Unknown backend")
		return
	}

	s.logBackendRequest(backendURL, reqBody)

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
	respBody, _ := io.ReadAll(resp.Body)
	s.logBackendResponse(backendURL, respBody, resp.StatusCode)
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

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

	// Gemini SDK 调用优先
	if route.Backend == "gemini" && s.geminiClient != nil {
		if err := s.serveOpenAIWithSDK(w, r, route, model, unified, start); err != nil {
			s.log.Error("Gemini SDK call failed",
				logger.LogField{Key: "error", Value: err.Error()},
			)
			s.writeOpenAIError(w, http.StatusInternalServerError, "SDK call failed")
		}
		return
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
		reqBody, err = protocolgemini.Convert(unified, model)
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

	s.logBackendRequest(backendURL, reqBody)

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

	// 读取后端响应体以便日志记录
	latency := time.Since(start).Milliseconds()
	respBody, _ := io.ReadAll(resp.Body)
	s.logBackendResponse(backendURL, respBody, resp.StatusCode)

	// 重新设置响应体供后续处理
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

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
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
		ToolCallID string `json:"tool_call_id,omitempty"`
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
		Tools       []struct {
			Type     string `json:"type"`
			Function struct {
				Name        string                 `json:"name"`
				Description string                 `json:"description"`
				Parameters  map[string]interface{} `json:"parameters"`
			} `json:"function"`
		} `json:"tools,omitempty"`
		ToolChoice interface{} `json:"tool_choice,omitempty"`
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
		// messages 格式：提取 content 文本，保留 tool_calls 和 tool_call_id
		messages = make([]types.MessageRole, len(rawRequest.Messages))
		for i, msg := range rawRequest.Messages {
			mr := types.MessageRole{
				Role:       msg.Role,
				Content:    extractContent(msg.Content),
				ToolCallID: msg.ToolCallID,
			}
			if len(msg.ToolCalls) > 0 {
				mr.ToolCalls = make([]types.ToolCall, len(msg.ToolCalls))
				for j, tc := range msg.ToolCalls {
					mr.ToolCalls[j] = types.ToolCall{
						ID:   tc.ID,
						Type: tc.Type,
						Function: types.FunctionCall{
							Name:      tc.Function.Name,
							Arguments: tc.Function.Arguments,
						},
					}
				}
			}
			messages[i] = mr
		}
	}

	if len(messages) == 0 {
		s.writeError(w, http.StatusBadRequest, "Either 'prompt' or 'messages' must be provided")
		return
	}

	unified := &types.UnifiedMessage{
		Model:         rawRequest.Model,
		Stream:        rawRequest.Stream,
		Messages:      messages,
		MaxTokens:     rawRequest.MaxTokens,
		Temperature:   rawRequest.Temperature,
		TopP:          rawRequest.TopP,
		StopSequences: rawRequest.Stop,
		ToolChoice:    rawRequest.ToolChoice,
	}

	// 解析工具定义
	if len(rawRequest.Tools) > 0 {
		unified.Tools = make([]types.Tool, len(rawRequest.Tools))
		for i, t := range rawRequest.Tools {
			unified.Tools[i] = types.Tool{
				Type: t.Type,
				Function: types.FunctionDefinition{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			}
		}
	}

	// 复用现有后端处理逻辑
	model := unified.Model
	if model == "" {
		model = getDefaultModel(route.Backend)
	}

	// Gemini SDK 调用优先
	if route.Backend == "gemini" && s.geminiClient != nil {
		if err := s.serveCompletionsWithSDK(w, r, route, model, unified, start); err != nil {
			s.log.Error("Gemini SDK call failed (completions)",
				logger.LogField{Key: "error", Value: err.Error()},
			)
			s.writeError(w, http.StatusInternalServerError, "SDK call failed")
		}
		return
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
		reqBody, err = protocolgemini.Convert(unified, model)
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

	s.logBackendRequest(backendURL, reqBody)

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
	respBody, _ := io.ReadAll(resp.Body)
	s.logBackendResponse(backendURL, respBody, resp.StatusCode)
	resp.Body = io.NopCloser(bytes.NewReader(respBody))
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
		unified, err = protocolgemini.ParseResponse(body, model)
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
		unified, err = protocolgemini.ParseResponse(body, model)
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

// serveOpenAIWithSDK 使用 SDK 处理 OpenAI → Gemini 调用
func (s *Server) serveOpenAIWithSDK(w http.ResponseWriter, r *http.Request, route *router.Route, model string, unified *types.UnifiedMessage, start time.Time) error {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	sdkModel, contents, sysInst, config, tools, err := protocolgemini.UnifiedMessageToSDK(unified)
	if err != nil {
		return err
	}
	if sysInst != nil {
		config.SystemInstruction = sysInst
	}
	_ = tools // tools 已经在 config 中通过 UnifiedMessageToSDK 设置

	s.log.Info("Gemini SDK request started",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: unified.Stream},
	)

	if unified.Stream {
		iter := s.geminiClient.GenerateContentStream(ctx, sdkModel, contents, config)
		s.handleOpenAIStreamFromSDK(w, iter, model, r, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, sdkModel, contents, config)
		if err != nil {
			return err
		}

		// Debug: log SDK 响应体
		if s.debugRequests {
			if restData, err := protocolgemini.SDKResponseToREST(resp); err == nil {
				s.logResponseBody("/v1/chat/completions (sdk)", restData, 200)
			}
		}

		unifiedResp, err := protocolgemini.FromSDKResponse(resp, model)
		if err != nil {
			return err
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini SDK request completed",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
			logger.LogField{Key: "input_tokens", Value: unifiedResp.Usage.InputTokens},
			logger.LogField{Key: "output_tokens", Value: unifiedResp.Usage.OutputTokens},
		)

		respBody, err := openai.BuildResponse(unifiedResp)
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBody)
	}

	return nil
}

// serveRequestWithSDK 使用 SDK 处理 Anthropic 入口 → Gemini 调用
func (s *Server) serveRequestWithSDK(w http.ResponseWriter, r *http.Request, route *router.Route, model string, unified *types.UnifiedMessage, start time.Time) error {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	sdkModel, contents, sysInst, config, tools, err := protocolgemini.UnifiedMessageToSDK(unified)
	if err != nil {
		return err
	}
	if sysInst != nil {
		config.SystemInstruction = sysInst
	}
	_ = tools

	s.log.Info("Gemini SDK request started (Anthropic entry)",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: unified.Stream},
	)

	if unified.Stream {
		iter := s.geminiClient.GenerateContentStream(ctx, sdkModel, contents, config)
		s.handleStreamFromSDK(w, iter, model, r, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, sdkModel, contents, config)
		if err != nil {
			return err
		}

		if s.debugRequests {
			if restData, err := protocolgemini.SDKResponseToREST(resp); err == nil {
				s.logResponseBody("/v1/messages (sdk)", restData, 200)
			}
		}

		unifiedResp, err := protocolgemini.FromSDKResponse(resp, model)
		if err != nil {
			return err
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini SDK request completed",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
			logger.LogField{Key: "input_tokens", Value: unifiedResp.Usage.InputTokens},
			logger.LogField{Key: "output_tokens", Value: unifiedResp.Usage.OutputTokens},
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(unifiedResp)
	}

	return nil
}

// serveCompletionsWithSDK 使用 SDK 处理 Completions → Gemini 调用
func (s *Server) serveCompletionsWithSDK(w http.ResponseWriter, r *http.Request, route *router.Route, model string, unified *types.UnifiedMessage, start time.Time) error {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	sdkModel, contents, sysInst, config, tools, err := protocolgemini.UnifiedMessageToSDK(unified)
	if err != nil {
		return err
	}
	if sysInst != nil {
		config.SystemInstruction = sysInst
	}
	_ = tools

	s.log.Info("Gemini SDK request started (Completions)",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: unified.Stream},
	)

	if unified.Stream {
		iter := s.geminiClient.GenerateContentStream(ctx, sdkModel, contents, config)
		s.handleCompletionsStreamFromSDK(w, iter, model, r, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, sdkModel, contents, config)
		if err != nil {
			return err
		}

		unifiedResp, err := protocolgemini.FromSDKResponse(resp, model)
		if err != nil {
			return err
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini SDK request completed (Completions)",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
			logger.LogField{Key: "input_tokens", Value: unifiedResp.Usage.InputTokens},
			logger.LogField{Key: "output_tokens", Value: unifiedResp.Usage.OutputTokens},
		)

		// 构建 Completions 格式响应
		var text string
		if len(unifiedResp.Content) > 0 {
			text = unifiedResp.Content[0].Text
		}
		completionResp := map[string]interface{}{
			"id":      unifiedResp.ID,
			"object":  "text_completion",
			"created": time.Now().Unix(),
			"model":   unifiedResp.Model,
			"choices": []map[string]interface{}{{
				"text":          text,
				"index":         0,
				"finish_reason": unifiedResp.FinishReason,
			}},
		}
		if unifiedResp.Usage.InputTokens > 0 || unifiedResp.Usage.OutputTokens > 0 {
			completionResp["usage"] = map[string]interface{}{
				"prompt_tokens":     unifiedResp.Usage.InputTokens,
				"completion_tokens": unifiedResp.Usage.OutputTokens,
				"total_tokens":      unifiedResp.Usage.InputTokens + unifiedResp.Usage.OutputTokens,
			}
		}

		respBody, _ := json.Marshal(completionResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(respBody)
	}

	return nil
}

// handleStreamFromSDK 处理 Anthropic 入口流式响应
func (s *Server) handleStreamFromSDK(w http.ResponseWriter, streamFn iter.Seq2[*genai.GenerateContentResponse, error], model string, req *http.Request, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	for response, err := range streamFn {
		select {
		case <-clientDisconnected:
			return
		default:
		}

		if err != nil {
			return
		}

		// 转换为 Anthropic SSE 格式
		if len(response.Candidates) > 0 && response.Candidates[0].Content != nil {
			for _, part := range response.Candidates[0].Content.Parts {
				if part.Text != "" {
					chunk := map[string]interface{}{
						"type": "content_block_delta",
						"delta": map[string]interface{}{
							"type": "text_delta",
							"text": part.Text,
						},
					}
					data, _ := json.Marshal(chunk)
					w.Write([]byte("event: content_block_delta\ndata: "))
					w.Write(data)
					w.Write([]byte("\n\n"))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
			}
		}
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini SDK stream completed (Anthropic entry)",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}

// handleCompletionsStreamFromSDK 处理 Completions 流式响应
func (s *Server) handleCompletionsStreamFromSDK(w http.ResponseWriter, streamFn iter.Seq2[*genai.GenerateContentResponse, error], model string, req *http.Request, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	for response, err := range streamFn {
		select {
		case <-clientDisconnected:
			w.Write([]byte("data: [DONE]\n\n"))
			return
		default:
		}

		if err != nil {
			return
		}

		if len(response.Candidates) > 0 && response.Candidates[0].Content != nil {
			for _, part := range response.Candidates[0].Content.Parts {
				if part.Text != "" {
					chunk := map[string]interface{}{
						"choices": []map[string]interface{}{{
							"text":          part.Text,
							"finish_reason": nil,
						}},
					}
					data, _ := json.Marshal(chunk)
					w.Write(append([]byte("data: "), data...))
					w.Write([]byte("\n\n"))
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
				}
			}
		}
	}

	w.Write([]byte("data: [DONE]\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini SDK stream completed (Completions)",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}

// serveGeminiWithSDK 使用 SDK 处理 Gemini 原生端点
func (s *Server) serveGeminiWithSDK(w http.ResponseWriter, r *http.Request, route *router.Route, body []byte, start time.Time) {
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	// 解析原生请求
	var nativeReq map[string]interface{}
	if err := json.Unmarshal(body, &nativeReq); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// 提取模型名
	pathSuffix := strings.TrimPrefix(r.URL.Path, "/v1beta/models/")
	model := strings.SplitN(pathSuffix, ":", 2)[0]
	model = strings.TrimSuffix(model, "/")

	// 转换为 SDK contents
	contents, config := parseNativeGeminiRequest(nativeReq)

	// 检查是否流式
	isStream := strings.Contains(r.URL.Path, "streamGenerateContent")

	s.log.Info("Gemini native SDK request started",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "stream", Value: isStream},
	)

	if isStream {
		iter := s.geminiClient.GenerateContentStream(ctx, model, contents, config)
		s.serveGeminiStreamFromSDK(w, iter, model, start)
	} else {
		resp, err := s.geminiClient.GenerateContent(ctx, model, contents, config)
		if err != nil {
			s.writeError(w, http.StatusBadGateway, "SDK call failed")
			return
		}

		latency := time.Since(start).Milliseconds()
		s.log.Info("Gemini native SDK request completed",
			logger.LogField{Key: "model", Value: model},
			logger.LogField{Key: "latency_ms", Value: latency},
		)

		// 转换为 REST JSON 返回
		restBody, err := protocolgemini.SDKResponseToREST(resp)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to convert response")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(restBody)
	}
}

// parseNativeGeminiRequest 将原生 Gemini 请求转换为 SDK contents 和 config
func parseNativeGeminiRequest(req map[string]interface{}) ([]*genai.Content, *genai.GenerateContentConfig) {
	var contents []*genai.Content
	config := &genai.GenerateContentConfig{}

	// 解析 contents
	if rawContents, ok := req["contents"].([]interface{}); ok {
		for _, c := range rawContents {
			if cm, ok := c.(map[string]interface{}); ok {
				content := &genai.Content{}
				if role, ok := cm["role"].(string); ok {
					content.Role = role
				}
				if parts, ok := cm["parts"].([]interface{}); ok {
					for _, p := range parts {
						if pm, ok := p.(map[string]interface{}); ok {
							if text, ok := pm["text"].(string); ok {
								content.Parts = append(content.Parts, &genai.Part{Text: text})
							}
						}
					}
				}
				if len(content.Parts) > 0 {
					contents = append(contents, content)
				}
			}
		}
	}

	// 解析 generationConfig
	if genCfg, ok := req["generationConfig"].(map[string]interface{}); ok {
		if temp, ok := genCfg["temperature"].(float64); ok && temp > 0 {
			t := float32(temp)
			config.Temperature = &t
		}
		if topP, ok := genCfg["topP"].(float64); ok && topP > 0 {
			tp := float32(topP)
			config.TopP = &tp
		}
		if maxTokens, ok := genCfg["maxOutputTokens"].(float64); ok && maxTokens > 0 {
			config.MaxOutputTokens = int32(maxTokens)
		}
		if stops, ok := genCfg["stopSequences"].([]interface{}); ok && len(stops) > 0 {
			for _, s := range stops {
				if ss, ok := s.(string); ok {
					config.StopSequences = append(config.StopSequences, ss)
				}
			}
		}
	}

	return contents, config
}

// serveGeminiStreamFromSDK 处理 Gemini 原生端点流式响应
func (s *Server) serveGeminiStreamFromSDK(w http.ResponseWriter, streamFn iter.Seq2[*genai.GenerateContentResponse, error], model string, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for response, err := range streamFn {
		if err != nil {
			return
		}

		restData, err := protocolgemini.SDKResponseToREST(response)
		if err != nil {
			continue
		}

		w.Write([]byte("data: "))
		w.Write(restData)
		w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini native SDK stream completed",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}

// handleOpenAIStreamFromSDK 将 SDK 流式响应转换为 OpenAI SSE 格式
func (s *Server) handleOpenAIStreamFromSDK(w http.ResponseWriter, streamFn iter.Seq2[*genai.GenerateContentResponse, error], model string, req *http.Request, start time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientDisconnected := make(chan struct{})
	go func() {
		select {
		case <-req.Context().Done():
			close(clientDisconnected)
		}
	}()

	for response, err := range streamFn {
		select {
		case <-clientDisconnected:
			w.Write([]byte("data: [DONE]\n\n"))
			return
		default:
		}

		if err != nil {
			return
		}

		chunk := buildOpenAIStreamChunkFromSDK(response)
		if len(chunk) > 0 {
			w.Write(chunk)
			w.Write([]byte("\n\n"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}

	// 发送 [DONE]
	w.Write([]byte("data: [DONE]\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	latency := time.Since(start).Milliseconds()
	s.log.Info("Gemini SDK stream completed",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "latency_ms", Value: latency},
	)
}

func buildOpenAIStreamChunkFromSDK(resp *genai.GenerateContentResponse) []byte {
	if len(resp.Candidates) == 0 {
		return nil
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil
	}

	part := candidate.Content.Parts[0]

	var delta map[string]interface{}

	if part.FunctionCall != nil {
		argsJSON := "{}"
		if part.FunctionCall.Args != nil {
			if b, err := json.Marshal(part.FunctionCall.Args); err == nil {
				argsJSON = string(b)
			}
		}
		delta = map[string]interface{}{
			"role": "assistant",
			"tool_calls": []map[string]interface{}{{
				"index": 0,
				"id":    fmt.Sprintf("call_%s", part.FunctionCall.Name),
				"type":  "function",
				"function": map[string]interface{}{
					"name":      part.FunctionCall.Name,
					"arguments": argsJSON,
				},
			}},
		}
	} else if part.Text != "" {
		delta = map[string]interface{}{
			"role":    "assistant",
			"content": part.Text,
		}
	}

	if delta == nil {
		return nil
	}

	payload := map[string]interface{}{
		"id": "gemini-sdk-stream",
		"object": "chat.completion.chunk",
		"model":  resp.ModelVersion,
		"choices": []map[string]interface{}{{
			"index": 0,
			"delta": delta,
		}},
	}

	result, _ := json.Marshal(payload)
	return append([]byte("data: "), result...)
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

// serveGeminiRequest forwards Gemini native protocol requests directly to Gemini backend.
// Clients use the native Gemini API format - no protocol conversion needed.
func (s *Server) serveGeminiRequest(w http.ResponseWriter, r *http.Request) {
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

	// 只路由到 Gemini 后端
	if route.Backend != "gemini" {
		s.writeError(w, http.StatusBadRequest, "This endpoint only supports Gemini backend")
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	s.logRequestBody(r.URL.Path, body)

	// 使用 SDK 调用（如果可用）
	if s.geminiClient != nil {
		s.serveGeminiWithSDK(w, r, route, body, start)
		return
	}

	// Fallback: 直接转发
	baseURL := strings.TrimSuffix(s.cfg.Backends.Gemini.BaseURL, "/")
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasSuffix(baseURL, "/v1beta") && strings.HasPrefix(path, "v1beta/") {
		path = strings.TrimPrefix(path, "v1beta/")
	}
	backendURL := baseURL + "/" + path
	if r.URL.RawQuery != "" {
		backendURL += "?" + r.URL.RawQuery + "&key=" + route.BackendKey
	} else {
		backendURL += "?key=" + route.BackendKey
	}

	s.logBackendRequest(backendURL, body)

	backendReq, err := http.NewRequest(r.Method, backendURL, bytes.NewReader(body))
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create backend request")
		return
	}

	forwardHeaders := []string{
		"Content-Type",
		"User-Agent",
		"Accept",
		"Accept-Encoding",
		"Accept-Language",
	}
	for _, h := range forwardHeaders {
		if v := r.Header.Values(h); len(v) > 0 {
			backendReq.Header[h] = v
		}
	}
	backendReq.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()
	backendReq = backendReq.WithContext(ctx)

	resp, err := s.client.Do(backendReq)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Failed to reach backend")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	s.logBackendResponse(backendURL, respBody, resp.StatusCode)
	resp.Body = io.NopCloser(bytes.NewReader(respBody))

	latency := time.Since(start).Milliseconds()

	for k, vv := range resp.Header {
		if k != "Content-Length" {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	// 记录日志
	s.log.Info("Gemini native request completed",
		logger.LogField{Key: "status_code", Value: resp.StatusCode},
		logger.LogField{Key: "backend", Value: route.Backend},
		logger.LogField{Key: "latency_ms", Value: latency},
	)
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
		unified, err = protocolgemini.ParseResponse(body, model)
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
					Text         string `json:"text"`
					FunctionCall *struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
	}

	if err := json.Unmarshal(data, &geminiResp); err != nil {
		return nil
	}

	if len(geminiResp.Candidates) == 0 {
		return nil
	}

	candidate := geminiResp.Candidates[0]

	// Handle functionCall part
	if len(candidate.Content.Parts) > 0 {
		part := candidate.Content.Parts[0]
		if part.FunctionCall != nil {
			args, err := json.Marshal(part.FunctionCall.Arguments)
			if err != nil {
				return nil
			}
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"delta": map[string]interface{}{
							"role": "assistant",
							"tool_calls": []map[string]interface{}{
								{
									"index": 0,
									"id":    fmt.Sprintf("call_%s", part.FunctionCall.Name),
									"type":  "function",
									"function": map[string]interface{}{
										"name":      part.FunctionCall.Name,
										"arguments": string(args),
									},
								},
							},
						},
						"finish_reason": nil,
					},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
	}

	// Handle text part
	if len(candidate.Content.Parts) > 0 && candidate.Content.Parts[0].Text != "" {
		text := candidate.Content.Parts[0].Text
		finishReason := candidate.FinishReason

		var openaiFinishReason string
		switch finishReason {
		case "STOP":
			openaiFinishReason = "stop"
		case "MAX_TOKENS":
			openaiFinishReason = "length"
		default:
			openaiFinishReason = "stop"
		}

		if text != "" {
			payload := map[string]interface{}{
				"choices": []map[string]interface{}{
					{"delta": map[string]interface{}{"content": text}, "finish_reason": nil},
				},
			}
			result, _ := json.Marshal(payload)
			return append([]byte("data: "), result...)
		}
		if finishReason != "" {
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
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		OwnedBy string `json:"owned_by"`
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
					ID      string `json:"id"`
					Object  string `json:"object"`
					Created int64  `json:"created"`
					OwnedBy string `json:"owned_by"`
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

// isStandardGeminiAPI 判断是否为标准 Gemini API URL（非测试 mock）
func isStandardGeminiAPI(baseURL string) bool {
	return strings.Contains(baseURL, "generativelanguage.googleapis.com") ||
		strings.Contains(baseURL, "ai.google.dev")
}

// findGeminiBackendKey 从路由配置中查找 Gemini 后端的 API Key
func findGeminiBackendKey(routes []config.RouteConfig) string {
	for _, r := range routes {
		if r.Backend == "gemini" {
			return r.BackendKey
		}
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

// logBackendRequest logs the backend request body if debug_requests is enabled.
func (s *Server) logBackendRequest(url string, body []byte) {
	if !s.debugRequests || len(body) == 0 {
		return
	}
	truncated, total := truncateBody(body, s.debugMaxBody)
	s.log.Debug("Backend request",
		logger.LogField{Key: "url", Value: url},
		logger.LogField{Key: "body", Value: truncated},
		logger.LogField{Key: "total_bytes", Value: total},
	)
}

// logBackendResponse logs the backend response body if debug_requests is enabled.
func (s *Server) logBackendResponse(url string, body []byte, statusCode int) {
	if !s.debugRequests || len(body) == 0 {
		return
	}
	truncated, total := truncateBody(body, s.debugMaxBody)
	s.log.Debug("Backend response",
		logger.LogField{Key: "url", Value: url},
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
