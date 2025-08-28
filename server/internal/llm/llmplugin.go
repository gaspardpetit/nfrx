package llm

import (
	"time"

    "github.com/gaspardpetit/nfrx/sdk/spi"
	"github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/api"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/server/internal/metrics"
)

// Plugin implements the llm subsystem as a plugin.
type Plugin struct {
	cfg     config.ServerConfig
	version string
	sha     string
	date    string

	reg     *ctrlsrv.Registry
	metrics *ctrlsrv.MetricsRegistry
	sched   ctrlsrv.Scheduler
	mcp     *mcpbroker.Registry
	opts    map[string]string
}

// New constructs a new LLM plugin.
func New(cfg config.ServerConfig, version, sha, date string, mcp *mcpbroker.Registry, opts map[string]string) *Plugin {
	reg := ctrlsrv.NewRegistry()
	metricsReg := ctrlsrv.NewMetricsRegistry(version, sha, date)
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	return &Plugin{cfg: cfg, version: version, sha: sha, date: date, reg: reg, metrics: metricsReg, sched: sched, mcp: mcp, opts: opts}
}

func (p *Plugin) ID() string { return "llm" }

// RegisterRoutes wires the HTTP endpoints.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	r.Handle("/connect", ctrlsrv.WSHandler(p.reg, p.metrics, p.cfg.ClientKey))
	r.Group(func(g spi.Router) {
		if p.cfg.APIKey != "" {
			g.Use(api.APIKeyMiddleware(p.cfg.APIKey))
		}
		g.Route("/v1", func(v1 spi.Router) {
			openai.Mount(
				v1,
				adapters.NewWorkerRegistry(p.reg),
				adapters.NewScheduler(p.sched),
				adapters.NewMetrics(p.metrics),
				openai.Options{RequestTimeout: p.cfg.RequestTimeout, MaxParallelEmbeddings: p.cfg.MaxParallelEmbeddings},
			)
		})
	})

	go func() {
		ticker := time.NewTicker(ctrlsrv.HeartbeatInterval)
		for range ticker.C {
			p.reg.PruneExpired(ctrlsrv.HeartbeatExpiry)
		}
	}()
}

// Scheduler returns the plugin's scheduler.
func (p *Plugin) Scheduler() spi.Scheduler { return adapters.NewScheduler(p.sched) }

// RegisterMetrics registers Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {
	metrics.Register(reg)
	metrics.SetServerBuildInfo(p.version, p.sha, p.date)
}

// RegisterState registers state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: "llm", Data: func() any { return p.metrics.Snapshot() }})
}

// Registry exposes the worker registry for tests.
func (p *Plugin) Registry() *ctrlsrv.Registry { return p.reg }

// MetricsRegistry exposes metrics for tests.
func (p *Plugin) MetricsRegistry() *ctrlsrv.MetricsRegistry { return p.metrics }

// Sched exposes scheduler for tests.
func (p *Plugin) Sched() ctrlsrv.Scheduler { return p.sched }

var _ spi.Plugin = (*Plugin)(nil)
var _ spi.WorkerProvider = (*Plugin)(nil)
