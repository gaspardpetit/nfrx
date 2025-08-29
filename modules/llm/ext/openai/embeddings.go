package openai

import (
    "encoding/json"
    "io"
    "net/http"
    "strings"
    "time"

    chiMiddleware "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"

    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
    basemetrics "github.com/gaspardpetit/nfrx/sdk/base/metrics"
    "github.com/gaspardpetit/nfrx/core/logx"
    
)

// EmbeddingsHandler handles POST /api/llm/v1/embeddings as a pass-through.
func EmbeddingsHandler(reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, timeout time.Duration, maxParallel int) http.HandlerFunc {
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
        var meta struct { Model string `json:"model"` }
        _ = json.Unmarshal(body, &meta)

        // Determine if the request input is an array so we can batch it.
        payload := map[string]json.RawMessage{}
        _ = json.Unmarshal(body, &payload)
        if raw, ok := payload["input"]; ok {
            var inputs []json.RawMessage
            if err := json.Unmarshal(raw, &inputs); err == nil && len(inputs) > 0 {
                // Already an array: use partitioned path
                handlePartitionedEmbeddings(w, r, reg, sched, metrics, timeout, meta.Model, payload, inputs, maxParallel)
                return
            }
            // Not an array: normalize to a single-element array for uniform handling
            handlePartitionedEmbeddings(w, r, reg, sched, metrics, timeout, meta.Model, payload, []json.RawMessage{raw}, maxParallel)
            return
        }

        // Fallback to original pass-through behaviour for non-array inputs.
        exact := reg.WorkersForLabel(meta.Model)
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
        // Generic metrics
        basemetrics.RecordRequest("llm", "worker", "llm.embedding", meta.Model)
        defer reg.DecInFlight(worker.ID())

        reqID := uuid.NewString()
        logID := chiMiddleware.GetReqID(r.Context())
        logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Msg("dispatch")
        ch := make(chan interface{}, 16)
        worker.AddJob(reqID, ch)
        defer worker.RemoveJob(reqID)

        headers := map[string]string{}
        headers["Content-Type"] = r.Header.Get("Content-Type")
        if v := r.Header.Get("Accept"); v != "" { headers["Accept"] = v }
        if v := r.Header.Get("Accept-Language"); v != "" { headers["Accept-Language"] = v }
        if v := r.Header.Get("User-Agent"); v != "" { headers["User-Agent"] = v }
        rid := r.Header.Get("X-Request-Id"); if rid == "" { rid = logID }
        headers["X-Request-Id"] = rid
        headers["Cache-Control"] = "no-store"

        msg := ctrl.HTTPProxyRequestMessage{ Type: "http_proxy_request", RequestID: reqID, Method: http.MethodPost, Path: "/embeddings", Headers: headers, Stream: false, Body: body }
        switch {
        case func() bool { select { case worker.SendChan() <- msg: return true; default: return false } }():
            // Mark started only after successful enqueue
            basemetrics.RecordStart("llm", "worker", "llm.embedding", meta.Model)
            metrics.RecordJobStart(worker.ID())
            metrics.SetWorkerStatus(worker.ID(), spi.StatusWorking)
        default:
            logx.Log.Warn().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Msg("worker busy")
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusServiceUnavailable)
            _, _ = w.Write([]byte(`{"error":"worker_busy"}`))
            return
        }

        flusher, _ := w.(http.Flusher)
        ctx := r.Context()
        start := time.Now()
        headersSent := false
        bytesSent := false
        success := false
        var errMsg string
        embeddingsCount := uint64(1)
        var idle *time.Timer
        var timeoutCh <-chan time.Time
        if timeout > 0 { idle = time.NewTimer(timeout); timeoutCh = idle.C; defer idle.Stop() }

        errMsgIf := func(cond bool, msg string) string { if cond { return msg }; return "" }
        defer func() {
            dur := time.Since(start)
            metrics.RecordJobEnd(worker.ID(), meta.Model, dur, 0, 0, embeddingsCount, success, errMsg)
            metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
            // Generic request metrics for single-item embedding
            basemetrics.RecordComplete("llm", "worker", "llm.embedding", meta.Model, errMsgIf(!success, errMsg), success, dur)
            basemetrics.AddSize("llm", "worker", "llm.embedding", meta.Model, "embeddings", embeddingsCount)
        }()

        for {
            select {
            case <-ctx.Done():
                select { case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}: default: }
                return
            case <-timeoutCh:
                hb := worker.LastHeartbeat(); since := time.Since(hb)
                if since > timeout {
                    errMsg = "timeout"
                    select { case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}: default: }
                    if !headersSent {
                        w.Header().Set("Content-Type", "application/json")
                        w.WriteHeader(http.StatusGatewayTimeout)
                        _, _ = w.Write([]byte(`{"error":"timeout"}`))
                    }
                    return
                }
                if idle != nil { idle.Reset(timeout - since); timeoutCh = idle.C }
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
                if idle != nil { if !idle.Stop() { <-timeoutCh } ; idle.Reset(timeout); timeoutCh = idle.C }
                switch m := msg.(type) {
                case ctrl.HTTPProxyResponseHeadersMessage:
                    headersSent = true
                    for k, v := range m.Headers { if !strings.EqualFold(k, "Transfer-Encoding") && !strings.EqualFold(k, "Connection") { w.Header().Set(k, v) } }
                    if strings.EqualFold(w.Header().Get("Content-Type"), "text/event-stream") { w.Header().Set("Cache-Control", "no-store") }
                    w.WriteHeader(m.Status)
                    if m.Status >= http.StatusBadRequest {
                        lvl := logx.Log.Warn(); if m.Status >= http.StatusInternalServerError || m.Status == http.StatusUnauthorized || m.Status == http.StatusForbidden { lvl = logx.Log.Error() }
                        lvl.Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Int("status", m.Status).Msg("upstream response")
                    }
                    if flusher != nil { flusher.Flush() }
                case ctrl.HTTPProxyResponseChunkMessage:
                    if len(m.Data) > 0 {
                        if _, err := w.Write(m.Data); err != nil { logx.Log.Error().Err(err).Msg("write chunk") } else { bytesSent = true; if flusher != nil { flusher.Flush() } }
                    }
                case ctrl.HTTPProxyResponseEndMessage:
                    if m.Error != nil && !bytesSent {
                        if !headersSent { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(http.StatusBadGateway) }
                        _, _ = w.Write([]byte(`{"error":"upstream_error"}`))
                        errMsg = m.Error.Message
                        logx.Log.Error().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Str("error_code", m.Error.Code).Str("error", m.Error.Message).Msg("upstream error")
                    } else { success = true }
                    logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", meta.Model).Dur("duration", time.Since(start)).Msg("complete")
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

// handlePartitionedEmbeddings uses the generic partitioned job handler to fan out
// embedding requests across compatible workers, then assembles a single response.
func handlePartitionedEmbeddings(w http.ResponseWriter, r *http.Request, reg spi.WorkerRegistry, sched spi.Scheduler, metrics spi.Metrics, timeout time.Duration, model string, payload map[string]json.RawMessage, inputs []json.RawMessage, maxParallel int) {
    ctx := r.Context()
    logID := chiMiddleware.GetReqID(ctx)
    headers := map[string]string{}
    headers["Content-Type"] = r.Header.Get("Content-Type")
    if v := r.Header.Get("Accept"); v != "" { headers["Accept"] = v }
    if v := r.Header.Get("Accept-Language"); v != "" { headers["Accept-Language"] = v }
    if v := r.Header.Get("User-Agent"); v != "" { headers["User-Agent"] = v }
    rid := r.Header.Get("X-Request-Id"); if rid == "" { rid = logID }
    headers["X-Request-Id"] = rid
    headers["Cache-Control"] = "no-store"

    job := newEmbeddingPartitionJob(payload, inputs)
    // Record generic request metric (job-level). We record start in the worker path/observer.
    basemetrics.RecordRequest("llm", "worker", "llm.embedding", model)
    body, status, ok, errMsg := baseworker.HandlePartitionedJob(ctx, reg, sched, metrics, model, headers, job, maxParallel, timeout, logID)
    w.Header().Set("Content-Type", "application/json")
    if !ok {
        if status == 0 { status = http.StatusBadGateway }
        w.WriteHeader(status)
        if len(body) > 0 { _, _ = w.Write(body) } else { _, _ = w.Write([]byte(`{"error":"` + errMsg + `"}`)) }
        return
    }
    if _, err := w.Write(body); err != nil { logx.Log.Error().Err(err).Msg("write embeddings response") }
}
