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
func WSHandler(reg *Registry, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := ""
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tok = strings.TrimPrefix(auth, "Bearer ")
		}
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if token != "" && tok != token {
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
