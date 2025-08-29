package worker

import (
    "bytes"
    "context"
    "errors"
    "net/http"
    "sort"
    "time"

    "github.com/google/uuid"

    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    "github.com/gaspardpetit/nfrx/sdk/api/spi"
    "github.com/gaspardpetit/nfrx/core/logx"
)

// HandlePartitionedJob dispatches a partitionable request across one or more workers
// and returns the combined response body, status, success flag and error message.
func HandlePartitionedJob(
    ctx context.Context,
    reg spi.WorkerRegistry,
    sched spi.Scheduler,
    metrics spi.Metrics,
    model string,
    headers map[string]string,
    job spi.PartitionJob,
    maxParallel int,
    timeout time.Duration,
    logID string,
) ([]byte, int, bool, string) {
    size := job.Size()
    if size <= 0 { return nil, http.StatusBadRequest, false, "empty" }
    if maxParallel <= 0 { maxParallel = 1 }

    // Determine eligible workers
    workers := reg.WorkersForModel(model)
    if len(workers) == 0 {
        // Fallback to best single worker via scheduler scoring
        w, err := sched.PickWorker(model)
        if err != nil { return []byte(`{"error":"no_worker"}`), http.StatusNotFound, false, "no_worker" }
        workers = []spi.WorkerRef{w}
    }
    sort.Slice(workers, func(i, j int) bool {
        if workers[i].InFlight() == workers[j].InFlight() {
            return workers[i].EmbeddingBatchSize() > workers[j].EmbeddingBatchSize()
        }
        return workers[i].InFlight() < workers[j].InFlight()
    })
    if len(workers) > maxParallel { workers = workers[:maxParallel] }
    if len(workers) > size { workers = workers[:size] }

    // Single-worker sequential batching
    if len(workers) == 1 {
        wk := workers[0]
        for start := 0; start < size; {
            batch := wk.EmbeddingBatchSize()
            remaining := size - start
            if batch <= 0 || batch > remaining { batch = remaining }
            body, n := job.MakeChunk(start, batch)
            if n <= 0 { break }
            reqID := uuid.NewString()
            reg.IncInFlight(wk.ID())
            respBody, status, ok, errMsg := proxyHTTPOnce(ctx, wk, reqID, logID, model, headers, job.Path(), body, metrics, n, timeout)
            reg.DecInFlight(wk.ID())
            wk.RemoveJob(reqID)
            if !ok { return respBody, status, false, errMsg }
            if err := job.Append(respBody, start); err != nil { return []byte(`{"error":"aggregate_failed"}`), http.StatusBadGateway, false, "aggregate_failed" }
            start += n
        }
        return job.Result(), http.StatusOK, true, ""
    }

    // Multi-worker partitioning
    type task struct { start, count int; attempted map[string]bool }
    // Weights based on embedding batch size; default to remaining size
    weights := make([]int, len(workers))
    total := 0
    for i, w := range workers { n := w.EmbeddingBatchSize(); if n <= 0 { n = size } ; weights[i] = n ; total += n }
    tasks := make([]task, len(workers))
    remaining := size
    offset := 0
    remW := total
    for i := range workers {
        cnt := remaining
        if i < len(workers)-1 {
            portion := remaining * weights[i] / remW
            if portion < 1 { portion = 1 }
            if portion > remaining { portion = remaining }
            cnt = portion
            remaining -= portion
            remW -= weights[i]
        }
        tasks[i] = task{start: offset, count: cnt, attempted: make(map[string]bool)}
        offset += cnt
    }

    type result struct { start int; body []byte; status int; err string; ok bool }
    resCh := make(chan result, len(tasks))

    // Worker acquire/release among the pool
    used := map[string]bool{}
    for _, w := range workers { used[w.ID()] = true }
    mu := make(chan struct{}, 1); mu <- struct{}{}
    cond := make(chan struct{}, 1)
    acquire := func(exclude map[string]bool) (spi.WorkerRef, error) {
        for {
            <-mu
            for _, w := range workers {
                if used[w.ID()] || exclude[w.ID()] { continue }
                used[w.ID()] = true
                mu <- struct{}{}
                return w, nil
            }
            mu <- struct{}{}
            if len(exclude) >= len(workers) { return nil, errors.New("no worker") }
            <-cond
        }
    }
    release := func(w spi.WorkerRef) { <-mu; delete(used, w.ID()); mu <- struct{}{} ; select { case cond <- struct{}{}: default: } }

    for i, w := range workers {
        t := tasks[i]
        go func(first spi.WorkerRef, t task) {
            current := first
            for {
                body, n := job.MakeChunk(t.start, t.count)
                if n <= 0 { resCh <- result{start: t.start, body: nil, status: http.StatusBadRequest, err: "empty", ok: false}; return }
                reqID := uuid.NewString()
                reg.IncInFlight(current.ID())
                rb, status, ok, errMsg := proxyHTTPOnce(ctx, current, reqID, logID, model, headers, job.Path(), body, metrics, n, timeout)
                reg.DecInFlight(current.ID())
                current.RemoveJob(reqID)
                if ok { resCh <- result{start: t.start, body: rb, ok: true}; release(current); return }
                t.attempted[current.ID()] = true
                release(current)
                if len(t.attempted) >= len(workers) { resCh <- result{start: t.start, body: rb, status: status, err: errMsg, ok: false}; return }
                next, err := acquire(t.attempted)
                if err != nil { resCh <- result{start: t.start, body: rb, status: status, err: errMsg, ok: false}; return }
                current = next
            }
        }(w, t)
    }

    // Collect results
    for i := 0; i < len(tasks); i++ {
        r := <-resCh
        if !r.ok { return r.body, r.status, false, r.err }
        if err := job.Append(r.body, r.start); err != nil { return []byte(`{"error":"aggregate_failed"}`), http.StatusBadGateway, false, "aggregate_failed" }
    }
    return job.Result(), http.StatusOK, true, ""
}

