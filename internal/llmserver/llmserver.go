package llmserver

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/internal/config"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/internal/extension"
	mcphub "github.com/gaspardpetit/nfrx/internal/mcphub"
	"github.com/gaspardpetit/nfrx/internal/metrics"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// Plugin implements the llm subsystem as a server extension.
type Plugin struct {
	cfg     config.ServerConfig
	version string
	sha     string
	date    string

	reg     *ctrlsrv.Registry
	metrics *ctrlsrv.MetricsRegistry
	sched   ctrlsrv.Scheduler
	mcp     *mcphub.Registry
	opts    map[string]string
}

// New constructs a new LLM extension.
func New(cfg config.ServerConfig, version, sha, date string, mcp *mcphub.Registry, opts map[string]string) *Plugin {
	reg := ctrlsrv.NewRegistry()
	metricsReg := ctrlsrv.NewMetricsRegistry(version, sha, date)
	sched := &ctrlsrv.LeastBusyScheduler{Reg: reg}
	return &Plugin{cfg: cfg, version: version, sha: sha, date: date, reg: reg, metrics: metricsReg, sched: sched, mcp: mcp, opts: opts}
}

func (p *Plugin) ID() string { return "llm" }

// Scheduler returns the extension's scheduler.
func (p *Plugin) Scheduler() ctrlsrv.Scheduler { return p.sched }

// RegisterMetrics registers Prometheus collectors.
func (p *Plugin) RegisterMetrics(reg *prometheus.Registry) {
	metrics.Register(reg)
	metrics.SetServerBuildInfo(p.version, p.sha, p.date)
}

// RegisterState registers state elements.
func (p *Plugin) RegisterState(reg *serverstate.Registry) {
	reg.Add(serverstate.Element{ID: "llm", Data: func() interface{} { return p.metrics.Snapshot() }})
}

// Registry exposes the worker registry for tests.
func (p *Plugin) Registry() *ctrlsrv.Registry { return p.reg }

// MetricsRegistry exposes metrics for tests.
func (p *Plugin) MetricsRegistry() *ctrlsrv.MetricsRegistry { return p.metrics }

// Sched exposes scheduler for tests.
func (p *Plugin) Sched() ctrlsrv.Scheduler { return p.sched }

var _ extension.Plugin = (*Plugin)(nil)
var _ extension.WorkerProvider = (*Plugin)(nil)
