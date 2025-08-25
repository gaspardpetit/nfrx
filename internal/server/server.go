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
	"github.com/gaspardpetit/nfrx/internal/api"
	"github.com/gaspardpetit/nfrx/internal/config"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	mcpbroker "github.com/gaspardpetit/nfrx/internal/mcpbroker"
	"github.com/gaspardpetit/nfrx/internal/plugin"
	"github.com/gaspardpetit/nfrx/internal/serverstate"
)

// New constructs the HTTP handler for the server.
func New(reg *ctrlsrv.Registry, metrics *ctrlsrv.MetricsRegistry, sched ctrlsrv.Scheduler, mcpReg *mcpbroker.Registry, cfg config.ServerConfig, stateReg *serverstate.Registry, plugins []plugin.Plugin) http.Handler {
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
	preg, _ := prometheus.DefaultRegisterer.(*prometheus.Registry)
	plugin.Load(plugin.Context{Router: r, Metrics: preg, State: stateReg}, plugins)

	impl := &api.API{Reg: reg, Metrics: metrics, MCP: mcpReg, Sched: sched, Timeout: cfg.RequestTimeout, MaxParallelEmbeddings: cfg.MaxParallelEmbeddings}
	wrapper := generated.ServerInterfaceWrapper{Handler: impl}

	r.Route("/api/client", func(r chi.Router) {
		r.Get("/openapi.json", api.OpenAPIHandler())
		r.Get("/*", api.SwaggerHandler())
	})

	r.Group(func(public chi.Router) {
		public.Get("/healthz", wrapper.GetHealthz)
		public.Get("/state", StateHandler())
	})

	r.Route("/api", func(apiGroup chi.Router) {
		if cfg.APIKey != "" {
			apiGroup.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		apiGroup.Route("/v1", func(v1 chi.Router) {
			v1.Post("/chat/completions", wrapper.PostApiV1ChatCompletions)
			v1.Post("/embeddings", wrapper.PostApiV1Embeddings)
			v1.Get("/models", wrapper.GetApiV1Models)
			v1.Get("/models/{id}", wrapper.GetApiV1ModelsId)
		})
		apiGroup.Get("/state", wrapper.GetApiState)
		apiGroup.Get("/state/stream", wrapper.GetApiStateStream)
	})
	if mcpReg != nil {
		r.Post("/api/mcp/id/{id}", mcpReg.HTTPHandler())
		r.Handle("/api/mcp/connect", mcpReg.WSHandler(cfg.ClientKey))
	}
	r.Handle("/api/workers/connect", ctrlsrv.WSHandler(reg, metrics, cfg.ClientKey))

	if cfg.MetricsAddr == fmt.Sprintf(":%d", cfg.Port) {
		r.Handle("/metrics", promhttp.Handler())
	}

	go func() {
		ticker := time.NewTicker(ctrlsrv.HeartbeatInterval)
		for range ticker.C {
			reg.PruneExpired(ctrlsrv.HeartbeatExpiry)
		}
	}()

	return r
}
