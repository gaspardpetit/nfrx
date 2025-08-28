package workerplugin

import (
    "github.com/gaspardpetit/nfrx/sdk/spi"
)

// Plugin is a minimal example implementing plugin.Plugin and plugin.WorkerProvider.
type Plugin struct{}

// New returns a new instance of the plugin.
func New() *Plugin { return &Plugin{} }

// ID returns the plugin identifier.
func (p *Plugin) ID() string { return "worker-template" }

// RegisterRoutes installs HTTP routes served by this plugin.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	// r.Post("/api/example", p.handleRequest)
	// r.Get("/api/example/connect", p.handleConnect)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	// reg.Add(spi.StateElement{ID: "example", Data: func() any { return "ok" }})
}

// Scheduler returns the dispatch scheduler for this plugin.
func (p *Plugin) Scheduler() spi.Scheduler {
	// return myScheduler
	return nil
}
