package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gaspardpetit/llamapool/api/generated"
	"github.com/gaspardpetit/llamapool/internal/api"
	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/mcp"
)

// New constructs the HTTP handler for the server.
func New(reg *ctrl.Registry, metrics *ctrl.MetricsRegistry, sched ctrl.Scheduler, mcpReg *mcp.Registry, cfg config.ServerConfig) http.Handler {
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

	impl := &api.API{Reg: reg, Metrics: metrics, MCP: mcpReg, Sched: sched, Timeout: cfg.RequestTimeout}
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
		r.Handle("/api/mcp/connect", mcpReg.WSHandler())
	}
	r.Handle("/api/workers/connect", ctrl.WSHandler(reg, metrics, cfg.WorkerKey))

	metricsPort := cfg.MetricsPort
	if metricsPort == 0 {
		metricsPort = cfg.Port
	}
	if metricsPort == cfg.Port {
		r.Handle("/metrics", promhttp.Handler())
	}

	go func() {
		ticker := time.NewTicker(ctrl.HeartbeatInterval)
		for range ticker.C {
			reg.PruneExpired(ctrl.HeartbeatExpiry)
		}
	}()

	return r
}
