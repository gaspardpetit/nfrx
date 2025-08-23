package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/gaspardpetit/infero/internal/ctrl"
	"github.com/gaspardpetit/infero/internal/logx"
	"github.com/gaspardpetit/infero/internal/metrics"
	"github.com/gaspardpetit/infero/internal/serverstate"
)

// EmbeddingsHandler handles POST /api/v1/embeddings as a pass-through.
func EmbeddingsHandler(reg *ctrl.Registry, sched ctrl.Scheduler, metricsReg *ctrl.MetricsRegistry, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if serverstate.IsDraining() {
			http.Error(w, "server draining", http.StatusServiceUnavailable)
			return
		}
		if r.Body == nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var meta struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &meta)

		// Determine if the request input is an array so we can batch it.
		payload := map[string]json.RawMessage{}
		_ = json.Unmarshal(body, &payload)
		if raw, ok := payload["input"]; ok {
			var inputs []json.RawMessage
			if err := json.Unmarshal(raw, &inputs); err == nil && len(inputs) > 0 {
				handleEmbeddingBatches(w, r, reg, sched, metricsReg, timeout, meta.Model, payload, inputs)
				return
			}
		}

		// Fallback to original pass-through behaviour for non-array inputs.
		exact := reg.WorkersForModel(meta.Model)
		worker, err := sched.PickWorker(meta.Model)
		if err != nil {
			logx.Log.Warn().Str("model", meta.Model).Msg("no worker")
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		if len(exact) == 0 {
			if key, ok := ctrl.AliasKey(meta.Model); ok {
				logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", meta.Model).Str("alias_key", key).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("alias fallback")
			}
		}
		reg.IncInFlight(worker.ID)
		defer reg.DecInFlight(worker.ID)

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Msg("dispatch")
		ch := make(chan interface{}, 16)
		worker.AddJob(reqID, ch)
		defer worker.RemoveJob(reqID)

		headers := map[string]string{}
		headers["Content-Type"] = r.Header.Get("Content-Type")
		if v := r.Header.Get("Accept"); v != "" {
			headers["Accept"] = v
		}
		if v := r.Header.Get("Accept-Language"); v != "" {
			headers["Accept-Language"] = v
		}
		if v := r.Header.Get("User-Agent"); v != "" {
			headers["User-Agent"] = v
		}
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = logID
		}
		headers["X-Request-Id"] = rid
		headers["Cache-Control"] = "no-store"

		msg := ctrl.HTTPProxyRequestMessage{
			Type:      "http_proxy_request",
			RequestID: reqID,
			Method:    http.MethodPost,
			Path:      "/embeddings",
			Headers:   headers,
			Stream:    false,
			Body:      body,
		}
		select {
		case worker.Send <- msg:
			metricsReg.RecordJobStart(worker.ID)
			metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusWorking)
		default:
			logx.Log.Warn().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Msg("worker busy")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			if _, err := w.Write([]byte(`{"error":"worker_busy"}`)); err != nil {
				logx.Log.Error().Err(err).Msg("write worker busy")
			}
			return
		}

		flusher, _ := w.(http.Flusher)
		ctx := r.Context()
		start := time.Now()
		headersSent := false
		bytesSent := false
		success := false
		var errMsg string
		var idle *time.Timer
		var timeoutCh <-chan time.Time
		if timeout > 0 {
			idle = time.NewTimer(timeout)
			timeoutCh = idle.C
			defer idle.Stop()
		}

		defer func() {
			dur := time.Since(start)
			metricsReg.RecordJobEnd(worker.ID, meta.Model, dur, 0, 0, success, errMsg)
			metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
			metrics.ObserveRequestDuration(worker.ID, meta.Model, dur)
			metrics.RecordModelRequest(meta.Model, success)
		}()
		for {
			select {
			case <-ctx.Done():
				select {
				case worker.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				return
			case <-timeoutCh:
				hb := worker.LastHeartbeat
				since := time.Since(hb)
				if since > timeout {
					errMsg = "timeout"
					select {
					case worker.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
					default:
					}
					if !headersSent {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusGatewayTimeout)
						_, _ = w.Write([]byte(`{"error":"timeout"}`))
					}
					return
				}
				if idle != nil {
					idle.Reset(timeout - since)
					timeoutCh = idle.C
				}
			case msg, ok := <-ch:
				if !ok {
					if !headersSent {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						if _, err := w.Write([]byte(`{"error":"upstream_error"}`)); err != nil {
							logx.Log.Error().Err(err).Msg("write upstream error")
						}
					}
					errMsg = "closed"
					return
				}
				if idle != nil {
					if !idle.Stop() {
						<-timeoutCh
					}
					idle.Reset(timeout)
					timeoutCh = idle.C
				}
				switch m := msg.(type) {
				case ctrl.HTTPProxyResponseHeadersMessage:
					headersSent = true
					for k, v := range m.Headers {
						if strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
							continue
						}
						w.Header().Set(k, v)
					}
					if strings.EqualFold(w.Header().Get("Content-Type"), "text/event-stream") {
						w.Header().Set("Cache-Control", "no-store")
					}
					w.WriteHeader(m.Status)
					if m.Status >= http.StatusBadRequest {
						lvl := logx.Log.Warn()
						if m.Status >= http.StatusInternalServerError || m.Status == http.StatusUnauthorized || m.Status == http.StatusForbidden {
							lvl = logx.Log.Error()
						}
						lvl.Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Int("status", m.Status).Msg("upstream response")
					}
					if flusher != nil {
						flusher.Flush()
					}
				case ctrl.HTTPProxyResponseChunkMessage:
					if len(m.Data) > 0 {
						if _, err := w.Write(m.Data); err != nil {
							logx.Log.Error().Err(err).Msg("write chunk")
						} else {
							bytesSent = true
							if flusher != nil {
								flusher.Flush()
							}
						}
					}
				case ctrl.HTTPProxyResponseEndMessage:
					if m.Error != nil && !bytesSent {
						if !headersSent {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusBadGateway)
						}
						if _, err := w.Write([]byte(`{"error":"upstream_error"}`)); err != nil {
							logx.Log.Error().Err(err).Msg("write upstream error")
						}
						errMsg = m.Error.Message
						logx.Log.Error().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Str("error_code", m.Error.Code).Str("error", m.Error.Message).Msg("upstream error")
					} else {
						success = true
					}
					logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Dur("duration", time.Since(start)).Msg("complete")
					return
				}
			}
		}
	}
}

type embeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type embeddingResponse struct {
	Object string            `json:"object"`
	Data   []json.RawMessage `json:"data"`
	Model  string            `json:"model"`
	Usage  embeddingUsage    `json:"usage"`
}

func handleEmbeddingBatches(w http.ResponseWriter, r *http.Request, reg *ctrl.Registry, sched ctrl.Scheduler, metricsReg *ctrl.MetricsRegistry, timeout time.Duration, model string, payload map[string]json.RawMessage, inputs []json.RawMessage) {
	logID := chiMiddleware.GetReqID(r.Context())
	exact := reg.WorkersForModel(model)
	remaining := inputs
	var allData []json.RawMessage
	var usage embeddingUsage
	var respModel string

	for len(remaining) > 0 {
		worker, err := sched.PickWorker(model)
		if err != nil {
			logx.Log.Warn().Str("model", model).Msg("no worker")
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		if len(exact) == 0 && len(allData) == 0 {
			if key, ok := ctrl.AliasKey(model); ok {
				logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", model).Str("alias_key", key).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("alias fallback")
			}
		}

		reg.IncInFlight(worker.ID)
		reqID := uuid.NewString()
		batch := worker.EmbeddingBatchSize
		if batch <= 0 || batch > len(remaining) {
			batch = len(remaining)
		}
		b, _ := json.Marshal(remaining[:batch])
		payload["input"] = b
		body, _ := json.Marshal(payload)
		headers := map[string]string{}
		headers["Content-Type"] = r.Header.Get("Content-Type")
		if v := r.Header.Get("Accept"); v != "" {
			headers["Accept"] = v
		}
		if v := r.Header.Get("Accept-Language"); v != "" {
			headers["Accept-Language"] = v
		}
		if v := r.Header.Get("User-Agent"); v != "" {
			headers["User-Agent"] = v
		}
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = logID
		}
		headers["X-Request-Id"] = rid
		headers["Cache-Control"] = "no-store"

		logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Msg("dispatch")
		respBody, status, success, errMsg := proxyEmbeddingOnce(r.Context(), worker, reqID, logID, model, headers, body, metricsReg, timeout)
		reg.DecInFlight(worker.ID)
		worker.RemoveJob(reqID)
		if !success {
			w.Header().Set("Content-Type", "application/json")
			if status == 0 {
				status = http.StatusBadGateway
			}
			w.WriteHeader(status)
			if len(respBody) > 0 {
				_, _ = w.Write(respBody)
			} else {
				_, _ = w.Write([]byte(`{"error":"` + errMsg + `"}`))
			}
			return
		}

		var resp embeddingResponse
		_ = json.Unmarshal(respBody, &resp)
		allData = append(allData, resp.Data...)
		usage.PromptTokens += resp.Usage.PromptTokens
		usage.TotalTokens += resp.Usage.TotalTokens
		if respModel == "" {
			respModel = resp.Model
		}
		remaining = remaining[batch:]
	}

	final := embeddingResponse{Object: "list", Data: allData, Model: respModel, Usage: usage}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(final); err != nil {
		logx.Log.Error().Err(err).Msg("write embeddings response")
	}
}

