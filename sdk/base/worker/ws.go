package worker

import (
    "encoding/json"
    "errors"
    "net/http"
    "strings"
    "time"

    "github.com/coder/websocket"
    "github.com/gaspardpetit/nfrx/core/logx"
    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    "strconv"
)

// WSHandler handles incoming client websocket connections for worker-style agents.
func WSHandler(reg *Registry, metrics *MetricsRegistry, clientKey string, state spi.ServerState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Reject new worker connections when server is draining
		if state != nil && state.IsDraining() {
			http.Error(w, "draining", http.StatusServiceUnavailable)
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			logx.Log.Error().Err(err).Str("remote", r.RemoteAddr).Msg("ws accept")
			return
		}
		ctx := r.Context()
		defer func() { _ = c.Close(websocket.StatusInternalError, "server error") }()

		_, data, err := c.Read(ctx)
		if err != nil {
			logx.Log.Error().Err(err).Str("remote", r.RemoteAddr).Msg("ws read register")
			return
		}
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &env); err != nil || env.Type != "register" {
			logx.Log.Warn().Err(err).Str("remote", r.RemoteAddr).Msg("ws invalid first message; expected register")
			_ = c.Close(websocket.StatusPolicyViolation, "expected register")
			return
		}
		var rm ctrl.RegisterMessage
		if err := json.Unmarshal(data, &rm); err != nil {
			logx.Log.Error().Err(err).Str("remote", r.RemoteAddr).Msg("ws decode register")
			return
		}
		key := rm.ClientKey
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
        prefBatch := 0
        if rm.AgentConfig != nil {
            if s, ok := rm.AgentConfig["embedding_batch_size"]; ok {
                if v, err := strconv.Atoi(s); err == nil { prefBatch = v }
            }
        }
        wk := &Worker{ID: rm.WorkerID, Name: name, Labels: map[string]bool{}, MaxConcurrency: rm.MaxConcurrency, PreferredBatchSize: prefBatch, InFlight: 0, LastHeartbeat: time.Now(), Send: make(chan interface{}, 32), Jobs: make(map[string]chan interface{})}
		for _, m := range rm.Models {
			wk.Labels[m] = true
		}
		// Add worker and update server readiness if this is the first one
		reg.Add(wk)
		if state != nil && reg.WorkerCount() == 1 {
			// First worker became available: mark server as ready
			state.SetStatus("ready")
		}
        metrics.UpsertWorker(wk.ID, wk.Name, rm.Version, rm.BuildSHA, rm.BuildDate, rm.MaxConcurrency, prefBatch, rm.Models)
		status := StatusIdle
		if rm.MaxConcurrency == 0 {
			status = StatusNotReady
		}
		metrics.SetWorkerStatus(wk.ID, status)
		logx.Log.Info().Str("worker_id", wk.ID).Str("worker_name", wk.Name).Int("label_count", len(wk.Labels)).Msg("registered")
		defer func() {
			reg.Remove(wk.ID)
			metrics.RemoveWorker(wk.ID)
			// If no workers remain and we're not draining, mark server not_ready
			if state != nil && !state.IsDraining() && reg.WorkerCount() == 0 {
				state.SetStatus("not_ready")
			}
		}()

		go func() {
			for msg := range wk.Send {
				b, err := json.Marshal(msg)
				if err != nil {
					continue
				}
				if err := c.Write(ctx, websocket.MessageText, b); err != nil {
					logx.Log.Error().Err(err).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Msg("ws write")
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
				logx.Log.Debug().Err(err).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Msg("ws decode message")
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
                prefBatch := 0
                if m.AgentConfig != nil {
                    if s, ok := m.AgentConfig["embedding_batch_size"]; ok {
                        if v, err := strconv.Atoi(s); err == nil { prefBatch = v }
                    }
                }
                wk.PreferredBatchSize = prefBatch
					if m.Models != nil {
						wk.Labels = map[string]bool{}
						for _, mm := range m.Models {
							wk.Labels[mm] = true
						}
					}
					wk.mu.Unlock()
                metrics.UpdateWorker(wk.ID, m.MaxConcurrency, prefBatch, m.Models)
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
			default:
				logx.Log.Debug().Str("type", env.Type).Str("worker_id", wk.ID).Msg("ws unknown message type")
			}
		}
	}
}
