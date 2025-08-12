package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/you/llamapool/internal/ctrl"
	"github.com/you/llamapool/internal/logx"
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
func RelayGenerateStream(ctx context.Context, reg *ctrl.Registry, sched ctrl.Scheduler, req GenerateRequest, w http.ResponseWriter) error {
	worker, err := sched.PickWorker(req.Model)
	if err != nil {
		return ErrNoWorker
	}
	reg.IncInFlight(worker.ID)
	defer reg.DecInFlight(worker.ID)

	jobID := uuid.NewString()
	reqID := chiMiddleware.GetReqID(ctx)
	logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("dispatch")
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	select {
	case worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}:
	default:
		return ErrWorkerBusy
	}
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	doneSent := false
	for {
		select {
		case <-ctx.Done():
			select {
			case worker.Send <- ctrl.CancelJobMessage{Type: "cancel_job", JobID: jobID}:
			default:
			}
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				if !doneSent {
					enc.Encode(map[string]any{"done": true})
				}
				return ErrWorkerFailed
			}
			switch m := msg.(type) {
			case ctrl.JobChunkMessage:
				var data map[string]interface{}
				if err := json.Unmarshal(m.Data, &data); err == nil {
					enc.Encode(data)
					if flusher != nil {
						flusher.Flush()
					}
					if done, ok := data["done"].(bool); ok && done {
						doneSent = true
						logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("complete")
						return nil
					}
				}
			case ctrl.JobErrorMessage:
				if !doneSent {
					enc.Encode(map[string]any{"done": true})
				}
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("error")
				return ErrWorkerFailed
			case ctrl.JobResultMessage:
				var data map[string]interface{}
				if err := json.Unmarshal(m.Data, &data); err == nil {
					enc.Encode(data)
				}
				if flusher != nil {
					flusher.Flush()
				}
				doneSent = true
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("complete")
				if done, ok := data["done"].(bool); ok && done {
				} else {
					enc.Encode(map[string]any{"done": true})
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
func RelayGenerateOnce(ctx context.Context, reg *ctrl.Registry, sched ctrl.Scheduler, req GenerateRequest) (any, error) {
	worker, err := sched.PickWorker(req.Model)
	if err != nil {
		return nil, ErrNoWorker
	}
	reg.IncInFlight(worker.ID)
	defer reg.DecInFlight(worker.ID)

	jobID := uuid.NewString()
	reqID := chiMiddleware.GetReqID(ctx)
	logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("dispatch")
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	select {
	case worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}:
	default:
		return nil, ErrWorkerBusy
	}

	for {
		select {
		case <-ctx.Done():
			select {
			case worker.Send <- ctrl.CancelJobMessage{Type: "cancel_job", JobID: jobID}:
			default:
			}
			return nil, ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil, ErrWorkerFailed
			}
			switch m := msg.(type) {
			case ctrl.JobResultMessage:
				var v any
				if err := json.Unmarshal(m.Data, &v); err != nil {
					return nil, err
				}
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("complete")
				return v, nil
			case ctrl.JobErrorMessage:
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("error")
				return nil, ErrWorkerFailed
			case ctrl.JobChunkMessage:
				var v any
				if err := json.Unmarshal(m.Data, &v); err != nil {
					return nil, err
				}
				logx.Log.Info().Str("request_id", reqID).Str("job_id", jobID).Str("worker_id", worker.ID).Msg("complete")
				return v, nil
			}
		}
	}
}
