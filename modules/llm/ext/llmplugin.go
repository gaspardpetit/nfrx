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
    srvOpts    spi.Options
    stateFn    func() any

    // build info for metrics registration
    version string
    sha     string
    date    string
}

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
            // Adapt shared options to OpenAI-specific options
            oa := openai.Options{RequestTimeout: p.srvOpts.RequestTimeout, MaxParallelEmbeddings: p.srvOpts.MaxParallelEmbeddings}
            openai.Mount(v1, p.wr, p.sch, p.mx, oa)
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
    reg.Add(spi.StateElement{ID: "llm", Data: sf, HTML: func() string {
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
    connect http.Handler,
    workers spi.WorkerRegistry,
    sched spi.Scheduler,
    metrics spi.Metrics,
    stateProvider func() any,
    version, sha, date string,
    srvOpts spi.Options,
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
        srvOpts:    srvOpts,
        stateFn:    stateProvider,
    }
}

// (compat constructor removed) — use New with spi.Options
