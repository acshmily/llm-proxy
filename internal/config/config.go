package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Logging  LoggingConfig  `yaml:"logging"`
	Routes   []RouteConfig  `yaml:"routes"`
	Backends BackendsConfig `yaml:"backends"`
	Retry    RetryConfig    `yaml:"retry"`
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
