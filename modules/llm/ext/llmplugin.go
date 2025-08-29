package llm

import (
    "net/http"
    "time"

    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    baseplugin "github.com/gaspardpetit/nfrx/sdk/base/plugin"
    baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
    opt "github.com/gaspardpetit/nfrx/core/options"
    "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
    llmmetrics "github.com/gaspardpetit/nfrx/modules/llm/ext/metrics"
    llmadapt "github.com/gaspardpetit/nfrx/modules/llm/ext/adapters"
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
    authMW  spi.Middleware
    srvOpts spi.Options
}

// RegisterRoutes wires the HTTP endpoints.
func (p *Plugin) RegisterRoutes(r spi.Router) {
    // Register base route (501) at "/api/llm/" and then mount specific endpoints
    p.Base.RegisterRoutes(r)
    // Mount LLM worker connect endpoint owned by the extension
    r.Handle("/connect", baseworker.WSHandler(p.reg, p.mxreg, p.srvOpts.ClientKey, p.srvState))
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
        if p.authMW != nil { g.Use(p.authMW) }
        g.Route("/v1", func(v1 spi.Router) {
            // Adapt shared options to OpenAI-specific options; allow plugin option override for embeddings
            mpe := opt.Int(p.srvOpts.PluginOptions, "llm", "max_parallel_embeddings", 8)
            oa := openai.Options{RequestTimeout: p.srvOpts.RequestTimeout, MaxParallelEmbeddings: mpe}
            // Adapt internal control plane to SPI
            wr := llmadapt.NewWorkerRegistry(p.reg)
            sch := llmadapt.NewScheduler(p.sch)
            mx := llmadapt.NewMetrics(p.mxreg)
            openai.Mount(v1, wr, sch, mx, oa)
        })
    })
}

// Scheduler returns the plugin's scheduler.
func (p *Plugin) Scheduler() spi.Scheduler { return llmadapt.NewScheduler(p.sch) }

// RegisterMetrics registers LLM extension metrics collectors.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {
    llmmetrics.Register(reg)
}

// RegisterState registers state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
    reg.Add(spi.StateElement{ID: "llm", Data: func() any { return p.mxreg.Snapshot() }, HTML: func() string {
        return `
<div class="llm-view">
  <div class="llm-workers"></div>
  <script>(function(){
    function statusColor(w){
      if (w.last_error) return 'red';
      if (Date.now() - new Date(w.last_heartbeat).getTime() > 15000) return 'orange';
      if (w.inflight > 0) return 'gold';
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
        var busy=Math.min(1, (w.inflight + w.queue_len) / (w.max_concurrency || 1));
        div.innerHTML=
          '<div class="busy-bar"><div class="fill" style="height:'+Math.round(busy*100)+'%"></div></div>'+
          '<div class="emoji">🦙</div>'+
          '<div class="name"><span class="status-dot" style="background:'+status+'"></span>'+(w.name||w.id)+'</div>'+
          '<div>'+w.status+'</div>'+
          '<div>inflight: '+w.inflight+'</div>'+
          '<div>embed batch: '+w.embedding_batch_size+'</div>'+
          '<div>tokens in/out: '+(w.tokens_in_total||0)+'/'+(w.tokens_out_total||0)+'</div>'+
          '<div>total tokens: '+((w.tokens_total)||((w.tokens_in_total||0)+(w.tokens_out_total||0)))+'</div>'+
          '<div>avg rate: '+(((w.avg_tokens_per_second)||0).toFixed? (w.avg_tokens_per_second).toFixed(2): w.avg_tokens_per_second)+' tok/s</div>'+
          '<div>embeddings: '+(w.embeddings_total||0)+'</div>'+
          '<div>avg embed rate: '+(((w.avg_embeddings_per_second)||0).toFixed? (w.avg_embeddings_per_second).toFixed(2): w.avg_embeddings_per_second)+' emb/s</div>';
        host.appendChild(div);
      });
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
    sch := &baseworker.LeastBusyScheduler{Reg: reg}
    // Start pruning expired workers in the background
    go func() {
        tick := srvOpts.AgentHeartbeatInterval
        if tick == 0 { tick = baseworker.HeartbeatInterval }
        expire := srvOpts.AgentHeartbeatExpiry
        if expire == 0 { expire = baseworker.HeartbeatExpiry }
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
    return &Plugin{ Base: baseplugin.NewBase("llm"), reg: reg, mxreg: mx, sch: sch, authMW: authMW, srvOpts: srvOpts, srvState: state }
}

// (compat constructor removed) — use New with spi.Options
