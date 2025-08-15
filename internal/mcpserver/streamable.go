package mcpserver

import (
	"context"
	"net/http"

	sdkserver "github.com/mark3labs/mcp-go/server"
)

// NewHandler constructs a Streamable HTTP MCP handler.
// It exposes a transport-compliant endpoint without registering any tools or resources.
func NewHandler() http.Handler {
	srv := sdkserver.NewMCPServer(
		"llamapool",
		"dev",
		sdkserver.WithResourceCapabilities(false, false),
		sdkserver.WithToolCapabilities(false),
		sdkserver.WithPromptCapabilities(false),
	)
	handler := sdkserver.NewStreamableHTTPServer(
		srv,
		sdkserver.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return ctx
		}),
	)
	return handler
}
