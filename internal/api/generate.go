package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/relay"
)

// GenerateHandler handles POST /api/generate.
func GenerateHandler(reg *ctrl.Registry, metrics *ctrl.MetricsRegistry, sched ctrl.Scheduler, timeout time.Duration) http.HandlerFunc {
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
			w.Header().Set("Cache-Control", "no-store")
			if err := relay.RelayGenerateStream(ctx, reg, metrics, sched, req, w); err != nil {
				handleRelayErr(w, err)
			}
			return
		}
		res, err := relay.RelayGenerateOnce(ctx, reg, metrics, sched, req)
		if err != nil {
			handleRelayErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(res); err != nil {
			logx.Log.Error().Err(err).Msg("encode generate result")
		}
	}
}

func handleRelayErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, relay.ErrNoWorker):
		logx.Log.Warn().Err(err).Msg("no worker")
		http.Error(w, "no worker", http.StatusNotFound)
	case errors.Is(err, relay.ErrWorkerBusy):
		logx.Log.Warn().Err(err).Msg("worker busy")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "worker_busy"}); err != nil {
			logx.Log.Error().Err(err).Msg("encode worker busy")
		}
	case errors.Is(err, context.DeadlineExceeded):
		logx.Log.Warn().Err(err).Msg("timeout")
		http.Error(w, "timeout", http.StatusGatewayTimeout)
	default:
		logx.Log.Error().Err(err).Msg("worker failure")
		http.Error(w, "worker failure", http.StatusBadGateway)
	}
}
