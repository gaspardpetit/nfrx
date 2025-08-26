package relayextension

import (
	"github.com/gaspardpetit/nfrx/internal/extension"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// Extension is a minimal example implementing extension.Plugin and extension.RelayProvider.
type Extension struct{}

// New returns a new instance of the extension.
func New() *Extension { return &Extension{} }

// ID returns the extension identifier.
func (p *Extension) ID() string { return "relay-template" }

// RegisterRoutes installs HTTP routes served by this extension.
func (p *Extension) RegisterRoutes(r chi.Router) {
	// r.Post("/api/relay", p.handleRequest)
}

// RegisterMetrics adds Prometheus collectors.
func (p *Extension) RegisterMetrics(reg *prometheus.Registry) {
	// reg.MustRegister(myCollector)
}

// RegisterState exposes values under /state.
func (p *Extension) RegisterState(reg *serverstate.Registry) {
	// reg.Add("relay", serverstate.StringValue("ok"))
}

// RegisterRelayEndpoints attaches relay-specific routes.
func (p *Extension) RegisterRelayEndpoints(r chi.Router) {
	// r.HandleFunc("/api/relay/connect", p.handleRelay)
}

var _ extension.Plugin = (*Extension)(nil)
var _ extension.RelayProvider = (*Extension)(nil)
