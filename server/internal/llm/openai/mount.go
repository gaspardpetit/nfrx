package openai

import (
	"time"

	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	"github.com/go-chi/chi/v5"
)

func Mount(v1 chi.Router, reg *ctrlsrv.Registry, sched ctrlsrv.Scheduler, metrics *ctrlsrv.MetricsRegistry, timeout time.Duration, maxParallel int) {
	v1.Post("/chat/completions", ChatCompletionsHandler(reg, sched, metrics, timeout))
	v1.Post("/embeddings", EmbeddingsHandler(reg, sched, metrics, timeout, maxParallel))
	v1.Get("/models", ListModelsHandler(reg))
	v1.Get("/models/{id}", GetModelHandler(reg))
}
