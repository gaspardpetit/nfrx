package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/gaspardpetit/llamapool/api/generated"
	"github.com/gaspardpetit/llamapool/internal/api"
	"github.com/gaspardpetit/llamapool/internal/config"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

// New constructs the HTTP handler for the server.
func New(reg *ctrl.Registry, metrics *ctrl.MetricsRegistry, sched ctrl.Scheduler, cfg config.ServerConfig) http.Handler {
	r := chi.NewRouter()
	for _, m := range api.MiddlewareChain() {
		r.Use(m)
	}

	impl := &api.API{Reg: reg, Metrics: metrics, Sched: sched, Timeout: cfg.RequestTimeout}
	wrapper := generated.ServerInterfaceWrapper{Handler: impl}

	r.Route("/api/client", func(r chi.Router) {
		r.Get("/openapi.json", api.OpenAPIHandler())
		r.Get("/*", api.SwaggerHandler())
	})

	r.Group(func(public chi.Router) {
		public.Get("/healthz", wrapper.GetHealthz)
		public.Get("/status", StatusHandler())
	})

	r.Group(func(apiGroup chi.Router) {
		if cfg.APIKey != "" {
			apiGroup.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		apiGroup.Get("/api/state", wrapper.GetApiState)
		apiGroup.Get("/api/state/stream", wrapper.GetApiStateStream)
	})

	r.Group(func(openai chi.Router) {
		if cfg.APIKey != "" {
			openai.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		openai.Post("/v1/chat/completions", wrapper.PostV1ChatCompletions)
		openai.Get("/v1/models", wrapper.GetV1Models)
		openai.Get("/v1/models/{id}", wrapper.GetV1ModelsId)
	})
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
