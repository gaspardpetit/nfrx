package api

import (
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/you/llamapool/internal/ctrl"
)

// NewRouter builds the API router.
func NewRouter(reg *ctrl.Registry, sched ctrl.Scheduler, timeout time.Duration) chi.Router {
	r := chi.NewRouter()
	for _, m := range middlewareChain() {
		r.Use(m)
	}
	r.Post("/generate", GenerateHandler(reg, sched, timeout))
	r.Get("/tags", TagsHandler(reg))
	return r
}
