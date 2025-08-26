package workerplugin

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// Plugin is a minimal example implementing plugin.Plugin and plugin.WorkerProvider.
type Plugin struct{}

// New returns a new instance of the plugin.
func New() *Plugin { return &Plugin{} }

// ID returns the plugin identifier.
func (p *Plugin) ID() string { return "worker-template" }

// RegisterRoutes installs HTTP routes served by this plugin.
func (p *Plugin) RegisterRoutes(r chi.Router) {
	// r.Post("/api/example", p.handleRequest)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Plugin) RegisterState(reg *serverstate.Registry) {
	// reg.Add("example", serverstate.StringValue("ok"))
}

// RegisterWebSocket registers the worker connect endpoint.
func (p *Plugin) RegisterWebSocket(r chi.Router) {
	// r.Get("/api/example/connect", p.handleConnect)
}

// Scheduler returns the dispatch scheduler for this plugin.
func (p *Plugin) Scheduler() ctrlsrv.Scheduler {
	// return myScheduler
	return nil
}
