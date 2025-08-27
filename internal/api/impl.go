package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/api/generated"
	ctrlsrv "github.com/gaspardpetit/nfrx/internal/ctrlsrv"
	openai "github.com/gaspardpetit/nfrx/modules/llm/ext/openai"
	mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
)

type HealthChecker interface {
	Healthy() bool
}

type API struct {
	Reg                   *ctrlsrv.Registry
	Metrics               *ctrlsrv.MetricsRegistry
	MCP                   *mcpbroker.Registry
	Sched                 ctrlsrv.Scheduler
	Timeout               time.Duration
	MaxParallelEmbeddings int
	Health                HealthChecker
}

var _ generated.ServerInterface = (*API)(nil)

func (a *API) GetHealthz(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	code := http.StatusOK
	if a.Health != nil && !a.Health.Healthy() {
		status = "unavailable"
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `{"status":"%s"}`, status)
}

func (a *API) PostApiV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	openai.ChatCompletionsHandler(a.Reg, a.Sched, a.Metrics, a.Timeout)(w, r)
}

func (a *API) PostApiV1Embeddings(w http.ResponseWriter, r *http.Request) {
	openai.EmbeddingsHandler(a.Reg, a.Sched, a.Metrics, a.Timeout, a.MaxParallelEmbeddings)(w, r)
}

func (a *API) GetApiV1Models(w http.ResponseWriter, r *http.Request) {
	openai.ListModelsHandler(a.Reg)(w, r)
}

func (a *API) GetApiV1ModelsId(w http.ResponseWriter, r *http.Request, id string) {
	openai.GetModelHandler(a.Reg)(w, r)
}

func (a *API) GetApiState(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics, MCP: a.MCP}).GetState(w, r)
}

func (a *API) GetApiStateStream(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics, MCP: a.MCP}).GetStateStream(w, r)
}

func (a *API) GetApiWorkersConnect(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
