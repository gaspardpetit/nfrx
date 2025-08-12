package server

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/you/llamapool/internal/api"
	"github.com/you/llamapool/internal/config"
	"github.com/you/llamapool/internal/ctrl"
)

// New constructs the HTTP handler for the server.
func New(reg *ctrl.Registry, sched ctrl.Scheduler, cfg config.ServerConfig) http.Handler {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		r.Mount("/", api.NewRouter(reg, sched, cfg.RequestTimeout))
	})
	r.Route("/v1", func(r chi.Router) {
		if cfg.APIKey != "" {
			r.Use(api.APIKeyMiddleware(cfg.APIKey))
		}
		r.Get("/models", api.ListModelsHandler(reg))
		r.Get("/models/{id}", api.GetModelHandler(reg))
	})
	r.Handle(cfg.WSPath, ctrl.WSHandler(reg, cfg.WorkerKey))
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	r.Handle("/metrics", promhttp.Handler())

	go func() {
		ticker := time.NewTicker(ctrl.HeartbeatInterval)
		for range ticker.C {
			reg.PruneExpired(ctrl.HeartbeatExpiry)
		}
	}()

	return r
}
