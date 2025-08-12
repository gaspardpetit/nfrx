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
	r.Mount("/api", api.NewRouter(reg, sched, cfg.RequestTimeout))
	r.Handle(cfg.WSPath, ctrl.WSHandler(reg, cfg.WorkerToken))
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	r.Handle("/metrics", promhttp.Handler())

	go func() {
		ticker := time.NewTicker(ctrl.HeartbeatInterval)
		for range ticker.C {
			reg.PruneExpired(ctrl.HeartbeatExpiry)
		}
	}()

	return r
}
