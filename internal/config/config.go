package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
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

// TLS 指纹模式常量
const (
	TLSModeChrome   = "chrome"
	TLSModeFirefox  = "firefox"
	TLSModeSafari   = "safari"
	TLSModeRandom   = "random"
)

// Config 配置结构
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Logging    LoggingConfig    `yaml:"logging"`
	Routes     []RouteConfig    `yaml:"routes"`
	Backends   BackendsConfig   `yaml:"backends"`
	Retry      RetryConfig      `yaml:"retry"`
	Protection ProtectionConfig `yaml:"protection,omitempty"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
}

type LoggingConfig struct {
	Format        string `yaml:"format"` // "json" or "text"
	Level         string `yaml:"level"`
	DebugRequests bool   `yaml:"debug_requests"`    // 开启请求/响应体日志
	DebugMaxBody  int    `yaml:"debug_max_body"`    // 截断长度（字节），默认 2048
}

type RouteConfig struct {
	APIKey       string        `yaml:"api_key"`
	Backend      string        `yaml:"backend"`
	BackendKey   string        `yaml:"backend_api_key"`
	Timeout      time.Duration `yaml:"timeout"`
}

type BackendsConfig struct {
	OpenAI    BackendConfig `yaml:"openai"`
	Anthropic BackendConfig `yaml:"anthropic"`
	Gemini    BackendConfig `yaml:"gemini"`
}

type BackendConfig struct {
	BaseURL   string `yaml:"base_url"`
	HttpProxy string `yaml:"http_proxy,omitempty"`
}

type RetryConfig struct {
	MaxAttempts int   `yaml:"max_attempts"`
	RetryErrors []int `yaml:"retry_errors"`
}

// ProtectionConfig 防护配置
type ProtectionConfig struct {
	Enabled            bool                     `yaml:"enabled"`
	TrafficCamouflage  TrafficCamouflageConfig  `yaml:"traffic_camouflage,omitempty"`
	BehaviorJitter     BehaviorJitterConfig     `yaml:"behavior_jitter,omitempty"`
	TrafficObfuscation TrafficObfuscationConfig `yaml:"traffic_obfuscation,omitempty"`
	Logging            ProtectionLoggingConfig  `yaml:"logging,omitempty"`
}

// TrafficCamouflageConfig 流量伪装配置
type TrafficCamouflageConfig struct {
	TLSFingerprint  TLSFingerprintConfig  `yaml:"tls_fingerprint,omitempty"`
	BrowserHeaders  BrowserHeadersConfig  `yaml:"browser_headers,omitempty"`
	ContentEncoding ContentEncodingConfig `yaml:"content_encoding,omitempty"`
}

type TLSFingerprintConfig struct {
	Enabled bool   `yaml:"enabled"`
	Mode    string `yaml:"mode"` // chrome, firefox, safari, random
}

type BrowserHeadersConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Mode      string `yaml:"mode"` // random, chrome, firefox, safari
	CustomUA  string `yaml:"custom_user_agent,omitempty"`
}

type ContentEncodingConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Algorithms []string `yaml:"algorithms"` // gzip, deflate, br
}

// BehaviorJitterConfig 行为打散配置
type BehaviorJitterConfig struct {
	RequestDelay    RequestDelayConfig    `yaml:"request_delay,omitempty"`
	ConnectionReuse ConnectionReuseConfig `yaml:"connection_reuse,omitempty"`
	RequestPadding  RequestPaddingConfig  `yaml:"request_padding,omitempty"`
}

type RequestDelayConfig struct {
	Enabled      bool   `yaml:"enabled"`
	MinMs        int    `yaml:"min_ms"`
	MaxMs        int    `yaml:"max_ms"`
	Distribution string `yaml:"distribution"` // uniform, exponential
}

type ConnectionReuseConfig struct {
	Enabled   bool    `yaml:"enabled"`
	ReuseRate float64 `yaml:"reuse_rate"` // 0.0 - 1.0
}

type RequestPaddingConfig struct {
	Enabled   bool   `yaml:"enabled"`
	MinBytes  int    `yaml:"min_bytes"`
	MaxBytes  int    `yaml:"max_bytes"`
	Mode      string `yaml:"mode"` // random, fixed
	FixedSize int    `yaml:"fixed_size,omitempty"`
}

// TrafficObfuscationConfig 流量混淆配置
type TrafficObfuscationConfig struct {
	WebSocketTunnel WebSocketTunnelConfig `yaml:"websocket_tunnel,omitempty"`
	RequestSharding RequestShardingConfig `yaml:"request_sharding,omitempty"`
}

type WebSocketTunnelConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Path           string `yaml:"path"`
	PingIntervalMs int    `yaml:"ping_interval_ms"`
}

type RequestShardingConfig struct {
	Enabled      bool `yaml:"enabled"`
	MaxChunkSize int  `yaml:"max_chunk_size"`
}

// ProtectionLoggingConfig 防护日志配置
type ProtectionLoggingConfig struct {
	LogProtectedRequests bool `yaml:"log_protected_requests"`
	LogJitterApplied     bool `yaml:"log_jitter_applied"`
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	// 服务器配置验证
	if c.Server.Listen == "" {
		return errors.New("server.listen is required")
	}

	// 路由配置验证
	if len(c.Routes) == 0 {
		return errors.New("at least one route is required")
	}
	for i, route := range c.Routes {
		if route.APIKey == "" {
			return fmt.Errorf("routes[%d].api_key is required", i)
		}
		if route.Backend == "" {
			return fmt.Errorf("routes[%d].backend is required", i)
		}
		if route.BackendKey == "" {
			return fmt.Errorf("routes[%d].backend_api_key is required or use environment variable", i)
		}
	}

	// 后端配置验证
	if c.Backends.OpenAI.BaseURL == "" && c.Backends.Anthropic.BaseURL == "" && c.Backends.Gemini.BaseURL == "" {
		return errors.New("at least one backend base_url is required")
	}

	// 防护配置验证
	if err := c.Protection.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate 验证防护配置有效性
func (p *ProtectionConfig) Validate() error {
	if !p.Enabled {
		return nil
	}

	// 验证 TLS 指纹配置
	if p.TrafficCamouflage.TLSFingerprint.Enabled {
		mode := p.TrafficCamouflage.TLSFingerprint.Mode
		if mode != "" && mode != TLSModeChrome && mode != TLSModeFirefox && mode != TLSModeSafari && mode != TLSModeRandom {
			return errors.New("protection.traffic_camouflage.tls_fingerprint.mode must be one of: chrome, firefox, safari, random")
		}
	}

	// 验证浏览器头部配置
	if p.TrafficCamouflage.BrowserHeaders.Enabled {
		mode := p.TrafficCamouflage.BrowserHeaders.Mode
		if mode != "" && mode != BrowserModeChrome && mode != BrowserModeFirefox && mode != BrowserModeSafari && mode != BrowserModeRandom {
			return errors.New("protection.traffic_camouflage.browser_headers.mode must be one of: chrome, firefox, safari, random")
		}
	}

	// 验证请求延迟配置
	if p.BehaviorJitter.RequestDelay.Enabled {
		if p.BehaviorJitter.RequestDelay.MinMs < 0 {
			return errors.New("protection.behavior_jitter.request_delay.min_ms must be >= 0")
		}
		if p.BehaviorJitter.RequestDelay.MaxMs < p.BehaviorJitter.RequestDelay.MinMs {
			return errors.New("protection.behavior_jitter.request_delay.max_ms must be >= min_ms")
		}
		dist := p.BehaviorJitter.RequestDelay.Distribution
		if dist != "" && dist != DistributionUniform && dist != DistributionExponential {
			return errors.New("protection.behavior_jitter.request_delay.distribution must be one of: uniform, exponential")
		}
	}

	// 验证连接复用配置
	if p.BehaviorJitter.ConnectionReuse.Enabled {
		rate := p.BehaviorJitter.ConnectionReuse.ReuseRate
		if rate < 0 || rate > 1 {
			return errors.New("protection.behavior_jitter.connection_reuse.reuse_rate must be between 0.0 and 1.0")
		}
	}

	// 验证请求填充配置
	if p.BehaviorJitter.RequestPadding.Enabled {
		if p.BehaviorJitter.RequestPadding.MinBytes < 0 {
			return errors.New("protection.behavior_jitter.request_padding.min_bytes must be >= 0")
		}
		if p.BehaviorJitter.RequestPadding.MaxBytes < p.BehaviorJitter.RequestPadding.MinBytes {
			return errors.New("protection.behavior_jitter.request_padding.max_bytes must be >= min_bytes")
		}
	}

	return nil
}

// LoadConfig 加载配置文件
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

