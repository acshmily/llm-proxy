package config

import (
	"os"
	"testing"
	"time"
)

func TestConfig_Validate_ServerConfig(t *testing.T) {
	t.Run("fails when server listen is empty", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ""},
			Routes: []RouteConfig{
				{APIKey: "sk-test", Backend: "openai", BackendKey: "sk-backend"},
			},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty server listen, got nil")
		}
		if err != nil && err.Error() != "server.listen is required" {
			t.Errorf("expected 'server.listen is required', got %v", err)
		}
	})
}

func TestConfig_Validate_Routes(t *testing.T) {
	t.Run("fails when no routes defined", func(t *testing.T) {
		cfg := &Config{
			Server:   ServerConfig{Listen: ":8080"},
			Routes:   []RouteConfig{},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty routes, got nil")
		}
		if err != nil && err.Error() != "at least one route is required" {
			t.Errorf("expected 'at least one route is required', got %v", err)
		}
	})

	t.Run("fails when route api_key is empty", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8080"},
			Routes: []RouteConfig{
				{APIKey: "", Backend: "openai", BackendKey: "sk-backend"},
			},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty api_key, got nil")
		}
	})

	t.Run("fails when route backend is empty", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8080"},
			Routes: []RouteConfig{
				{APIKey: "sk-test", Backend: "", BackendKey: "sk-backend"},
			},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty backend, got nil")
		}
	})

	t.Run("fails when route backend_api_key is empty", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8080"},
			Routes: []RouteConfig{
				{APIKey: "sk-test", Backend: "openai", BackendKey: ""},
			},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty backend_api_key, got nil")
		}
	})
}

func TestConfig_Validate_Backends(t *testing.T) {
	t.Run("fails when no backend base_url defined", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8080"},
			Routes: []RouteConfig{
				{APIKey: "sk-test", Backend: "openai", BackendKey: "sk-backend"},
			},
			Backends: BackendsConfig{},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for empty backend base_url, got nil")
		}
	})
}

func TestConfig_Validate_Protection(t *testing.T) {
	t.Run("passes with valid protection config", func(t *testing.T) {
		cfg := &Config{
			Server: ServerConfig{Listen: ":8080"},
			Routes: []RouteConfig{
				{APIKey: "sk-test", Backend: "openai", BackendKey: "sk-backend"},
			},
			Backends: BackendsConfig{
				OpenAI: BackendConfig{BaseURL: "https://api.openai.com/v1"},
			},
			Protection: ProtectionConfig{
				Enabled: true,
				TrafficCamouflage: TrafficCamouflageConfig{
					BrowserHeaders: BrowserHeadersConfig{
						Enabled: true,
						Mode:    BrowserModeChrome,
					},
				},
				BehaviorJitter: BehaviorJitterConfig{
					RequestDelay: RequestDelayConfig{
						Enabled:      true,
						MinMs:        50,
						MaxMs:        200,
						Distribution: DistributionUniform,
					},
					ConnectionReuse: ConnectionReuseConfig{
						Enabled:   true,
						ReuseRate: 0.7,
					},
					RequestPadding: RequestPaddingConfig{
						Enabled:  true,
						MinBytes: 10,
						MaxBytes: 100,
						Mode:     PaddingModeRandom,
					},
				},
			},
		}

		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestProtectionConfig_Validate_TLSFingerprint(t *testing.T) {
	t.Run("fails with invalid tls mode", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: TrafficCamouflageConfig{
				TLSFingerprint: TLSFingerprintConfig{
					Enabled: true,
					Mode:    "invalid-mode",
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for invalid tls mode, got nil")
		}
	})

	t.Run("passes with valid tls modes", func(t *testing.T) {
		validModes := []string{TLSModeChrome, TLSModeFirefox, TLSModeSafari, TLSModeRandom, ""}
		for _, mode := range validModes {
			cfg := &ProtectionConfig{
				Enabled: true,
				TrafficCamouflage: TrafficCamouflageConfig{
					TLSFingerprint: TLSFingerprintConfig{
						Enabled: true,
						Mode:    mode,
					},
				},
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for mode %s, got %v", mode, err)
			}
		}
	})
}

func TestProtectionConfig_Validate_BrowserHeaders(t *testing.T) {
	t.Run("fails with invalid browser mode", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: TrafficCamouflageConfig{
				BrowserHeaders: BrowserHeadersConfig{
					Enabled: true,
					Mode:    "invalid-mode",
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for invalid browser mode, got nil")
		}
	})

	t.Run("passes with valid browser modes", func(t *testing.T) {
		validModes := []string{BrowserModeChrome, BrowserModeFirefox, BrowserModeSafari, BrowserModeRandom, ""}
		for _, mode := range validModes {
			cfg := &ProtectionConfig{
				Enabled: true,
				TrafficCamouflage: TrafficCamouflageConfig{
					BrowserHeaders: BrowserHeadersConfig{
						Enabled: true,
						Mode:    mode,
					},
				},
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for mode %s, got %v", mode, err)
			}
		}
	})
}

func TestProtectionConfig_Validate_RequestDelay(t *testing.T) {
	t.Run("fails when min_ms is negative", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				RequestDelay: RequestDelayConfig{
					Enabled: true,
					MinMs:   -1,
					MaxMs:   200,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for negative min_ms, got nil")
		}
	})

	t.Run("fails when max_ms is less than min_ms", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				RequestDelay: RequestDelayConfig{
					Enabled: true,
					MinMs:   200,
					MaxMs:   50,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for max_ms < min_ms, got nil")
		}
	})

	t.Run("fails with invalid distribution", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				RequestDelay: RequestDelayConfig{
					Enabled:      true,
					MinMs:        50,
					MaxMs:        200,
					Distribution: "invalid",
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for invalid distribution, got nil")
		}
	})

	t.Run("passes with valid distribution", func(t *testing.T) {
		validDist := []string{DistributionUniform, DistributionExponential, ""}
		for _, dist := range validDist {
			cfg := &ProtectionConfig{
				Enabled: true,
				BehaviorJitter: BehaviorJitterConfig{
					RequestDelay: RequestDelayConfig{
						Enabled:      true,
						MinMs:        50,
						MaxMs:        200,
						Distribution: dist,
					},
				},
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for distribution %s, got %v", dist, err)
			}
		}
	})
}

