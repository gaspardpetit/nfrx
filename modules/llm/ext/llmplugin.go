package llm

import (
	"net/http"
	"time"

	opt "github.com/gaspardpetit/nfrx/core/options"
	llmadapt "github.com/gaspardpetit/nfrx/modules/llm/ext/adapters"
	"github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"github.com/gaspardpetit/nfrx/sdk/base/inflight"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	baseplugin "github.com/gaspardpetit/nfrx/sdk/base/plugin"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// Plugin implements the llm subsystem as a plugin.
type Plugin struct {
	baseplugin.Base
	// internal control plane
	reg   *baseworker.Registry
	mxreg *baseworker.MetricsRegistry
	sch   baseworker.Scheduler

	// dependency-injected server state + options
	srvState spi.ServerState
	authMW   spi.Middleware
	srvOpts  spi.Options
}

// RegisterRoutes wires the HTTP endpoints.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	// Register base descriptor endpoint at "/api/llm/" and then mount specific endpoints
	p.Base.RegisterRoutes(r)
	// Mount LLM worker connect endpoint owned by the extension
	r.Handle("/connect", baseworker.WSHandler(p.reg, p.mxreg, p.srvOpts.ClientKey, p.srvState, p.srvOpts.ClientHTTPRoles...))
	r.Group(func(g spi.Router) {
		// During server drain, reject new public API requests for this extension.
		if p.srvState != nil {
			g.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if p.srvState != nil && p.srvState.IsDraining() {
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
		g.Use(inflight.DrainableMiddleware())
		// Adapt shared options to OpenAI-specific options
		mpe := opt.Int(p.srvOpts.PluginOptions, p.ID(), "max_parallel_embeddings", 8)
		qsz := opt.Int(p.srvOpts.PluginOptions, p.ID(), "queue_size", 100)
		qus := opt.Int(p.srvOpts.PluginOptions, p.ID(), "queue_update_seconds", 10)
		oa := openai.Options{RequestTimeout: p.srvOpts.RequestTimeout, MaxParallelEmbeddings: mpe, QueueSize: qsz, QueueUpdateSeconds: qus}
		// Adapt internal control plane to SPI
		wr := llmadapt.NewWorkerRegistry(p.reg)
		sch := llmadapt.NewScheduler(p.sch)
		mx := llmadapt.NewMetrics(p.mxreg)
		// Construct a global completion queue for chat requests and surface capacity in state
		var cq *openai.CompletionQueue
		if qsz > 0 {
			cq = openai.NewCompletionQueue(p.mxreg, qsz)
		} else {
			// still set capacity so UI reflects disabled queue
			p.mxreg.SetSchedulerQueueCapacity(0)
		}
		g.Route("/v1", func(v1 spi.Router) {
			openai.Mount(v1, wr, sch, mx, oa, cq)
		})
		g.Route("/id/{id}/v1", func(v1 spi.Router) {
			openai.MountTargeted(v1, wr, mx, oa, cq)
		})
	})
}

// Scheduler returns the plugin's scheduler.
func (p *Plugin) Scheduler() spi.Scheduler { return llmadapt.NewScheduler(p.sch) }

// RegisterMetrics registers LLM extension metrics collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {
	// Register generic request-* metrics
	basemetrics.Register(reg)
}

// RegisterState registers state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: p.ID(), Data: func() any { return p.mxreg.Snapshot() }, HTML: func() string {
		return `
<div class="llm-view">
  <div class="llm-workers"></div>
  <script>(function(){
    function statusColor(w){
      var lastHb = w.last_heartbeat || w.LastHeartbeat;
      var inflight = (w.inflight!=null? w.inflight : (w.Inflight||0));
      if (w.last_error || w.LastError) return 'red';
      if (lastHb && (Date.now() - new Date(lastHb).getTime() > 15000)) return 'orange';
      if (inflight > 0) return 'gold';
      return 'green';
    }
    function sortWorkers(list, sortBy){
      return list.slice().sort(function(a,b){
        switch (sortBy){
          case 'youngest': return new Date(b.connected_at) - new Date(a.connected_at);
          case 'busyness': {
            var ba = (a.inflight + a.queue_len) / (a.max_concurrency || 1);
            var bb = (b.inflight + b.queue_len) / (b.max_concurrency || 1);
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
      var host = container.querySelector('.llm-workers');
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
        var rawStatus=(w.status || '').toLowerCase();
        var inflight=(w.inflight||0);
        var qlen=(w.queue_len||0);
        var maxc=(w.max_concurrency||1);
        var busy=Math.min(1, (inflight + qlen) / (maxc || 1));
        var name=(w.name || w.id || 'worker');
        var avg=(w.avg_processing_ms||0);
        var avgText=(avg && avg.toFixed)? avg.toFixed(0) : avg;
        var processed=(w.processed_total||0);
        var hostInfo=(w.host_info || {});
        var cpu=(typeof w.host_cpu_percent === 'number') ? w.host_cpu_percent.toFixed(1) : '';
        var ram=(typeof w.host_ram_used_percent === 'number') ? w.host_ram_used_percent.toFixed(1) : '';
        var inputTokens=(w.input_tokens_total||0);
        var outputTokens=(w.output_tokens_total||0);
        var versionParts=['client: '+(hostInfo.worker_version || 'unknown')];
        if (hostInfo.backend_version) {
          versionParts.push((hostInfo.backend_family || 'backend')+': '+hostInfo.backend_version);
        }
        var versionText=versionParts.join(' / ');
        var hostParts=[hostInfo.hostname, hostInfo.os_name, hostInfo.os_version].filter(Boolean);
        var hostText=hostParts.length ? hostParts.join(' / ') : 'unknown';
        var workerID=(w.id || '');
        var workerBaseURL=new URL('/api/llm/id/'+encodeURIComponent(workerID)+'/v1', window.location.origin).toString();
        var workerModelsURL=workerBaseURL+'/models';
        var lastHb=(w.last_heartbeat || w.LastHeartbeat);
        var stale=lastHb && (Date.now() - new Date(lastHb).getTime() > 15000);
        var statusBadge='';
        if (w.last_error || w.LastError) {
          statusBadge='<span class="worker-status-badge error">error</span>';
        } else if (stale || rawStatus === 'gone') {
          statusBadge='<span class="worker-status-badge warn">stale</span>';
        } else if (rawStatus === 'draining') {
          statusBadge='<span class="worker-status-badge warn">draining</span>';
        } else if (rawStatus === 'not_ready' || rawStatus === 'connected') {
          statusBadge='<span class="worker-status-badge warn">'+rawStatus.replace('_', ' ')+'</span>';
        } else if (inflight > 0 || qlen > 0 || rawStatus === 'working') {
          statusBadge='<span class="worker-status-badge busy"><span class="worker-status-spinner"></span>busy</span>';
        } else if (rawStatus === 'idle' || !rawStatus) {
          statusBadge='<span class="worker-status-badge">idle</span>';
        }
        div.innerHTML=
          '<div class="busy-bar"><div class="fill" style="height:'+Math.round(busy*100)+'%"></div></div>'+
          '<div class="worker-head">'+
            '<div class="worker-head-main">'+
              '<div class="name"><span class="emoji">🦙</span><span class="status-dot" style="background:'+status+'"></span><span class="name-text">'+name+'</span>'+statusBadge+'</div>'+
            '</div>'+
            '<div class="worker-meta">'+
              '<div class="worker-version">'+versionText+'</div>'+
              '<div class="worker-hostline">'+hostText+'</div>'+
            '</div>'+
          '</div>'+
          '<div class="worker-metrics">'+
            '<div class="metric"><div class="metric-label">Host CPU</div><div class="metric-value">'+(cpu || '0.0')+'%</div></div>'+
            '<div class="metric"><div class="metric-label">Host RAM</div><div class="metric-value">'+(ram || '0.0')+'%</div></div>'+
            '<div class="metric"><div class="metric-label">Tokens In</div><div class="metric-value">'+inputTokens+'</div></div>'+
            '<div class="metric"><div class="metric-label">Tokens Out</div><div class="metric-value">'+outputTokens+'</div></div>'+
          '</div>'+
          '<div class="worker-details">'+
            '<div class="detail"><div class="detail-label">Inflight</div><div class="detail-value"><strong>'+inflight+'</strong> / '+maxc+'</div></div>'+
            '<div class="detail"><div class="detail-label">Processed</div><div class="detail-value"><strong>'+processed+'</strong> total</div></div>'+
            '<div class="detail"><div class="detail-label">Avg Processing</div><div class="detail-value"><strong>'+avgText+'</strong> ms</div></div>'+
            '<div class="detail"><div class="detail-label">Embed Batch</div><div class="detail-value"><strong>'+(w.embedding_batch_size||0)+'</strong></div></div>'+
          '</div>'+
          '<div class="worker-links"><a href="#" class="copy-link" data-copy-text="'+workerBaseURL+'">'+workerID+' '+(window.copyIconSVG ? window.copyIconSVG() : '')+'</a><a href="'+workerModelsURL+'">models</a></div>';
        host.appendChild(div);
      });
      if (window.NFRX && typeof window.NFRX.bindCopyLinks === 'function') {
        window.NFRX.bindCopyLinks(host);
      }
    }
    if (!window.NFRX) window.NFRX = { _renderers:{}, registerRenderer:function(id,fn){ this._renderers[id]=fn; } };
    var section = (document.currentScript && document.currentScript.closest('section')) || null;
    var id = (section && section.dataset && section.dataset.pluginId) || 'llm';
    window.NFRX.registerRenderer(id, function(state, container, envelope){ render(state, container); });
  })();</script>
</div>`
	}})
}

