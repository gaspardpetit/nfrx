package ctrlsrv

import (
	"sync"
	"time"

    ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
    "github.com/gaspardpetit/nfrx/core/logx"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatExpiry   = 3 * HeartbeatInterval
)

type Worker struct {
	ID                 string
	Name               string
	Models             map[string]bool
	MaxConcurrency     int
	EmbeddingBatchSize int
	InFlight           int
	LastHeartbeat      time.Time
	Send               chan interface{}
	Jobs               map[string]chan interface{}
	mu                 sync.Mutex
}

type Registry struct {
	mu             sync.RWMutex
	workers        map[string]*Worker
	modelFirstSeen map[string]int64
}

func NewRegistry() *Registry {
	return &Registry{workers: make(map[string]*Worker), modelFirstSeen: make(map[string]int64)}
}

func (r *Registry) Add(w *Worker) {
	r.mu.Lock()
	r.workers[w.ID] = w
	for m := range w.Models {
		if _, ok := r.modelFirstSeen[m]; !ok {
			r.modelFirstSeen[m] = time.Now().Unix()
		}
	}
	r.mu.Unlock()
}

func (r *Registry) Remove(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		delete(r.workers, id)
		w.mu.Lock()
		for id, ch := range w.Jobs {
			if ch != nil {
				close(ch)
			}
			delete(w.Jobs, id)
		}
		w.mu.Unlock()
		if w.Send != nil {
			close(w.Send)
		}
	}
	r.mu.Unlock()
}

func (r *Registry) WorkerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.workers)
}

func (r *Registry) UpdateHeartbeat(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.LastHeartbeat = time.Now()
	}
	r.mu.Unlock()
}

func (r *Registry) UpdateModels(id string, models []string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.Models = make(map[string]bool)
		for _, m := range models {
			w.Models[m] = true
			if _, ok := r.modelFirstSeen[m]; !ok {
				r.modelFirstSeen[m] = time.Now().Unix()
			}
		}
	}
	r.mu.Unlock()
}

func (r *Registry) WorkersForModel(model string) []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*Worker
	for _, w := range r.workers {
		w.mu.Lock()
		if w.Models[model] && w.InFlight < w.MaxConcurrency {
			res = append(res, w)
		}
		w.mu.Unlock()
	}
	return res
}

// WorkersForAlias returns any worker that exposes a model whose alias matches the alias of requested.
func (r *Registry) WorkersForAlias(requested string) []*Worker {
	key, ok := ctrl.AliasKey(requested)
	if !ok {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []*Worker
	for _, w := range r.workers {
		w.mu.Lock()
		for m := range w.Models {
			if ak, ok := ctrl.AliasKey(m); ok && ak == key && w.InFlight < w.MaxConcurrency {
				res = append(res, w)
				break
			}
		}
		w.mu.Unlock()
	}
	return res
}

func (r *Registry) IncInFlight(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.InFlight++
	}
	r.mu.Unlock()
}

func (r *Registry) DecInFlight(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok && w.InFlight > 0 {
		w.InFlight--
	}
	r.mu.Unlock()
}

func (r *Registry) Models() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := make(map[string]struct{})
	for _, w := range r.workers {
		w.mu.Lock()
		for m := range w.Models {
			set[m] = struct{}{}
		}
		w.mu.Unlock()
	}
	var models []string
	for m := range set {
		models = append(models, m)
	}
	return models
}

func (r *Registry) PruneExpired(maxAge time.Duration) {
	r.mu.Lock()
	for id, w := range r.workers {
		if time.Since(w.LastHeartbeat) > maxAge {
			delete(r.workers, id)
			w.mu.Lock()
			for jobID, ch := range w.Jobs {
				if ch != nil {
					close(ch)
				}
				delete(w.Jobs, jobID)
			}
			w.mu.Unlock()
			close(w.Send)
			logx.Log.Info().Str("worker_id", id).Str("reason", "heartbeat_expired").Msg("evicted")
		}
	}
	r.mu.Unlock()
}

func (w *Worker) AddJob(id string, ch chan interface{}) {
	w.mu.Lock()
	w.Jobs[id] = ch
	w.mu.Unlock()
}

func (w *Worker) RemoveJob(id string) {
	w.mu.Lock()
	if ch, ok := w.Jobs[id]; ok {
		delete(w.Jobs, id)
		if ch != nil {
			close(ch)
		}
	}
	w.mu.Unlock()
}
