package openai

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/gaspardpetit/nfrx/sdk/api/spi"
)

type targetedRegistry struct {
	base     spi.WorkerRegistry
	targetID string
}

func (r targetedRegistry) WorkersForLabel(label string) []spi.WorkerRef {
	var out []spi.WorkerRef
	for _, w := range r.base.WorkersForLabel(label) {
		if w.ID() == r.targetID {
			out = append(out, w)
		}
	}
	return out
}

func (r targetedRegistry) IncInFlight(id string) { r.base.IncInFlight(id) }
func (r targetedRegistry) DecInFlight(id string) { r.base.DecInFlight(id) }
func (r targetedRegistry) AggregatedModels() []spi.ModelInfo {
	return r.base.WorkerModels(r.targetID)
}
func (r targetedRegistry) AggregatedModel(id string) (spi.ModelInfo, bool) {
	for _, m := range r.base.WorkerModels(r.targetID) {
		if m.ID == id {
			return m, true
		}
	}
	return spi.ModelInfo{}, false
}
func (r targetedRegistry) HasWorker(id string) bool {
	return id == r.targetID && r.base.HasWorker(id)
}
func (r targetedRegistry) WorkerModels(id string) []spi.ModelInfo {
	if id != r.targetID {
		return nil
	}
	return r.base.WorkerModels(id)
}

type targetedScheduler struct {
	reg      spi.WorkerRegistry
	targetID string
}

func (s targetedScheduler) PickWorker(model string) (spi.WorkerRef, error) {
	workers := s.reg.WorkersForLabel(model)
	if len(workers) == 0 {
		return nil, errors.New("no worker")
	}
	best := workers[0]
	for _, w := range workers[1:] {
		if w.InFlight() < best.InFlight() {
			best = w
		}
	}
	return best, nil
}

func targetFromRequest(reg spi.WorkerRegistry, r *http.Request) (spi.WorkerRegistry, spi.Scheduler, string) {
	id := chi.URLParam(r, "id")
	tr := targetedRegistry{base: reg, targetID: id}
	return tr, targetedScheduler{reg: tr, targetID: id}, id
}

func TargetedListModelsHandler(reg spi.WorkerRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, _, id := targetFromRequest(reg, r)
		if !tr.HasWorker(id) {
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		ListModelsHandler(tr).ServeHTTP(w, r)
	}
}

func TargetedGetModelHandler(reg spi.WorkerRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, _, id := targetFromRequest(reg, r)
		if !tr.HasWorker(id) {
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		modelID := chi.URLParam(r, "model")
		m, ok := tr.AggregatedModel(modelID)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "model_not_found"})
			return
		}
		resp := struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		}{ID: m.ID, Object: "model", Created: m.Created, OwnedBy: strings.Join(m.Owners, ",")}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TargetedChatCompletionsHandler(reg spi.WorkerRegistry, metrics spi.Metrics, opts Options, queue *CompletionQueue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, ts, id := targetFromRequest(reg, r)
		if !tr.HasWorker(id) {
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		ChatCompletionsHandler(tr, ts, metrics, opts, queue).ServeHTTP(w, r)
	}
}

func TargetedResponsesHandler(reg spi.WorkerRegistry, metrics spi.Metrics, opts Options, queue *CompletionQueue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, ts, id := targetFromRequest(reg, r)
		if !tr.HasWorker(id) {
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		ResponsesHandler(tr, ts, metrics, opts, queue).ServeHTTP(w, r)
	}
}

func TargetedEmbeddingsHandler(reg spi.WorkerRegistry, metrics spi.Metrics, timeout time.Duration, maxParallel int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, ts, id := targetFromRequest(reg, r)
		if !tr.HasWorker(id) {
			http.Error(w, "no worker", http.StatusNotFound)
			return
		}
		EmbeddingsHandler(tr, ts, metrics, timeout, maxParallel).ServeHTTP(w, r)
	}
}
