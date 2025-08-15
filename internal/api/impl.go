package api

import (
	"net/http"
	"time"

	"github.com/you/llamapool/api/generated"
	"github.com/you/llamapool/internal/ctrl"
)

type API struct {
	Reg     *ctrl.Registry
	Metrics *ctrl.MetricsRegistry
	Sched   ctrl.Scheduler
	Timeout time.Duration
}

var _ generated.ServerInterface = (*API)(nil)

func (a *API) GetHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (a *API) PostV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	ChatCompletionsHandler(a.Reg, a.Sched)(w, r)
}

func (a *API) GetV1Models(w http.ResponseWriter, r *http.Request) {
	ListModelsHandler(a.Reg)(w, r)
}

func (a *API) GetV1ModelsId(w http.ResponseWriter, r *http.Request, id string) {
	GetModelHandler(a.Reg)(w, r)
}

func (a *API) GetApiState(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics}).GetState(w, r)
}

func (a *API) GetApiStateStream(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics}).GetStateStream(w, r)
}

func (a *API) GetApiWorkersConnect(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}
