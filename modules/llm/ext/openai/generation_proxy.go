package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/gaspardpetit/nfrx/core/logx"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
	"github.com/gaspardpetit/nfrx/sdk/api/spi"
	basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
)

type generationQueueStatusWriter func(w http.ResponseWriter, flusher http.Flusher, reqID, model string, pos int) bool

type generationProxySpec struct {
	endpointPath      string
	operationName     string
	queueStatusWriter generationQueueStatusWriter
}

func generationProxyHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options, queue *CompletionQueue, spec generationProxySpec) http.HandlerFunc {
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

		reqID := uuid.NewString()
		logID := chiMiddleware.GetReqID(r.Context())

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
		var upstreamStatus int
		var errorBytes int
		debugErrorBody := logx.Log.Debug().Enabled()
		var errorBody []byte
		var idle *time.Timer
		var timeoutCh <-chan time.Time

		writeQueueStatus := func(pos int) {
			if !meta.Stream || flusher == nil || spec.queueStatusWriter == nil {
				return
			}
			if spec.queueStatusWriter(w, flusher, reqID, meta.Model, pos) {
				headersSent = true
			}
		}

		tryDispatch := func() (dispatched bool, worker spi.WorkerRef, ch chan interface{}) {
			wk, err := sched.PickWorker(meta.Model)
			if err != nil {
				return false, nil, nil
			}
			exact := reg.WorkersForLabel(meta.Model)
			if len(exact) == 0 {
				if key, ok := ctrl.AliasKey(meta.Model); ok {
					logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", meta.Model).Str("alias_key", key).Str("worker_id", wk.ID()).Str("worker_name", wk.Name()).Msg("alias fallback")
				}
			}
			ch = make(chan interface{}, 16)
			wk.AddJob(reqID, ch)
			sent := false
			msg := ctrl.HTTPProxyRequestMessage{Type: "http_proxy_request", RequestID: reqID, Method: http.MethodPost, Path: spec.endpointPath, Headers: headers, Stream: meta.Stream, Body: body}
			select {
			case wk.SendChan() <- msg:
				sent = true
			default:
			}
			if !sent {
				wk.RemoveJob(reqID)
				return false, nil, nil
			}
			basemetrics.RecordStart("llm", "worker", spec.operationName, meta.Model)
			metrics.RecordJobStart(wk.ID())
			metrics.SetWorkerStatus(wk.ID(), spi.StatusWorking)
			reg.IncInFlight(wk.ID())
			logx.Log.Info().Str("request_id", logID).Str("worker_id", wk.ID()).Str("worker_name", wk.Name()).Str("model", meta.Model).Bool("stream", meta.Stream).Str("path", spec.endpointPath).Msg("dispatch")
			return true, wk, ch
		}

		modelSupported := func(model string) bool {
			if _, ok := reg.AggregatedModel(model); ok {
				return true
			}
			if ak, ok := ctrl.AliasKey(model); ok {
				for _, m := range reg.AggregatedModels() {
					if mk, ok2 := ctrl.AliasKey(m.ID); ok2 && mk == ak {
						return true
					}
				}
			}
			return false
		}

		dispatched, worker, ch := tryDispatch()
		if !dispatched {
			if queue == nil || opts.QueueSize == 0 {
				if !modelSupported(meta.Model) {
					http.Error(w, "no worker", http.StatusNotFound)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"error":"worker_busy"}`))
				}
				return
			}
			if !modelSupported(meta.Model) {
				http.Error(w, "no worker", http.StatusNotFound)
				return
			}
			if pos, ok := queue.Enter(reqID, meta.Model); !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"worker_busy"}`))
				return
			} else {
				writeQueueStatus(pos)
			}

			var statusTicker *time.Ticker
			if meta.Stream && opts.QueueUpdateSeconds > 0 && spec.queueStatusWriter != nil {
				statusTicker = time.NewTicker(time.Duration(opts.QueueUpdateSeconds) * time.Second)
				defer statusTicker.Stop()
			}
			retryTicker := time.NewTicker(200 * time.Millisecond)
			defer retryTicker.Stop()

			for {
				select {
				case <-ctx.Done():
					queue.Leave(reqID)
					return
				case <-retryTicker.C:
					if !modelSupported(meta.Model) {
						queue.Leave(reqID)
						http.Error(w, "no worker", http.StatusNotFound)
						return
					}
					if queue.IsFirstDispatchable(reqID, func(model string) bool {
						if !modelSupported(model) {
							return false
						}
						_, err := sched.PickWorker(model)
						return err == nil
					}) {
						if d, wk, c := tryDispatch(); d {
							queue.Leave(reqID)
							worker, ch = wk, c
							goto PROXY
						}
					}
				case <-func() <-chan time.Time {
					if statusTicker != nil {
						return statusTicker.C
					}
					return make(chan time.Time)
				}():
					if pos := queue.Position(reqID); pos > 0 {
						writeQueueStatus(pos)
					}
				}
			}
		}

	PROXY:
		defer func() {
			if worker != nil {
				worker.RemoveJob(reqID)
				dur := time.Since(start)
				metrics.RecordJobEnd(worker.ID(), meta.Model, dur, tokensIn, tokensOut, 0, success, errMsg)
				metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
				basemetrics.RecordComplete("llm", "worker", spec.operationName, meta.Model, func() string {
					if !success {
						return errMsg
					}
					return ""
				}(), success, dur)
				if tokensIn > 0 {
					metrics.RecordWorkerTokens(worker.ID(), "in", tokensIn)
					basemetrics.AddSize("llm", "worker", spec.operationName, meta.Model, "tokens_in", tokensIn)
				}
				if tokensOut > 0 {
					metrics.RecordWorkerTokens(worker.ID(), "out", tokensOut)
					basemetrics.AddSize("llm", "worker", spec.operationName, meta.Model, "tokens_out", tokensOut)
					basemetrics.AddSize("llm", "worker", spec.operationName, meta.Model, "tokens_total", tokensIn+tokensOut)
				}
				reg.DecInFlight(worker.ID())
			}
		}()

		if opts.RequestTimeout > 0 {
			idle = time.NewTimer(opts.RequestTimeout)
			timeoutCh = idle.C
			defer idle.Stop()
		}

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
				if since > opts.RequestTimeout {
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
					idle.Reset(opts.RequestTimeout - since)
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
					idle.Reset(opts.RequestTimeout)
					timeoutCh = idle.C
				}
				switch m := msg.(type) {
				case ctrl.HTTPProxyResponseHeadersMessage:
					priorHeadersSent := headersSent
					upstreamStatus = m.Status
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
					if !priorHeadersSent {
						w.WriteHeader(m.Status)
					}
					if m.Status >= http.StatusBadRequest {
						lvl := logx.Log.Warn()
						if m.Status >= http.StatusInternalServerError || m.Status == http.StatusUnauthorized || m.Status == http.StatusForbidden {
							lvl = logx.Log.Error()
						}
						lvl.Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Int("status", m.Status).Str("path", spec.endpointPath).Msg("upstream response")
					}
					if flusher != nil {
						flusher.Flush()
					}
				case ctrl.HTTPProxyResponseChunkMessage:
					if len(m.Data) > 0 {
						if upstreamStatus >= http.StatusBadRequest {
							errorBytes += len(m.Data)
							if debugErrorBody {
								errorBody = append(errorBody, m.Data...)
							}
						}
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
							in, out := extractUsageFromJSON([]byte(payload))
							if in > 0 {
								tokensIn = in
							}
							if out > 0 {
								tokensOut = out
							}
						}
					} else {
						bodyBuf = append(bodyBuf, m.Data...)
					}
				case ctrl.HTTPProxyResponseEndMessage:
					if !meta.Stream {
						in, out := extractUsageFromJSON(bodyBuf)
						if in > 0 {
							tokensIn = in
						}
						if out > 0 {
							tokensOut = out
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
						logx.Log.Error().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Str("error_code", m.Error.Code).Str("error", m.Error.Message).Str("path", spec.endpointPath).Msg("upstream error")
					} else {
						success = true
					}
					if upstreamStatus >= http.StatusBadRequest && errorBytes > 0 {
						lvl := logx.Log.Warn()
						if upstreamStatus >= http.StatusInternalServerError || upstreamStatus == http.StatusUnauthorized || upstreamStatus == http.StatusForbidden {
							lvl = logx.Log.Error()
						}
						lvl.Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Int("status", upstreamStatus).Str("path", spec.endpointPath).Int("body_bytes", errorBytes).Msg("upstream response body observed")
						if debugErrorBody {
							logx.Log.Debug().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Int("status", upstreamStatus).Str("path", spec.endpointPath).Bytes("body", errorBody).Msg("upstream response body detail")
						}
					}
					logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Bool("stream", meta.Stream).Str("path", spec.endpointPath).Dur("duration", time.Since(start)).Msg("complete")
					return
				}
			}
		}
	}
}

