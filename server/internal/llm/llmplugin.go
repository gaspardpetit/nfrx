package llm

import (
    "net/http"

    "github.com/gaspardpetit/nfrx/sdk/spi"
    "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
)

// Plugin implements the llm subsystem as a plugin.
type Plugin struct {
    // dependency-injected fields (SPI-only)
    connect    http.Handler
    wr         spi.WorkerRegistry
    sch        spi.Scheduler
    mx         spi.Metrics
    authMW     spi.Middleware
    openaiOpts openai.Options
    stateFn    func() any

    // build info for metrics registration
    version string
    sha     string
    date    string
}

// (legacy New removed; use NewWithDeps)

func (p *Plugin) ID() string { return "llm" }

// RegisterRoutes wires the HTTP endpoints.
func (p *Plugin) RegisterRoutes(r spi.Router) {
    if p.connect != nil {
        r.Handle("/connect", p.connect)
    }
    r.Group(func(g spi.Router) {
        if p.authMW != nil {
            g.Use(p.authMW)
        }
        g.Route("/v1", func(v1 spi.Router) {
            openai.Mount(v1, p.wr, p.sch, p.mx, p.openaiOpts)
        })
    })
}

// Scheduler returns the plugin's scheduler.
func (p *Plugin) Scheduler() spi.Scheduler { return p.sch }

// RegisterMetrics registers Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {}

// RegisterState registers state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
    sf := p.stateFn
    if sf == nil {
        sf = func() any { return nil }
    }
    reg.Add(spi.StateElement{ID: "llm", Data: sf})
}

var _ spi.Plugin = (*Plugin)(nil)
var _ spi.WorkerProvider = (*Plugin)(nil)

// NewWithDeps constructs a new LLM plugin using injected SPI dependencies.
// This keeps existing New() intact for tests and legacy wiring.
func NewWithDeps(
    connect http.Handler,
    workers spi.WorkerRegistry,
    sched spi.Scheduler,
    metrics spi.Metrics,
    stateProvider func() any,
    openaiOpts openai.Options,
    version, sha, date string,
    opts map[string]string,
    authMW spi.Middleware,
) *Plugin {
    return &Plugin{
        version:    version,
        sha:        sha,
        date:       date,
        connect:    connect,
        wr:         workers,
        sch:        sched,
        mx:         metrics,
        authMW:     authMW,
        openaiOpts: openaiOpts,
        stateFn:    stateProvider,
    }
}
