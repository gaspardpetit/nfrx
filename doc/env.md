# Configuration reference

This document lists configuration options for the infero tools. Settings can be supplied via environment variables, command line flags, or configuration files. Sample config templates with defaults live under `examples/config/`. `DEBUG` affects logging across all binaries.

## Common

| Variable | Config key | Purpose | Default | CLI flag |
|----------|------------|---------|---------|----------|
| `DEBUG` | — | enable verbose logging | info level when unset | — |

## infero

The server optionally reads settings from a YAML config file. Defaults:

- macOS: `~/Library/Application Support/infero/server.yaml`
- Windows: `%ProgramData%\\infero\\server.yaml`
- Linux: `/etc/infero/server.yaml`

| Variable | Config key | Purpose | Default | CLI flag |
|----------|------------|---------|---------|----------|
| `CONFIG_FILE` | — | server config file path | OS-specific | `--config` |
| `PORT` | — | HTTP listen port for the public API | `8080` | `--port` |
| `METRICS_PORT` | `metrics_addr` | Prometheus metrics listen address or port | same as `PORT` | `--metrics-port` |
| `API_KEY` | — | client API key required for HTTP requests | unset (auth disabled) | `--api-key` |
| `CLIENT_KEY` | — | shared key clients must present when registering | unset | `--client-key` |
| `REQUEST_TIMEOUT` | — | seconds without worker or MCP activity before timing out a request | `120` | `--request-timeout` |
| `DRAIN_TIMEOUT` | — | time to wait for in-flight requests on shutdown | `5m` | `--drain-timeout` |
| `ALLOWED_ORIGINS` | — | comma separated list of allowed CORS origins | unset (deny all) | `--allowed-origins` |
| `BROKER_MAX_REQ_BYTES` | — | maximum MCP request size in bytes | `10485760` | — |
| `BROKER_MAX_RESP_BYTES` | — | maximum MCP response size in bytes | `10485760` | — |
| `BROKER_WS_HEARTBEAT_MS` | — | MCP WebSocket heartbeat interval in milliseconds | `15000` | — |
| `BROKER_WS_DEAD_AFTER_MS` | — | MCP WebSocket idle timeout in milliseconds | `45000` | — |
| `BROKER_MAX_CONCURRENCY_PER_CLIENT` | — | maximum concurrent MCP sessions per client | `16` | — |

## infero-llm

The worker optionally reads settings from a YAML config file. Defaults:

- macOS: `~/Library/Application Support/infero/worker.yaml`
- Windows: `%ProgramData%\\infero\\worker.yaml`
- Linux: `/etc/infero/worker.yaml`

| Variable | Config key | Purpose | Default | CLI flag |
|----------|------------|---------|---------|----------|
| `CONFIG_FILE` | — | worker config file path | OS-specific | `--config` |
| `LOG_DIR` | — | directory for worker log files | OS-specific (none on Linux) | `--log-dir` |
| `SERVER_URL` | `server_url` | server WebSocket URL for registration | `ws://localhost:8080/api/workers/connect` | `--server-url` |
| `CLIENT_KEY` | `client_key` | shared secret for authenticating with the server | unset | `--client-key` |
| `COMPLETION_BASE_URL` | `completion_base_url` | base URL of the completion API | `http://127.0.0.1:11434/v1` | `--completion-base-url` |
| `COMPLETION_API_KEY` | — | API key for the completion API | unset | `--completion-api-key` |
| `MAX_CONCURRENCY` | `max_concurrency` | maximum number of jobs processed concurrently | `2` | `--max-concurrency` |
| `CLIENT_ID` | — | client identifier (random if unset) | unset | `--client-id` |
| `STATUS_ADDR` | `status_addr` | local status HTTP listen address | unset (disabled) | `--status-addr` |
| `METRICS_PORT` | `metrics_addr` | Prometheus metrics listen address or port | unset (disabled) | `--metrics-port` |
| `DRAIN_TIMEOUT` | — | time to wait for in-flight jobs on shutdown | `1m` | `--drain-timeout` |
| `MODEL_POLL_INTERVAL` | — | interval for polling Ollama for model changes | `1m` | `--model-poll-interval` |
| `CLIENT_NAME` | — | worker display name | hostname (or random) | `--client-name` |
| `RECONNECT` | — | reconnect to server on failure | `false` | `--reconnect`, `-r` |
| `REQUEST_TIMEOUT` | — | seconds without backend feedback before failing a job | `300` | `--request-timeout` |


## infero-mcp

