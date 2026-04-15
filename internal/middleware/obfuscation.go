package middleware

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

// TrafficObfuscationMiddleware 流量混淆中间件
type TrafficObfuscationMiddleware struct {
	cfg *config.TrafficObfuscationConfig
	mu  sync.Mutex
}

// NewTrafficObfuscationMiddleware 创建流量混淆中间件
func NewTrafficObfuscationMiddleware(cfg *config.TrafficObfuscationConfig) *TrafficObfuscationMiddleware {
	return &TrafficObfuscationMiddleware{
		cfg: cfg,
	}
}

// IsEnabled 检查流量混淆是否启用
func (m *TrafficObfuscationMiddleware) IsEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg != nil && (m.cfg.WebSocketTunnel.Enabled || m.cfg.RequestSharding.Enabled)
}

// ShouldShardRequest 判断是否应该分片请求
func (m *TrafficObfuscationMiddleware) ShouldShardRequest(bodySize int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil || !m.cfg.RequestSharding.Enabled {
		return false
	}

	maxChunkSize := m.cfg.RequestSharding.MaxChunkSize
	if maxChunkSize <= 0 {
		return false
	}

	return bodySize > maxChunkSize
}

// ShardRequest 将请求体分片
// 返回分片后的数据列表，每个元素是一个分片的 base64 编码
func (m *TrafficObfuscationMiddleware) ShardRequest(body []byte) ([]string, error) {
	m.mu.Lock()
	maxChunkSize := m.cfg.RequestSharding.MaxChunkSize
	m.mu.Unlock()

	if maxChunkSize <= 0 {
		maxChunkSize = 1024 // 默认值
	}

	var chunks []string
	for i := 0; i < len(body); i += maxChunkSize {
		end := i + maxChunkSize
		if end > len(body) {
			end = len(body)
		}
		chunk := body[i:end]
		// Base64 编码便于传输
		chunks = append(chunks, base64.StdEncoding.EncodeToString(chunk))
	}

	return chunks, nil
}

// ReassembleChunks 重组分片数据
func (m *TrafficObfuscationMiddleware) ReassembleChunks(chunks []string) ([]byte, error) {
	var buf bytes.Buffer

	for _, chunk := range chunks {
		data, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil {
			return nil, err
		}
		buf.Write(data)
	}

	return buf.Bytes(), nil
}

// WrapShardedRequest 将分片数据包装为 HTTP 请求
func (m *TrafficObfuscationMiddleware) WrapShardedRequest(originalReq *http.Request, chunks []string) (*http.Request, error) {
	// 创建分片请求体
	shardBody := map[string]interface{}{
		"sharded": true,
		"chunks":  chunks,
		"total":   len(chunks),
	}

	bodyBytes, err := json.Marshal(shardBody)
	if err != nil {
		return nil, err
	}

	// 创建新请求
	newReq, err := http.NewRequest(
		originalReq.Method,
		originalReq.URL.String(),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, err
	}

	// 复制头部
	for key, values := range originalReq.Header {
		for _, value := range values {
			newReq.Header.Add(key, value)
		}
	}

	// 添加分片标识
	newReq.Header.Set("X-Request-Sharded", "true")
	newReq.Header.Set("X-Chunk-Count", string(rune(len(chunks))))
	newReq.Header.Set("Content-Type", "application/json")

	return newReq, nil
}

// UnwrapShardedRequest 解包分片请求
func (m *TrafficObfuscationMiddleware) UnwrapShardedRequest(req *http.Request) (*http.Request, []byte, error) {
	if req.Header.Get("X-Request-Sharded") != "true" {
		return nil, nil, nil // 不是分片请求
	}

	// 读取请求体
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, nil, err
	}
	defer req.Body.Close()

	// 解析 JSON
	var shardData map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &shardData); err != nil {
		return nil, nil, err
	}

	// 提取分片数据
	chunksRaw, ok := shardData["chunks"].([]interface{})
	if !ok {
		return nil, nil, nil
	}

	var chunks []string
	for _, c := range chunksRaw {
		if s, ok := c.(string); ok {
			chunks = append(chunks, s)
		}
	}

	// 重组数据
	reassembled, err := m.ReassembleChunks(chunks)
	if err != nil {
		return nil, nil, err
	}

	// 创建新请求（使用重组后的数据）
	newReq, err := http.NewRequest(
		req.Method,
		req.URL.String(),
		bytes.NewReader(reassembled),
	)
	if err != nil {
		return nil, nil, err
	}

	// 复制头部（移除分片相关头部）
	for key, values := range req.Header {
		if key != "X-Request-Sharded" && key != "X-Chunk-Count" {
			for _, value := range values {
				newReq.Header.Add(key, value)
			}
		}
	}

	return newReq, reassembled, nil
}

// IsShardedRequest 检查请求是否是分片请求
func (m *TrafficObfuscationMiddleware) IsShardedRequest(req *http.Request) bool {
	return req.Header.Get("X-Request-Sharded") == "true"
}
