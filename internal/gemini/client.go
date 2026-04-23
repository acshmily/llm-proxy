package gemini

import (
	"context"
	"iter"
	"net/http"
	"net/url"

	"google.golang.org/genai"

	"github.com/claude-projetc/llm-proxy/internal/logger"
)

// GeminiClient SDK 客户端封装
type GeminiClient struct {
	client  *genai.Client
	log     *logger.Logger
	debug   bool
	maxBody int
}

// NewGeminiClient 创建 Gemini SDK 客户端
func NewGeminiClient(apiKey string, proxy string, log *logger.Logger, debug bool, maxBody int) (*GeminiClient, error) {
	cfg := &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	}

	// 配置代理
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		cfg.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		}
	}

	client, err := genai.NewClient(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	return &GeminiClient{
		client:  client,
		log:     log,
		debug:   debug,
		maxBody: maxBody,
	}, nil
}

// GenerateContent 非流式调用
func (c *GeminiClient) GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	c.log.Debug("Gemini SDK GenerateContent",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "contents_count", Value: len(contents)},
	)

	resp, err := c.client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, err
	}

	c.log.Debug("Gemini SDK response received",
		logger.LogField{Key: "model", Value: model},
	)

	return resp, nil
}

// GenerateContentStream 流式调用，返回 iter.Seq2
func (c *GeminiClient) GenerateContentStream(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) iter.Seq2[*genai.GenerateContentResponse, error] {
	c.log.Debug("Gemini SDK GenerateContentStream",
		logger.LogField{Key: "model", Value: model},
		logger.LogField{Key: "contents_count", Value: len(contents)},
	)

	return c.client.Models.GenerateContentStream(ctx, model, contents, config)
}
