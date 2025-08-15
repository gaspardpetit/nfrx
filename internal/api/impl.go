package api

import (
	"net/http"
	"time"

	"github.com/gaspardpetit/llamapool/api/generated"
	"github.com/gaspardpetit/llamapool/internal/ctrl"
)

type API struct {
	Reg     *ctrl.Registry
	Metrics *ctrl.MetricsRegistry
	Sched   ctrl.Scheduler
	Timeout time.Duration
}

var _ generated.ServerInterface = (*API)(nil)

func (a *API) PostApiGenerate(w http.ResponseWriter, r *http.Request) {
	GenerateHandler(a.Reg, a.Metrics, a.Sched, a.Timeout)(w, r)
}

func (a *API) GetApiTags(w http.ResponseWriter, r *http.Request) {
	TagsHandler(a.Reg)(w, r)
}

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

func (a *API) GetV1State(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics}).GetState(w, r)
}

func (a *API) GetV1StateStream(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{Metrics: a.Metrics}).GetStateStream(w, r)
}
