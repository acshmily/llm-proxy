package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/claude-projetc/proxy-gemini-go/internal/config"
	"github.com/claude-projetc/proxy-gemini-go/internal/logger"
	"github.com/claude-projetc/proxy-gemini-go/internal/protocol/anthropic"
	"github.com/claude-projetc/proxy-gemini-go/internal/protocol/openai"
	"github.com/claude-projetc/proxy-gemini-go/internal/protocol/claude"
	"github.com/claude-projetc/proxy-gemini-go/internal/protocol/gemini"
	"github.com/claude-projetc/proxy-gemini-go/internal/router"
	"github.com/claude-projetc/proxy-gemini-go/internal/stream"
	"github.com/claude-projetc/proxy-gemini-go/pkg/types"
)

type Server struct {
	cfg    *config.Config
	router *router.Router
	log    *logger.Logger
	client *http.Client
}

func New(cfg *config.Config, r *router.Router, log *logger.Logger) *Server {
	return &Server{
		cfg:    cfg,
		router: r,
		log:    log,
		client: &http.Client{},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// 设置后端认证
	backendReq.Header.Set("Authorization", "Bearer "+route.BackendKey)
	backendReq.Header.Set("Content-Type", "application/json")

	// 执行请求
	ctx, cancel := context.WithTimeout(r.Context(), route.Timeout)
	defer cancel()

	resp, err := s.client.Do(backendReq.WithContext(ctx))
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "Backend request failed")
		return
	}
	defer resp.Body.Close()

	latency := time.Since(start).Milliseconds()

	// 处理响应
	if unified.Stream {
		s.handleStream(w, resp, route.Backend)
	} else {
		s.handleNonStream(w, resp, route.Backend, latency, start)
	}
}

func (s *Server) handleStream(w http.ResponseWriter, resp *http.Response, backend string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	stream.ParseSSE(resp.Body, func(event string, data []byte) {
		w.Write(data)
		w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})
}

func (s *Server) handleNonStream(w http.ResponseWriter, resp *http.Response, backend string, latency int64, start time.Time) {
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
	s.log.Info("Request completed",
		logger.LogField{Key: "latency_ms", Value: latency},
		logger.LogField{Key: "status_code", Value: resp.StatusCode},
		logger.LogField{Key: "input_tokens", Value: unified.Usage.InputTokens},
		logger.LogField{Key: "output_tokens", Value: unified.Usage.OutputTokens},
		logger.LogField{Key: "backend", Value: backend},
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(unified)
}

func (s *Server) writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.APIError{
		Type:    "error",
		Message: msg,
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
