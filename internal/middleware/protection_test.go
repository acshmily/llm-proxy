package middleware

import (
	"net/http"
	"strings"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

func TestProtectionMiddleware_IsEnabled(t *testing.T) {
	t.Run("returns false when protection is disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		result := middleware.IsEnabled()

		// Assert
		if result != false {
			t.Errorf("expected false, got %v", result)
		}
	})

	t.Run("returns true when protection is enabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		result := middleware.IsEnabled()

		// Assert
		if result != true {
			t.Errorf("expected true, got %v", result)
		}
	})

	t.Run("returns false when cfg is nil", func(t *testing.T) {
		// Arrange
		middleware := NewProtectionMiddleware(nil)

		// Act
		result := middleware.IsEnabled()

		// Assert
		if result != false {
			t.Errorf("expected false for nil cfg, got %v", result)
		}
	})
}

func TestProtectionMiddleware_ApplyBrowserHeaders(t *testing.T) {
	t.Run("does not modify headers when disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)
		headers.Set("User-Agent", "original")

		// Act
		middleware.ApplyBrowserHeaders(&headers)

		// Assert
		if headers.Get("User-Agent") != "original" {
			t.Errorf("expected 'original', got %v", headers.Get("User-Agent"))
		}
	})

	t.Run("applies Chrome headers when enabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    "chrome",
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)

		// Act
		middleware.ApplyBrowserHeaders(&headers)

		// Assert
		ua := headers.Get("User-Agent")
		if ua == "" {
			t.Error("expected User-Agent to be set")
		}
		if ua != "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36" {
			t.Errorf("expected Chrome UA, got %v", ua)
		}
	})

	t.Run("uses custom User-Agent when provided", func(t *testing.T) {
		// Arrange
		customUA := "CustomBot/1.0"
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled:   true,
					CustomUA:  customUA,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)

		// Act
		middleware.ApplyBrowserHeaders(&headers)

		// Assert
		ua := headers.Get("User-Agent")
		if ua != customUA {
			t.Errorf("expected %s, got %s", customUA, ua)
		}
	})

	t.Run("handles nil header gracefully", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act - should not panic
		middleware.ApplyBrowserHeaders(nil)

		// Assert - test passes if no panic
	})
}

func TestProtectionMiddleware_GetRequestDelay(t *testing.T) {
	t.Run("returns 0 when disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		delay := middleware.GetRequestDelay()

		// Assert
		if delay != 0 {
			t.Errorf("expected 0, got %d", delay)
		}
	})

	t.Run("returns delay in range when enabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			BehaviorJitter: config.BehaviorJitterConfig{
				RequestDelay: config.RequestDelayConfig{
					Enabled: true,
					MinMs:   50,
					MaxMs:   200,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act & Assert - run multiple times to check range
		for i := 0; i < 100; i++ {
			delay := middleware.GetRequestDelay()
			if delay < 50 || delay > 200 {
				t.Errorf("delay %d out of range [50, 200]", delay)
			}
		}
	})
}

func TestProtectionMiddleware_ShouldReuseConnection(t *testing.T) {
	t.Run("returns true when disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		result := middleware.ShouldReuseConnection()

		// Assert
		if result != true {
			t.Errorf("expected true, got %v", result)
		}
	})

	t.Run("respects reuse rate", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			BehaviorJitter: config.BehaviorJitterConfig{
				ConnectionReuse: config.ConnectionReuseConfig{
					Enabled:   true,
					ReuseRate: 0.5, // 50%
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act & Assert - should be roughly 50%
		reuseCount := 0
		iterations := 1000
		for i := 0; i < iterations; i++ {
			if middleware.ShouldReuseConnection() {
				reuseCount++
			}
		}

		// Allow 10% variance
		rate := float64(reuseCount) / float64(iterations)
		if rate < 0.4 || rate > 0.6 {
			t.Errorf("reuse rate %.2f outside expected range [0.4, 0.6]", rate)
		}
	})
}

