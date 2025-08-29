package workerproxy

import (
	"context"

	"github.com/gaspardpetit/nfrx/core/logx"
	"github.com/gaspardpetit/nfrx/sdk/base/agent"
)

// StartMetricsServer reuses the base agent metrics server; this agent does not
// expose custom collectors beyond defaults.
func StartMetricsServer(ctx context.Context, addr string) (string, error) {
	addrOut, err := agent.StartMetricsServer(ctx, addr, nil)
	if err == nil {
		logx.Log.Info().Str("addr", addrOut).Msg("metrics server started")
	}
	return addrOut, err
}
