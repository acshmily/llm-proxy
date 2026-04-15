package middleware

import (
	crypto_rand "crypto/rand"
	"encoding/base64"
	"math/big"
	math_rand "math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

// 浏览器模式常量
const (
	BrowserModeChrome   = "chrome"
	BrowserModeFirefox  = "firefox"
	BrowserModeSafari   = "safari"
	BrowserModeRandom   = "random"
)

// 分布模式常量
const (
	DistributionUniform     = "uniform"
	DistributionExponential = "exponential"
)

// 填充模式常量
const (
	PaddingModeRandom = "random"
	PaddingModeFixed  = "fixed"
)

// 指数分布因子（控制更接近最小值的程度）
const exponentialFactor = 3.0

// browserUserAgents 浏览器 User-Agent 列表（私有）
var browserUserAgents = map[string][]string{
	BrowserModeChrome: {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	},
	BrowserModeFirefox: {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
	},
	BrowserModeSafari: {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	},
}

// BrowserHeaders 浏览器特征头部
type BrowserHeaders struct {
	UserAgent       string
	Accept          string
	AcceptLanguage  string
	AcceptEncoding  string
	SecChUa         string
	SecChUaMobile   string
	SecChUaPlatform string
}

// browserHeadersMap 浏览器头部映射（私有）
var browserHeadersMap = map[string]BrowserHeaders{
	BrowserModeChrome: {
		UserAgent:       "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Accept:          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		AcceptLanguage:  "en-US,en;q=0.5",
		AcceptEncoding:  "gzip, deflate, br",
		SecChUa:         `"Not_A Brand";v="8", "Chromium";v="120"`,
		SecChUaMobile:   "?0",
		SecChUaPlatform: `"macOS"`,
	},
	BrowserModeFirefox: {
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0",
		Accept:         "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		AcceptLanguage: "en-US,en;q=0.5",
		AcceptEncoding: "gzip, deflate, br",
	},
	BrowserModeSafari: {
		UserAgent:      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
		Accept:         "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		AcceptLanguage: "en-us",
		AcceptEncoding: "gzip, deflate, br",
	},
}

// ProtectionMiddleware 防护中间件
// 所有方法都是并发安全的
type ProtectionMiddleware struct {
	cfg *config.ProtectionConfig
	mu  sync.Mutex
	rng *math_rand.Rand
}

// NewProtectionMiddleware 创建防护中间件
func NewProtectionMiddleware(cfg *config.ProtectionConfig) *ProtectionMiddleware {
	return &ProtectionMiddleware{
		cfg: cfg,
		rng: math_rand.New(math_rand.NewSource(time.Now().UnixNano())),
	}
}

// IsEnabled 检查防护是否启用
// 并发安全
func (m *ProtectionMiddleware) IsEnabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg != nil && m.cfg.Enabled
}

// applyBrowserHeaders 应用浏览器头部（内部方法，需要持有锁）
func (m *ProtectionMiddleware) applyBrowserHeadersLocked(req *http.Header) BrowserHeaders {
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
	if mode == BrowserModeRandom {
		modes := []string{BrowserModeChrome, BrowserModeFirefox, BrowserModeSafari}
		n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(len(modes))))
		mode = modes[n.Int64()]
	}

	headers, ok := browserHeadersMap[mode]
	if !ok {
		headers = browserHeadersMap[BrowserModeChrome]
	}

	return headers
}

