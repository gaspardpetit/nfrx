package asr

import (
	"net/http"
	"time"

	opt "github.com/gaspardpetit/nfrx/core/options"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	baseplugin "github.com/gaspardpetit/nfrx/sdk/base/plugin"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// Plugin implements the ASR worker-style extension.
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
			v1.Post("/audio/transcriptions", transcribeHandler(p.reg, p.sch, p.mxreg, timeout))
		})
	})
}

func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) { basemetrics.Register(reg) }

func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: p.ID(), Data: func() any { return p.mxreg.Snapshot() }, HTML: func() string {
		return `
<div class="asr-view">
  <div class="asr-workers"></div>
  <script>(function(){
    function statusColor(w){
      var lastHb = w.last_heartbeat;
      var inflight = (w.inflight||0);
      if (w.last_error) return 'red';
      if (lastHb && (Date.now() - new Date(lastHb).getTime() > 15000)) return 'orange';
      if (inflight > 0) return 'gold';
      return 'green';
    }
    function sortWorkers(list, sortBy){
      return list.slice().sort(function(a,b){
        switch (sortBy){
          case 'youngest': return new Date(b.connected_at) - new Date(a.connected_at);
          case 'busyness': {
            var ba = ((a.inflight||0) + (a.queue_len||0)) / ((a.max_concurrency||1));
            var bb = ((b.inflight||0) + (b.queue_len||0)) / ((b.max_concurrency||1));
            return bb - ba;
          }
          case 'name': return (a.name||'').localeCompare(b.name||'');
          case 'completed': return (b.processed_total|0) - (a.processed_total|0);
          case 'errors': return (b.failures_total|0) - (a.failures_total|0);
          case 'oldest': default: return new Date(a.connected_at) - new Date(b.connected_at);
        }
      });
    }
    function render(state, container){
      var host = container.querySelector('.asr-workers');
      if (!host) return;
      var workers = (state && state.workers) || [];
      var sortSel = document.getElementById('sort');
      var sortBy = sortSel ? sortSel.value : 'oldest';
      var list = sortWorkers(workers, sortBy);
      host.innerHTML='';
      list.forEach(function(w){
        var div=document.createElement('div');
        div.className='worker';
        var status=statusColor(w);
        var inflight=(w.inflight||0);
        var qlen=(w.queue_len||0);
        var maxc=(w.max_concurrency||1);
        var busy=Math.min(1, (inflight + qlen) / (maxc || 1));
        var name=(w.name || w.id || 'worker');
        var avg=(w.avg_processing_ms||0);
        var avgText=(avg && avg.toFixed)? avg.toFixed(0) : avg;
        var processed=(w.processed_total||0);
        div.innerHTML=
          '<div class="busy-bar"><div class="fill" style="height:'+Math.round(busy*100)+'%"></div></div>'+
          '<div class="name"><span class="status-dot" style="background:'+status+'"></span>'+name+'</div>'+
          '<div>'+(w.status || '')+'</div>'+
          '<div>inflight: '+inflight+'</div>'+
          '<div>processed: '+processed+'</div>'+
          '<div>avg processing: '+avgText+' ms</div>';
        host.appendChild(div);
      });
    }
    if (!window.NFRX) window.NFRX = { _renderers:{}, registerRenderer:function(id,fn){ this._renderers[id]=fn; } };
    var section = (document.currentScript && document.currentScript.closest('section')) || null;
    var id = (section && section.dataset && section.dataset.pluginId) || 'asr';
    window.NFRX.registerRenderer(id, function(state, container, envelope){ render(state, container); });
  })();</script>
</div>`
	}})
}

var _ spi.Plugin = (*Plugin)(nil)

func New(state spi.ServerState, version, sha, date string, srvOpts spi.Options, authMW spi.Middleware) *Plugin {
	reg := baseworker.NewRegistry()
	mx := baseworker.NewMetricsRegistry(version, sha, date, func() string { return "" })
	minScore := opt.Float(srvOpts.PluginOptions, Descriptor().ID, "min_score", 0.01)
	sch := baseworker.NewScoreSchedulerWithMinScore(reg, NewASRScorer(), minScore)
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
