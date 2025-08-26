package relayplugin

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// Plugin is a minimal example implementing plugin.Plugin and plugin.RelayProvider.
type Plugin struct{}

// New returns a new instance of the plugin.
func New() *Plugin { return &Plugin{} }

// ID returns the plugin identifier.
func (p *Plugin) ID() string { return "relay-template" }

// RegisterRoutes installs HTTP routes served by this plugin.
func (p *Plugin) RegisterRoutes(r chi.Router) {
	// r.Post("/api/relay", p.handleRequest)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Plugin) RegisterState(reg *serverstate.Registry) {
	// reg.Add("relay", serverstate.StringValue("ok"))
}

// RegisterRelayEndpoints attaches relay-specific routes.
func (p *Plugin) RegisterRelayEndpoints(r chi.Router) {
	// r.HandleFunc("/api/relay/connect", p.handleRelay)
}
