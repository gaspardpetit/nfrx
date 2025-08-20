package agent

import (
	"context"
	"time"

	"github.com/gaspardpetit/llamapool/internal/logx"
	reconnect "github.com/gaspardpetit/llamapool/internal/reconnect"
)

// RunWithReconnect repeatedly invokes connect until it succeeds or the context ends.
// The connect function should return whether a connection was established before an error occurred.
func RunWithReconnect(ctx context.Context, shouldReconnect bool, connect func(context.Context) (bool, error)) error {
	attempt := 0
	for {
		connected, err := connect(ctx)
		if err == nil || !shouldReconnect {
			return err
		}
		if connected {
			attempt = 0
		}
		delay := reconnect.Delay(attempt)
		attempt++
		logx.Log.Warn().Dur("backoff", delay).Err(err).Msg("connection lost; retrying")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}
