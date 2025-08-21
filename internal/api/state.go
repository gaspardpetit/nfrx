package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/drain"
	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/mcp"
)

// StateHandler serves state snapshots and streams.
type StateHandler struct {
	Metrics *ctrl.MetricsRegistry
	MCP     *mcp.Registry
}

// GetState returns a JSON snapshot of metrics.
func (h *StateHandler) GetState(w http.ResponseWriter, r *http.Request) {
	state := h.Metrics.Snapshot()
	if h.MCP != nil {
		state.MCP = h.MCP.Snapshot()
	}
	if drain.IsDraining() {
		state.State = "draining"
	} else if state.WorkersSummary.Connected+state.WorkersSummary.Working+state.WorkersSummary.Idle > 0 {
		state.State = "ready"
	} else {
		state.State = "not_ready"
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(state); err != nil {
		// ignore write error but log
		logx.Log.Error().Err(err).Msg("encode state")
	}
}

// GetStateStream streams state snapshots as Server-Sent Events.
func (h *StateHandler) GetStateStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			state := h.Metrics.Snapshot()
			if h.MCP != nil {
				state.MCP = h.MCP.Snapshot()
			}
			if drain.IsDraining() {
				state.State = "draining"
			} else if state.WorkersSummary.Connected+state.WorkersSummary.Working+state.WorkersSummary.Idle > 0 {
				state.State = "ready"
			} else {
				state.State = "not_ready"
			}
			b, _ := json.Marshal(state)
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(b); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
