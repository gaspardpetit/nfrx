package openai

import (
	"github.com/gaspardpetit/nfrx-sdk/spi"
)

func Mount(v1 spi.Router, reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options) {
	v1.Post("/chat/completions", ChatCompletionsHandler(reg, sched, metrics, opts.RequestTimeout))
	v1.Post("/embeddings", EmbeddingsHandler(reg, sched, metrics, opts.RequestTimeout, opts.MaxParallelEmbeddings))
	v1.Get("/models", ListModelsHandler(reg))
	v1.Get("/models/{id}", GetModelHandler(reg))
}
