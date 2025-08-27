package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/api/generated"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	mcpbroker "github.com/gaspardpetit/nfrx/server/internal/mcp/mcpbroker"
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

func (a *API) GetApiState(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics, MCP: a.MCP}).GetState(w, r)
}

func (a *API) GetApiStateStream(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics, MCP: a.MCP}).GetStateStream(w, r)
}

func (a *API) GetApiWorkersConnect(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
