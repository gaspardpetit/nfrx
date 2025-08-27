package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	ctrl "github.com/gaspardpetit/nfrx-sdk/contracts/control"
	"github.com/gaspardpetit/nfrx/modules/common/logx"
	ctrlsrv "github.com/gaspardpetit/nfrx/server/internal/ctrlsrv"
	"github.com/gaspardpetit/nfrx/server/internal/metrics"
	"github.com/gaspardpetit/nfrx/server/internal/serverstate"
)

// EmbeddingsHandler handles POST /api/v1/embeddings as a pass-through.
func EmbeddingsHandler(reg *ctrlsrv.Registry, sched ctrlsrv.Scheduler, metricsReg *ctrlsrv.MetricsRegistry, timeout time.Duration, maxParallel int) http.HandlerFunc {
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
				handleEmbeddingBatches(w, r, reg, metricsReg, timeout, meta.Model, payload, inputs, maxParallel)
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
			metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusWorking)
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
		embeddingsCount := uint64(1)
		var idle *time.Timer
		var timeoutCh <-chan time.Time
		if timeout > 0 {
			idle = time.NewTimer(timeout)
			timeoutCh = idle.C
			defer idle.Stop()
		}

		defer func() {
			dur := time.Since(start)
			metricsReg.RecordJobEnd(worker.ID, meta.Model, dur, 0, 0, embeddingsCount, success, errMsg)
			metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusIdle)
			metrics.ObserveRequestDuration(worker.ID, meta.Model, dur)
			metrics.RecordModelRequest(meta.Model, success)
			if success {
				metrics.RecordWorkerEmbeddingProcessingTime(worker.ID, dur)
				metrics.RecordWorkerEmbeddings(worker.ID, embeddingsCount)
				metrics.RecordModelEmbeddings(meta.Model, embeddingsCount)
			}
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

