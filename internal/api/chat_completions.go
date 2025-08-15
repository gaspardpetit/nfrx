package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/gaspardpetit/llamapool/internal/ctrl"
	"github.com/gaspardpetit/llamapool/internal/logx"
)

// ChatCompletionsHandler handles POST /v1/chat/completions as a pass-through.
func ChatCompletionsHandler(reg *ctrl.Registry, sched ctrl.Scheduler) http.HandlerFunc {
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
				logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", meta.Model).Str("alias_key", key).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("alias fallback")
			}
		}
		reg.IncInFlight(worker.ID)
		defer reg.DecInFlight(worker.ID)

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Bool("stream", meta.Stream).Msg("dispatch")
		ch := make(chan interface{}, 16)
		worker.AddJob(reqID, ch)
		defer func() {
			worker.RemoveJob(reqID)
			close(ch)
		}()

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
			Path:      "/v1/chat/completions",
			Headers:   headers,
			Stream:    meta.Stream,
			Body:      body,
		}
		select {
		case worker.Send <- msg:
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
		for {
			select {
			case <-ctx.Done():
				select {
				case worker.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				return
			case msg, ok := <-ch:
				if !ok {
					if !headersSent {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						if _, err := w.Write([]byte(`{"error":"upstream_error"}`)); err != nil {
							logx.Log.Error().Err(err).Msg("write upstream error")
						}
					}
					return
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
					}
					logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", meta.Model).Bool("stream", meta.Stream).Dur("duration", time.Since(start)).Msg("complete")
					return
				}
			}
		}
	}
}
