package plugin

import (
	"encoding/json"
	"net/http"

	"github.com/gaspardpetit/nfrx/core/secret"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// Base provides a minimal implementation of the spi.Plugin interface that
// plugins can embed to inherit default behaviors. It mounts a base route
// that returns the plugin descriptor and masked options as JSON; concrete
// plugins can call Base.RegisterRoutes before wiring their own endpoints.
type Base struct {
	desc spi.PluginDescriptor
	opts map[string]string
}

// NewBase constructs a Base with the given plugin descriptor and the plugin's
// option map (values) for masking and display. Options may be nil.
func NewBase(desc spi.PluginDescriptor, opts map[string]string) Base {
	return Base{desc: desc, opts: opts}
}

// ID returns the plugin identifier used in routing (e.g., "/api/{id}").
func (b Base) ID() string { return b.desc.ID }

// RegisterRoutes registers a default handler at the plugin root that returns
// the plugin descriptor and current options (with secrets masked) as JSON.
// Concrete plugins should call this to provide a default for the base path and
// then add their specific endpoints.
func (b Base) RegisterRoutes(r spi.Router) {
	r.Get("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Build masked options
		masked := map[string]string{}
		if b.opts != nil {
			sec := map[string]bool{}
			for _, a := range b.desc.Args {
				if a.Secret {
					sec[a.ID] = true
				}
			}
			for k, v := range b.opts {
				if sec[k] && v != "" {
					masked[k] = secret.Mask(v)
				} else {
					masked[k] = v
				}
			}
		}
		resp := struct {
			ID      string            `json:"id"`
			Name    string            `json:"name"`
			Summary string            `json:"summary"`
			Args    []spi.ArgSpec     `json:"args"`
			Options map[string]string `json:"options,omitempty"`
		}{ID: b.desc.ID, Name: b.desc.Name, Summary: b.desc.Summary, Args: b.desc.Args, Options: masked}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// RegisterMetrics is a no-op in the base implementation.
func (b Base) RegisterMetrics(_ spi.MetricsRegistry) {}

// RegisterState is a no-op in the base implementation.
func (b Base) RegisterState(_ spi.StateRegistry) {}

var _ spi.Plugin = (*Base)(nil)
