package mcp

import (
    mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
    "github.com/gaspardpetit/nfrx/sdk/spi"
)

// Plugin implements the MCP relay as a plugin.
type Plugin struct {
    broker     *mcpbroker.Registry
    pluginOpts spi.Options
    clientKey  string
}

// New constructs a new MCP plugin using the common server options.
func New(state spi.ServerState, opts spi.Options) *Plugin {
    reg := mcpbroker.NewRegistry(opts.RequestTimeout, state)
    return &Plugin{broker: reg, pluginOpts: opts, clientKey: opts.ClientKey}
}

func (p *Plugin) ID() string { return "mcp" }

// RegisterRoutes registers HTTP routes; MCP uses relay endpoints only.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	r.Handle("/connect", p.broker.WSHandler(p.clientKey))
	r.Handle("/id/{id}", p.broker.HTTPHandler())
}

// RegisterMetrics registers Prometheus collectors; MCP has none currently.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {}

// RegisterState registers MCP state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: "mcp", Data: func() any { return p.broker.Snapshot() }})
}

// Registry exposes the underlying broker for tests.
func (p *Plugin) Registry() *mcpbroker.Registry { return p.broker }

var _ spi.Plugin = (*Plugin)(nil)
