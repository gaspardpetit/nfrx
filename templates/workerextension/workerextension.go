package workerextension

import (
	"github.com/gaspardpetit/nfrx/internal/extension"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// Extension is a minimal example implementing extension.Plugin and extension.WorkerProvider.
type Extension struct{}

// New returns a new instance of the extension.
func New() *Extension { return &Extension{} }

// ID returns the extension identifier.
func (p *Extension) ID() string { return "worker-template" }

// RegisterRoutes installs HTTP routes served by this extension.
func (p *Extension) RegisterRoutes(r chi.Router) {
	// r.Post("/api/example", p.handleRequest)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Extension) RegisterMetrics(reg *prometheus.Registry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Extension) RegisterState(reg *serverstate.Registry) {
	// reg.Add("example", serverstate.StringValue("ok"))
}

// RegisterWebSocket registers the worker connect endpoint.
func (p *Extension) RegisterWebSocket(r chi.Router) {
	// r.Get("/api/example/connect", p.handleConnect)
}

// Scheduler returns the dispatch scheduler for this extension.
func (p *Extension) Scheduler() ctrlsrv.Scheduler {
	// return myScheduler
	return nil
}

var _ extension.Plugin = (*Extension)(nil)
var _ extension.WorkerProvider = (*Extension)(nil)
