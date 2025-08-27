package mcpplugin

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/internal/config"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
	mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
)

// Plugin implements the MCP relay as a plugin.
type Plugin struct {
	cfg    config.ServerConfig
	broker *mcpbroker.Registry
	opts   map[string]string
}

// New constructs a new MCP plugin.
func New(cfg config.ServerConfig, opts map[string]string) *Plugin {
	reg := mcpbroker.NewRegistry(cfg.RequestTimeout)
	return &Plugin{cfg: cfg, broker: reg, opts: opts}
}

func (p *Plugin) ID() string { return "mcp" }

// RegisterRoutes registers HTTP routes; MCP uses relay endpoints only.
func (p *Plugin) RegisterRoutes(r chi.Router) {}

// RegisterMetrics registers Prometheus collectors; MCP has none currently.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {}

// RegisterState registers MCP state elements.
func (p *Plugin) RegisterState(reg *serverstate.Registry) {
	reg.Add(serverstate.Element{ID: "mcp", Data: func() interface{} { return p.broker.Snapshot() }})
}

// RegisterRelayEndpoints wires MCP relay HTTP/WS endpoints.
func (p *Plugin) RegisterRelayEndpoints(r chi.Router) {
	r.Handle("/api/mcp/connect", p.broker.WSHandler(p.cfg.ClientKey))
	r.Post("/api/mcp/id/{id}", p.broker.HTTPHandler())
}

// Registry exposes the underlying broker for tests.
func (p *Plugin) Registry() *mcpbroker.Registry { return p.broker }

var _ plugin.Plugin = (*Plugin)(nil)
var _ plugin.RelayProvider = (*Plugin)(nil)
