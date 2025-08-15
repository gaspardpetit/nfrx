package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/you/llamapool/internal/api"
	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
)

// New constructs the HTTP handler for the server.
func New(reg *ctrl.Registry, metrics *ctrl.MetricsRegistry, sched ctrl.Scheduler, cfg config.ServerConfig) http.Handler {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		r.Mount("/", api.NewRouter(reg, metrics, sched, cfg.RequestTimeout))
	})
	r.Route("/v1", func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		r.Get("/models", api.ListModelsHandler(reg))
		r.Get("/models/{id}", api.GetModelHandler(reg))
		r.Post("/chat/completions", api.ChatCompletionsHandler(reg, sched))
	})
	r.Handle(cfg.WSPath, ctrl.WSHandler(reg, metrics, cfg.WorkerKey))
	r.Get("/status", StatusHandler())
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			logx.Log.Error().Err(err).Msg("write healthz")
		}
	})
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
