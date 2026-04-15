package router

import (
	"sync"
	"time"

	"github.com/claude-projetc/proxy-gemini-go/internal/config"
)

type Route struct {
	Backend    string
	BackendKey string
	Timeout    time.Duration
}

type Router struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

func New(routes []config.RouteConfig) *Router {
	r := &Router{
		routes: make(map[string]*Route),
	}
	for _, rc := range routes {
		r.routes[rc.APIKey] = &Route{
			Backend:    rc.Backend,
			BackendKey: rc.BackendKey,
			Timeout:    rc.Timeout,
		}
	}
	return r
}

func (r *Router) FindRoute(apiKey string) (*Route, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	route, found := r.routes[apiKey]
	return route, found
}
