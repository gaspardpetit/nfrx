# MCP Client Guide

This guide covers operation and development of the **infx-mcp** connector which bridges `infx` to third-party Model Context Protocol (MCP) providers.

## Operator Guide

The client tries transports in order until one initializes successfully. Configure the order and options via environment variables or YAML config:

| Transport | Key settings |
|-----------|--------------|
| stdio | `MCP_STDIO_COMMAND`, `MCP_STDIO_ARGS`, `MCP_STDIO_WORKDIR` |
| HTTP | `MCP_HTTP_URL`, `MCP_HTTP_TIMEOUT` |
| OAuth HTTP | enable with `MCP_OAUTH_ENABLED`, set `MCP_OAUTH_CLIENT_ID`, `MCP_OAUTH_TOKEN_URL`, `MCP_OAUTH_SCOPES`, and optionally `MCP_OAUTH_TOKEN_FILE` |
| Legacy SSE | enable with `MCP_ENABLE_LEGACY_SSE=true` |

Common errors and remedies:

- **405 on POST** – the provider does not support streamable HTTP.
- **MCP_PROVIDER_UNAVAILABLE** – the relay cannot reach the provider or the process exited.
- **TLS verification error** – set `MCP_HTTP_INSECURE_SKIP_VERIFY=true` only for testing.

## Developer Notes

`internal/mcpclient` defines a transport-agnostic `Connector` interface:

```go
Start(ctx context.Context) error
Initialize(ctx context.Context, req mcp.InitializeRequest) (*mcp.InitializeResult, error)
DoRPC(ctx context.Context, method string, params any, result any) error
Close() error
```

The `Orchestrator` tries each transport factory in `Config.Order` with exponential backoff. To add a new transport, implement a constructor returning `*transportConnector` and register it in `NewOrchestrator`.

Run the compatibility and chaos test matrix locally with:

```bash
go test ./internal/mcpclient -run Compatibility
```

## Compatibility

By default the client attempts stdio and streamable HTTP transports. OAuth is used when enabled. Legacy SSE support is behind the `MCP_ENABLE_LEGACY_SSE` flag and is disabled by default.

