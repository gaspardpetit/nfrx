package inflight

import (
	"context"
	"net/http"
	"sync"
)

// Counter tracks in-flight requests that should block draining.
type Counter struct {
	mu     sync.Mutex
	count  int64
	zeroCh chan struct{}
}

// Inc increments the in-flight counter.
func (c *Counter) Inc() {
	c.mu.Lock()
	if c.zeroCh == nil {
		c.zeroCh = make(chan struct{})
		if c.count == 0 {
			close(c.zeroCh)
		}
	}
	if c.count == 0 {
		c.zeroCh = make(chan struct{})
	}
	c.count++
	c.mu.Unlock()
}

// Dec decrements the in-flight counter.
func (c *Counter) Dec() {
	c.mu.Lock()
	if c.zeroCh == nil {
		c.zeroCh = make(chan struct{})
		if c.count == 0 {
			close(c.zeroCh)
		}
	}
	if c.count > 0 {
		c.count--
		if c.count == 0 {
			close(c.zeroCh)
		}
	}
	c.mu.Unlock()
}

// Load returns the current in-flight count.
func (c *Counter) Load() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

// WaitForZero blocks until the count is zero or the context is done.
func (c *Counter) WaitForZero(ctx context.Context) bool {
	ch := c.zeroChannel()
	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Counter) zeroChannel() chan struct{} {
	c.mu.Lock()
	if c.zeroCh == nil {
		c.zeroCh = make(chan struct{})
		if c.count == 0 {
			close(c.zeroCh)
		}
	}
	ch := c.zeroCh
	c.mu.Unlock()
	return ch
}

// Middleware increments the counter for the duration of a request.
func (c *Counter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Inc()
			defer c.Dec()
			next.ServeHTTP(w, r)
		})
	}
}

var drainable Counter

// Drainable returns the shared counter used for drainable requests.
func Drainable() *Counter { return &drainable }

// DrainableMiddleware returns middleware that tracks drainable requests.
func DrainableMiddleware() func(http.Handler) http.Handler { return drainable.Middleware() }

// DrainableCount returns the current drainable request count.
func DrainableCount() int64 { return drainable.Load() }

// DrainableWaitForZero blocks until drainable count reaches zero or ctx ends.
func DrainableWaitForZero(ctx context.Context) bool { return drainable.WaitForZero(ctx) }
