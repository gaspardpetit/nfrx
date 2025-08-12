package ctrl

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/you/llamapool/internal/logx"

	"nhooyr.io/websocket"
)

// WSHandler handles incoming worker websocket connections.
func WSHandler(reg *Registry, expectedKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			provided = strings.TrimPrefix(auth, "Bearer ")
		}
		if provided == "" {
			provided = r.URL.Query().Get("worker_key")
		}
		if provided == "" {
			if tok := r.URL.Query().Get("token"); tok != "" {
				logx.Log.Warn().Msg("token query param deprecated, use worker_key")
				provided = tok
			}
		}
		if expectedKey != "" && provided != expectedKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := r.Context()
		defer c.Close(websocket.StatusInternalError, "server error")

		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Type != "register" {
			c.Close(websocket.StatusPolicyViolation, "expected register")
			return
		}
		var rm RegisterMessage
		if err := json.Unmarshal(data, &rm); err != nil {
			return
		}
		if rm.WorkerKey == "" && rm.Token != "" {
			logx.Log.Warn().Msg("token field deprecated in register message, use worker_key")
			rm.WorkerKey = rm.Token
		}
		if expectedKey != "" && rm.WorkerKey != expectedKey {
			c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		wk := &Worker{
			ID:             rm.WorkerID,
			Models:         map[string]bool{},
			MaxConcurrency: rm.MaxConcurrency,
			InFlight:       0,
			LastHeartbeat:  time.Now(),
			Send:           make(chan interface{}, 32),
			Jobs:           make(map[string]chan interface{}),
		}
		for _, m := range rm.Models {
			wk.Models[m] = true
		}
		reg.Add(wk)
		logx.Log.Info().Str("worker_id", wk.ID).Str("remote_addr", r.RemoteAddr).Strs("models", rm.Models).Msg("registered")
		defer reg.Remove(wk.ID)

		go func() {
			for msg := range wk.Send {
				b, _ := json.Marshal(msg)
				c.Write(ctx, websocket.MessageText, b)
			}
		}()

		for {
			_, msg, err := c.Read(ctx)
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			switch env.Type {
			case "heartbeat":
				reg.UpdateHeartbeat(wk.ID)
			case "job_chunk":
				var m JobChunkMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					if ch, ok := wk.Jobs[m.JobID]; ok {
						ch <- m
					}
				}
			case "job_result":
				var m JobResultMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					if ch, ok := wk.Jobs[m.JobID]; ok {
						ch <- m
						close(ch)
						delete(wk.Jobs, m.JobID)
					}
				}
			case "job_error":
				var m JobErrorMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					if ch, ok := wk.Jobs[m.JobID]; ok {
						ch <- m
						close(ch)
						delete(wk.Jobs, m.JobID)
					}
				}
			}
		}
	}
}
