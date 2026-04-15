package middleware

import (
	"math/rand"
	"net/http"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

// ProtectionMiddleware 防护中间件
type ProtectionMiddleware struct {
	cfg *config.ProtectionConfig
	rng *rand.Rand
}

// BrowserUserAgents 浏览器 User-Agent 列表
var BrowserUserAgents = map[string][]string{
	"chrome": {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
	"firefox": {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
	},
	"safari": {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	},
}

// BrowserHeaders 浏览器特征头部
type BrowserHeaders struct {
	UserAgent         string
	Accept            string
	AcceptLanguage    string
	AcceptEncoding    string
	SecChUa           string
	SecChUaMobile     string
	SecChUaPlatform   string
	SecFetchDest      string
	SecFetchMode      string
	SecFetchSite      string
}

var browserHeadersMap = map[string]BrowserHeaders{
	"chrome": {
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:            "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		AcceptLanguage:    "en-US,en;q=0.5",
		AcceptEncoding:    "gzip, deflate, br",
		SecChUa:           `"Not_A Brand";v="8", "Chromium";v="120"`,
		SecChUaMobile:     "?0",
		SecChUaPlatform:   `"macOS"`,
		SecFetchDest:      "document",
		SecFetchMode:      "navigate",
		SecFetchSite:      "none",
	},
	"firefox": {
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
		Accept:            "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		AcceptLanguage:    "en-US,en;q=0.5",
		AcceptEncoding:    "gzip, deflate, br",
		SecChUa:           "",
		SecChUaMobile:     "",
		SecChUaPlatform:   "",
		SecFetchDest:      "document",
		SecFetchMode:      "navigate",
		SecFetchSite:      "none",
	},
	"safari": {
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		Accept:            "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage:    "en-us",
		AcceptEncoding:    "gzip, deflate, br",
		SecChUa:           "",
		SecChUaMobile:     "",
		SecChUaPlatform:   "",
		SecFetchDest:      "",
		SecFetchMode:      "",
		SecFetchSite:      "",
	},
}

// NewProtectionMiddleware 创建防护中间件
func NewProtectionMiddleware(cfg *config.ProtectionConfig) *ProtectionMiddleware {
	return &ProtectionMiddleware{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// IsEnabled 检查防护是否启用
func (m *ProtectionMiddleware) IsEnabled() bool {
	return m.cfg != nil && m.cfg.Enabled
}

// ApplyBrowserHeaders 应用浏览器头部
func (m *ProtectionMiddleware) ApplyBrowserHeaders(req *http.Header) {
	if !m.IsEnabled() || !m.cfg.TrafficCamouflage.BrowserHeaders.Enabled {
		return
	}

	headers := m.getBrowserHeaders()

	req.Set("User-Agent", headers.UserAgent)
	req.Set("Accept", headers.Accept)
	req.Set("Accept-Language", headers.AcceptLanguage)
	req.Set("Accept-Encoding", headers.AcceptEncoding)

	if headers.SecChUa != "" {
		req.Set("Sec-Ch-Ua", headers.SecChUa)
		req.Set("Sec-Ch-Ua-Mobile", headers.SecChUaMobile)
		req.Set("Sec-Ch-Ua-Platform", headers.SecChUaPlatform)
	}
}

// getBrowserHeaders 获取浏览器头部
func (m *ProtectionMiddleware) getBrowserHeaders() BrowserHeaders {
	mode := m.cfg.TrafficCamouflage.BrowserHeaders.Mode

	// 自定义 User-Agent
	if m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA != "" {
		return BrowserHeaders{
			UserAgent:      m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA,
			Accept:         "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			AcceptLanguage: "en-US,en;q=0.9",
			AcceptEncoding: "gzip, deflate, br",
		}
	}

	// 随机选择浏览器
	if mode == "random" {
		modes := []string{"chrome", "firefox", "safari"}
		mode = modes[m.rng.Intn(len(modes))]
	}

	headers, ok := browserHeadersMap[mode]
	if !ok {
		headers = browserHeadersMap["chrome"]
	}

	return headers
}

// GetRequestDelay 获取请求延迟时间（毫秒）
func (m *ProtectionMiddleware) GetRequestDelay() int {
	if !m.IsEnabled() || !m.cfg.BehaviorJitter.RequestDelay.Enabled {
		return 0
	}

	minMs := m.cfg.BehaviorJitter.RequestDelay.MinMs
	maxMs := m.cfg.BehaviorJitter.RequestDelay.MaxMs

	if minMs >= maxMs {
		return minMs
	}

	// 均匀分布
	if m.cfg.BehaviorJitter.RequestDelay.Distribution == "exponential" {
		// 指数分布：更接近最小值
		return minMs + int(rand.ExpFloat64()*float64(maxMs-minMs)/3)
	}

	// 均匀分布
	return minMs + m.rng.Intn(maxMs-minMs)
}

// ShouldReuseConnection 是否应该复用连接
func (m *ProtectionMiddleware) ShouldReuseConnection() bool {
	if !m.IsEnabled() || !m.cfg.BehaviorJitter.ConnectionReuse.Enabled {
		return true
	}

	reuseRate := m.cfg.BehaviorJitter.ConnectionReuse.ReuseRate
	if reuseRate <= 0 {
		return false
	}
	if reuseRate >= 1 {
		return true
	}

	return m.rng.Float64() < reuseRate
}

// GetPaddingSize 获取填充数据大小
func (m *ProtectionMiddleware) GetPaddingSize() int {
	if !m.IsEnabled() || !m.cfg.BehaviorJitter.RequestPadding.Enabled {
		return 0
	}

	minBytes := m.cfg.BehaviorJitter.RequestPadding.MinBytes
	maxBytes := m.cfg.BehaviorJitter.RequestPadding.MaxBytes

	// 固定大小模式
	if m.cfg.BehaviorJitter.RequestPadding.Mode == "fixed" {
		if m.cfg.BehaviorJitter.RequestPadding.FixedSize > 0 {
			return m.cfg.BehaviorJitter.RequestPadding.FixedSize
		}
		return minBytes
	}

	// 随机模式
	if minBytes >= maxBytes {
		return minBytes
	}
	return minBytes + m.rng.Intn(maxBytes-minBytes)
}

// GeneratePadding 生成填充数据
func (m *ProtectionMiddleware) GeneratePadding(size int) string {
	if size <= 0 {
		return ""
	}

	// 生成随机 Base64 字符串作为填充
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	padding := make([]byte, size)
	for i := 0; i < size; i++ {
		padding[i] = chars[m.rng.Intn(len(chars))]
	}
	return string(padding)
}

// GetBrowserUserAgent 获取浏览器 User-Agent
func (m *ProtectionMiddleware) GetBrowserUserAgent() string {
	if !m.IsEnabled() || !m.cfg.TrafficCamouflage.BrowserHeaders.Enabled {
		return ""
	}

	// 自定义 UA 优先
	if m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA != "" {
		return m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA
	}

	mode := m.cfg.TrafficCamouflage.BrowserHeaders.Mode
	if mode == "random" {
		modes := []string{"chrome", "firefox", "safari"}
		mode = modes[m.rng.Intn(len(modes))]
	}

	agents, ok := BrowserUserAgents[mode]
	if !ok || len(agents) == 0 {
		agents = BrowserUserAgents["chrome"]
	}

	return agents[m.rng.Intn(len(agents))]
}