func queueStatusWriter(w http.ResponseWriter, flusher http.Flusher, reqID, model string, pos int) bool {
	if !headerWritten(w.Header()) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}
	payload := fmt.Sprintf(`{"type":"nfrx.queue","request_id":"%s","model":"%s","position":%d}`,
		reqID, model, pos)
	_, _ = w.Write([]byte("event: nfrx.queue\n"))
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write([]byte(payload))
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
	return true
}

func headerWritten(h http.Header) bool {
	return h.Get("Content-Type") != ""
}

func extractUsageFromJSON(data []byte) (uint64, uint64) {
	if len(data) == 0 {
		return 0, 0
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, 0
	}
	return findUsage(v)
}

func findUsage(v any) (uint64, uint64) {
	switch x := v.(type) {
	case map[string]any:
		if usage, ok := x["usage"]; ok {
			if in, out, found := usageFromValue(usage); found {
				return in, out
			}
		}
		for _, vv := range x {
			if in, out := findUsage(vv); in > 0 || out > 0 {
				return in, out
			}
		}
	case []any:
		for _, vv := range x {
			if in, out := findUsage(vv); in > 0 || out > 0 {
				return in, out
			}
		}
	}
	return 0, 0
}

func usageFromValue(v any) (uint64, uint64, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return 0, 0, false
	}
	in := numberField(m, "prompt_tokens")
	if in == 0 {
		in = numberField(m, "input_tokens")
	}
	out := numberField(m, "completion_tokens")
	if out == 0 {
		out = numberField(m, "output_tokens")
	}
	if in == 0 && out == 0 {
		return 0, 0, false
	}
	return in, out, true
}

func numberField(m map[string]any, key string) uint64 {
	switch n := m[key].(type) {
	case float64:
		if n > 0 {
			return uint64(n)
		}
	case int:
		if n > 0 {
			return uint64(n)
		}
	case int64:
		if n > 0 {
			return uint64(n)
		}
	case json.Number:
		if v, err := n.Int64(); err == nil && v > 0 {
			return uint64(v)
		}
	}
	return 0
}
