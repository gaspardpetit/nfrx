package agent

import (
    "context"
    "time"

    reconnect "github.com/gaspardpetit/nfrx/core/reconnect"
)

// RunWithReconnect runs fn and, when it returns an error and shouldReconnect is true,
// retries with exponential backoff until ctx is done.
// The function may do a full run or a single connection attempt.
func RunWithReconnect(ctx context.Context, shouldReconnect bool, fn func(context.Context) error) error {
    attempt := 0
    for {
        err := fn(ctx)
        if err == nil || !shouldReconnect {
            return err
        }
        delay := reconnect.Delay(attempt)
        attempt++
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(delay):
        }
    }
}

