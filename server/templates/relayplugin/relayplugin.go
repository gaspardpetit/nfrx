package relayplugin

import (
	"github.com/gaspardpetit/nfrx-sdk/spi"
)

// Plugin is a minimal example implementing plugin.Plugin.
type Plugin struct{}

// New returns a new instance of the plugin.
func New() *Plugin { return &Plugin{} }

// ID returns the plugin identifier.
func (p *Plugin) ID() string { return "relay-template" }

// RegisterRoutes installs HTTP routes served by this plugin.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	// r.Post("/api/relay", p.handleRequest)
	// r.Handle("/api/relay/connect", p.handleRelay)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	// reg.Add(spi.StateElement{ID: "relay", Data: func() any { return "ok" }})
}
