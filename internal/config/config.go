package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Logging   LoggingConfig   `yaml:"logging"`
	Routes    []RouteConfig   `yaml:"routes"`
	Backends  BackendsConfig  `yaml:"backends"`
	Retry     RetryConfig     `yaml:"retry"`
	Protection ProtectionConfig `yaml:"protection,omitempty"`
}

type ServerConfig struct {
	Listen string `yaml:"listen"`
}

type LoggingConfig struct {
	Format string `yaml:"format"` // "json" or "text"
	Level  string `yaml:"level"`
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
	BaseURL string `yaml:"base_url"`
}

type RetryConfig struct {
	MaxAttempts int   `yaml:"max_attempts"`
	RetryErrors []int `yaml:"retry_errors"`
}

// ProtectionConfig 防护配置
type ProtectionConfig struct {
	Enabled          bool                  `yaml:"enabled"`
	TrafficCamouflage TrafficCamouflageConfig `yaml:"traffic_camouflage,omitempty"`
	BehaviorJitter   BehaviorJitterConfig  `yaml:"behavior_jitter,omitempty"`
	TrafficObfuscation TrafficObfuscationConfig `yaml:"traffic_obfuscation,omitempty"`
	Logging          ProtectionLoggingConfig `yaml:"logging,omitempty"`
}

// TrafficCamouflageConfig 流量伪装配置
type TrafficCamouflageConfig struct {
	TLSFingerprint   TLSFingerprintConfig   `yaml:"tls_fingerprint,omitempty"`
	BrowserHeaders   BrowserHeadersConfig   `yaml:"browser_headers,omitempty"`
	ContentEncoding  ContentEncodingConfig  `yaml:"content_encoding,omitempty"`
}

type TLSFingerprintConfig struct {
	Enabled bool   `yaml:"enabled"`
	Mode    string `yaml:"mode"` // chrome, firefox, safari, random
}

type BrowserHeadersConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Mode       string `yaml:"mode"` // random, chrome, firefox, safari
	CustomUA   string `yaml:"custom_user_agent,omitempty"`
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
	Enabled    bool    `yaml:"enabled"`
	ReuseRate  float64 `yaml:"reuse_rate"` // 0.0 - 1.0
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
	Enabled         bool   `yaml:"enabled"`
	Path            string `yaml:"path"`
	PingIntervalMs  int    `yaml:"ping_interval_ms"`
}

type RequestShardingConfig struct {
	Enabled       bool `yaml:"enabled"`
	MaxChunkSize  int  `yaml:"max_chunk_size"`
}

// ProtectionLoggingConfig 防护日志配置
type ProtectionLoggingConfig struct {
	LogProtectedRequests bool `yaml:"log_protected_requests"`
	LogJitterApplied     bool `yaml:"log_jitter_applied"`
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
	return &cfg, nil
}

