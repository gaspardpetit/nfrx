package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/you/llamapool/internal/ctrl"
)

// StateHandler serves state snapshots and streams.
type StateHandler struct{ Metrics *ctrl.MetricsRegistry }

// GetState returns a JSON snapshot of metrics.
func (h *StateHandler) GetState(w http.ResponseWriter, r *http.Request) {
	state := h.Metrics.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
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
			b, _ := json.Marshal(state)
			w.Write([]byte("data: "))
			w.Write(b)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
