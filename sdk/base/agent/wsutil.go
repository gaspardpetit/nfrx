package agent

import "context"

// Send attempts to send a payload on a channel respecting context cancelation.
// If the context is done before the send can proceed, it returns without sending.
func Send[T any](ctx context.Context, ch chan<- T, payload T) {
    // Avoid racing with channel closure when ctx is already done.
    select {
    case <-ctx.Done():
        return
    default:
    }
    select {
    case ch <- payload:
    case <-ctx.Done():
    }
}

