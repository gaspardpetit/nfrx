package docling

import (
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	baseplugin "github.com/gaspardpetit/nfrx/sdk/base/plugin"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// Plugin implements the docling worker-style extension.
type Plugin struct {
	baseplugin.Base
	reg      *baseworker.Registry
	mxreg    *baseworker.MetricsRegistry
	sch      baseworker.Scheduler
	srvState spi.ServerState
	srvOpts  spi.Options
	authMW   spi.Middleware
}

func (p *Plugin) RegisterRoutes(r spi.Router) {
	p.Base.RegisterRoutes(r)
	r.Handle("/connect", baseworker.WSHandler(p.reg, p.mxreg, p.srvOpts.ClientKey, p.srvState))
	r.Group(func(g spi.Router) {
		if p.srvState != nil {
			g.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if p.srvState.IsDraining() {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"error":"draining"}`))
						return
					}
					next.ServeHTTP(w, r)
				})
			})
		}
		if p.authMW != nil {
			g.Use(p.authMW)
		}
		g.Route("/v1", func(v1 spi.Router) {
			timeout := p.srvOpts.RequestTimeout
			// Mount pass-through endpoints
			v1.Post("/convert/file", proxyHandler(p.reg, p.sch, p.mxreg, "/v1/convert/file", timeout))
			v1.Post("/convert/source", proxyHandler(p.reg, p.sch, p.mxreg, "/v1/convert/source", timeout))
		})
	})
}

func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) { basemetrics.Register(reg) }

func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: p.ID(), Data: func() any { return p.mxreg.Snapshot() }})
}

var _ spi.Plugin = (*Plugin)(nil)

func New(state spi.ServerState, version, sha, date string, srvOpts spi.Options, authMW spi.Middleware) *Plugin {
	reg := baseworker.NewRegistry()
	mx := baseworker.NewMetricsRegistry(version, sha, date, func() string { return "" })
	scorer := AlwaysEligibleScorer{}
	sch := baseworker.NewScoreScheduler(reg, scorer)
	// Start pruning expired workers in the background
	go func() {
		tick := srvOpts.AgentHeartbeatInterval
		if tick == 0 {
			tick = baseworker.HeartbeatInterval
		}
		expire := srvOpts.AgentHeartbeatExpiry
		if expire == 0 {
			expire = baseworker.HeartbeatExpiry
		}
		ticker := time.NewTicker(tick)
		for range ticker.C {
			reg.PruneExpired(expire)
			if state != nil && !state.IsDraining() && reg.WorkerCount() == 0 {
				state.SetStatus("not_ready")
			}
		}
	}()
	id := Descriptor().ID
	return &Plugin{Base: baseplugin.NewBase(Descriptor(), srvOpts.PluginOptions[id]), reg: reg, mxreg: mx, sch: sch, srvState: state, srvOpts: srvOpts, authMW: authMW}
}
