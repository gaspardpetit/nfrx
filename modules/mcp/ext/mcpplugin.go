package mcp

import (
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/modules/common/spi"
	mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
)

// Plugin implements the MCP relay as a plugin.
type Plugin struct {
	broker     *mcpbroker.Registry
	pluginOpts map[string]string
	clientKey  string
}

// New constructs a new MCP plugin.
func New(state spi.ServerState, opts Options, pluginOpts map[string]string) *Plugin {
	reg := mcpbroker.NewRegistry(opts.RequestTimeout, state)
	return &Plugin{broker: reg, pluginOpts: pluginOpts, clientKey: opts.ClientKey}
}

func (p *Plugin) ID() string { return "mcp" }

// RegisterRoutes registers HTTP routes; MCP uses relay endpoints only.
func (p *Plugin) RegisterRoutes(r chi.Router) {}

// RegisterMetrics registers Prometheus collectors; MCP has none currently.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {}

// RegisterState registers MCP state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: "mcp", Data: func() any { return p.broker.Snapshot() }})
}

// RegisterRelayEndpoints wires MCP relay HTTP/WS endpoints.
func (p *Plugin) RegisterRelayEndpoints(r chi.Router) {
	r.Handle("/connect", p.broker.WSHandler(p.clientKey))
	r.Handle("/id/{id}", p.broker.HTTPHandler())
}

// Registry exposes the underlying broker for tests.
func (p *Plugin) Registry() *mcpbroker.Registry { return p.broker }

var _ spi.Plugin = (*Plugin)(nil)
var _ spi.RelayProvider = (*Plugin)(nil)
