package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/modules/common/logx"
	mcpbroker "github.com/gaspardpetit/nfrx/modules/mcp/ext/mcpbroker"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
)

// StateHandler serves state snapshots and streams.
type StateHandler struct {
	Metrics *ctrlsrv.MetricsRegistry
	MCP     *mcpbroker.Registry
}

// GetState returns a JSON snapshot of metrics.
func (h *StateHandler) GetState(w http.ResponseWriter, r *http.Request) {
	state := h.Metrics.Snapshot()
	if h.MCP != nil {
		state.MCP = h.MCP.Snapshot()
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
