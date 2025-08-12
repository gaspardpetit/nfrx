package relay

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/you/llamapool/internal/ctrl"
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
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
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
						return nil
					}
				}
			case ctrl.JobErrorMessage:
				return ErrWorkerFailed
			case ctrl.JobResultMessage:
				var data map[string]interface{}
				if err := json.Unmarshal(m.Data, &data); err == nil {
					enc.Encode(data)
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
	ch := make(chan interface{}, 16)
	worker.AddJob(jobID, ch)
	defer func() {
		worker.RemoveJob(jobID)
		close(ch)
	}()

	worker.Send <- ctrl.JobRequestMessage{Type: "job_request", JobID: jobID, Endpoint: "generate", Payload: req}

	for {
		select {
		case <-ctx.Done():
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
				return v, nil
			case ctrl.JobErrorMessage:
				return nil, ErrWorkerFailed
			case ctrl.JobChunkMessage:
				var v any
				if err := json.Unmarshal(m.Data, &v); err != nil {
					return nil, err
				}
				return v, nil
			}
		}
	}
}
