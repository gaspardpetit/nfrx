# llamapool-server

## Overview
`llamapool-server` exposes an OpenAI-compatible HTTP API and coordinates
workers and MCP relays over WebSocket.

## Features
- Dynamic worker discovery and least-busy routing
- Separate authentication keys for API clients and workers
- Real-time state API, HTML dashboard, and Prometheus metrics
- Optional relay for Model Context Protocol (MCP) calls

## Configuration
The server reads configuration from environment variables or an optional YAML
file (`CONFIG_FILE`). Common settings:

- `API_KEY` – bearer token for API clients
- `CLIENT_KEY` – shared secret for workers and MCP relays
- `LISTEN_ADDR` – address for the public HTTP API

See [env.md](env.md) for a complete list and defaults. Sample config files live
under `examples/config/`.

Start the server:

```bash
API_KEY=test123 CLIENT_KEY=worker456 ./llamapool-server --listen :8080
```

## HTTP API
OpenAI-compatible endpoints are available under `/api/v1/`. For a detailed list
of all HTTP and WebSocket endpoints, see [server-endpoints.md](server-endpoints.md).

Example requests:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/api/v1/models
curl http://localhost:8080/healthz
```

Administrative endpoints:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/api/state
curl http://localhost:8080/metrics
```

## Further reading
- [Architecture](architecture.md)
- [Getting Started](getting-started.md)
- [Worker guide](worker.md)
- [MCP relay](mcp.md)