func TestProtectionConfig_Validate_ConnectionReuse(t *testing.T) {
	t.Run("fails when reuse_rate is negative", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				ConnectionReuse: ConnectionReuseConfig{
					Enabled:   true,
					ReuseRate: -0.1,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for negative reuse_rate, got nil")
		}
	})

	t.Run("fails when reuse_rate is greater than 1", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				ConnectionReuse: ConnectionReuseConfig{
					Enabled:   true,
					ReuseRate: 1.1,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for reuse_rate > 1, got nil")
		}
	})

	t.Run("passes with valid reuse_rate", func(t *testing.T) {
		validRates := []float64{0.0, 0.5, 0.7, 1.0}
		for _, rate := range validRates {
			cfg := &ProtectionConfig{
				Enabled: true,
				BehaviorJitter: BehaviorJitterConfig{
					ConnectionReuse: ConnectionReuseConfig{
						Enabled:   true,
						ReuseRate: rate,
					},
				},
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected no error for rate %f, got %v", rate, err)
			}
		}
	})
}

func TestProtectionConfig_Validate_RequestPadding(t *testing.T) {
	t.Run("fails when min_bytes is negative", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				RequestPadding: RequestPaddingConfig{
					Enabled:  true,
					MinBytes: -1,
					MaxBytes: 100,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for negative min_bytes, got nil")
		}
	})

	t.Run("fails when max_bytes is less than min_bytes", func(t *testing.T) {
		cfg := &ProtectionConfig{
			Enabled: true,
			BehaviorJitter: BehaviorJitterConfig{
				RequestPadding: RequestPaddingConfig{
					Enabled:  true,
					MinBytes: 100,
					MaxBytes: 50,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for max_bytes < min_bytes, got nil")
		}
	})
}

func TestLoadConfig(t *testing.T) {
	t.Run("loads valid config file", func(t *testing.T) {
		content := `
server:
  listen: :8080
logging:
  format: json
  level: info
routes:
  - api_key: "sk-test"
    backend: "openai"
    backend_api_key: "sk-backend"
    timeout: 60s
backends:
  openai:
    base_url: "https://api.openai.com/v1"
`
		tmpFile, err := os.CreateTemp("", "config-*.yaml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		cfg, err := LoadConfig(tmpFile.Name())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if cfg == nil {
			t.Error("expected config, got nil")
		}
		if cfg.Server.Listen != ":8080" {
			t.Errorf("expected listen :8080, got %s", cfg.Server.Listen)
		}
	})

	t.Run("fails when config file does not exist", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file, got nil")
		}
	})

	t.Run("fails with invalid yaml", func(t *testing.T) {
		content := `
server:
  listen: :8080
routes:
  - invalid yaml here
`
		tmpFile, err := os.CreateTemp("", "config-*.yaml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		_, err = LoadConfig(tmpFile.Name())
		if err == nil {
			t.Error("expected error for invalid yaml, got nil")
		}
	})
}

func TestConfig_TimeoutParsing(t *testing.T) {
	t.Run("parses duration correctly", func(t *testing.T) {
		content := `
server:
  listen: :8080
routes:
  - api_key: "sk-test"
    backend: "openai"
    backend_api_key: "sk-backend"
    timeout: 120s
backends:
  openai:
    base_url: "https://api.openai.com/v1"
`
		tmpFile, err := os.CreateTemp("", "config-*.yaml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		cfg, err := LoadConfig(tmpFile.Name())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expected := 120 * time.Second
		if cfg.Routes[0].Timeout != expected {
			t.Errorf("expected timeout %v, got %v", expected, cfg.Routes[0].Timeout)
		}
	})
}

func TestConfig_ProtectionDefaults(t *testing.T) {
	t.Run("protection config is optional", func(t *testing.T) {
		content := `
server:
  listen: :8080
routes:
  - api_key: "sk-test"
    backend: "openai"
    backend_api_key: "sk-backend"
    timeout: 60s
backends:
  openai:
    base_url: "https://api.openai.com/v1"
`
		tmpFile, err := os.CreateTemp("", "config-*.yaml")
		if err != nil {
			t.Fatalf("failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.WriteString(content); err != nil {
			t.Fatalf("failed to write to temp file: %v", err)
		}
		tmpFile.Close()

		cfg, err := LoadConfig(tmpFile.Name())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Protection config should exist but disabled
		if cfg.Protection.Enabled {
			t.Error("expected protection disabled by default")
		}
	})
}
