package middleware

import (
	"net/http"
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

