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

// ChatCompletionsHandler handles POST /api/llm/v1/chat/completions with optional pre-dispatch queueing.
func ChatCompletionsHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, opts Options, queue *CompletionQueue) http.HandlerFunc {
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
        // Prepare request identity and log context
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
        var idle *time.Timer
        var timeoutCh <-chan time.Time

        // Prepare request identity and log context
        // Helper to emit queued status SSE lines.
        writeQueueStatus := func(pos int) {
            if !meta.Stream || flusher == nil {
                return
            }
            if !headersSent {
                // Emit synthetic SSE headers once
                w.Header().Set("Content-Type", "text/event-stream")
                w.Header().Set("Cache-Control", "no-store")
                w.WriteHeader(http.StatusOK)
                flusher.Flush()
                headersSent = true
            }
            created := time.Now().Unix()
            payload := fmt.Sprintf(`{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"%s","system_fingerprint":"nfrx","choices":[{"index":0,"delta":{"role":"assistant","content":"","reasoning":"Requests queued, number %d in line"},"finish_reason":null}]}`,
                reqID, created, meta.Model, pos)
            _, _ = w.Write([]byte("data: "))
            _, _ = w.Write([]byte(payload))
            _, _ = w.Write([]byte("\n\n"))
            flusher.Flush()
        }

        // Fast path: attempt immediate dispatch without queuing.
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
            msg := ctrl.HTTPProxyRequestMessage{Type: "http_proxy_request", RequestID: reqID, Method: http.MethodPost, Path: "/chat/completions", Headers: headers, Stream: meta.Stream, Body: body}
            select {
            case wk.SendChan() <- msg:
                sent = true
            default:
                // busy
            }
            if !sent {
                wk.RemoveJob(reqID)
                return false, nil, nil
            }
            // Mark started only after successful enqueue
            basemetrics.RecordStart("llm", "worker", "llm.completion", meta.Model)
            metrics.RecordJobStart(wk.ID())
            metrics.SetWorkerStatus(wk.ID(), spi.StatusWorking)
            reg.IncInFlight(wk.ID())
            logx.Log.Info().Str("request_id", logID).Str("worker_id", wk.ID()).Str("worker_name", wk.Name()).Str("model", meta.Model).Bool("stream", meta.Stream).Msg("dispatch")
            return true, wk, ch
        }

        // Helper to determine if any worker could theoretically serve this model (ignoring capacity),
        // considering alias fallback semantics.
        modelSupported := func(model string) bool {
            // exact first
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

        // First attempt to dispatch immediately.
        dispatched, worker, ch := tryDispatch()
        if !dispatched {
            // Queue path
            if queue == nil || opts.QueueSize == 0 {
                // No queueing configured: behave as busy
                // Distinguish between unsupported model and temporary saturation
                if !modelSupported(meta.Model) {
                    http.Error(w, "no worker", http.StatusNotFound)
                } else {
                    w.Header().Set("Content-Type", "application/json")
                    w.WriteHeader(http.StatusServiceUnavailable)
                    _, _ = w.Write([]byte(`{"error":"worker_busy"}`))
                }
                return
            }
            // Before entering the queue, reject unsupported models
            if !modelSupported(meta.Model) {
                http.Error(w, "no worker", http.StatusNotFound)
                return
            }
            if pos, ok := queue.Enter(reqID); !ok {
                // Queue full -> 503 busy
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusServiceUnavailable)
                _, _ = w.Write([]byte(`{"error":"worker_busy"}`))
                return
            } else {
                // emit immediate status if streaming
                writeQueueStatus(pos)
            }

            // Periodic status updates per configuration
            var statusTicker *time.Ticker
            if meta.Stream && opts.QueueUpdateSeconds > 0 {
                statusTicker = time.NewTicker(time.Duration(opts.QueueUpdateSeconds) * time.Second)
                defer statusTicker.Stop()
            }
            // Internal quick retry ticker to react to capacity changes promptly
            retryTicker := time.NewTicker(200 * time.Millisecond)
            defer retryTicker.Stop()

            for {
                select {
                case <-ctx.Done():
                    queue.Leave(reqID)
                    return
                case <-retryTicker.C:
                    // If model is no longer supported (e.g., workers disconnected/updated), cancel with 404
                    if !modelSupported(meta.Model) {
                        queue.Leave(reqID)
                        http.Error(w, "no worker", http.StatusNotFound)
                        return
                    }
                    if queue.IsHead(reqID) {
                        if d, wk, c := tryDispatch(); d {
                            // leave queue and continue with proxy loop
                            queue.Leave(reqID)
                            dispatched, worker, ch = true, wk, c
                            goto PROXY
                        }
                    }
                case <-func() <-chan time.Time { if statusTicker != nil { return statusTicker.C }; return make(chan time.Time) }():
                    if pos := queue.Position(reqID); pos > 0 {
                        writeQueueStatus(pos)
                    }
                }
            }
        }

    PROXY:
        // Ensure in-flight is decremented at the end if we dispatched to a worker.
        defer func() {
            if worker != nil {
                // Per-request metrics (only once dispatched)
                dur := time.Since(start)
                metrics.RecordJobEnd(worker.ID(), meta.Model, dur, tokensIn, tokensOut, 0, success, errMsg)
                metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
                basemetrics.RecordComplete("llm", "worker", "llm.completion", meta.Model, func() string { if !success { return errMsg }; return "" }(), success, dur)
                if tokensIn > 0 {
                    metrics.RecordWorkerTokens(worker.ID(), "in", tokensIn)
                    basemetrics.AddSize("llm", "worker", "llm.completion", meta.Model, "tokens_in", tokensIn)
                }
                if tokensOut > 0 {
                    metrics.RecordWorkerTokens(worker.ID(), "out", tokensOut)
                    basemetrics.AddSize("llm", "worker", "llm.completion", meta.Model, "tokens_out", tokensOut)
                    basemetrics.AddSize("llm", "worker", "llm.completion", meta.Model, "tokens_total", tokensIn+tokensOut)
                }
                reg.DecInFlight(worker.ID())
            }
        }()

        // Initialize activity timeout only once proxying to a worker starts.
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
                    if !meta.Stream { // if we already sent synthetic headers for stream, avoid duplicating
                        w.WriteHeader(m.Status)
                    }
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
