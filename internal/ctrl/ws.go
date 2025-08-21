package ctrl

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/gaspardpetit/llamapool/internal/logx"
	"github.com/gaspardpetit/llamapool/internal/serverstate"
)

// WSHandler handles incoming client websocket connections.
func WSHandler(reg *Registry, metrics *MetricsRegistry, clientKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverstate.IsDraining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := r.Context()
		defer func() {
			_ = c.Close(websocket.StatusInternalError, "server error")
		}()

		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Type != "register" {
			_ = c.Close(websocket.StatusPolicyViolation, "expected register")
			return
		}
		var rm RegisterMessage
		if err := json.Unmarshal(data, &rm); err != nil {
			return
		}
		key := rm.ClientKey
		if key == "" && rm.Token != "" {
			logx.Log.Warn().Msg("register message 'token' field is deprecated; use 'client_key'")
			key = rm.Token
		}
		if clientKey == "" && key != "" {
			_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		if clientKey != "" && key != clientKey {
			_ = c.Close(websocket.StatusPolicyViolation, "unauthorized")
			return
		}
		name := rm.WorkerName
		if name == "" {
			if len(rm.WorkerID) >= 8 {
				name = rm.WorkerID[:8]
			} else if rm.WorkerID != "" {
				name = rm.WorkerID
			} else {
				name = strings.Split(r.RemoteAddr, ":")[0]
			}
		}
		wk := &Worker{
			ID:             rm.WorkerID,
			Name:           name,
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
		metrics.UpsertWorker(wk.ID, wk.Name, rm.Version, rm.BuildSHA, rm.BuildDate, rm.MaxConcurrency, rm.Models)
		status := StatusIdle
		if rm.MaxConcurrency == 0 {
			status = StatusNotReady
		}
		metrics.SetWorkerStatus(wk.ID, status)
		logx.Log.Info().Str("worker_id", wk.ID).Str("worker_name", wk.Name).Int("model_count", len(wk.Models)).Msg("registered")
		if reg.WorkerCount() == 1 {
			serverstate.SetState("ready")
		}
		defer func() {
			reg.Remove(wk.ID)
			metrics.RemoveWorker(wk.ID)
			if reg.WorkerCount() == 0 {
				serverstate.SetState("not_ready")
			}
		}()

		go func() {
			for msg := range wk.Send {
				b, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				if err := c.Write(ctx, websocket.MessageText, b); err != nil {
					return
				}
			}
		}()

		for {
			_, msg, err := c.Read(ctx)
			if err != nil {
				var ce websocket.CloseError
				if errors.As(err, &ce) {
					lvl := logx.Log.Info()
					if ce.Code != websocket.StatusNormalClosure {
						lvl = logx.Log.Error()
					}
					lvl.Str("worker_id", wk.ID).Str("worker_name", wk.Name).Str("reason", ce.Reason).Msg("disconnected")
				} else {
					logx.Log.Error().Err(err).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Msg("disconnected")
				}
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
				metrics.RecordHeartbeat(wk.ID)
			case "status_update":
				var m StatusUpdateMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					wk.MaxConcurrency = m.MaxConcurrency
					if m.Models != nil {
						wk.Models = map[string]bool{}
						for _, mm := range m.Models {
							wk.Models[mm] = true
						}
					}
					wk.mu.Unlock()
					metrics.UpdateWorker(wk.ID, m.MaxConcurrency, m.Models)
					if m.Status != "" {
						metrics.SetWorkerStatus(wk.ID, WorkerStatus(m.Status))
					}
				}
			case "job_chunk":
				var m JobChunkMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.JobID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "job_result":
				var m JobResultMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.JobID]
					if ok {
						delete(wk.Jobs, m.JobID)
					}
					wk.mu.Unlock()
					if ok {
						ch <- m
						close(ch)
					}
				}
			case "job_error":
				var m JobErrorMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.JobID]
					if ok {
						delete(wk.Jobs, m.JobID)
					}
					wk.mu.Unlock()
					if ok {
						ch <- m
						close(ch)
					}
				}
			case "http_proxy_response_headers":
				var m HTTPProxyResponseHeadersMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.RequestID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "http_proxy_response_chunk":
				var m HTTPProxyResponseChunkMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.RequestID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "http_proxy_response_end":
				var m HTTPProxyResponseEndMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.RequestID]
					if ok {
						delete(wk.Jobs, m.RequestID)
					}
					wk.mu.Unlock()
					if ok {
						ch <- m
						close(ch)
					}
				}
			}
		}
	}
}
