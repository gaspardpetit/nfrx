package openai

import (
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// Mount wires OpenAI-compatible endpoints. Requires a shared completion queue for chat requests.
func Mount(v1 spi.Router, reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options, queue *CompletionQueue) {
	v1.Post("/chat/completions", ChatCompletionsHandler(reg, sched, metrics, opts, queue))
	v1.Post("/embeddings", EmbeddingsHandler(reg, sched, metrics, opts.RequestTimeout, opts.MaxParallelEmbeddings))
	v1.Get("/models", ListModelsHandler(reg))
	v1.Get("/models/{id}", GetModelHandler(reg))
	// Keep state metrics in sync with queue capacity on mount.
	if queue != nil {
		if mx, ok := any(metrics).(interface{ RecordWorkerTokens(string, string, uint64) }); ok {
			_ = mx // keep linter happy; actual capacity is set by the queue constructor against the base registry
		}
		// No-op here; capacity is set by the plugin when constructing the queue against base metrics.
		_ = baseworker.StatusIdle
	}
}