func TestProtectionMiddleware_GetPaddingSize(t *testing.T) {
	t.Run("returns 0 when disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		size := middleware.GetPaddingSize()

		// Assert
		if size != 0 {
			t.Errorf("expected 0, got %d", size)
		}
	})

	t.Run("returns size in range when enabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			BehaviorJitter: config.BehaviorJitterConfig{
				RequestPadding: config.RequestPaddingConfig{
					Enabled:  true,
					MinBytes: 10,
					MaxBytes: 100,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act & Assert
		for i := 0; i < 100; i++ {
			size := middleware.GetPaddingSize()
			if size < 10 || size > 100 {
				t.Errorf("size %d out of range [10, 100]", size)
			}
		}
	})

	t.Run("returns fixed size when mode is fixed", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: true,
			BehaviorJitter: config.BehaviorJitterConfig{
				RequestPadding: config.RequestPaddingConfig{
					Enabled:   true,
					Mode:      "fixed",
					FixedSize: 64,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act & Assert
		for i := 0; i < 10; i++ {
			size := middleware.GetPaddingSize()
			if size != 64 {
				t.Errorf("expected fixed size 64, got %d", size)
			}
		}
	})
}

func TestProtectionMiddleware_GeneratePadding(t *testing.T) {
	t.Run("returns empty slice for size 0", func(t *testing.T) {
		// Arrange
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		// Act
		padding := middleware.GeneratePadding(0)

		// Assert
		if len(padding) != 0 {
			t.Errorf("expected empty slice, got length %d", len(padding))
		}
	})

	t.Run("returns correct size padding", func(t *testing.T) {
		// Arrange
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		// Act
		padding := middleware.GeneratePadding(100)

		// Assert
		if len(padding) != 100 {
			t.Errorf("expected length 100, got %d", len(padding))
		}
	})

	t.Run("generates different padding each time", func(t *testing.T) {
		// Arrange
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		// Act
		padding1 := middleware.GeneratePadding(50)
		padding2 := middleware.GeneratePadding(50)

		// Assert
		for i := 0; i < len(padding1); i++ {
			if padding1[i] != padding2[i] {
				return // At least one byte different, test passes
			}
		}
		t.Error("expected different padding, got same")
	})
}

func TestProtectionMiddleware_GetBrowserUserAgent(t *testing.T) {
	t.Run("returns empty when disabled", func(t *testing.T) {
		// Arrange
		cfg := &config.ProtectionConfig{
			Enabled: false,
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		ua := middleware.GetBrowserUserAgent()

		// Assert
		if ua != "" {
			t.Errorf("expected empty, got %s", ua)
		}
	})

	t.Run("returns custom UA when provided", func(t *testing.T) {
		// Arrange
		customUA := "MyCustomBot/1.0"
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled:  true,
					CustomUA: customUA,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Act
		ua := middleware.GetBrowserUserAgent()

		// Assert
		if ua != customUA {
			t.Errorf("expected %s, got %s", customUA, ua)
		}
	})
}

func TestProtectionMiddleware_GeneratePaddingString(t *testing.T) {
	t.Run("returns empty string for size 0", func(t *testing.T) {
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		result := middleware.GeneratePaddingString(0)

		if result != "" {
			t.Errorf("expected empty string, got %s", result)
		}
	})

	t.Run("returns non-empty string for positive size", func(t *testing.T) {
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		result := middleware.GeneratePaddingString(100)

		if result == "" {
			t.Error("expected non-empty string, got empty")
		}
	})

	t.Run("generates different strings each time", func(t *testing.T) {
		middleware := NewProtectionMiddleware(&config.ProtectionConfig{})

		s1 := middleware.GeneratePaddingString(50)
		s2 := middleware.GeneratePaddingString(50)

		if s1 == s2 {
			t.Error("expected different strings, got same")
		}
	})
}

func TestProtectionMiddleware_GetConfig(t *testing.T) {
	t.Run("returns nil when cfg is nil", func(t *testing.T) {
		middleware := NewProtectionMiddleware(nil)

		result := middleware.GetConfig()

		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("returns config copy when cfg is set", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
		}
		middleware := NewProtectionMiddleware(cfg)

		result := middleware.GetConfig()

		if result == nil {
			t.Error("expected config, got nil")
		}
		if result.Enabled != true {
			t.Errorf("expected Enabled=true, got %v", result.Enabled)
		}
	})

	t.Run("returns independent copy", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
		}
		middleware := NewProtectionMiddleware(cfg)

		result := middleware.GetConfig()

		// Modify original config
		cfg.Enabled = false

		// Returned config should be independent
		if result.Enabled != true {
			t.Error("expected independent copy, changes to original affected returned config")
		}
	})
}

