package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/api/generated"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

type HealthChecker interface {
	Healthy() bool
}

type API struct {
	Timeout  time.Duration
	Health   HealthChecker
	StateReg *serverstate.Registry
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
	(&StateHandler{State: a.StateReg}).GetState(w, r)
}

func (a *API) GetApiStateStream(w http.ResponseWriter, r *http.Request) {
	(&StateHandler{State: a.StateReg}).GetStateStream(w, r)
}
