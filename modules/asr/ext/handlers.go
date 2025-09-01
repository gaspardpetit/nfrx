package asr

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/gaspardpetit/nfrx/core/logx"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// transcribeHandler proxies transcription requests to an eligible worker.
func transcribeHandler(reg *baseworker.Registry, sched baseworker.Scheduler, mx *baseworker.MetricsRegistry, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var model string
		var stream bool
		if _, params, err := mime.ParseMediaType(ct); err == nil {
			boundary := params["boundary"]
			mr := multipart.NewReader(bytes.NewReader(body), boundary)
			for {
				part, err := mr.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				switch name := part.FormName(); name {
				case "model":
					mb, _ := io.ReadAll(part)
					model = string(mb)
				case "stream":
					sb, _ := io.ReadAll(part)
					stream, _ = strconv.ParseBool(strings.TrimSpace(string(sb)))
				}
				_ = part.Close()
			}
		}
		if model == "" {
			http.Error(w, "no model", http.StatusBadRequest)
			return
		}
		exact := reg.WorkersForLabel(model)
		wk, err := sched.PickWorker(model)
		if err != nil {
			logx.Log.Warn().Str("model", model).Msg("no worker")
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		if len(exact) == 0 {
			if key, ok := ctrl.AliasKey(model); ok {
				logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", model).Str("alias_key", key).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Msg("alias fallback")
			}
		}
		reg.IncInFlight(wk.ID)
		defer reg.DecInFlight(wk.ID)
		basemetrics.RecordRequest("asr", "worker", "asr.transcribe", model)

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", logID).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Str("model", model).Bool("stream", stream).Msg("dispatch")

		ch := make(chan interface{}, 16)
		wk.AddJob(reqID, ch)
		defer wk.RemoveJob(reqID)

		headers := map[string]string{}
		for k, vals := range r.Header {
			if len(vals) == 0 {
				continue
			}
			headers[k] = vals[0]
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
			Path:      "/audio/transcriptions",
			Headers:   headers,
			Stream:    stream,
			Body:      body,
		}
		select {
		case wk.Send <- msg:
			basemetrics.RecordStart("asr", "worker", "asr.transcribe", model)
			mx.RecordJobStart(wk.ID)
			mx.SetWorkerStatus(wk.ID, baseworker.StatusWorking)
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"worker_busy"}`))
			return
		}

		flusher, _ := w.(http.Flusher)
		ctx := r.Context()
		start := time.Now()
		headersSent := false
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
			mx.RecordJobEnd(wk.ID, "asr", dur, 0, 0, 0, success, errMsg)
			mx.SetWorkerStatus(wk.ID, baseworker.StatusIdle)
			basemetrics.RecordComplete("asr", "worker", "asr.transcribe", model, errMsgIf(!success, errMsg), success, dur)
		}()

		for {
			select {
			case <-ctx.Done():
				select {
				case wk.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				return
			case <-timeoutCh:
				errMsg = "timeout"
				select {
				case wk.Send <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}:
				default:
				}
				if !headersSent {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusGatewayTimeout)
					_, _ = w.Write([]byte(`{"error":"timeout"}`))
				}
				return
			case msg, ok := <-ch:
				if !ok {
					if !headersSent {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						_, _ = w.Write([]byte(`{"error":"upstream_error"}`))
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
						if k == "Transfer-Encoding" || k == "Connection" {
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
						_, _ = w.Write(m.Data)
						if flusher != nil {
							flusher.Flush()
						}
					}
				case ctrl.HTTPProxyResponseEndMessage:
					if m.Error != nil {
						errMsg = m.Error.Code
					} else {
						success = true
					}
					return
				}
			}
		}
	}
}

func errMsgIf(cond bool, msg string) string {
	if cond {
		return msg
	}
	return ""
}
