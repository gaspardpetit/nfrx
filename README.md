[![Build](https://github.com/gaspardpetit/infero/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/infero/actions/workflows/ci.yml)
[![Docker](https://github.com/gaspardpetit/infero/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/gaspardpetit/infero/actions/workflows/docker-publish.yml)
[![.deb](https://github.com/gaspardpetit/infero/actions/workflows/release-deb.yml/badge.svg)](https://github.com/gaspardpetit/infero/actions/workflows/release-deb.yml)

# īnferō

īnferō lets you expose your private AI services (LLM runtimes, tools, RAG processes) through a secure, public, OpenAI-compatible API — without exposing your local machines.

- **Run locally:** Keep Ollama, vLLM, MCP servers, or custom RAG processes on your own Macs, PCs, or servers.
- **Connect out:** Each worker/tool connects outbound to a single public **infero** server (no inbound connections to your LAN).
- **Use securely:** Clients and commercial LLMs (e.g., OpenAI, Claude) talk to **infero** via standard OpenAI endpoints or MCP URLs.
- **Scale flexibly:** Add multiple heterogeneous machines; **infero** routes requests to the right one, queues when busy, and supports graceful draining for maintenance.

## Getting Started

### Prerequisites (all setups)

- A public host (cloud/VPS) with a domain or public IP for the **infero** server.
- TLS (recommended) — terminate HTTPS/WSS at **infero** or your reverse proxy.
- Two credentials:
  - **API_KEY** — authenticates clients calling the public API.
  - **CLIENT_KEY** — authenticates private connectors (llm/mcp/rag) when they dial out.

For the next examples, define:

```bash
export MY_API_KEY='test123'
export MY_CLIENT_KEY='secret'
export MY_SERVER_PORT=8080
export MY_SERVER_ADDR="localhost:${MY_SERVER_PORT}"
```

### Run the public server

Typically, this goes behind a publicly available URL or IP.

##### Docker

```bash
docker run --rm \
  -p ${MY_SERVER_PORT}:8080 \
  -e CLIENT_KEY="${MY_CLIENT_KEY}" \
  -e API_KEY="${MY_API_KEY}" \
  ghcr.io/gaspardpetit/infero:main
```

##### Bare (Linux)

```bash
PORT=${MY_SERVER_PORT} CLIENT_KEY="${MY_CLIENT_KEY}" API_KEY="${MY_API_KEY}" \
  infero   # or: go run ./cmd/infero
```

You may then choose to expose an LLM provider, an MCP server and/or a RAG system from private hardware behind a NAT/Firewall.

### Expose a local LLM worker (Ollama shown)

##### Docker

```bash
docker run --rm \
  -e SERVER_URL="wss://${MY_SERVER_ADDR}/api/llm/connect" \
  -e CLIENT_KEY="${MY_CLIENT_KEY}" \
  -e COMPLETION_BASE_URL="http://host.docker.internal:11434/v1" \
  ghcr.io/gaspardpetit/infero-llm:main
```

##### Bare (Linux)

```bash
SERVER_URL="wss://${MY_SERVER_ADDR}/api/llm/connect" \
CLIENT_KEY="${MY_CLIENT_KEY}" \
COMPLETION_BASE_URL="http://127.0.0.1:11434/v1" \
infero-llm   # or: go run ./cmd/infero-llm
```

After connecting, you can reach your private instance from the public endpoint:

```bash
curl -N -X POST "https://${MY_SERVER_ADDR}/api/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${MY_API_KEY}" \
  -d '{
        "model":"gpt-oss:20b",
        "messages":[{"role":"user","content":"What's a caliper?"}],
        "stream":true
      }'
```

### Expose a local MCP server (FastMCP shown)

Start a minimal FastMCP server on port 7777:

```bash
python3 - <<'PY'
from datetime import datetime
from fastmcp import FastMCP

app = FastMCP('preferences', stateless_http=True, json_response=True)

@app.tool('favorite/color')
def favorite_color():
    return 'blue with a hint of green'

app.run('http', host='127.0.0.1', port=7777)
PY
```

You should see:

> Starting MCP server 'preferences' with transport 'http' on http://127.0.0.1:7777/mcp

Now expose this MCP server to infero:

```bash
docker run --rm \
  -e SERVER_URL="wss://${MY_SERVER_ADDR}/api/mcp/connect" \
  -e CLIENT_KEY="${MY_CLIENT_KEY}" \
  -e CLIENT_ID="my-mcp-server-123" \
  ghcr.io/gaspardpetit/infero-mcp:main
```

Use your private MCP server with a public LLM (OpenAI Responses API example):

```bash
curl https://api.openai.com/v1/responses \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  --data '{
  "model": "gpt-5",
  "input": "This is a test for using tools. You retrieve your favorite color from the preferences tool to complete this test.",
    "tools": [
      {
        "type": "mcp",
        "server_label": "preferences",
        "server_url": "https://${MY_SERVER_ADDR}/api/mcp/id/my-mcp-server-123",
        "authorization": "${MY_API_KEY}",
        "require_approval": "never"
      }
    ]
}'
```

You should see something like:

> "My favorite color is: blue with a hint of green."

## Overview

**infero** is a lightweight, distributed worker pool that exposes an OpenAI-compatible `chat/completions` API, forwarding requests to one or more connected **LLM workers**.
It sits in front of existing LLM runtimes such as [Ollama](https://github.com/ollama/ollama), [vLLM](https://github.com/vllm-project/vllm), or [Open-WebUI](https://github.com/open-webui/open-webui), allowing you to scale, load-balance, and securely access them from anywhere.

In addition to LLM workers, infero now supports relaying [Model Context Protocol](https://github.com/modelcontextprotocol) calls. 

The server exposes a Streamable HTTP MCP endpoint at `POST /api/mcp/id/{id}` and forwards requests verbatim over WebSocket to a connected `infero-mcp` process. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. When the relay is started with `AUTH_TOKEN`, clients must supply `Authorization: Bearer <token>` when calling this endpoint. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments.

Server-initiated JSON-RPC requests (for example sampling calls) are forwarded across the WebSocket bridge and relayed back to clients, preserving full protocol semantics.

The new `infero-mcp` binary connects a private MCP provider to the public `infero`, allowing clients to invoke MCP methods via `POST /api/mcp/id/{id}`. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments. By default `infero-mcp` requires absolute stdio commands and verifies TLS certificates; set `MCP_STDIO_ALLOW_RELATIVE=true` or `MCP_HTTP_INSECURE_SKIP_VERIFY=true` to relax these checks, and `MCP_OAUTH_TOKEN_FILE` to securely cache OAuth tokens on disk.
By default the MCP relay exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s). When enabled, it also probes the MCP provider and remains in a `not_ready` state until the provider becomes reachable.

`infero-mcp` reads configuration from a YAML file when `CONFIG_FILE` is set. Values in the file—such as transport order, protocol version preference, or stdio working directory—are used as defaults and can be overridden by environment variables or CLI flags (e.g. `--mcp-http-url`, `--mcp-stdio-workdir`).

For transport configuration, common errors, and developer guidance see [doc/mcpclient.md](doc/mcpclient.md). For a comprehensive list of configuration options, see [doc/env.md](doc/env.md). Sample YAML configuration templates with defaults are available under `examples/config/`.
Server state can be shared across multiple infero instances by setting `REDIS_ADDR` to a Redis connection URL (including Sentinel or cluster deployments).
For project direction and future enhancements, see [doc/roadmap.md](doc/roadmap.md).

A typical deployment looks like this:

- **`infero`** is deployed to a public or semi-public location (e.g., Azure, GCP, AWS, or a self-hosted server with dynamic DNS).
- **`infero-llm`** runs on private machines (e.g., a Mac Studio or personal GPU workstation) alongside an LLM service.
  When a worker connects, its available models are registered with the server and become accessible via the public API.

## macOS Menu Bar App

An early-stage macOS menu bar companion lives under `desktop/macos/infero/`. It polls `http://127.0.0.1:4555/status` every two seconds to display live worker status and can manage a per-user LaunchAgent to start or stop a local `infero-llm` and toggle launching at login. A simple preferences window lets you edit worker connection settings which are written to `~/Library/Application Support/infero/worker.yaml`, and the menu offers quick links to open the config and logs folders, view live logs, copy diagnostics to the Desktop, and check for updates via Sparkle.

### Packaging

The macOS app can be distributed as a signed and notarized DMG. After building the `infero` scheme in Release, create the disk image and submit it for notarization:

```bash
# Create a DMG with an /Applications symlink
ci/create-dmg.sh path/to/infero.app build/infero.dmg

# Notarize (requires AC_API_KEY_ID, AC_API_ISSUER_ID and AC_API_P8)
ci/notarize.sh build/infero.dmg
xcrun stapler staple build/infero.dmg
```

`AC_API_P8` must contain a base64-encoded App Store Connect API key. Once notarization completes, the DMG can be distributed and will pass Gatekeeper on clean systems.

When using the GitHub Actions workflow, provide the `AC_TEAM_ID` secret with your Apple Developer Team ID so the archive can be signed and exported.

## Windows Tray App

A Windows tray companion lives under `desktop/windows/`. It polls `http://127.0.0.1:4555/status` every two seconds to display worker status.
The tray can start or stop the local `infero` Windows service, toggle whether it launches automatically with Windows, edit worker connection settings, open the config and logs folders, view live logs, and collect diagnostics to the Desktop. When the worker exposes lifecycle control endpoints, the tray also provides **Drain**, **Undrain**, and **Shutdown after drain** actions.

The Windows service runs `infero-llm` with the `--reconnect` flag and shuts down if the worker process exits, preventing orphaned workers. The worker is attached to a job object so it also terminates if the service process is killed.

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
│                                 infero                      │
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
     │      infero-llm   │           │      infero-llm    │
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
- infero API:
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

- **Service isolation**: Debian packages run the daemons as the dedicated `infero` user with systemd-managed directories
  (`/var/lib/infero`, `/var/cache/infero`, `/run/infero`) and hardening flags like `NoNewPrivileges=true` and
  `ProtectSystem=full`.

## Monitoring & Observability

- **Prometheus** (`/metrics`, configurable address via `METRICS_PORT` or `--metrics-port`):
  - `infero_build_info{component="server",version,sha,date}`
  - `infero_model_requests_total{model,outcome}`
  - `infero_model_tokens_total{model,kind}`
  - `infero_request_duration_seconds{worker_id,model}` (histogram)
  - (Optionally) per-worker gauges/counters if enabled.
- **Worker metrics** (`METRICS_PORT` or `--metrics-port`):
  - Exposes `infero_worker_*` series such as
    `infero_worker_connected_to_server`,
    `infero_worker_connected_to_backend`,
    `infero_worker_current_jobs`,
    `infero_worker_max_concurrency`,
    `infero_worker_jobs_started_total`,
    `infero_worker_jobs_succeeded_total`,
    `infero_worker_jobs_failed_total`, and
    `infero_worker_job_duration_seconds` (histogram).
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
wget https://github.com/gaspardpetit/infero/releases/download/v1.3.0/infero_1.3.0-1_amd64.deb
sudo dpkg -i infero_1.3.0-1_amd64.deb
sudo systemctl status infero
```

## Build

On Linux:

```bash
make build
```

On Windows:
```
go build -o .\bin\infero.exe .\cmd\infero
go build -o .\bin\infero-llm.exe .\cmd\infero-llm
```

### Version

Both binaries expose a `--version` flag that prints the build metadata:

```bash
infero --version
infero-llm --version
```

The output includes the version, git SHA and build date.
The same version information appears at the top of `--help` output.

## Run

### Server

On Linux:

```bash
PORT=8080 CLIENT_KEY=secret API_KEY=test123 go run ./cmd/infero
# or to expose metrics on a different port:
# PORT=8080 METRICS_PORT=9090 CLIENT_KEY=secret API_KEY=test123 go run ./cmd/infero
```

Workers register with the server at `/api/workers/connect`.
`infero-mcp` connects to the server at `ws://<server>/api/mcp/connect` and receives a unique id which is used by clients when calling `POST /api/mcp/id/{id}`.

Sending `SIGTERM` to the server stops acceptance of new worker, MCP, and inference requests while allowing in-flight work to complete. The server waits up to `--drain-timeout` (default 5m) before shutting down.

On Windows (CMD)

```
set PORT=8080
set CLIENT_KEY=secret
set API_KEY=test123
go run .\cmd\infero
REM or if you built:
.\bin\infero.exe
```

On Windows (Powershell)

```
$env:PORT = "8080"; $env:CLIENT_KEY = "secret"; $env:API_KEY = "test123"
go run .\cmd\infero
# or if you built:
.\bin\infero.exe
```


### Worker

On Linux:

```bash
SERVER_URL=ws://localhost:8080/api/workers/connect CLIENT_KEY=secret COMPLETION_BASE_URL=http://127.0.0.1:11434/v1 CLIENT_NAME=Alpha go run ./cmd/infero-llm
```
Optionally set `COMPLETION_API_KEY` to forward an API key to the backend. The worker proxies requests to `${COMPLETION_BASE_URL}/chat/completions`.

On Windows (CMD)

```
set SERVER_URL=ws://localhost:8080/api/workers/connect
set CLIENT_KEY=secret
set COMPLETION_BASE_URL=http://127.0.0.1:11434/v1
go run .\cmd\infero-llm
REM or if you built:
.\bin\infero-llm.exe
```

On Windows (Powershell)

```
$env:SERVER_URL = "ws://localhost:8080/api/workers/connect"
$env:CLIENT_KEY = "secret"
$env:COMPLETION_BASE_URL = "http://127.0.0.1:11434/v1"
$env:CLIENT_NAME = "Alpha"
go run .\cmd\infero-llm
# or:
.\bin\infero-llm.exe
```

By default the worker exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s).
When enabled, the worker also retries its Ollama backend and remains connected to the server in a `not_ready` state with zero concurrency until the backend becomes available.



## Run with Docker

Pre-built images are available:

- Server: `ghcr.io/gaspardpetit/infero:main`
- Worker: `ghcr.io/gaspardpetit/infero-llm:main`

Tagged releases follow semantic versioning (e.g., `v1.2.3`, `v1.2`, `v1`, `latest`); the `main` tag tracks the latest development snapshot.

### Server

```bash
docker run --rm -p 8080:8080 -e CLIENT_KEY=secret -e API_KEY=test123 \
  ghcr.io/gaspardpetit/infero:main
```

### Worker

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/api/workers/connect \
  -e CLIENT_KEY=secret \
  -e COMPLETION_BASE_URL=http://host.docker.internal:11434/v1 \
  ghcr.io/gaspardpetit/infero-llm:main
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
`connected_to_backend` and `models` stay current in the `/status` output.
If the model list changes, the worker proactively notifies the server so
`/api/v1/models` reflects the latest information. Configure the poll interval
with `MODEL_POLL_INTERVAL` or `--model-poll-interval`.

Control endpoints require an `X-Auth-Token` header. The token is generated on
first run and stored alongside the worker config as `worker.token`.

## Configuration

When running as a systemd service, both components read optional configuration
files managed by systemd. The server loads variables from
`/etc/infero/server.env` while the worker reads `/etc/infero/worker.env`.
Each file contains `KEY=value` pairs, for example:

```bash
# /etc/infero/server.env
# API_KEY=your_api_key
# CLIENT_KEY=secret
```

Commented example files are installed to `/etc/infero/`; edit these files to configure the services.

When no explicit paths are provided, the worker falls back to OS defaults for
its configuration and logs:

- **Linux:** `/etc/infero/worker.yaml`
- **macOS:** `~/Library/Application Support/infero/worker.yaml` and
  `~/Library/Logs/infero/`
- **Windows:** `%ProgramData%\infero\worker.yaml` and
  `%ProgramData%\infero\Logs\`

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

The HTML dashboard at `/state` visualizes workers and reports per-worker token totals and average token processing rates.

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
A WiX-based MSI installer in `desktop/windows/Installer` installs the worker and tray binaries, registers the `infero` service, creates `%ProgramData%\infero` with a default configuration, and adds a Start Menu shortcut for the tray.
The service wrapper launches `infero-llm.exe` installed at
`%ProgramFiles%\infero\infero-llm.exe` with its working directory set to
`%ProgramData%\infero`. Configuration is read from
`%ProgramData%\infero\worker.yaml` and log output is written to
`%ProgramData%\infero\Logs\worker.log`. The service is registered as
`infero` with delayed automatic start. The tray app now polls the local worker
every two seconds and updates its menu and tooltip to reflect the current status.
A details dialog shows connection information, job counts, and any last error. A preferences window can edit the worker configuration and write it back to the YAML file, and the menu offers quick links to open the config and logs folders, view live logs, and collect diagnostics.
The tray app checks the [infero GitHub releases](https://github.com/gaspardpetit/infero/releases) once per day and notifies when a new version is available.
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
| Token usage tracking | ✅ | Per-model and per-worker token totals (in/out) with average rate on dashboard |
| Per-model success/error rates | ✅ | `infero_model_requests_total{outcome=...}` |
| Build info (server & worker) | ✅ | Server ldflags; worker-reported version/SHA/date reflected in state |
| Draining | ✅ | Workers can be configured to drain before exiting to avoid interrupting an ongoing request with `--drain-timeout` |
| Linux Deamons | ✅ | Debian packages are provided to install the worker and server as daemons |
| Desktop Trays | In Progress | Windows and macOS tray applications to launch, configure and monitor the worker |
| Server dashboard | ✅ | `/state` HTML page visualizes workers via SSE |
| MCP endpoint bearer auth | ✅ | `infero-mcp` requires `Authorization: Bearer <AUTH_TOKEN>` when set |
| Private MCP Endpoints | ✅ | Allow clients to expose an ephemeral MCP server through the `infero-mcp` relay |
