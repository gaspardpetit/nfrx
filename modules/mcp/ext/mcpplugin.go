package mcp

import (
	"net/http"
	"time"

	opt "github.com/gaspardpetit/nfrx/core/options"
	"github.com/gaspardpetit/nfrx/modules/mcp/ext/adapters"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	"github.com/gaspardpetit/nfrx/sdk/base/inflight"
	baseplugin "github.com/gaspardpetit/nfrx/sdk/base/plugin"
	"github.com/gaspardpetit/nfrx/sdk/base/tunnel"
	"github.com/go-chi/chi/v5"
)

// Plugin implements the MCP relay as a plugin.
type Plugin struct {
	baseplugin.Base
	reg       *tunnel.Registry
	srvOpts   spi.Options
	clientKey string
}

// New constructs a new MCP plugin using the common server options.
func New(
	state spi.ServerState,
	connect http.Handler,
	workers spi.WorkerRegistry,
	sched spi.Scheduler,
	metrics spi.Metrics,
	stateProvider func() any,
	version, sha, date string,
	opts spi.Options,
	authMW spi.Middleware,
) *Plugin {
	// Build broker config from plugin options if provided
	var cfg struct {
		MaxReqBytes             int64
		MaxRespBytes            int64
		Heartbeat, DeadAfter    time.Duration
		MaxConcurrencyPerClient int
	}
	po := opts.PluginOptions
	id := Descriptor().ID
	// Only set fields when plugin options are provided; leave zero to allow env defaults in broker
	cfg.MaxReqBytes = opt.Int64(po, id, "max_req_bytes", 10*1024*1024)
	cfg.MaxRespBytes = opt.Int64(po, id, "max_resp_bytes", 10*1024*1024)
	hb := opt.Int(po, id, "ws_heartbeat_ms", 15000)
	dd := opt.Int(po, id, "ws_dead_after_ms", 45000)
	cfg.Heartbeat = time.Duration(hb) * time.Millisecond
	cfg.DeadAfter = time.Duration(dd) * time.Millisecond
	cfg.MaxConcurrencyPerClient = opt.Int(po, id, "max_concurrency_per_client", 16)
	reg := tunnel.New(tunnel.Config{Heartbeat: cfg.Heartbeat, DeadAfter: cfg.DeadAfter, MaxConcurrencyPerClient: cfg.MaxConcurrencyPerClient}, func() bool { return state != nil && state.IsDraining() })
	return &Plugin{Base: baseplugin.NewBase(Descriptor(), opts.PluginOptions[id]), reg: reg, srvOpts: opts, clientKey: opts.ClientKey}
}

// RegisterRoutes registers HTTP routes; MCP uses relay endpoints only.
func (p *Plugin) RegisterRoutes(r spi.Router) {
	// Register base descriptor endpoint at "/api/mcp/" and then specific endpoints
	p.Base.RegisterRoutes(r)
	r.Handle("/connect", p.reg.WSHandler(p.clientKey, adapters.MCPRegisterDecoder, adapters.MCPReadLoop, p.srvOpts.ClientHTTPRoles...))
	getID := func(req *http.Request) string { return chi.URLParam(req, "id") }
	r.Group(func(g spi.Router) {
		g.Use(inflight.DrainableMiddleware())
		g.Handle("/id/{id}", p.reg.HTTPHandler("mcp", getID, adapters.MCPAdapter{}, p.srvOpts.RequestTimeout, opt.Int64(p.srvOpts.PluginOptions, p.ID(), "max_req_bytes", 10*1024*1024), opt.Int64(p.srvOpts.PluginOptions, p.ID(), "max_resp_bytes", 10*1024*1024)))
	})
}

// RegisterMetrics registers Prometheus collectors; MCP has none currently.
func (p *Plugin) RegisterMetrics(reg spi.MetricsRegistry) {}

// RegisterState registers MCP state elements.
func (p *Plugin) RegisterState(reg spi.StateRegistry) {
	reg.Add(spi.StateElement{ID: p.ID(), Data: func() any { return adapters.TunnelStateToMCP(p.reg.Snapshot()) }, HTML: func() string {
		return `
<div class="mcp-view">
  <div class="mcp-clients"></div>
  <div class="mcp-sessions"></div>
  <script>(function(){
    function render(state, container){
      var clientsHost = container.querySelector('.mcp-clients');
      var sessionsHost = container.querySelector('.mcp-sessions');
      if (!clientsHost || !sessionsHost) return;
      var clients = (state && state.clients) || [];
      var sessions = (state && state.sessions) || [];
      clientsHost.innerHTML = '';
      clients.forEach(function(c){
        var funcs = Object.entries(c.functions||{}).map(function(kv){ return kv[0]+':'+kv[1]; }).join(', ');
        var div = document.createElement('div');
        var name = c.name ? (' '+c.name) : '';
        div.textContent = c.id+name+' ('+c.status+') '+funcs;
        clientsHost.appendChild(div);
      });
      sessionsHost.innerHTML = '';
      sessions.forEach(function(s){
        var div = document.createElement('div');
        var dur = Math.round((s.duration_ms||0)/1000);
        div.textContent = s.id+' '+s.client_id+' '+s.method+' '+dur+'s';
        sessionsHost.appendChild(div);
      });
    }
    if (!window.NFRX) window.NFRX = { _renderers:{}, registerRenderer:function(id,fn){ this._renderers[id]=fn; } };
    var section = (document.currentScript && document.currentScript.closest('section')) || null;
    var id = (section && section.dataset && section.dataset.pluginId) || 'mcp';
    window.NFRX.registerRenderer(id, function(state, container){ render(state, container); });
  })();</script>
</div>`
	}})
}

// Registry exposes the underlying tunnel registry for tests.
func (p *Plugin) Registry() *tunnel.Registry { return p.reg }

var _ spi.Plugin = (*Plugin)(nil)
