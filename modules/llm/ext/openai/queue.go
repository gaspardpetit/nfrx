package openai

import (
	"sync"

	baseworker "github.com/gaspardpetit/nfrx/sdk/base/worker"
)

// CompletionQueue is a simple process-wide FIFO queue for chat completion requests.
// It tracks length and capacity in the LLM metrics registry for state reporting.
type CompletionQueue struct {
	mu    sync.Mutex
	items []string
	cap   int
	mx    *baseworker.MetricsRegistry
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
func (q *CompletionQueue) Enter(id string) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.cap <= 0 {
		return 0, false
	}
	if len(q.items) >= q.cap {
		return 0, false
	}
	q.items = append(q.items, id)
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
		if v == id {
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
		if v == id {
			return i + 1
		}
	}
	return 0
}

// IsHead reports whether id is currently at the head (position 1).
func (q *CompletionQueue) IsHead(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items) > 0 && q.items[0] == id
}

// Len returns the current queue length.
func (q *CompletionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
