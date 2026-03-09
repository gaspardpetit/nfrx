package openai

import (
	"net/http"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

// ResponsesHandler handles POST /api/llm/v1/responses with the same dispatch and queue semantics as chat completions.
func ResponsesHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options, queue *CompletionQueue) http.HandlerFunc {
	return generationProxyHandler(reg, sched, metrics, opts, queue, generationProxySpec{
		endpointPath:      "/responses",
		operationName:     "llm.response",
		queueStatusWriter: queueStatusWriter,
	})
}