// ApplyBrowserHeaders 应用浏览器头部
// 并发安全
func (m *ProtectionMiddleware) ApplyBrowserHeaders(req *http.Header) {
	if req == nil {
		return
	}

	m.mu.Lock()
	if !m.IsEnabled() || !m.cfg.TrafficCamouflage.BrowserHeaders.Enabled {
		m.mu.Unlock()
		return
	}

	headers := m.applyBrowserHeadersLocked(req)
	m.mu.Unlock()

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

// GetRequestDelay 获取请求延迟时间（毫秒）
// 并发安全
func (m *ProtectionMiddleware) GetRequestDelay() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsEnabled() || !m.cfg.BehaviorJitter.RequestDelay.Enabled {
		return 0
	}

	minMs := m.cfg.BehaviorJitter.RequestDelay.MinMs
	maxMs := m.cfg.BehaviorJitter.RequestDelay.MaxMs

	if minMs >= maxMs {
		return minMs
	}

	// 指数分布：更接近最小值
	if m.cfg.BehaviorJitter.RequestDelay.Distribution == DistributionExponential {
		expValue := m.rng.ExpFloat64() / exponentialFactor
		return minMs + int(expValue*float64(maxMs-minMs))
	}

	// 均匀分布
	n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(maxMs-minMs)))
	return minMs + int(n.Int64())
}

// ShouldReuseConnection 是否应该复用连接
// 并发安全
func (m *ProtectionMiddleware) ShouldReuseConnection() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(1000))
	return float64(n.Int64())/1000 < reuseRate
}

// GetPaddingSize 获取填充数据大小
// 并发安全
func (m *ProtectionMiddleware) GetPaddingSize() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsEnabled() || !m.cfg.BehaviorJitter.RequestPadding.Enabled {
		return 0
	}

	minBytes := m.cfg.BehaviorJitter.RequestPadding.MinBytes
	maxBytes := m.cfg.BehaviorJitter.RequestPadding.MaxBytes

	// 固定大小模式
	if m.cfg.BehaviorJitter.RequestPadding.Mode == PaddingModeFixed {
		if m.cfg.BehaviorJitter.RequestPadding.FixedSize > 0 {
			return m.cfg.BehaviorJitter.RequestPadding.FixedSize
		}
		return minBytes
	}

	// 随机模式
	if minBytes >= maxBytes {
		return minBytes
	}

	n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(maxBytes-minBytes)))
	return minBytes + int(n.Int64())
}

// GeneratePadding 生成填充数据（字节切片）
// 并发安全，使用 crypto/rand 保证线程安全
func (m *ProtectionMiddleware) GeneratePadding(size int) []byte {
	if size <= 0 {
		return []byte{}
	}

	padding := make([]byte, size)
	_, err := crypto_rand.Read(padding)
	if err != nil {
		// 如果 crypto/rand 失败，回退到数学常数填充
		for i := range padding {
			padding[i] = byte(32 + (i % 95))
		}
	}
	return padding
}

// GeneratePaddingString 生成填充数据（字符串）
// 并发安全
func (m *ProtectionMiddleware) GeneratePaddingString(size int) string {
	padding := m.GeneratePadding(size)
	return base64.RawURLEncoding.EncodeToString(padding)
}

// GetBrowserUserAgent 获取浏览器 User-Agent
// 并发安全
func (m *ProtectionMiddleware) GetBrowserUserAgent() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.IsEnabled() || !m.cfg.TrafficCamouflage.BrowserHeaders.Enabled {
		return ""
	}

	// 自定义 UA 优先
	if m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA != "" {
		return m.cfg.TrafficCamouflage.BrowserHeaders.CustomUA
	}

	mode := m.cfg.TrafficCamouflage.BrowserHeaders.Mode
	if mode == BrowserModeRandom {
		modes := []string{BrowserModeChrome, BrowserModeFirefox, BrowserModeSafari}
		n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(len(modes))))
		mode = modes[n.Int64()]
	}

	agents, ok := browserUserAgents[mode]
	if !ok || len(agents) == 0 {
		agents = browserUserAgents[BrowserModeChrome]
	}

	n, _ := crypto_rand.Int(crypto_rand.Reader, big.NewInt(int64(len(agents))))
	return agents[n.Int64()]
}

// GetConfig 获取配置副本（用于日志等）
// 并发安全
func (m *ProtectionMiddleware) GetConfig() *config.ProtectionConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cfg == nil {
		return nil
	}

	// 返回配置的浅拷贝
	cfg := *m.cfg
	return &cfg
}
