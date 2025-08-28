package server

import (
    "fmt"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/cors"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"

    "github.com/gaspardpetit/nfrx/api/generated"
    "github.com/gaspardpetit/nfrx/server/internal/adapters"
    "github.com/gaspardpetit/nfrx/server/internal/api"
    "github.com/gaspardpetit/nfrx/server/internal/config"
    "github.com/gaspardpetit/nfrx/server/internal/plugin"
    "github.com/gaspardpetit/nfrx/server/internal/serverstate"
    "github.com/gaspardpetit/nfrx/server/internal/metrics"
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
    preg := prometheus.NewRegistry()
    prometheus.DefaultRegisterer = preg
    prometheus.DefaultGatherer = preg
    // Register global collectors used by runtime metrics
    metrics.Register(plugin.PromAdapter{Registry: preg})
    plugin.Load(r, preg, adapters.NewStateRegistry(stateReg), plugins)

    impl := &api.API{StateReg: stateReg}
    wrapper := generated.ServerInterfaceWrapper{Handler: impl}

	r.Get("/healthz", wrapper.GetHealthz)
		r.Route("/api", func(ar chi.Router) {
		ar.Route("/client", func(cr chi.Router) {
			cr.Get("/openapi.json", api.OpenAPIHandler())
			cr.Get("/*", api.SwaggerHandler())
		})
		ar.Group(func(g chi.Router) {
			if cfg.APIKey != "" {
				g.Use(api.APIKeyMiddleware(cfg.APIKey))
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
