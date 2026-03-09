package openai

import (
	"net/http"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// ChatCompletionsHandler handles POST /api/llm/v1/chat/completions with optional pre-dispatch queueing.
func ChatCompletionsHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options, queue *CompletionQueue) http.HandlerFunc {
	return generationProxyHandler(reg, sched, metrics, opts, queue, generationProxySpec{
		endpointPath:      "/chat/completions",
		operationName:     "llm.completion",
		queueStatusWriter: queueStatusWriter,
	})
}
