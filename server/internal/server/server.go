package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gaspardpetit/nfrx/api/generated"
	baseauth "github.com/gaspardpetit/nfrx/sdk/base/auth"
	"github.com/gaspardpetit/nfrx/sdk/base/inflight"
	"github.com/gaspardpetit/nfrx/server/internal/adapters"
	"github.com/gaspardpetit/nfrx/server/internal/api"
	"github.com/gaspardpetit/nfrx/server/internal/config"
	"github.com/gaspardpetit/nfrx/server/internal/jobs"
	"github.com/gaspardpetit/nfrx/server/internal/metrics"
	"github.com/gaspardpetit/nfrx/server/internal/plugin"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
	"github.com/gaspardpetit/nfrx/server/internal/transfer"
)

// New constructs the HTTP handler for the server.
func New(cfg config.ServerConfig, stateReg *serverstate.Registry, plugins []plugin.Plugin) http.Handler {
	r := chi.NewRouter()
	if len(cfg.AllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins: cfg.AllowedOrigins,
			AllowedMethods: []string{"GET", "POST", "OPTIONS"},
			AllowedHeaders: []string{"*"},
		}))
	}
	for _, m := range api.MiddlewareChain() {
		r.Use(m)
	}

	if stateReg == nil {
		stateReg = serverstate.NewRegistry()
	}
	stateReg.Add(serverstate.Element{
		ID: "server",
		Data: func() interface{} {
			return map[string]any{
				"drainable_inflight": inflight.DrainableCount(),
			}
		},
		HTML: func() string {
			return `
<div class="server-view">
  <div class="server-metric">drainable inflight: <span class="drainable-count">0</span></div>
  <script>(function(){
    function render(state, container){
      var el = container.querySelector('.drainable-count');
      if (!el) return;
      var n = 0;
      if (state) {
        if (state.drainable_inflight != null) n = state.drainable_inflight;
        else if (state.DrainableInflight != null) n = state.DrainableInflight;
      }
      el.textContent = n;
    }
    if (!window.NFRX) window.NFRX = { _renderers:{}, registerRenderer:function(id,fn){ this._renderers[id]=fn; } };
    var section = (document.currentScript && document.currentScript.closest('section')) || null;
    var id = (section && section.dataset && section.dataset.pluginId) || 'server';
    window.NFRX.registerRenderer(id, function(state, container){ render(state, container); });
  })();</script>
</div>`
		},
	})
	preg := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = preg
	prometheus.DefaultGatherer = preg
	// Register global collectors used by runtime metrics
	metrics.Register(plugin.PromAdapter{Registry: preg})
	plugin.Load(r, preg, adapters.NewStateRegistry(stateReg), plugins)

	impl := &api.API{StateReg: stateReg}
	wrapper := generated.ServerInterfaceWrapper{Handler: impl}
	transferReg := transfer.NewRegistry(60 * time.Second)
	jobReg := jobs.NewRegistry(transferReg, cfg.JobsSSECloseDelay, cfg.JobsClientTTL)

	r.Get("/healthz", wrapper.GetHealthz)
	r.Route("/api", func(ar chi.Router) {
		ar.Route("/client", func(cr chi.Router) {
			cr.Get("/openapi.json", api.OpenAPIHandler())
			cr.Get("/*", api.SwaggerHandler())
		})
		ar.Route("/transfer", func(tr chi.Router) {
			roles := append([]string{}, cfg.APIHTTPRoles...)
			roles = append(roles, cfg.ClientHTTPRoles...)
			secrets := make([]string, 0, 2)
			if cfg.APIKey != "" {
				secrets = append(secrets, cfg.APIKey)
			}
			if cfg.ClientKey != "" && cfg.ClientKey != cfg.APIKey {
				secrets = append(secrets, cfg.ClientKey)
			}
			tr.Use(baseauth.BearerAnyOrRolesMiddleware(secrets, roles))
			tr.Post("/", transferReg.HandleCreate)
			tr.Get("/{channel_id}", func(w http.ResponseWriter, r *http.Request) {
				transferReg.HandleReader(w, r, chi.URLParam(r, "channel_id"))
			})
			tr.Post("/{channel_id}", func(w http.ResponseWriter, r *http.Request) {
				transferReg.HandleWriter(w, r, chi.URLParam(r, "channel_id"))
			})
		})
		ar.Route("/", func(jr chi.Router) {
			clientRoles := append([]string{}, cfg.APIHTTPRoles...)
			clientSecrets := make([]string, 0, 1)
			if cfg.APIKey != "" {
				clientSecrets = append(clientSecrets, cfg.APIKey)
			}
			jr.Group(func(cr chi.Router) {
				cr.Use(baseauth.BearerAnyOrRolesMiddleware(clientSecrets, clientRoles))
				jobReg.RegisterClientRoutes(cr)
			})

			workerRoles := append([]string{}, cfg.ClientHTTPRoles...)
			workerSecrets := make([]string, 0, 1)
			if cfg.ClientKey != "" {
				workerSecrets = append(workerSecrets, cfg.ClientKey)
			}
			jr.Group(func(wr chi.Router) {
				wr.Use(baseauth.BearerAnyOrRolesMiddleware(workerSecrets, workerRoles))
				jobReg.RegisterWorkerRoutes(wr)
			})
		})
		ar.Group(func(g chi.Router) {
			if cfg.APIKey != "" || len(cfg.APIHTTPRoles) > 0 {
				g.Use(api.APIAuthMiddleware(cfg.APIKey, cfg.APIHTTPRoles))
			}
			g.Get("/state", wrapper.GetApiState)
			g.Get("/state/stream", wrapper.GetApiStateStream)
			g.Get("/state/view/{id}.html", StateViewHTML(stateReg))
			g.Get("/state/descriptors", StateDescriptors())
		})
	})

	r.Get("/state", StatePageHandler())

	if cfg.MetricsAddr == fmt.Sprintf(":%d", cfg.Port) {
		r.Handle("/metrics", promhttp.HandlerFor(preg, promhttp.HandlerOpts{}))
	}

	return r
}
