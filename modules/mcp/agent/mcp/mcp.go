package mcp

import (
	"context"
	"time"

	"github.com/coder/websocket"
	internal "github.com/gaspardpetit/nfrx/modules/mcp/agent/internal/mcp"
)

// Config re-exports the agent's configuration.
// (no config type yet; placeholder for future expansion)

// NewRelayClient exposes the internal MCP relay client.
func NewRelayClient(conn *websocket.Conn, providerURL, token string, timeout time.Duration) *internal.RelayClient {
	return internal.NewRelayClient(conn, providerURL, token, timeout)
}

// StartMetricsServer exposes the internal metrics server helper.
func StartMetricsServer(ctx context.Context, addr string) (string, error) {
	return internal.StartMetricsServer(ctx, addr)
}
