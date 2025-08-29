package mcp

import (
    "context"

    "github.com/gaspardpetit/nfrx/sdk/base/agent"
)

// StartMetricsServer starts an HTTP server exposing Prometheus metrics on /metrics.
// It returns the address it is listening on.
func StartMetricsServer(ctx context.Context, addr string) (string, error) {
    return agent.StartMetricsServer(ctx, addr, nil)
}
