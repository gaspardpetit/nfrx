package mcpserver

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx-sdk/config"
	"github.com/gaspardpetit/nfrx-server/internal/extension"
	mcphub "github.com/gaspardpetit/nfrx-server/internal/mcphub"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
)

// Plugin implements the MCP relay as a server extension.
type Plugin struct {
	cfg    config.ServerConfig
	broker *mcphub.Registry
	opts   map[string]string
}

// New constructs a new MCP extension.
func New(cfg config.ServerConfig, opts map[string]string) *Plugin {
	reg := mcphub.NewRegistry(cfg.RequestTimeout)
	return &Plugin{cfg: cfg, broker: reg, opts: opts}
}

func (p *Plugin) ID() string { return "mcp" }

// RegisterMetrics registers Prometheus collectors; MCP has none currently.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {}

// RegisterState registers MCP state elements.
func (p *Plugin) RegisterState(reg *serverstate.Registry) {
	reg.Add(serverstate.Element{ID: "mcp", Data: func() interface{} { return p.broker.Snapshot() }})
}

// Registry exposes the underlying broker for tests.
func (p *Plugin) Registry() *mcphub.Registry { return p.broker }

var _ extension.Plugin = (*Plugin)(nil)
var _ extension.RelayProvider = (*Plugin)(nil)
