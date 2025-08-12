package ctrl

import (
	"sync"
	"time"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatExpiry   = 3 * HeartbeatInterval
)

type Worker struct {
	ID             string
	Models         map[string]bool
	MaxConcurrency int
	InFlight       int
	LastHeartbeat  time.Time
	Send           chan interface{}
	Jobs           map[string]chan interface{}
	mu             sync.Mutex
}

type Registry struct {
	mu      sync.RWMutex
	workers map[string]*Worker
}

func NewRegistry() *Registry {
	return &Registry{workers: make(map[string]*Worker)}
}

func (r *Registry) Add(w *Worker) {
	r.mu.Lock()
	r.workers[w.ID] = w
	r.mu.Unlock()
}

func (r *Registry) Remove(id string) {
	r.mu.Lock()
	delete(r.workers, id)
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
