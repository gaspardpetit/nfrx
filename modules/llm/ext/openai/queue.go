package openai

import (
	"sync"

	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// CompletionQueue is a simple process-wide FIFO queue for generative LLM requests.
// It tracks length and capacity in the LLM metrics registry for state reporting.
type CompletionQueue struct {
	mu    sync.Mutex
	items []queuedRequest
	cap   int
	mx    *baseworker.MetricsRegistry
}

type queuedRequest struct {
	id    string
	model string
}

func NewCompletionQueue(mx *baseworker.MetricsRegistry, capacity int) *CompletionQueue {
	q := &CompletionQueue{mx: mx}
	q.SetCapacity(capacity)
	return q
}

func (q *CompletionQueue) SetCapacity(n int) {
	q.mu.Lock()
	q.cap = n
	if q.mx != nil {
		q.mx.SetSchedulerQueueCapacity(n)
		q.mx.SetSchedulerQueueLen(len(q.items))
	}
	q.mu.Unlock()
}

// Enter enqueues id if capacity allows; returns 1-based position and ok.
func (q *CompletionQueue) Enter(id, model string) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cap <= 0 {
		return 0, false
	}
	if len(q.items) >= q.cap {
		return 0, false
	}
	q.items = append(q.items, queuedRequest{id: id, model: model})
	if q.mx != nil {
		q.mx.SetSchedulerQueueLen(len(q.items))
	}
	return len(q.items), true
}

// Leave removes id from queue if present.
func (q *CompletionQueue) Leave(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, v := range q.items {
		if v.id == id {
			q.items = append(q.items[:i], q.items[i+1:]...)
			if q.mx != nil {
				q.mx.SetSchedulerQueueLen(len(q.items))
			}
			return
		}
	}
}

// Position returns the current 1-based position of id, or 0 if not present.
func (q *CompletionQueue) Position(id string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, v := range q.items {
		if v.id == id {
			return i + 1
		}
	}
	return 0
}

// IsFirstDispatchable reports whether id is the first queue entry that satisfies canDispatch.
// Entries earlier in the queue that are not currently dispatchable do not block later compatible entries.
func (q *CompletionQueue) IsFirstDispatchable(id string, canDispatch func(model string) bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range q.items {
		if !canDispatch(item.model) {
			continue
		}
		return item.id == id
	}
	return false
}

// Len returns the current queue length.
func (q *CompletionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