func handleEmbeddingBatches(w http.ResponseWriter, r *http.Request, reg *ctrlsrv.Registry, metricsReg *ctrlsrv.MetricsRegistry, timeout time.Duration, model string, payload map[string]json.RawMessage, inputs []json.RawMessage, maxParallel int) {
	ctx := r.Context()
	var cancel context.CancelFunc
	logID := chiMiddleware.GetReqID(ctx)
	exact := reg.WorkersForModel(model)
	if maxParallel <= 0 {
		maxParallel = 1
	}

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

	basePayload := make(map[string]json.RawMessage, len(payload))
	for k, v := range payload {
		if k != "input" {
			basePayload[k] = v
		}
	}

	workers := reg.WorkersForModel(model)
	if len(workers) == 0 {
		workers = reg.WorkersForAlias(model)
	}
	if len(workers) == 0 {
		logx.Log.Warn().Str("model", model).Msg("no worker")
		http.Error(w, "no worker", http.StatusNotFound)
		return
	}
	sort.Slice(workers, func(i, j int) bool {
		if workers[i].InFlight == workers[j].InFlight {
			return workers[i].EmbeddingBatchSize > workers[j].EmbeddingBatchSize
		}
		return workers[i].InFlight < workers[j].InFlight
	})
	if len(workers) > maxParallel {
		workers = workers[:maxParallel]
	}
	if len(workers) > len(inputs) {
		workers = workers[:len(inputs)]
	}
	if len(exact) == 0 {
		if key, ok := ctrl.AliasKey(model); ok {
			logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", model).Str("alias_key", key).Str("worker_id", workers[0].ID).Str("worker_name", workers[0].Name).Msg("alias fallback")
		}
	}

	// If only one worker is available, fall back to sequential batching respecting its batch size.
	if len(workers) == 1 {
		worker := workers[0]
		remaining := inputs
		var allData []json.RawMessage
		var usage embeddingUsage
		var respModel string
		for len(remaining) > 0 {
			batch := worker.EmbeddingBatchSize
			if batch <= 0 || batch > len(remaining) {
				batch = len(remaining)
			}
			b, _ := json.Marshal(remaining[:batch])
			mp := make(map[string]json.RawMessage, len(basePayload)+1)
			for k, v := range basePayload {
				mp[k] = v
			}
			mp["input"] = b
			body, _ := json.Marshal(mp)
			reg.IncInFlight(worker.ID)
			reqID := uuid.NewString()
			logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Msg("dispatch")
			respBody, status, success, errMsg := proxyEmbeddingOnce(ctx, worker, reqID, logID, model, headers, body, metricsReg, batch, timeout)
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
		return
	}

	type batchTask struct {
		start     int
		inputs    []json.RawMessage
		attempted map[string]bool
	}

	// Split inputs proportionally to workers' batch sizes.
	totalWeight := 0
	weights := make([]int, len(workers))
	for i, wkr := range workers {
		weight := wkr.EmbeddingBatchSize
		if weight <= 0 {
			weight = len(inputs)
		}
		weights[i] = weight
		totalWeight += weight
	}
	tasks := make([]batchTask, len(workers))
	remaining := len(inputs)
	offset := 0
	remainingWeight := totalWeight
	for i := range workers {
		size := remaining
		if i < len(workers)-1 {
			portion := remaining * weights[i] / remainingWeight
			if portion < 1 {
				portion = 1
			}
			if portion > remaining {
				portion = remaining
			}
			size = portion
			remaining -= portion
			remainingWeight -= weights[i]
		}
		tasks[i] = batchTask{start: offset, inputs: inputs[offset : offset+size], attempted: make(map[string]bool)}
		offset += size
	}

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	type taskResult struct {
		start  int
		data   []json.RawMessage
		usage  embeddingUsage
		model  string
		status int
		body   []byte
		errMsg string
		failed bool
	}

	resCh := make(chan taskResult, len(tasks))

	var mu sync.Mutex
	used := make(map[string]bool)
	cond := sync.NewCond(&mu)
	for _, wkr := range workers {
		used[wkr.ID] = true
	}
	acquire := func(exclude map[string]bool) (*ctrlsrv.Worker, error) {
		mu.Lock()
		defer mu.Unlock()
		for {
			for _, w := range workers {
				if used[w.ID] || exclude[w.ID] {
					continue
				}
				used[w.ID] = true
				return w, nil
			}
			if len(exclude) >= len(workers) {
				return nil, errors.New("no worker")
			}
			cond.Wait()
		}
	}
	release := func(wk *ctrlsrv.Worker) {
		mu.Lock()
		delete(used, wk.ID)
		mu.Unlock()
		cond.Signal()
	}

	var wg sync.WaitGroup
	for i, wk := range workers {
		t := tasks[i]
		wg.Add(1)
		go func(t batchTask, wk *ctrlsrv.Worker) {
			defer wg.Done()
			current := wk
			for {
				reqID := uuid.NewString()
				reg.IncInFlight(current.ID)
				b, _ := json.Marshal(t.inputs)
				mp := make(map[string]json.RawMessage, len(basePayload)+1)
				for k, v := range basePayload {
					mp[k] = v
				}
				mp["input"] = b
				body, _ := json.Marshal(mp)
				logx.Log.Info().Str("request_id", logID).Str("worker_id", current.ID).Str("worker_name", current.Name).Str("model", model).Msg("dispatch")
				respBody, status, success, errMsg := proxyEmbeddingOnce(ctx, current, reqID, logID, model, headers, body, metricsReg, len(t.inputs), timeout)
				reg.DecInFlight(current.ID)
				current.RemoveJob(reqID)
				if success {
					var resp embeddingResponse
					_ = json.Unmarshal(respBody, &resp)
					resCh <- taskResult{start: t.start, data: resp.Data, usage: resp.Usage, model: resp.Model}
					release(current)
					return
				}
				t.attempted[current.ID] = true
				release(current)
				if len(t.attempted) >= len(workers) {
					resCh <- taskResult{start: t.start, status: status, body: respBody, errMsg: errMsg, failed: true}
					return
				}
				next, err := acquire(t.attempted)
				if err != nil {
					resCh <- taskResult{start: t.start, status: status, body: respBody, errMsg: errMsg, failed: true}
					return
				}
				current = next
			}
		}(t, wk)
	}

	wg.Wait()
	close(resCh)

	finalData := make([]json.RawMessage, len(inputs))
	var usage embeddingUsage
	var respModel string
	failed := false
	var errBody []byte
	var status int
	var errMsg string
	for res := range resCh {
		if res.failed {
			failed = true
			if status == 0 {
				status = res.status
			}
			if len(errBody) == 0 {
				errBody = res.body
			}
			if errMsg == "" {
				errMsg = res.errMsg
			}
			continue
		}
		copy(finalData[res.start:], res.data)
		usage.PromptTokens += res.usage.PromptTokens
		usage.TotalTokens += res.usage.TotalTokens
		if respModel == "" {
			respModel = res.model
		}
	}

	if failed {
		w.Header().Set("Content-Type", "application/json")
		if status == 0 {
			status = http.StatusBadGateway
		}
		w.WriteHeader(status)
		if len(errBody) > 0 {
			_, _ = w.Write(errBody)
		} else {
			if errMsg == "" {
				errMsg = "upstream_error"
			}
			_, _ = w.Write([]byte(`{"error":"` + errMsg + `"}`))
		}
		return
	}

	final := embeddingResponse{Object: "list", Data: finalData, Model: respModel, Usage: usage}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(final); err != nil {
		logx.Log.Error().Err(err).Msg("write embeddings response")
	}
}

func proxyEmbeddingOnce(ctx context.Context, worker *ctrlsrv.Worker, reqID, logID, model string, headers map[string]string, body []byte, metricsReg *ctrlsrv.MetricsRegistry, embeddings int, timeout time.Duration) ([]byte, int, bool, string) {
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
		metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusWorking)
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
			metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, 0, success, errMsg)
			metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusIdle)
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
				metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, 0, false, errMsg)
				metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusIdle)
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
				metricsReg.RecordJobEnd(worker.ID, model, time.Since(start), 0, 0, 0, false, errMsg)
				metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusIdle)
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
				embCount := uint64(0)
				if success {
					embCount = uint64(embeddings)
				}
				metricsReg.RecordJobEnd(worker.ID, model, dur, 0, 0, embCount, success, errMsg)
				if success {
					metrics.RecordWorkerEmbeddingProcessingTime(worker.ID, dur)
					metrics.RecordWorkerEmbeddings(worker.ID, embCount)
					metrics.RecordModelEmbeddings(model, embCount)
				}
				metricsReg.SetWorkerStatus(worker.ID, ctrlsrv.StatusIdle)
				metrics.ObserveRequestDuration(worker.ID, model, dur)
				metrics.RecordModelRequest(model, success)
				logx.Log.Info().Str("request_id", logID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Str("model", model).Dur("duration", dur).Msg("complete")
				return buf.Bytes(), status, success && errMsg == "", errMsg
			}
		}
	}
}
