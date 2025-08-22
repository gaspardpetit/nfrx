# llamapool-mcp

## Overview
`llamapool-mcp` relays Model Context Protocol (MCP) calls through a
`llamapool-server`, allowing clients to reach private MCP providers via
`POST /api/mcp/id/{id}`.

## Setup
Configure the relay using environment variables or a YAML file. Common
settings:

- `SERVER_URL` – WebSocket URL of the server
- `AUTH_TOKEN` – bearer token expected by the server when set
- `MCP_STDIO_COMMAND` or `MCP_HTTP_URL` – how to reach the MCP provider

To keep retrying when the server or provider is unavailable, start with `--reconnect`.

Example using a config file:

```bash
CONFIG_FILE=examples/config/mcp.yaml ./llamapool-mcp --reconnect
```

## Transport options
The relay tries transports in order until one initializes successfully. Control
the order and options via config or environment variables:

| Transport | Key settings |
|-----------|--------------|
| stdio | `MCP_STDIO_COMMAND`, `MCP_STDIO_ARGS`, `MCP_STDIO_WORKDIR` |
| HTTP | `MCP_HTTP_URL`, `MCP_HTTP_TIMEOUT` |
| OAuth HTTP | enable with `MCP_OAUTH_ENABLED`, set `MCP_OAUTH_CLIENT_ID`, `MCP_OAUTH_TOKEN_URL`, `MCP_OAUTH_SCOPES`, and optionally `MCP_OAUTH_TOKEN_FILE` |
| Legacy SSE | enable with `MCP_ENABLE_LEGACY_SSE=true` |

For all available options see [env.md](env.md).

## Troubleshooting
Common errors and fixes:

- **405 on POST** – the provider does not support streamable HTTP
- **MCP_PROVIDER_UNAVAILABLE** – the relay cannot reach the provider or the process exited
- **TLS verification error** – set `MCP_HTTP_INSECURE_SKIP_VERIFY=true` only for testing

## Developer notes
`internal/mcpclient` defines a transport-agnostic `Connector` interface. The
`Orchestrator` tries each transport factory with exponential backoff. To add a
new transport, implement a constructor returning `*transportConnector` and
register it in `NewOrchestrator`.

Run the compatibility and chaos test matrix locally with:

```bash
go test ./internal/mcpclient -run Compatibility
```

## Further reading
- `examples/mcp-proxy/example.md` – end-to-end example
- [Getting Started](getting-started.md)
- [Architecture](architecture.md)
