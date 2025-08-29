package worker

import (
	"sync"
	"time"

	"github.com/gaspardpetit/nfrx/core/logx"
)

const (
	HeartbeatInterval = 5 * time.Second
	HeartbeatExpiry   = 3 * HeartbeatInterval
)

type Worker struct {
	ID                 string
	Name               string
	Labels             map[string]bool
	MaxConcurrency     int
	PreferredBatchSize int
	InFlight           int
	LastHeartbeat      time.Time
	Send               chan interface{}
	Jobs               map[string]chan interface{}
	mu                 sync.Mutex
}

// NameValue safely returns the worker's name.
func (w *Worker) NameValue() string { w.mu.Lock(); defer w.mu.Unlock(); return w.Name }

// LabelKeys returns a copy of the worker's label set as a slice.
func (w *Worker) LabelKeys() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	keys := make([]string, 0, len(w.Labels))
	for k := range w.Labels {
		keys = append(keys, k)
	}
	return keys
}

// HasLabel reports whether the worker supports the given label.
func (w *Worker) HasLabel(label string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Labels[label]
}

type Registry struct {
	mu      sync.RWMutex
	workers map[string]*Worker
}

func NewRegistry() *Registry { return &Registry{workers: make(map[string]*Worker)} }

func (r *Registry) Add(w *Worker) {
	r.mu.Lock()
	r.workers[w.ID] = w
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

func (r *Registry) WorkerCount() int { r.mu.RLock(); defer r.mu.RUnlock(); return len(r.workers) }

func (r *Registry) UpdateHeartbeat(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.LastHeartbeat = time.Now()
	}
	r.mu.Unlock()
}

// UpdateLabels replaces the label set for a worker.
func (r *Registry) UpdateLabels(id string, labels []string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		w.mu.Lock()
		w.Labels = make(map[string]bool)
		for _, m := range labels {
			w.Labels[m] = true
		}
		w.mu.Unlock()
	}
	r.mu.Unlock()
}

func (r *Registry) WorkersForLabel(model string) []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var res []*Worker
	for _, w := range r.workers {
		w.mu.Lock()
		if w.Labels[model] && w.InFlight < w.MaxConcurrency {
			res = append(res, w)
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

// Snapshot returns a point-in-time slice of worker pointers.
// Callers must not mutate the returned workers without holding their locks.
func (r *Registry) Snapshot() []*Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*Worker, 0, len(r.workers))
	for _, w := range r.workers {
		res = append(res, w)
	}
	return res
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

func (w *Worker) AddJob(id string, ch chan interface{}) { w.mu.Lock(); w.Jobs[id] = ch; w.mu.Unlock() }
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
