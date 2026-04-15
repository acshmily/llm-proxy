package router

import (
	"testing"
	"time"

	"github.com/claude-projetc/llm-proxy/internal/config"
)

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
	})

	t.Run("not found route", func(t *testing.T) {
		_, found := r.FindRoute("sk-unknown")
		if found {
			t.Error("Expected not to find route")
		}
	})
}