`infero-mcp` reads additional settings from a YAML file when `CONFIG_FILE` is set. Defaults:

- macOS: `~/Library/Application Support/infero/mcp.yaml`
- Windows: `%ProgramData%\\infero\\mcp.yaml`
- Linux: `/etc/infero/mcp.yaml`

| Variable | Config key | Purpose | Default | CLI flag |
|----------|------------|---------|---------|----------|
| `RECONNECT` | — | reconnect to server on failure | `false` | `--reconnect`, `-r` |
| `SERVER_URL` | — | broker WebSocket URL | `ws://localhost:8080/api/mcp/connect` | — |
| `CLIENT_ID` | — | client identifier (assigned when empty) | unset | `--client-id` |
| `CLIENT_NAME` | — | client display name | hostname (or random) | `--client-name` |
| `PROVIDER_URL` | — | MCP provider URL | `http://127.0.0.1:7777/` | — |
| `AUTH_TOKEN` | — | authorization token for broker requests | unset | — |
| `CLIENT_KEY` | — | shared secret for authenticating with the server | unset | `--client-key` |
| `CONFIG_FILE` | — | path to YAML config file | OS-specific | `--config` |
| `METRICS_PORT` | `metrics_addr` | Prometheus metrics listen address or port | unset (disabled) | `--metrics-port` |
| `REQUEST_TIMEOUT` | — | seconds to wait for MCP provider responses | `300` | `--request-timeout` |
| `MCP_TRANSPORT_ORDER` | `order` | comma separated transport order | `stdio,http,oauth` | `--mcp-transport-order` |
| `MCP_INIT_TIMEOUT` | `initTimeout` | timeout for transport startup | `5s` | `--mcp-init-timeout` |
| `MCP_PROTOCOL_VERSION` | `protocolVersion` | preferred MCP protocol version | negotiated automatically | `--mcp-protocol-version` |
| `MCP_MAX_INFLIGHT` | `maxInFlight` | maximum concurrent MCP RPCs | `0` (unlimited) | `--mcp-max-inflight` |
| `MCP_STDIO_COMMAND` | `stdio.command` | command for stdio transport | unset | `--mcp-stdio-command` |
| `MCP_STDIO_ARGS` | `stdio.args` | stdio command arguments | unset | `--mcp-stdio-args` |
| `MCP_STDIO_ENV` | `stdio.env` | stdio environment variables | unset | `--mcp-stdio-env` |
| `MCP_STDIO_WORKDIR` | `stdio.workDir` | stdio working directory | unset | `--mcp-stdio-workdir` |
| `MCP_STDIO_ALLOW_RELATIVE` | `stdio.allowRelative` | allow relative stdio command path | `false` | `--mcp-stdio-allow-relative` |
| `MCP_HTTP_URL` | `http.url` | HTTP MCP server base URL | unset | `--mcp-http-url` |
| `MCP_HTTP_TIMEOUT` | `http.timeout` | HTTP client timeout | `30s` | `--mcp-http-timeout` |
| `MCP_HTTP_ENABLE_PUSH` | `http.enablePush` | enable server-push SSE channel | `false` | `--mcp-http-enable-push` |
| `MCP_HTTP_INSECURE_SKIP_VERIFY` | `http.insecureSkipVerify` | skip TLS certificate verification | `false` | `--mcp-http-insecure-skip-verify` |
| `MCP_OAUTH_ENABLED` | `oauth.enabled` | enable OAuth for HTTP transport | `false` | `--mcp-oauth-enabled` |
| `MCP_OAUTH_TOKEN_URL` | `oauth.tokenURL` | OAuth token endpoint | unset | `--mcp-oauth-token-url` |
| `MCP_OAUTH_CLIENT_ID` | `oauth.clientID` | OAuth client ID | unset | `--mcp-oauth-client-id` |
| `MCP_OAUTH_CLIENT_SECRET` | `oauth.clientSecret` | OAuth client secret | unset | `--mcp-oauth-client-secret` |
| `MCP_OAUTH_SCOPES` | `oauth.scopes` | OAuth scopes | unset | `--mcp-oauth-scopes` |
| `MCP_OAUTH_TOKEN_FILE` | `oauth.tokenFile` | path to OAuth token cache file | unset | `--mcp-oauth-token-file` |
| `MCP_ENABLE_LEGACY_SSE` | `enableLegacySSE` | enable legacy SSE transport | `false` | `--mcp-enable-legacy-sse` |

### Consistency notes

`SERVER_URL`, `CLIENT_KEY`, and `RECONNECT` remain shared between tools, providing predictable behavior.