func TestProtectionMiddleware_ApplyBrowserHeaders_Modes(t *testing.T) {
	t.Run("applies Firefox headers when mode is firefox", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeFirefox,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)

		middleware.ApplyBrowserHeaders(&headers)

		ua := headers.Get("User-Agent")
		if ua == "" {
			t.Error("expected User-Agent to be set")
		}
		if !strings.Contains(ua, "Firefox") {
			t.Errorf("expected Firefox in UA, got %s", ua)
		}
	})

	t.Run("applies Safari headers when mode is safari", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeSafari,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)

		middleware.ApplyBrowserHeaders(&headers)

		ua := headers.Get("User-Agent")
		if ua == "" {
			t.Error("expected User-Agent to be set")
		}
		if !strings.Contains(ua, "Safari") {
			t.Errorf("expected Safari in UA, got %s", ua)
		}
	})

	t.Run("randomly selects browser when mode is random", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeRandom,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Run multiple times to verify random selection works
		browsers := make(map[string]int)
		for i := 0; i < 30; i++ {
			headers := make(http.Header)
			middleware.ApplyBrowserHeaders(&headers)
			ua := headers.Get("User-Agent")
			if strings.Contains(ua, "Chrome") {
				browsers["Chrome"]++
			} else if strings.Contains(ua, "Firefox") {
				browsers["Firefox"]++
			} else if strings.Contains(ua, "Safari") && !strings.Contains(ua, "Chrome") {
				browsers["Safari"]++
			}
		}

		// Should have seen at least 2 different browsers
		if len(browsers) < 2 {
			t.Errorf("expected random selection, only saw %d browser type(s): %v", len(browsers), browsers)
		}
	})

	t.Run("falls back to Chrome for invalid mode", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    "invalid-mode",
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)
		headers := make(http.Header)

		middleware.ApplyBrowserHeaders(&headers)

		ua := headers.Get("User-Agent")
		if ua == "" {
			t.Error("expected User-Agent to be set")
		}
		// Should fall back to Chrome
		if !strings.Contains(ua, "Chrome") {
			t.Errorf("expected Chrome fallback, got %s", ua)
		}
	})
}

func TestProtectionMiddleware_GetBrowserUserAgent_Modes(t *testing.T) {
	t.Run("returns Chrome UA when mode is chrome", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeChrome,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		ua := middleware.GetBrowserUserAgent()

		if ua == "" {
			t.Error("expected non-empty UA")
		}
		if !strings.Contains(ua, "Chrome") {
			t.Errorf("expected Chrome in UA, got %s", ua)
		}
	})

	t.Run("returns Firefox UA when mode is firefox", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeFirefox,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		ua := middleware.GetBrowserUserAgent()

		if ua == "" {
			t.Error("expected non-empty UA")
		}
		if !strings.Contains(ua, "Firefox") {
			t.Errorf("expected Firefox in UA, got %s", ua)
		}
	})

	t.Run("returns Safari UA when mode is safari", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeSafari,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		ua := middleware.GetBrowserUserAgent()

		if ua == "" {
			t.Error("expected non-empty UA")
		}
		if !strings.Contains(ua, "Safari") {
			t.Errorf("expected Safari in UA, got %s", ua)
		}
	})

	t.Run("randomly selects UA when mode is random", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    config.BrowserModeRandom,
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		// Run multiple times to verify random selection
		browsers := make(map[string]int)
		for i := 0; i < 30; i++ {
			ua := middleware.GetBrowserUserAgent()
			if strings.Contains(ua, "Chrome") && !strings.Contains(ua, "Firefox") && !strings.Contains(ua, "Safari/605") {
				browsers["Chrome"]++
			} else if strings.Contains(ua, "Firefox") {
				browsers["Firefox"]++
			} else if strings.Contains(ua, "Safari") && !strings.Contains(ua, "Chrome") {
				browsers["Safari"]++
			}
		}

		// Should have seen at least 2 different browsers
		if len(browsers) < 2 {
			t.Errorf("expected random selection, only saw %d browser type(s): %v", len(browsers), browsers)
		}
	})

	t.Run("falls back to Chrome for invalid mode", func(t *testing.T) {
		cfg := &config.ProtectionConfig{
			Enabled: true,
			TrafficCamouflage: config.TrafficCamouflageConfig{
				BrowserHeaders: config.BrowserHeadersConfig{
					Enabled: true,
					Mode:    "invalid-mode",
				},
			},
		}
		middleware := NewProtectionMiddleware(cfg)

		ua := middleware.GetBrowserUserAgent()

		if ua == "" {
			t.Error("expected non-empty UA")
		}
		// Should fall back to Chrome
		if !strings.Contains(ua, "Chrome") {
			t.Errorf("expected Chrome fallback, got %s", ua)
		}
	})
}

