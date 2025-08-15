package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
	"github.com/you/llamapool/internal/metrics"
)

// GenerateRequest is the minimal request for generation.
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

var (
	ErrNoWorker     = errors.New("no worker")
	ErrWorkerFailed = errors.New("worker failure")
	ErrWorkerBusy   = errors.New("worker busy")
)

// RelayGenerateStream relays streaming generate requests to a worker.
func RelayGenerateStream(ctx context.Context, reg *ctrl.Registry, metricsReg *ctrl.MetricsRegistry, sched ctrl.Scheduler, req GenerateRequest, w http.ResponseWriter) error {
	exact := reg.WorkersForModel(req.Model)
	worker, err := sched.PickWorker(req.Model)
	if err != nil {
		return ErrNoWorker
	}
	if len(exact) == 0 {
		if key, ok := ctrl.AliasKey(req.Model); ok {
			logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", req.Model).Str("alias_key", key).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("alias fallback")
		}
	}
	reg.IncInFlight(worker.ID)
	defer reg.DecInFlight(worker.ID)

	jobID := uuid.NewString()
	reqID := chiMiddleware.GetReqID(ctx)
	logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("dispatch")
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	select {
	case worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}:
		metricsReg.RecordJobStart(worker.ID)
		metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusWorking)
	default:
		return ErrWorkerBusy
	}
	start := time.Now()
	var tokensIn, tokensOut uint64
	var success bool
	var errMsg string
	defer func() {
		dur := time.Since(start)
		metricsReg.RecordJobEnd(worker.ID, req.Model, dur, tokensIn, tokensOut, success, errMsg)
		metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
		metrics.ObserveRequestDuration(worker.ID, req.Model, dur)
		metrics.RecordModelRequest(req.Model, success)
		if tokensIn > 0 {
			metrics.RecordModelTokens(req.Model, "in", tokensIn)
		}
		if tokensOut > 0 {
			metrics.RecordModelTokens(req.Model, "out", tokensOut)
		}
	}()

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	for {
		select {
		case <-ctx.Done():
			errMsg = ctx.Err().Error()
			select {
			case worker.Send <- ctrl.CancelJobMessage{Type: "cancel_job", JobID: jobID}:
			default:
			}
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				errMsg = "closed"
				if err := enc.Encode(map[string]any{"done": true}); err != nil {
					return err
				}
				return ErrWorkerFailed
			}
			switch m := msg.(type) {
			case ctrl.JobChunkMessage:
				var data map[string]interface{}
				if err := json.Unmarshal(m.Data, &data); err == nil {
					if err := enc.Encode(data); err != nil {
						return err
					}
					if flusher != nil {
						flusher.Flush()
					}
					if done, ok := data["done"].(bool); ok && done {
						if v, ok := data["prompt_eval_count"].(float64); ok {
							tokensIn = uint64(v)
						}
						if v, ok := data["eval_count"].(float64); ok {
							tokensOut = uint64(v)
						}
						success = true
						logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("complete")
						return nil
					}
				}
			case ctrl.JobErrorMessage:
				errMsg = m.Message
				if err := enc.Encode(map[string]any{"done": true}); err != nil {
					return err
				}
				logx.Log.Warn().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("error")
				return ErrWorkerFailed
			case ctrl.JobResultMessage:
				var data map[string]interface{}
				if err := json.Unmarshal(m.Data, &data); err == nil {
					if err := enc.Encode(data); err != nil {
						return err
					}
					if v, ok := data["prompt_eval_count"].(float64); ok {
						tokensIn = uint64(v)
					}
					if v, ok := data["eval_count"].(float64); ok {
						tokensOut = uint64(v)
					}
				}
				if flusher != nil {
					flusher.Flush()
				}
				success = true
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("complete")
				if done, ok := data["done"].(bool); ok && done {
				} else {
					if err := enc.Encode(map[string]any{"done": true}); err != nil {
						return err
					}
					if flusher != nil {
						flusher.Flush()
					}
				}
				return nil
			}
		}
	}
}

// RelayGenerateOnce handles non-streaming requests.
func RelayGenerateOnce(ctx context.Context, reg *ctrl.Registry, metricsReg *ctrl.MetricsRegistry, sched ctrl.Scheduler, req GenerateRequest) (any, error) {
	exact := reg.WorkersForModel(req.Model)
	worker, err := sched.PickWorker(req.Model)
	if err != nil {
		return nil, ErrNoWorker
	}
	if len(exact) == 0 {
		if key, ok := ctrl.AliasKey(req.Model); ok {
			logx.Log.Info().Str("event", "alias_fallback").Str("requested_id", req.Model).Str("alias_key", key).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("alias fallback")
		}
	}
	reg.IncInFlight(worker.ID)
	defer reg.DecInFlight(worker.ID)

	jobID := uuid.NewString()
	reqID := chiMiddleware.GetReqID(ctx)
	logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("dispatch")
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	select {
	case worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}:
		metricsReg.RecordJobStart(worker.ID)
		metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusWorking)
	default:
		return nil, ErrWorkerBusy
	}
	start := time.Now()
	var tokensIn, tokensOut uint64
	var errMsg string
	var success bool
	defer func() {
		dur := time.Since(start)
		metricsReg.RecordJobEnd(worker.ID, req.Model, dur, tokensIn, tokensOut, success, errMsg)
		metricsReg.SetWorkerStatus(worker.ID, ctrl.StatusIdle)
		metrics.ObserveRequestDuration(worker.ID, req.Model, dur)
		metrics.RecordModelRequest(req.Model, success)
		if tokensIn > 0 {
			metrics.RecordModelTokens(req.Model, "in", tokensIn)
		}
		if tokensOut > 0 {
			metrics.RecordModelTokens(req.Model, "out", tokensOut)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			errMsg = ctx.Err().Error()
			select {
			case worker.Send <- ctrl.CancelJobMessage{Type: "cancel_job", JobID: jobID}:
			default:
			}
			return nil, ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				errMsg = "closed"
				return nil, ErrWorkerFailed
			}
			switch m := msg.(type) {
			case ctrl.JobResultMessage:
				var v map[string]interface{}
				if err := json.Unmarshal(m.Data, &v); err != nil {
					return nil, err
				}
				if val, ok := v["prompt_eval_count"].(float64); ok {
					tokensIn = uint64(val)
				}
				if val, ok := v["eval_count"].(float64); ok {
					tokensOut = uint64(val)
				}
				success = true
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("complete")
				return v, nil
			case ctrl.JobErrorMessage:
				errMsg = m.Message
				logx.Log.Warn().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("error")
				return nil, ErrWorkerFailed
			case ctrl.JobChunkMessage:
				var v map[string]interface{}
				if err := json.Unmarshal(m.Data, &v); err != nil {
					return nil, err
				}
				if val, ok := v["prompt_eval_count"].(float64); ok {
					tokensIn = uint64(val)
				}
				if val, ok := v["eval_count"].(float64); ok {
					tokensOut = uint64(val)
				}
				success = true
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Str("worker_name", worker.Name).Msg("complete")
				return v, nil
			}
		}
	}
}
