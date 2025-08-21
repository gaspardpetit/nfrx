[![Build](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml)
[![Docker](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml)
[![.deb](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml)
[![macOS Build](https://github.com/gaspardpetit/llamapool/actions/workflows/macos.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/macos.yml)
[![Windows Build](https://github.com/gaspardpetit/llamapool/actions/workflows/windows.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/windows.yml)


# llamapool

<div align="center">
  <img alt="llamapool" width="240" src="https://github.com/gaspardpetit/llamapool/blob/888f4e74e32c752adb75662813438d2da16513a4/doc/img/llamapool-logo-3.png">
</div>

## Overview

**llamapool** is a lightweight, distributed worker pool that exposes an OpenAI-compatible `chat/completions` API, forwarding requests to one or more connected **LLM workers**.
It sits in front of existing LLM runtimes such as [Ollama](https://github.com/ollama/ollama), [vLLM](https://github.com/vllm-project/vllm), or [Open-WebUI](https://github.com/open-webui/open-webui), allowing you to scale, load-balance, and securely access them from anywhere.

In addition to LLM workers, llamapool now supports relaying [Model Context Protocol](https://github.com/modelcontextprotocol) calls. 

The server exposes a Streamable HTTP MCP endpoint at `POST /api/mcp/id/{id}` and forwards requests verbatim over WebSocket to a connected `llamapool-mcp` process. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. When the relay is started with `AUTH_TOKEN`, clients must supply `Authorization: Bearer <token>` when calling this endpoint. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments.

Server-initiated JSON-RPC requests (for example sampling calls) are forwarded across the WebSocket bridge and relayed back to clients, preserving full protocol semantics.

The new `llamapool-mcp` binary connects a private MCP provider to the public `llamapool-server`, allowing clients to invoke MCP methods via `POST /api/mcp/id/{id}`. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments. By default `llamapool-mcp` requires absolute stdio commands and verifies TLS certificates; set `MCP_STDIO_ALLOW_RELATIVE=true` or `MCP_HTTP_INSECURE_SKIP_VERIFY=true` to relax these checks, and `MCP_OAUTH_TOKEN_FILE` to securely cache OAuth tokens on disk.
By default the MCP relay exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s). When enabled, it also probes the MCP provider and remains in a `not_ready` state until the provider becomes reachable.

`llamapool-mcp` reads configuration from a YAML file when `CONFIG_FILE` is set. Values in the file—such as transport order, protocol version preference, or stdio working directory—are used as defaults and can be overridden by environment variables or CLI flags (e.g. `--mcp-http-url`, `--mcp-stdio-workdir`).

For transport configuration, common errors, and developer guidance see [doc/mcpclient.md](doc/mcpclient.md).
For a comprehensive list of configuration options, see [doc/env.md](doc/env.md).

A typical deployment looks like this:

- **`llamapool-server`** is deployed to a public or semi-public location (e.g., Azure, GCP, AWS, or a self-hosted server with dynamic DNS).
- **`llamapool-worker`** runs on private machines (e.g., a Mac Studio or personal GPU workstation) alongside an LLM service.
  When a worker connects, its available models are registered with the server and become accessible via the public API.

## macOS Menu Bar App

An early-stage macOS menu bar companion lives under `desktop/macos/llamapool/`. It polls `http://127.0.0.1:4555/status` every two seconds to display live worker status and can manage a per-user LaunchAgent to start or stop a local `llamapool-worker` and toggle launching at login. A simple preferences window lets you edit worker connection settings which are written to `~/Library/Application Support/Llamapool/worker.yaml`, and the menu offers quick links to open the config and logs folders, view live logs, copy diagnostics to the Desktop, and check for updates via Sparkle.

### Packaging

The macOS app can be distributed as a signed and notarized DMG. After building the `Llamapool` scheme in Release, create the disk image and submit it for notarization:

```bash
# Create a DMG with an /Applications symlink
ci/create-dmg.sh path/to/Llamapool.app build/Llamapool.dmg

# Notarize (requires AC_API_KEY_ID, AC_API_ISSUER_ID and AC_API_P8)
ci/notarize.sh build/Llamapool.dmg
xcrun stapler staple build/Llamapool.dmg
```

`AC_API_P8` must contain a base64-encoded App Store Connect API key. Once notarization completes, the DMG can be distributed and will pass Gatekeeper on clean systems.

When using the GitHub Actions workflow, provide the `AC_TEAM_ID` secret with your Apple Developer Team ID so the archive can be signed and exported.

## Windows Tray App

A Windows tray companion lives under `desktop/windows/`. It polls `http://127.0.0.1:4555/status` every two seconds to display worker status.
The tray can start or stop the local `llamapool` Windows service, toggle whether it launches automatically with Windows, edit worker connection settings, open the config and logs folders, view live logs, and collect diagnostics to the Desktop. When the worker exposes lifecycle control endpoints, the tray also provides **Drain**, **Undrain**, and **Shutdown after drain** actions.

The Windows service runs `llamapool-worker` with the `--reconnect` flag and shuts down if the worker process exits, preventing orphaned workers. The worker is attached to a job object so it also terminates if the service process is killed.

### Key features
- **Dynamic worker discovery** – Workers can connect and disconnect at any time; the server updates the available model list in real-time.
- **Least-busy routing** – If multiple workers support the same model, the server dispatches requests to the one with the lowest current load.
- **Alias-based model fallback** – Requests for a missing quantization fall back to workers serving the same base model.
- **Security by design** –
  - Separate authentication keys for API clients (`API_KEY`) and workers or MCP relays (`CLIENT_KEY`).
  - Workers typically run behind firewalls and connect outbound over HTTPS/WSS.
  - All traffic is encrypted end-to-end.
- **Protocol compatibility** – OpenAI-compatible endpoints are available under `/api/v1/*` without altering JSON payloads.

### How it works
- The **server** accepts incoming HTTP requests from clients, authenticates them, and routes them to workers via WebSocket connections.
- **Workers** authenticate using a shared `CLIENT_KEY` and advertise the models they can serve.
- Requests are proxied directly to the worker’s LLM backend, and the responses are returned unmodified to the client.

## Architecture

```
                         ┌───────────────────────────────────────┐
                         │             Clients (API)             │
                         │  curl / SDKs / Apps / OpenAI clients  │
                         └───────────────┬───────────────────────┘
                                         │  REQUEST 
                                         │  (API_KEY)
┌────────────────────────────────────────▼──────────────────────────────┐
│                                 llamapool-server                      │
│                                                                       │
│  ┌──────────────────────────┐                     ┌───────────────┐   │
│  │  OpenAI-compatible API   │                     │  Observability│   │
│  │  /api/v1/chat/completions│                     │  /metrics     │   │
│  │  /api/v1/embeddings      │                     └───────────────┘   │
│  │  /api/v1/models (+/{id}) │                                         │
│  └──────────────┬────────── ┘                                         │
│                 │                                                     │
│          ┌──────▼──────────────────────────────────────────────────┐  │
│          │                 Router + Scheduler (Least Busy)         │  │
│          │  - Model registry (from workers)                        │  │
│          │  - Dispatch by model & load                             │  │
│          └──────┬───────────────────────────────────────────┬──────┘  │
│                 │  WebSocket (WSS)                          |         │
└────────────▲────|───────────────────────────────▲───────────|─────────┘
     CONNECT |    |                       CONNECT |           |  
(CLIENT_KEY) │    | REQUEST          (CLIENT_KEY) │           | REQUEST
     ┌───────┴────▼────────────┐           ┌──────┴───────────▼────────┐
     │      llamapool-worker   │           │      llamapool-worker    │
     │     (private/home lab)  │           │       (cloud/on-prem)    │
     │                         │           │                          │
     └─────────────┬───────────┘           └─────────────┬────────────┘
                   |                                     |
              ┌────▼──────┐                        ┌─────▼──────┐
              │  LLM      │                        │  LLM       │
              │ (Ollama,  │                        │ (Ollama,   │
              │  vLLM, …) │                        │  vLLM,   …)│
              └───────────┘                        └────────────┘

```

## Endpoints

- Health: `GET /healthz`
- OpenAI Models:
  - `GET /api/v1/models`
  - `GET /api/v1/models/{id}`
- OpenAI Chat Completions: `POST /api/v1/chat/completions`
- OpenAI Embeddings: `POST /api/v1/embeddings`
- llamapool API:
  - **State (JSON):** `GET /api/state`
  - **State (SSE):** `GET /api/state/stream`
- Prometheus metrics: `GET /metrics` (serve on separate address via `METRICS_PORT` or `--metrics-port`)
- API docs:
  - Swagger UI: `GET /api/client/`
  - OpenAPI schema: `GET /api/client/openapi.json`
  - Update schema: edit `api/openapi.yaml` then run `make generate`
- Web dashboard: `GET /state` (real-time view of workers with names, status indicators and sortable columns; prompts for an access token on 401)


## Security

- **Client authentication**: `API_KEY` required for `/api` routes (including `/api/v1`) via `Authorization: Bearer <API_KEY>`.
- **Client authentication**: `CLIENT_KEY` required for worker or MCP WebSocket registration.
- **Transport**: run behind TLS (HTTPS/WSS) via reverse proxy or terminate TLS in-process.
- **CORS**: cross-origin requests are denied unless explicitly allowed via `ALLOWED_ORIGINS` (comma separated) or the `--allowed-origins` flag.

- **Service isolation**: Debian packages run the daemons as the dedicated `llamapool` user with systemd-managed directories
  (`/var/lib/llamapool`, `/var/cache/llamapool`, `/run/llamapool`) and hardening flags like `NoNewPrivileges=true` and
  `ProtectSystem=full`.

## Monitoring & Observability

- **Prometheus** (`/metrics`, configurable address via `METRICS_PORT` or `--metrics-port`):
  - `llamapool_build_info{component="server",version,sha,date}`
  - `llamapool_model_requests_total{model,outcome}`
  - `llamapool_model_tokens_total{model,kind}`
  - `llamapool_request_duration_seconds{worker_id,model}` (histogram)
  - (Optionally) per-worker gauges/counters if enabled.
- **Worker metrics** (`METRICS_PORT` or `--metrics-port`):
  - Exposes `llamapool_worker_*` series such as
    `llamapool_worker_connected_to_server`,
    `llamapool_worker_connected_to_ollama`,
    `llamapool_worker_current_jobs`,
    `llamapool_worker_max_concurrency`,
    `llamapool_worker_jobs_started_total`,
    `llamapool_worker_jobs_succeeded_total`,
    `llamapool_worker_jobs_failed_total`, and
    `llamapool_worker_job_duration_seconds` (histogram).
- **MCP relay metrics** (`METRICS_PORT` or `--metrics-port`):
  - Exposes basic Go runtime metrics.

- **JSON/SSE State** (`/api/state`, `/api/state/stream`): suitable for custom dashboards showing:
  - worker list and status (connected/working/idle/gone)
  - per-worker totals (processed, inflight, failures, avg duration)
  - per-model availability (how many workers support each model)
  - MCP relay clients and active sessions
  - versions/build info for server & workers
- **Web state page** (`/state`): lightweight dashboard powered by the state stream
- **Logs**:
  - `Info` — lifecycle details like connections, disconnections, draining, and job dispatch/completion.
  - `Warn` — expected failures such as missing models or no available workers.
  - `Error` — unexpected issues requiring investigation. `Fatal` is used only for unrecoverable errors.


## Objectives

- Provide a **minimal, self-contained** worker pool that can be deployed anywhere.
- Make it easy to **expose private LLM hardware** securely to authorized clients.
- Support **multiple LLM runtimes** without protocol translation overhead.
- Enable **scalable and fault-tolerant** request routing across many workers.
- Offer **transparent monitoring and metrics** for operational insight.

## Install via .deb

```bash
wget https://github.com/gaspardpetit/llamapool/releases/download/v1.3.0/llamapool-server_1.3.0-1_amd64.deb
sudo dpkg -i llamapool-server_1.3.0-1_amd64.deb
sudo systemctl status llamapool-server
```

## Build

On Linux:

```bash
make build
```

On Windows:
```
go build -o .\bin\llamapool-server.exe .\cmd\llamapool-server
go build -o .\bin\llamapool-worker.exe .\cmd\llamapool-worker
```

### Version

Both binaries expose a `--version` flag that prints the build metadata:

```bash
llamapool-server --version
llamapool-worker --version
```

The output includes the version, git SHA and build date.
The same version information appears at the top of `--help` output.

## Run

### Server

On Linux:

```bash
PORT=8080 CLIENT_KEY=secret API_KEY=test123 go run ./cmd/llamapool-server
# or to expose metrics on a different port:
# PORT=8080 METRICS_PORT=9090 CLIENT_KEY=secret API_KEY=test123 go run ./cmd/llamapool-server
```

Workers register with the server at `/api/workers/connect`.
`llamapool-mcp` connects to the server at `ws://<server>/api/mcp/connect` and receives a unique id which is used by clients when calling `POST /api/mcp/id/{id}`.

On Windows (CMD)

```
set PORT=8080
set CLIENT_KEY=secret
set API_KEY=test123
go run .\cmd\llamapool-server
REM or if you built:
.\bin\llamapool-server.exe
```

On Windows (Powershell)

```
$env:PORT = "8080"; $env:CLIENT_KEY = "secret"; $env:API_KEY = "test123"
go run .\cmd\llamapool-server
# or if you built:
.\bin\llamapool-server.exe
```


### Worker

On Linux:

```bash
SERVER_URL=ws://localhost:8080/api/workers/connect CLIENT_KEY=secret OLLAMA_BASE_URL=http://127.0.0.1:11434 WORKER_NAME=Alpha go run ./cmd/llamapool-worker
```
Optionally set `OLLAMA_API_KEY` to forward an API key to the local Ollama instance. The worker proxies requests to `${OLLAMA_BASE_URL}/v1/chat/completions`.

On Windows (CMD)

```
set SERVER_URL=ws://localhost:8080/api/workers/connect
set CLIENT_KEY=secret
set OLLAMA_BASE_URL=http://127.0.0.1:11434
go run .\cmd\llamapool-worker
REM or if you built:
.\bin\llamapool-worker.exe
```

On Windows (Powershell)

```
$env:SERVER_URL = "ws://localhost:8080/api/workers/connect"
$env:CLIENT_KEY = "secret"
$env:OLLAMA_BASE_URL = "http://127.0.0.1:11434"
$env:WORKER_NAME = "Alpha"
go run .\cmd\llamapool-worker
# or:
.\bin\llamapool-worker.exe
```

By default the worker exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s).
When enabled, the worker also retries its Ollama backend and remains connected to the server in a `not_ready` state with zero concurrency until the backend becomes available.



## Run with Docker

Pre-built images are available:

- Server: `ghcr.io/gaspardpetit/llamapool-server:main`
- Worker: `ghcr.io/gaspardpetit/llamapool-worker:main`

Tagged releases follow semantic versioning (e.g., `v1.2.3`, `v1.2`, `v1`, `latest`); the `main` tag tracks the latest development snapshot.

### Server

```bash
docker run --rm -p 8080:8080 -e CLIENT_KEY=secret -e API_KEY=test123 \
  ghcr.io/gaspardpetit/llamapool-server:main
```

### Worker

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/api/workers/connect \
  -e CLIENT_KEY=secret \
  -e OLLAMA_BASE_URL=http://host.docker.internal:11434 \
  ghcr.io/gaspardpetit/llamapool-worker:main
```

When started with `--status-addr <addr>`, the worker serves local endpoints:

- `GET /status` returns the current worker state.
- `GET /version` returns build information.
- `POST /control/drain` begins graceful draining.
- `POST /control/undrain` resumes accepting jobs.
- `POST /control/shutdown` drains and exits.

Sending `SIGTERM` causes the worker to stop accepting new jobs. If no work is in
progress, the worker exits immediately; otherwise it waits up to
`--drain-timeout` (default 1m) for in-flight work to finish before exiting.
Send `SIGTERM` again to terminate immediately. Set `--drain-timeout=0` to exit
without waiting or `--drain-timeout=-1` to wait indefinitely.

The worker polls the local Ollama instance (default every 1m) so that
`connected_to_ollama` and `models` stay current in the `/status` output.
If the model list changes, the worker proactively notifies the server so
`/api/v1/models` reflects the latest information. Configure the poll interval
with `MODEL_POLL_INTERVAL` or `--model-poll-interval`.

Control endpoints require an `X-Auth-Token` header. The token is generated on
first run and stored alongside the worker config as `worker.token`.

## Configuration

When running as a systemd service, both components read optional configuration
files managed by systemd. The server loads variables from
`/etc/llamapool/server.env` while the worker reads `/etc/llamapool/worker.env`.
Each file contains `KEY=value` pairs, for example:

```bash
# /etc/llamapool/server.env
# API_KEY=your_api_key
# CLIENT_KEY=secret
```

Commented example files are installed to `/etc/llamapool/`; edit these files to configure the services.

When no explicit paths are provided, the worker falls back to OS defaults for
its configuration and logs:

- **Linux:** `/etc/llamapool/worker.yaml`
- **macOS:** `~/Library/Application Support/llamapool/worker.yaml` and
  `~/Library/Logs/llamapool/`
- **Windows:** `%ProgramData%\llamapool\worker.yaml` and
  `%ProgramData%\llamapool\Logs\`

These locations can be overridden via the `CONFIG_FILE` and `LOG_DIR`
environment variables or the `--config` and `--log-dir` flags.

## Example request

Ensure that the requested `model` is installed on the connected worker's local
Ollama instance. If the model is missing, the server responds with `no worker`.

On Linux:

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer test123' \
  -d '{"model":"llama3","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

On Windows (CMD):

```
curl -N -X POST "http://localhost:8080/api/v1/chat/completions" ^
  -H "Content-Type: application/json" ^
  -H "Authorization: Bearer test123" ^
  -d "{ \"model\": \"llama3\", \"messages\": [ { \"role\": \"user\", \"content\": \"Hello\" } ], \"stream\": true }"
```

On Windows (Powershell):

```
curl -N -X POST http://localhost:8080/api/v1/chat/completions `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer test123" `
  -d '{ "model": "llama3", "messages": [ { "role": "user", "content": "Hello" } ], "stream": true }'
```

The server also exposes a basic health check:

```bash
curl http://localhost:8080/healthz
```

The endpoint reports `503 Service Unavailable` if no MCP session is ready.

For server administration and monitoring:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/api/state
curl -H "Authorization: Bearer test123" http://localhost:8080/api/state/stream
# Prometheus metrics
curl http://localhost:8080/metrics
# or if `METRICS_PORT`/`--metrics-port` is set:
curl http://localhost:9090/metrics
```

The server also exposes OpenAI-style model listing endpoints:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/api/v1/models
curl -H "Authorization: Bearer test123" http://localhost:8080/api/v1/models/llama3:8b
```

For a full list of server endpoints, see [doc/server-endpoints.md](doc/server-endpoints.md).

## Testing

On Linux:

```bash
make lint
make test
```

On Windows:

```
golangci-lint run
go test ./...
```

## Windows integration (experimental)

An initial Windows tray application and service wrapper live under `desktop/windows/`.
A WiX-based MSI installer in `desktop/windows/Installer` installs the worker and tray binaries, registers the `llamapool` service, creates `%ProgramData%\llamapool` with a default configuration, and adds a Start Menu shortcut for the tray.
The service wrapper launches `llamapool-worker.exe` installed at
`%ProgramFiles%\llamapool\llamapool-worker.exe` with its working directory set to
`%ProgramData%\llamapool`. Configuration is read from
`%ProgramData%\llamapool\worker.yaml` and log output is written to
`%ProgramData%\llamapool\Logs\worker.log`. The service is registered as
`llamapool` with delayed automatic start. The tray app now polls the local worker
every two seconds and updates its menu and tooltip to reflect the current status.
A details dialog shows connection information, job counts, and any last error. A preferences window can edit the worker configuration and write it back to the YAML file, and the menu offers quick links to open the config and logs folders, view live logs, and collect diagnostics.
The tray app checks the [llamapool GitHub releases](https://github.com/gaspardpetit/llamapool/releases) once per day and notifies when a new version is available.
For manual end-to-end verification on a clean VM, see [desktop/windows/ACCEPTANCE.md](desktop/windows/ACCEPTANCE.md).

## Currently Supported

| Feature | Supported | Notes |
| --- | --- | --- |
| OpenAI-compatible `POST /api/v1/chat/completions` | ✅ | Proxied to workers without payload mutation |
| OpenAI-compatible `POST /api/v1/embeddings` | ✅ | Proxied to workers without payload mutation |
| Multiple worker registration | ✅ | Workers can join/leave dynamically; models registered on connect |
| Model-based routing (least-busy) | ✅ | `LeastBusyScheduler` selects worker by current load |
| Model alias fallback | ✅ | Falls back to base model when exact quantization not available |
| API key authentication for clients | ✅ | `Authorization: Bearer <API_KEY>` for `/api` (including `/api/v1`) |
| Client key authentication | ✅ | Workers authenticate over WebSocket using `CLIENT_KEY` |
| Dynamic model discovery | ✅ | Workers advertise supported models; server aggregates |
| HTTPS/WSS transport | ✅ | Use TLS terminator or run behind reverse proxy; WS path configurable |
| Prometheus metrics endpoint | ✅ | `/metrics`; includes build info, per-model counters, histograms; supports `METRICS_PORT`/`--metrics-port` |
| Real-time state API (JSON) | ✅ | `GET /api/state` returns full server/worker snapshot |
| Real-time state stream (SSE) | ✅ | `GET /api/state/stream` for dashboards |
| Token usage tracking | ✅ | Per-model and per-worker token totals (in/out) |
| Per-model success/error rates | ✅ | `llamapool_model_requests_total{outcome=...}` |
| Build info (server & worker) | ✅ | Server ldflags; worker-reported version/SHA/date reflected in state |
| Draining | ✅ | Workers can be configured to drain before exiting to avoid interrupting an ongoing request with `--drain-timeout` |
| Linux Deamons | ✅ | Debian packages are provided to install the worker and server as daemons |
| Desktop Trays | In Progress | Windows and macOS tray applications to launch, configure and monitor the worker |
| Server dashboard | ✅ | `/state` HTML page visualizes workers via SSE |
| MCP endpoint bearer auth | ✅ | `llamapool-mcp` requires `Authorization: Bearer <AUTH_TOKEN>` when set |
| Private MCP Endpoints | ✅ | Allow clients to expose an ephemeral MCP server through the `llamapool-mcp` relay |
