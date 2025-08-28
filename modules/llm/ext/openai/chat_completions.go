package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

    ctrl "github.com/gaspardpetit/nfrx/sdk/contracts/control"
    "github.com/gaspardpetit/nfrx/sdk/spi"
    "github.com/gaspardpetit/nfrx/core/logx"
)

// ChatCompletionsHandler handles POST /api/llm/v1/chat/completions as a pass-through.
func ChatCompletionsHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		_ = json.Unmarshal(body, &meta)
		exact := reg.WorkersForModel(meta.Model)
		worker, err := sched.PickWorker(meta.Model)
		if err != nil {
			logx.Log.Warn().Str("model", meta.Model).Msg("no worker")
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		if len(exact) == 0 {
			if key, ok := ctrl.AliasKey(meta.Model); ok {
				logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", meta.Model).Str("alias_key", key).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Msg("alias fallback")
			}
		}
		reg.IncInFlight(worker.ID())
		defer reg.DecInFlight(worker.ID())

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Bool("stream", meta.Stream).Msg("dispatch")
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
			Path:      "/chat/completions",
			Headers:   headers,
			Stream:    meta.Stream,
			Body:      body,
		}
		select {
		case worker.SendChan() <- msg:
			metrics.RecordJobStart(worker.ID())
			metrics.SetWorkerStatus(worker.ID(), spi.StatusWorking)
		default:
			logx.Log.Warn().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Msg("worker busy")
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
		var tokensIn, tokensOut uint64
		var sseBuf string
		var bodyBuf []byte
		var idle *time.Timer
		var timeoutCh <-chan time.Time
		if timeout > 0 {
			idle = time.NewTimer(timeout)
			timeoutCh = idle.C
			defer idle.Stop()
		}

		defer func() {
			dur := time.Since(start)
			metrics.RecordJobEnd(worker.ID(), meta.Model, dur, tokensIn, tokensOut, 0, success, errMsg)
			metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
			metrics.ObserveRequestDuration(worker.ID(), meta.Model, dur)
			metrics.RecordWorkerProcessingTime(worker.ID(), dur)
			metrics.RecordModelRequest(meta.Model, success)
			if tokensIn > 0 {
				metrics.RecordModelTokens(meta.Model, "in", tokensIn)
				metrics.RecordWorkerTokens(worker.ID(), "in", tokensIn)
			}
			if tokensOut > 0 {
				metrics.RecordModelTokens(meta.Model, "out", tokensOut)
				metrics.RecordWorkerTokens(worker.ID(), "out", tokensOut)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				select {
				case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				return
			case <-timeoutCh:
				hb := worker.LastHeartbeat()
				since := time.Since(hb)
				if since > timeout {
					errMsg = "timeout"
					select {
					case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
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
						lvl.Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Int("status", m.Status).Msg("upstream response")
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
					if meta.Stream {
						sseBuf += string(m.Data)
						for {
							idx := strings.Index(sseBuf, "\n")
							if idx == -1 {
								break
							}
							line := strings.TrimRight(sseBuf[:idx], "\r")
							sseBuf = sseBuf[idx+1:]
							if !strings.HasPrefix(line, "data:") {
								continue
							}
							payload := strings.TrimSpace(line[5:])
							if payload == "" || payload == "[DONE]" {
								continue
							}
							var v struct {
								Usage struct {
									PromptTokens     uint64 `json:"prompt_tokens"`
									CompletionTokens uint64 `json:"completion_tokens"`
								} `json:"usage"`
							}
							if err := json.Unmarshal([]byte(payload), &v); err == nil {
								if v.Usage.PromptTokens > 0 {
									tokensIn = v.Usage.PromptTokens
								}
								if v.Usage.CompletionTokens > 0 {
									tokensOut = v.Usage.CompletionTokens
								}
							}
						}
					} else {
						bodyBuf = append(bodyBuf, m.Data...)
					}
				case ctrl.HTTPProxyResponseEndMessage:
					if !meta.Stream {
						var v struct {
							Usage struct {
								PromptTokens     uint64 `json:"prompt_tokens"`
								CompletionTokens uint64 `json:"completion_tokens"`
							} `json:"usage"`
						}
						_ = json.Unmarshal(bodyBuf, &v)
						if v.Usage.PromptTokens > 0 {
							tokensIn = v.Usage.PromptTokens
						}
						if v.Usage.CompletionTokens > 0 {
							tokensOut = v.Usage.CompletionTokens
						}
					}
					if m.Error != nil && !bytesSent {
						if !headersSent {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(http.StatusBadGateway)
						}
						if _, err := w.Write([]byte(`{"error":"upstream_error"}`)); err != nil {
							logx.Log.Error().Err(err).Msg("write upstream error")
						}
						errMsg = m.Error.Message
						logx.Log.Error().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Str("error_code", m.Error.Code).Str("error", m.Error.Message).Msg("upstream error")
					} else {
						success = true
					}
					logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Bool("stream", meta.Stream).Dur("duration", time.Since(start)).Msg("complete")
					return
				}
			}
		}
	}
}
