package worker

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/gaspardpetit/nfrx/core/logx"
	ctrl "github.com/gaspardpetit/nfrx/sdk/api/control"
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
	Conn               *websocket.Conn
	Send               chan interface{}
	Jobs               map[string]chan interface{}
	mu                 sync.Mutex
	closed             bool
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

func (r *Registry) Add(w *Worker) (replaced *Worker) {
	r.mu.Lock()
	replaced = r.workers[w.ID]
	r.workers[w.ID] = w
	r.mu.Unlock()
	return replaced
}

func (r *Registry) Remove(id string) {
	r.mu.Lock()
	if w, ok := r.workers[id]; ok {
		delete(r.workers, id)
		r.mu.Unlock()
		closeWorker(w)
		return
	}
	r.mu.Unlock()
}

func (r *Registry) RemoveIfSame(id string, want *Worker) bool {
	r.mu.Lock()
	w, ok := r.workers[id]
	if !ok || w != want {
		r.mu.Unlock()
		return false
	}
	delete(r.workers, id)
	r.mu.Unlock()
	closeWorker(want)
	return true
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

func (r *Registry) PruneExpired(maxAge time.Duration) int {
	r.mu.Lock()
	var expired []*Worker
	for id, w := range r.workers {
		if time.Since(w.LastHeartbeat) > maxAge {
			delete(r.workers, id)
			expired = append(expired, w)
			logx.Log.Info().Str("worker_id", id).Str("reason", "heartbeat_expired").Msg("evicted")
		}
	}
	r.mu.Unlock()
	for _, w := range expired {
		closeWorker(w)
	}
	return len(expired)
}

func closeWorker(w *Worker) {
	closeWorkerWithReason(w, false)
}

func replaceWorker(w *Worker) {
	closeWorkerWithReason(w, true)
}

func closeWorkerWithReason(w *Worker, replaced bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	conn := w.Conn
	w.Conn = nil
	send := w.Send
	w.Send = nil
	jobs := w.Jobs
	w.Jobs = make(map[string]chan interface{})
	w.mu.Unlock()
	if conn != nil {
		if replaced {
			replacedMsg := ctrl.SessionReplacedMessage{Type: "session_replaced", WorkerID: w.ID, WorkerName: w.Name, Reason: "duplicate_worker_id"}
			if b, err := json.Marshal(replacedMsg); err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = conn.Write(ctx, websocket.MessageText, b)
				cancel()
			}
			_ = conn.Close(websocket.StatusPolicyViolation, "replaced by another session with same worker id")
		} else {
			_ = conn.Close(websocket.StatusNormalClosure, "closing")
		}
	}
	for id, ch := range jobs {
		if ch != nil {
			close(ch)
		}
		delete(jobs, id)
	}
	if send != nil {
		close(send)
	}
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
