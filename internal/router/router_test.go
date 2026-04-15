package router

import (
	"sync"
	"testing"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

func TestRouter_New(t *testing.T) {
	t.Run("creates router with empty routes", func(t *testing.T) {
		r := New([]config.RouteConfig{})
		if r == nil {
			t.Error("expected router instance, got nil")
		}
		if r.routes == nil {
			t.Error("expected routes map initialized")
		}
	})

	t.Run("creates router with multiple routes", func(t *testing.T) {
		routes := []config.RouteConfig{
			{APIKey: "sk-test-1", Backend: "openai", BackendKey: "sk-openai-xxx", Timeout: 60 * time.Second},
			{APIKey: "sk-test-2", Backend: "anthropic", BackendKey: "sk-claude-xxx", Timeout: 120 * time.Second},
			{APIKey: "sk-test-3", Backend: "gemini", BackendKey: "AIza-xxx", Timeout: 90 * time.Second},
		}

		r := New(routes)

		if len(r.routes) != 3 {
			t.Errorf("expected 3 routes, got %d", len(r.routes))
		}
	})
}

func TestRouter_FindRoute(t *testing.T) {
	routes := []config.RouteConfig{
		{APIKey: "sk-test-1", Backend: "openai", BackendKey: "sk-openai-xxx", Timeout: 60 * time.Second},
		{APIKey: "sk-test-2", Backend: "claude", BackendKey: "sk-claude-xxx", Timeout: 120 * time.Second},
	}

	r := New(routes)

	t.Run("found route", func(t *testing.T) {
		route, found := r.FindRoute("sk-test-1")
		if !found {
			t.Fatal("Expected to find route")
		}
		if route.Backend != "openai" {
			t.Errorf("Expected backend 'openai', got '%s'", route.Backend)
		}
		if route.BackendKey != "sk-openai-xxx" {
			t.Errorf("Expected backendKey 'sk-openai-xxx', got '%s'", route.BackendKey)
		}
		if route.Timeout != 60*time.Second {
			t.Errorf("Expected timeout 60s, got %v", route.Timeout)
		}
	})

	t.Run("not found route", func(t *testing.T) {
		_, found := r.FindRoute("sk-unknown")
		if found {
			t.Error("Expected not to find route")
		}
	})

	t.Run("returns correct backend for different routes", func(t *testing.T) {
		route, found := r.FindRoute("sk-test-2")
		if !found {
			t.Fatal("Expected to find route")
		}
		if route.Backend != "claude" {
			t.Errorf("Expected backend 'claude', got '%s'", route.Backend)
		}
		if route.Timeout != 120*time.Second {
			t.Errorf("Expected timeout 120s, got %v", route.Timeout)
		}
	})
}

func TestRouter_ConcurrencySafety(t *testing.T) {
	routes := []config.RouteConfig{
		{APIKey: "sk-test-1", Backend: "openai", BackendKey: "sk-openai-xxx", Timeout: 60 * time.Second},
		{APIKey: "sk-test-2", Backend: "anthropic", BackendKey: "sk-claude-xxx", Timeout: 120 * time.Second},
	}

	r := New(routes)

	// Concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.FindRoute("sk-test-1")
			r.FindRoute("sk-test-2")
			r.FindRoute("sk-unknown")
		}()
	}

	wg.Wait()
	// Test passes if no panic
}

func TestRouter_RouteStruct(t *testing.T) {
	t.Run("Route has correct fields", func(t *testing.T) {
		route := &Route{
			Backend:    "openai",
			BackendKey: "sk-test-key",
			Timeout:    90 * time.Second,
		}

		if route.Backend != "openai" {
			t.Errorf("Expected backend 'openai', got '%s'", route.Backend)
		}
		if route.BackendKey != "sk-test-key" {
			t.Errorf("Expected backendKey 'sk-test-key', got '%s'", route.BackendKey)
		}
		if route.Timeout != 90*time.Second {
			t.Errorf("Expected timeout 90s, got %v", route.Timeout)
		}
	})
}
