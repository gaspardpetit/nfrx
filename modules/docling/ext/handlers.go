package docling

import (
	"io"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/gaspardpetit/nfrx/core/logx"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// proxyHandler returns a handler that proxies the request body/headers to the selected worker path.
func proxyHandler(reg *baseworker.Registry, sched baseworker.Scheduler, mx *baseworker.MetricsRegistry, path string, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept any content-type; pass-through body.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Pick least-busy eligible worker (all score 1.0)
		wk, err := sched.PickWorker("")
		if err != nil {
			logx.Log.Warn().Msg("no worker")
			http.Error(w, "no worker", http.StatusServiceUnavailable)
			return
		}
		reg.IncInFlight(wk.ID)
		defer reg.DecInFlight(wk.ID)

		// Record generic request metric for docling.convert
		basemetrics.RecordRequest("docling", "worker", "docling.convert", "docling")

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())
		logx.Log.Info().Str("request_id", logID).Str("worker_id", wk.ID).Str("worker_name", wk.Name).Str("path", path).Msg("dispatch")

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
			Method:    r.Method,
			Path:      path,
			Headers:   headers,
			Stream:    false,
			Body:      body,
		}
		select {
		case wk.Send <- msg:
			basemetrics.RecordStart("docling", "worker", "docling.convert", "docling")
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
			mx.RecordJobEnd(wk.ID, "docling", dur, 0, 0, 0, success, errMsg)
			mx.SetWorkerStatus(wk.ID, baseworker.StatusIdle)
			basemetrics.RecordComplete("docling", "worker", "docling.convert", "docling", errMsgIf(!success, errMsg), success, dur)
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
				hb := wk.LastHeartbeat
				// LastHeartbeat is a field; approximate timeout behavior as in LLM
				// We cannot access it thread-safely here; simply cancel.
				_ = hb
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