// proxyHTTPOnce sends one HTTP proxy request to a worker and collects the full response body.
func proxyHTTPOnce(ctx context.Context, worker spi.WorkerRef, reqID, logID, model string, headers map[string]string, path string, body []byte, metrics spi.Metrics, elements int, timeout time.Duration) ([]byte, int, bool, string) {
    ch := make(chan interface{}, 16)
    worker.AddJob(reqID, ch)

    msg := ctrl.HTTPProxyRequestMessage{ Type: "http_proxy_request", RequestID: reqID, Method: http.MethodPost, Path: path, Headers: headers, Stream: false, Body: body }
    select {
    case worker.SendChan() <- msg:
        metrics.RecordJobStart(worker.ID())
        metrics.SetWorkerStatus(worker.ID(), spi.StatusWorking)
    default:
        logx.Log.Warn().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", model).Msg("worker busy")
        return []byte(`{"error":"worker_busy"}`), http.StatusServiceUnavailable, false, "worker_busy"
    }

    start := time.Now()
    var status int
    var buf bytes.Buffer
    success := false
    errMsg := ""
    var idle *time.Timer
    var timeoutCh <-chan time.Time
    if timeout > 0 { idle = time.NewTimer(timeout); timeoutCh = idle.C; defer idle.Stop() }
    for {
        select {
        case <-ctx.Done():
            select { case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}: default: }
            errMsg = "canceled"
            metrics.RecordJobEnd(worker.ID(), model, time.Since(start), 0, 0, 0, success, errMsg)
            metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
            return nil, status, false, errMsg
        case <-timeoutCh:
            hb := worker.LastHeartbeat(); since := time.Since(hb)
            if since > timeout {
                errMsg = "timeout"
                select { case worker.SendChan() <- ctrl.HTTPProxyCancelMessage{Type: "http_proxy_cancel", RequestID: reqID}: default: }
                metrics.RecordJobEnd(worker.ID(), model, time.Since(start), 0, 0, 0, false, errMsg)
                metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
                return nil, http.StatusGatewayTimeout, false, errMsg
            }
            if idle != nil { idle.Reset(timeout - since); timeoutCh = idle.C }
        case msg, ok := <-ch:
            if !ok {
                errMsg = "closed"
                metrics.RecordJobEnd(worker.ID(), model, time.Since(start), 0, 0, 0, false, errMsg)
                metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
                return nil, http.StatusBadGateway, false, errMsg
            }
            if idle != nil { if !idle.Stop() { <-timeoutCh } ; idle.Reset(timeout); timeoutCh = idle.C }
            switch m := msg.(type) {
            case ctrl.HTTPProxyResponseHeadersMessage:
                status = m.Status
            case ctrl.HTTPProxyResponseChunkMessage:
                if len(m.Data) > 0 { buf.Write(m.Data) }
            case ctrl.HTTPProxyResponseEndMessage:
                if m.Error != nil { errMsg = m.Error.Message } else { success = status < http.StatusBadRequest }
                dur := time.Since(start)
                embCount := uint64(0); if success { embCount = uint64(elements) }
                metrics.RecordJobEnd(worker.ID(), model, dur, 0, 0, embCount, success, errMsg)
                metrics.SetWorkerStatus(worker.ID(), spi.StatusIdle)
                logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID()).Str("worker_name", worker.Name()).Str("model", model).Dur("duration", dur).Msg("complete")
                return buf.Bytes(), status, success && errMsg == "", errMsg
            }
        }
    }
}

