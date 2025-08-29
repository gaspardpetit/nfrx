package agent

import (
    "context"
    "time"
)

// StartHeartbeat runs a ticker and invokes sendFn at each tick until ctx ends.
func StartHeartbeat(ctx context.Context, interval time.Duration, sendFn func(context.Context) error) {
    if interval <= 0 || sendFn == nil { return }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            _ = sendFn(ctx)
        case <-ctx.Done():
            return
        }
    }
}

