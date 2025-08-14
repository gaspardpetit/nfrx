package ctrl

import (
	"sync"
	"time"

	"github.com/you/llamapool/internal/logx"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatExpiry   = 3 * HeartbeatInterval
)

type Worker struct {
	ID             string
	Name           string
	Models         map[string]bool
	MaxConcurrency int
	InFlight       int
	LastHeartbeat  time.Time
	Send           chan interface{}
	Jobs           map[string]chan interface{}
	mu             sync.Mutex
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
		for _, ch := range w.Jobs {
			if ch != nil {
				close(ch)
			}
		}
		if w.Send != nil {
			close(w.Send)
		}
	}
	r.mu.Unlock()
}

func (r *Registry) UpdateHeartbeat(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.LastHeartbeat = time.Now()
	}
	r.mu.Unlock()
}

func (r *Registry) WorkersForModel(model string) []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*Worker
	for _, w := range r.workers {
		if w.Models[model] {
			res = append(res, w)
		}
	}
	return res
}

// WorkersForAlias returns any worker that exposes a model whose alias matches the alias of requested.
func (r *Registry) WorkersForAlias(requested string) []*Worker {
	key, ok := AliasKey(requested)
	if !ok {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []*Worker
	for _, w := range r.workers {
		for m := range w.Models {
			if ak, ok := AliasKey(m); ok && ak == key {
				res = append(res, w)
				break
			}
		}
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
		for m := range w.Models {
			set[m] = struct{}{}
		}
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
			for _, ch := range w.Jobs {
				close(ch)
			}
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
	delete(w.Jobs, id)
	w.mu.Unlock()
}
