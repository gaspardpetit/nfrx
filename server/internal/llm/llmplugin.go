package llm

import (
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/gaspardpetit/nfrx/api/generated"
	"github.com/gaspardpetit/nfrx/server/internal/api"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/server/internal/llm/openai"
	mcpbroker "github.com/gaspardpetit/nfrx/server/internal/mcp/mcpbroker"
	"github.com/gaspardpetit/nfrx/server/internal/metrics"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
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
func (p *Plugin) RegisterRoutes(r chi.Router) {
	impl := &api.API{Reg: p.reg, Metrics: p.metrics, MCP: p.mcp, Sched: p.sched, Timeout: p.cfg.RequestTimeout, MaxParallelEmbeddings: p.cfg.MaxParallelEmbeddings}
	wrapper := generated.ServerInterfaceWrapper{Handler: impl}

	r.Get("/healthz", wrapper.GetHealthz)
	r.Route("/api", func(apiGroup chi.Router) {
		if p.cfg.APIKey != "" {
			apiGroup.Use(api.APIKeyMiddleware(p.cfg.APIKey))
		}
		apiGroup.Route("/v1", func(v1 chi.Router) {
			openai.Mount(v1, p.reg, p.sched, p.metrics, p.cfg.RequestTimeout, p.cfg.MaxParallelEmbeddings)
		})
		apiGroup.Get("/state", wrapper.GetApiState)
		apiGroup.Get("/state/stream", wrapper.GetApiStateStream)
	})
	r.Route("/api/client", func(r chi.Router) {
		r.Get("/openapi.json", api.OpenAPIHandler())
		r.Get("/*", api.SwaggerHandler())
	})

	go func() {
		ticker := time.NewTicker(ctrlsrv.HeartbeatInterval)
		for range ticker.C {
			p.reg.PruneExpired(ctrlsrv.HeartbeatExpiry)
		}
	}()
}

// RegisterWebSocket attaches the worker connect endpoint.
func (p *Plugin) RegisterWebSocket(r chi.Router) {
	r.Handle("/api/workers/connect", ctrlsrv.WSHandler(p.reg, p.metrics, p.cfg.ClientKey))
}

// Scheduler returns the plugin's scheduler.
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

var _ plugin.Plugin = (*Plugin)(nil)
var _ plugin.WorkerProvider = (*Plugin)(nil)
