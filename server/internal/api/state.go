package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gaspardpetit/nfrx/core/logx"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

// StateHandler serves state snapshots and streams.
type StateHandler struct {
	// Aggregated plugin state from serverstate.Registry
	State *serverstate.Registry
}

// PluginsEnvelope is the generic, plugin-agnostic state payload.
type PluginsEnvelope struct {
	Plugins map[string]any `json:"plugins"`
}

// GetState returns a JSON snapshot of metrics.
func (h *StateHandler) GetState(w http.ResponseWriter, r *http.Request) {
	env := PluginsEnvelope{Plugins: map[string]any{}}
	if h.State != nil {
		for _, el := range h.State.Elements() {
			if el.Data != nil {
				env.Plugins[el.ID] = el.Data()
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(env); err != nil {
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
			// client closed stream
			logx.Log.Debug().Msg("state stream closed")
			return
		case <-ticker.C:
			env := PluginsEnvelope{Plugins: map[string]any{}}
			if h.State != nil {
				for _, el := range h.State.Elements() {
					if el.Data != nil {
						env.Plugins[el.ID] = el.Data()
					}
				}
			}
			b, _ := json.Marshal(env)
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