func proxyEmbeddingOnce(ctx context.Context, worker *ctrl.Worker, reqID, logID, model string, headers map[string]string, body []byte, metricsReg *ctrl.MetricsRegistry, timeout time.Duration) ([]byte, int, bool, string) {
	ch := make(chan interface{}, 16)
	worker.AddJob(reqID, ch)

	msg := ctrl.HTTPProxyRequestMessage{
		Type:      "http_proxy_request",
		RequestID: reqID,
		Method:    http.MethodPost,
		Path:      "/embeddings",
		Headers:   headers,
		Stream:    false,
		Body:      body,
	}
	select {
	case worker.Send <- msg:
		metricsReg.RecordJobStart(worker.ID)
		metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusWorking)
	default:
		logx.Log.Warn().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Msg("worker busy")
		return []byte(`{"error":"worker_busy"}`), http.StatusServiceUnavailable, false, "worker_busy"
	}

	start := time.Now()
	success := false
	var errMsg string
	var status int
	var buf bytes.Buffer
	var idle *time.Timer
	var timeoutCh <-chan time.Time
	if timeout > 0 {
		idle = time.NewTimer(timeout)
		timeoutCh = idle.C
		defer idle.Stop()
	}
	for {
		select {
		case <-ctx.Done():
			select {
			case worker.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
			default:
			}
			errMsg = "canceled"
			metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, success, errMsg)
			metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
			metrics.ObserveRequestDuration(worker.ID, model, time.Since(start))
			metrics.RecordModelRequest(model, false)
			return nil, status, false, errMsg
		case <-timeoutCh:
			hb := worker.LastHeartbeat
			since := time.Since(hb)
			if since > timeout {
				errMsg = "timeout"
				select {
				case worker.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, false, errMsg)
				metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
				metrics.ObserveRequestDuration(worker.ID, model, time.Since(start))
				metrics.RecordModelRequest(model, false)
				return nil, http.StatusGatewayTimeout, false, errMsg
			}
			if idle != nil {
				idle.Reset(timeout - since)
				timeoutCh = idle.C
			}
		case msg, ok := <-ch:
			if !ok {
				errMsg = "closed"
				metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, false, errMsg)
				metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
				metrics.ObserveRequestDuration(worker.ID, model, time.Since(start))
				metrics.RecordModelRequest(model, false)
				return nil, http.StatusBadGateway, false, errMsg
			}
			if idle != nil {
				if !idle.Stop() {
					<-timeoutCh
				}
				idle.Reset(timeout)
				timeoutCh = idle.C
			}
			switch m := msg.(type) {
			case ctrl.HTTPProxyResponseHeadersMessage:
				status = m.Status
			case ctrl.HTTPProxyResponseChunkMessage:
				if len(m.Data) > 0 {
					buf.Write(m.Data)
				}
			case ctrl.HTTPProxyResponseEndMessage:
				if m.Error != nil {
					errMsg = m.Error.Message
					logx.Log.Error().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Str("error_code", m.Error.Code).Str("error", m.Error.Message).Msg("upstream error")
				} else {
					success = status < http.StatusBadRequest
				}
				dur := time.Since(start)
				metricsReg.RecordJobEnd(worker.ID, model, dur, 0, 0, success, errMsg)
				metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
				metrics.ObserveRequestDuration(worker.ID, model, dur)
				metrics.RecordModelRequest(model, success)
				logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Dur("duration", dur).Msg("complete")
				return buf.Bytes(), status, success && errMsg == "", errMsg
			}
		}
	}
}