var _ spi.Plugin = (*Plugin)(nil)
var _ spi.WorkerProvider = (*Plugin)(nil)

// New constructs a new LLM plugin using the common server options,
// adapting them to the underlying OpenAI-specific configuration.
func New(
	state spi.ServerState,
	version, sha, date string,
	srvOpts spi.Options,
	authMW spi.Middleware,
) *Plugin {
	reg := baseworker.NewRegistry()
	mx := baseworker.NewMetricsRegistry(version, sha, date, func() string { return "" })
	// Use LLM-specific scorer (exact model match, then alias fallback).
	// Read min_score from plugin options (default 0.01) to allow alias matches by default.
	minScore := opt.Float(srvOpts.PluginOptions, Descriptor().ID, "min_score", 0.01)
	sch := baseworker.NewScoreSchedulerWithMinScore(reg, NewLLMScorer(), minScore)
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
			// Prune expired workers and update readiness if pool becomes empty
			reg.PruneExpired(expire)
			if state != nil && !state.IsDraining() && reg.WorkerCount() == 0 {
				// No active workers remain and we're not draining: mark server not_ready
				state.SetStatus("not_ready")
			}
		}
	}()
	id := Descriptor().ID
	return &Plugin{Base: baseplugin.NewBase(Descriptor(), srvOpts.PluginOptions[id]), reg: reg, mxreg: mx, sch: sch, authMW: authMW, srvOpts: srvOpts, srvState: state}
}

// (compat constructor removed) — use New with spi.Options
