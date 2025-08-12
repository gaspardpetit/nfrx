package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/relay"
)

// GenerateHandler handles POST /api/generate.
func GenerateHandler(reg *ctrl.Registry, sched ctrl.Scheduler, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req relay.GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if req.Stream {
			w.Header().Set("Content-Type", "application/json")
			if err := relay.RelayGenerateStream(ctx, reg, sched, req, w); err != nil {
				handleRelayErr(w, err)
			}
			return
		}
		res, err := relay.RelayGenerateOnce(ctx, reg, sched, req)
		if err != nil {
			handleRelayErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	}
}

func handleRelayErr(w http.ResponseWriter, err error) {
	if errors.Is(err, relay.ErrNoWorker) {
		http.Error(w, "no worker", http.StatusNotFound)
	} else if errors.Is(err, context.DeadlineExceeded) {
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	} else if err != nil {
		http.Error(w, "worker failure", http.StatusBadGateway)
	}
}
