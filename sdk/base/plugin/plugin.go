package plugin

import (
    "net/http"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// Base provides a minimal implementation of the spi.Plugin interface that
// plugins can embed to inherit default behaviors. For now it only mounts a
// base route that returns 501 Not Implemented; concrete plugins can call
// Base.RegisterRoutes before wiring their own endpoints.
type Base struct{
    id string
}

// NewBase constructs a Base with the given plugin identifier.
func NewBase(id string) Base { return Base{id: id} }

// ID returns the plugin identifier used in routing (e.g., "/api/{id}").
func (b Base) ID() string { return b.id }

// RegisterRoutes registers a default handler at the plugin root that returns
// 501 Not Implemented. Concrete plugins should call this to provide a sensible
// default for the base path and then add their specific endpoints.
func (b Base) RegisterRoutes(r spi.Router) {
    r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusNotImplemented)
        _, _ = w.Write([]byte(`{"error":"not_implemented"}`))
    }))
}

// RegisterMetrics is a no-op in the base implementation.
func (b Base) RegisterMetrics(_ spi.MetricsRegistry) {}

// RegisterState is a no-op in the base implementation.
func (b Base) RegisterState(_ spi.StateRegistry) {}

var _ spi.Plugin = (*Base)(nil)

