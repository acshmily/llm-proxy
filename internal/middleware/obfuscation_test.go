package middleware

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

func TestTrafficObfuscationMiddleware_IsEnabled(t *testing.T) {
	t.Run("returns false when config is nil", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)
		if middleware.IsEnabled() {
			t.Error("expected false for nil config")
		}
	})

	t.Run("returns false when both features disabled", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			WebSocketTunnel: config.WebSocketTunnelConfig{Enabled: false},
			RequestSharding: config.RequestShardingConfig{Enabled: false},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if middleware.IsEnabled() {
			t.Error("expected false when both features disabled")
		}
	})

	t.Run("returns true when websocket tunnel enabled", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			WebSocketTunnel: config.WebSocketTunnelConfig{Enabled: true},
			RequestSharding: config.RequestShardingConfig{Enabled: false},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if !middleware.IsEnabled() {
			t.Error("expected true when websocket tunnel enabled")
		}
	})

	t.Run("returns true when request sharding enabled", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			WebSocketTunnel: config.WebSocketTunnelConfig{Enabled: false},
			RequestSharding: config.RequestShardingConfig{Enabled: true},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if !middleware.IsEnabled() {
			t.Error("expected true when request sharding enabled")
		}
	})
}

func TestTrafficObfuscationMiddleware_ShouldShardRequest(t *testing.T) {
	t.Run("returns false when feature disabled", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      false,
				MaxChunkSize: 1024,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if middleware.ShouldShardRequest(2048) {
			t.Error("expected false when feature disabled")
		}
	})

	t.Run("returns false when body smaller than max chunk size", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 1024,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if middleware.ShouldShardRequest(512) {
			t.Error("expected false when body smaller than max chunk size")
		}
	})

	t.Run("returns true when body larger than max chunk size", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 1024,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if !middleware.ShouldShardRequest(2048) {
			t.Error("expected true when body larger than max chunk size")
		}
	})

	t.Run("returns false when max chunk size is 0", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 0,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)
		if middleware.ShouldShardRequest(2048) {
			t.Error("expected false when max chunk size is 0")
		}
	})
}

func TestTrafficObfuscationMiddleware_ShardRequest(t *testing.T) {
	t.Run("splits body into correct number of chunks", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 10,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)

		body := []byte("0123456789abcdefghijABCDEFGHIJ") // 30 bytes
		chunks, err := middleware.ShardRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(chunks) != 3 {
			t.Errorf("expected 3 chunks, got %d", len(chunks))
		}
	})

	t.Run("each chunk is base64 encoded", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 10,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)

		body := []byte("hello world")
		chunks, err := middleware.ShardRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for i, chunk := range chunks {
			_, err := base64.StdEncoding.DecodeString(chunk)
			if err != nil {
				t.Errorf("chunk %d is not valid base64: %v", i, err)
			}
		}
	})

	t.Run("empty body returns empty chunks", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 10,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)

		body := []byte{}
		chunks, err := middleware.ShardRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty body, got %d", len(chunks))
		}
	})
}

func TestTrafficObfuscationMiddleware_ReassembleChunks(t *testing.T) {
	t.Run("reassembles chunks correctly", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		original := []byte("hello world this is a test")
		maxChunkSize := 10

		// Manually create chunks
		var chunks []string
		for i := 0; i < len(original); i += maxChunkSize {
			end := i + maxChunkSize
			if end > len(original) {
				end = len(original)
			}
			chunk := original[i:end]
			chunks = append(chunks, base64.StdEncoding.EncodeToString(chunk))
		}

		reassembled, err := middleware.ReassembleChunks(chunks)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !bytes.Equal(original, reassembled) {
			t.Errorf("expected %s, got %s", string(original), string(reassembled))
		}
	})

	t.Run("returns error for invalid base64", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		chunks := []string{"invalid!", "base64"}
		_, err := middleware.ReassembleChunks(chunks)
		if err == nil {
			t.Error("expected error for invalid base64")
		}
	})

	t.Run("empty chunks returns empty slice", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		reassembled, err := middleware.ReassembleChunks([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(reassembled) != 0 {
			t.Errorf("expected empty slice, got %d bytes", len(reassembled))
		}
	})
}

