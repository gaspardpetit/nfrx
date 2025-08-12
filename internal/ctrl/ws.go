package ctrl

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/you/llamapool/internal/logx"

	"nhooyr.io/websocket"
)

// WSHandler handles incoming worker websocket connections.
func WSHandler(reg *Registry, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		if rm.Token != token {
			c.Close(websocket.StatusPolicyViolation, "bad token")
			return
		}
		wk := &Worker{
			ID:             rm.WorkerID,
			Models:         map[string]bool{},
			MaxConcurrency: rm.MaxConcurrency,
			InFlight:       0,
			LastHeartbeat:  time.Now(),
			Send:           make(chan interface{}, 16),
			Jobs:           make(map[string]chan interface{}),
		}
		for _, m := range rm.Models {
			wk.Models[m] = true
		}
		reg.Add(wk)
		logx.Log.Info().Str("worker", wk.ID).Msg("registered")
		defer func() {
			reg.Remove(wk.ID)
			for _, ch := range wk.Jobs {
				close(ch)
			}
			close(wk.Send)
		}()

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
