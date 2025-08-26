package ctrlsrv

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coder/websocket"

	ctrl "github.com/gaspardpetit/nfrx-sdk/ctrl"
	"github.com/gaspardpetit/nfrx-sdk/logx"
	"github.com/gaspardpetit/nfrx-server/internal/serverstate"
)

// WSHandler handles incoming client websocket connections.
func WSHandler(reg *Registry, metrics *MetricsRegistry, clientKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverstate.IsDraining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		id := r.URL.Query().Get("id")
		key := r.URL.Query().Get("key")
		if clientKey != "" && key != clientKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		wk, ok := reg.Get(id)
		if !ok {
			http.Error(w, "unknown worker", http.StatusNotFound)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		ctx := r.Context()
		defer func() {
			reg.Remove(wk.ID)
			metrics.RemoveWorker(wk.ID)
			if reg.WorkerCount() == 0 {
				serverstate.SetState("not_ready")
			}
			_ = c.Close(websocket.StatusInternalError, "server error")
		}()
		logx.Log.Info().Str("worker_id", wk.ID).Str("worker_name", wk.Name).Msg("worker connected")

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
				var m ctrl.StatusUpdateMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					wk.MaxConcurrency = m.MaxConcurrency
					wk.EmbeddingBatchSize = m.EmbeddingBatchSize
					if m.Models != nil {
						wk.Models = map[string]bool{}
						for _, mm := range m.Models {
							wk.Models[mm] = true
						}
					}
					wk.mu.Unlock()
					metrics.UpdateWorker(wk.ID, m.MaxConcurrency, m.EmbeddingBatchSize, m.Models)
					if m.Status != "" {
						metrics.SetWorkerStatus(wk.ID, WorkerStatus(m.Status))
					}
				}
			case "job_chunk":
				var m ctrl.JobChunkMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.JobID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "job_result":
				var m ctrl.JobResultMessage
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
				var m ctrl.JobErrorMessage
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
				var m ctrl.HTTPProxyResponseHeadersMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.RequestID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "http_proxy_response_chunk":
				var m ctrl.HTTPProxyResponseChunkMessage
				if err := json.Unmarshal(msg, &m); err == nil {
					wk.mu.Lock()
					ch, ok := wk.Jobs[m.RequestID]
					wk.mu.Unlock()
					if ok {
						ch <- m
					}
				}
			case "http_proxy_response_end":
				var m ctrl.HTTPProxyResponseEndMessage
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