func TestTrafficObfuscationMiddleware_ShardAndReassemble_RoundTrip(t *testing.T) {
	t.Run("shard then reassemble produces original data", func(t *testing.T) {
		cfg := &config.TrafficObfuscationConfig{
			RequestSharding: config.RequestShardingConfig{
				Enabled:      true,
				MaxChunkSize: 100,
			},
		}
		middleware := NewTrafficObfuscationMiddleware(cfg)

		original := []byte("This is a test of the emergency broadcast system. " +
			"This is only a test. If this were a real emergency, " +
			"you would be instructed to take protective action.")

		chunks, err := middleware.ShardRequest(original)
		if err != nil {
			t.Fatalf("ShardRequest failed: %v", err)
		}

		reassembled, err := middleware.ReassembleChunks(chunks)
		if err != nil {
			t.Fatalf("ReassembleChunks failed: %v", err)
		}

		if !bytes.Equal(original, reassembled) {
			t.Error("round-trip failed: reassembled data differs from original")
		}
	})
}

func TestTrafficObfuscationMiddleware_IsShardedRequest(t *testing.T) {
	t.Run("returns true for sharded request", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)
		req := &http.Request{
			Header: http.Header{
				"X-Request-Sharded": []string{"true"},
			},
		}
		if !middleware.IsShardedRequest(req) {
			t.Error("expected true for sharded request")
		}
	})

	t.Run("returns false for non-sharded request", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)
		req := &http.Request{
			Header: http.Header{},
		}
		if middleware.IsShardedRequest(req) {
			t.Error("expected false for non-sharded request")
		}
	})
}

func TestTrafficObfuscationMiddleware_WrapShardedRequest(t *testing.T) {
	t.Run("wraps request with shard metadata", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		body := []byte("test body")
		req, err := http.NewRequest("POST", "http://example.com", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		chunks := []string{"chunk1", "chunk2", "chunk3"}
		shardReq, err := middleware.WrapShardedRequest(req, chunks)
		if err != nil {
			t.Fatalf("WrapShardedRequest failed: %v", err)
		}

		if shardReq.Header.Get("X-Request-Sharded") != "true" {
			t.Error("expected X-Request-Sharded header to be set")
		}

		if shardReq.Header.Get("X-Chunk-Count") != string(rune(3)) {
			t.Errorf("expected X-Chunk-Count to be %c, got %s", 3, shardReq.Header.Get("X-Chunk-Count"))
		}

		if shardReq.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type to be application/json, got %s", shardReq.Header.Get("Content-Type"))
		}
	})
}

func TestTrafficObfuscationMiddleware_UnwrapShardedRequest(t *testing.T) {
	t.Run("unwraps sharded request correctly", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		originalBody := []byte("original request body")

		// Create chunks
		var chunks []string
		for i := 0; i < len(originalBody); i += 10 {
			end := i + 10
			if end > len(originalBody) {
				end = len(originalBody)
			}
			chunk := originalBody[i:end]
			chunks = append(chunks, base64.StdEncoding.EncodeToString(chunk))
		}

		// Create wrapped request
		wrappedReq, err := middleware.WrapShardedRequest(
			&http.Request{
				Method: "POST",
				URL:    &url.URL{Scheme: "http", Host: "example.com", Path: "/test"},
				Header: http.Header{"Authorization": []string{"Bearer token"}},
			},
			chunks,
		)
		if err != nil {
			t.Fatalf("WrapShardedRequest failed: %v", err)
		}

		// Unwrap
		unwrappedReq, reassembled, err := middleware.UnwrapShardedRequest(wrappedReq)
		if err != nil {
			t.Fatalf("UnwrapShardedRequest failed: %v", err)
		}

		if !bytes.Equal(originalBody, reassembled) {
			t.Errorf("expected %s, got %s", string(originalBody), string(reassembled))
		}

		if unwrappedReq.Header.Get("Authorization") != "Bearer token" {
			t.Error("expected Authorization header to be preserved")
		}

		if unwrappedReq.Header.Get("X-Request-Sharded") != "" {
			t.Error("expected X-Request-Sharded header to be removed")
		}
	})

	t.Run("returns nil for non-sharded request", func(t *testing.T) {
		middleware := NewTrafficObfuscationMiddleware(nil)

		body := []byte("normal body")
		req, err := http.NewRequest("POST", "http://example.com", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		newReq, data, err := middleware.UnwrapShardedRequest(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if newReq != nil {
			t.Error("expected nil for non-sharded request")
		}
		if data != nil {
			t.Error("expected nil data for non-sharded request")
		}
	})
}

func TestTrafficObfuscationMiddleware_ConcurrencySafety(t *testing.T) {
	cfg := &config.TrafficObfuscationConfig{
		RequestSharding: config.RequestShardingConfig{
			Enabled:      true,
			MaxChunkSize: 100,
		},
	}
	middleware := NewTrafficObfuscationMiddleware(cfg)

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			body := []byte("test body for concurrency")
			middleware.ShouldShardRequest(len(body))
			middleware.IsEnabled()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// Test passes if no panic
}
