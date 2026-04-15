package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 客户端配置
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Listen  ListenConfig  `yaml:"listen"`
	Logging LoggingConfig `yaml:"logging"`
	Health  HealthConfig  `yaml:"health"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Address        string `yaml:"address"`
	PingIntervalMs int    `yaml:"ping_interval_ms"`
}

// ListenConfig 监听配置
type ListenConfig struct {
	Address string `yaml:"address"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Format string `yaml:"format"`
	Level  string `yaml:"level"`
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Enabled bool   `yaml:"enabled"`
	Address string `yaml:"address"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:        "ws://localhost:8080/ws-tunnel",
			PingIntervalMs: 30000,
		},
		Listen: ListenConfig{
			Address: ":8081",
		},
		Logging: LoggingConfig{
			Format: "text",
			Level:  "info",
		},
		Health: HealthConfig{
			Enabled: true,
			Address: ":8082",
		},
	}
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	// 使用默认配置
	cfg := DefaultConfig()

	// 如果指定了配置文件，加载它
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	}

	// 应用环境变量覆盖
	cfg.applyEnv()

	return cfg, nil
}

// applyEnv 应用环境变量覆盖配置
func (c *Config) applyEnv() {
	// WS_TUNNEL_SERVER 覆盖服务器地址
	if env := os.Getenv("WS_TUNNEL_SERVER"); env != "" {
		c.Server.Address = env
	}

	// WS_TUNNEL_LISTEN 覆盖监听地址
	if env := os.Getenv("WS_TUNNEL_LISTEN"); env != "" {
		c.Listen.Address = env
	}

	// WS_TUNNEL_PING_INTERVAL_MS 覆盖心跳间隔
	if env := os.Getenv("WS_TUNNEL_PING_INTERVAL_MS"); env != "" {
		if interval, err := strconv.Atoi(env); err == nil && interval > 0 {
			c.Server.PingIntervalMs = interval
		}
	}

	// WS_TUNNEL_HEALTH_ENABLED 覆盖健康检查开关
	if env := os.Getenv("WS_TUNNEL_HEALTH_ENABLED"); env != "" {
		c.Health.Enabled = env == "true" || env == "1"
	}

	// WS_TUNNEL_HEALTH_ADDRESS 覆盖健康检查地址
	if env := os.Getenv("WS_TUNNEL_HEALTH_ADDRESS"); env != "" {
		c.Health.Address = env
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证服务器地址
	if c.Server.Address == "" {
		return fmt.Errorf("server address is required")
	}

	// 验证心跳间隔
	if c.Server.PingIntervalMs <= 0 {
		return fmt.Errorf("ping interval must be positive")
	}

	// 验证监听地址
	if c.Listen.Address == "" {
		return fmt.Errorf("listen address is required")
	}

	// 验证日志格式
	validFormats := map[string]bool{"json": true, "text": true}
	if c.Logging.Format != "" && !validFormats[c.Logging.Format] {
		return fmt.Errorf("invalid log format: %s (must be 'json' or 'text')", c.Logging.Format)
	}

	// 验证日志级别
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if c.Logging.Level != "" && !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level: %s (must be 'debug', 'info', 'warn', or 'error')", c.Logging.Level)
	}

	return nil
}

// PingInterval 返回心跳间隔时间
func (c *ServerConfig) PingInterval() time.Duration {
	return time.Duration(c.PingIntervalMs) * time.Millisecond
}
