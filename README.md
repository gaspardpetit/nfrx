[![Build](https://github.com/gaspardpetit/nfrx/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/nfrx/actions/workflows/ci.yml)
[![Docker](https://github.com/gaspardpetit/nfrx/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/gaspardpetit/nfrx/actions/workflows/docker-publish.yml)
[![.deb](https://github.com/gaspardpetit/nfrx/actions/workflows/release-deb.yml/badge.svg)](https://github.com/gaspardpetit/nfrx/actions/workflows/release-deb.yml)

# nfrx

nfrx lets you expose private AI services (LLM runtimes, tools, RAG processes) through a public gateway.

- **Run locally:** Keep Ollama, vLLM, MCP servers, docling, or custom RAG processes on your own Macs, PCs, or servers.
- **Connect out:** Each worker/tool connects outbound to a single public **nfrx** server (no inbound connections to your LAN).
- **Use securely:** Clients and commercial LLMs (e.g., OpenAI, Claude) talk to **nfrx** via standard OpenAI endpoints or MCP URLs.
- **Scale flexibly:** Add multiple heterogeneous machines; **nfrx** routes requests to the right one, queues when busy, and supports graceful draining for maintenance.

Two main usage patterns are covered:

## Worker agents

 Registering worker agents configured for specific tasks, ex.
   - llm agents providing services for listed models;
   - document transformer providing OCR or conversion to text or markdown; and
   - transcription of audio files.

These can be implemented to support compatibility routing, load balancing and workload distribution.
Workers can be added dynamically or drained/removed to scale up and down as needed.
The workers can be on private hardware, behind NAT, since connection is achieved with a local agent running next to the local LLM.

## Private Resources

Exposing private resources such as documents, ex. 
 - Allowing a public LLM to search through local documents (RAGs); and
 - Allowing a public LLM to execute functions on a local MCP serve.

This is achieved by having a local agent connecting to the nfrx server an opening a route using a private id and secret.

## Getting Started

### Prerequisites (all setups)

- A public host (cloud/VPS) with a domain or public IP for the **nfrx** server.
- TLS (recommended) — terminate HTTPS/WSS at **nfrx** or your reverse proxy.
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
  ghcr.io/gaspardpetit/nfrx:main
```

##### Bare (Linux)

```bash
PORT=${MY_SERVER_PORT} CLIENT_KEY="${MY_CLIENT_KEY}" API_KEY="${MY_API_KEY}" \
  nfrx   # or: go run ./nfrx-server/cmd/nfrx
```

The server also starts a gRPC control endpoint on `PORT+1` for agent registration and heartbeats.
Agents default to dialing this address based on `SERVER_URL`, but it can be overridden via `CONTROL_GRPC_ADDR` or `CONTROL_GRPC_SOCKET`.

You may then choose to expose an LLM provider, an MCP server and/or a RAG system from private hardware behind a NAT/Firewall.

Set `PLUGINS` to control which modules load (`llm` and/or `mcp`; defaults to both). For example, `PLUGINS=llm` disables the MCP relay.

### Expose a local LLM worker (Ollama shown)

##### Docker

```bash
docker run --rm \
  -e SERVER_URL="wss://${MY_SERVER_ADDR}/api/workers/connect" \
  -e CLIENT_KEY="${MY_CLIENT_KEY}" \
  -e COMPLETION_BASE_URL="http://host.docker.internal:11434/v1" \
  ghcr.io/gaspardpetit/nfrx-llm:main
```

##### Bare (Linux)

```bash
SERVER_URL="wss://${MY_SERVER_ADDR}/api/workers/connect" \
CLIENT_KEY="${MY_CLIENT_KEY}" \
COMPLETION_BASE_URL="http://127.0.0.1:11434/v1" \
nfrx-llm   # or: go run ./nfrx-plugins-llm/cmd/nfrx-llm
```

After connecting, you can reach your private instance from the public endpoint:

```bash
curl -N -X POST "https://${MY_SERVER_ADDR}/api/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${MY_API_KEY}" \
  -d '{
        "model":"gpt-oss:20b",
        "messages":[{"role":"user","content":"What is a caliper?"}],
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

Now expose this MCP server to nfrx:

```bash
docker run --rm \
  -e SERVER_URL="wss://${MY_SERVER_ADDR}/api/mcp/connect" \
  -e CLIENT_KEY="${MY_CLIENT_KEY}" \
  -e CLIENT_ID="my-mcp-server-123" \
  ghcr.io/gaspardpetit/nfrx-mcp:main
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

**nfrx** is a lightweight, distributed worker pool that exposes an OpenAI-compatible `chat/completions` API, forwarding requests to one or more connected **LLM workers**.
It sits in front of existing LLM runtimes such as [Ollama](https://github.com/ollama/ollama), [vLLM](https://github.com/vllm-project/vllm), or [Open-WebUI](https://github.com/open-webui/open-webui), allowing you to scale, load-balance, and securely access them from anywhere.

In addition to LLM workers, nfrx now supports relaying [Model Context Protocol](https://github.com/modelcontextprotocol) calls. 

The server exposes a Streamable HTTP MCP endpoint at `POST /api/mcp/id/{id}` and forwards requests verbatim over WebSocket to a connected `nfrx-mcp` process. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. When the relay is started with `AUTH_TOKEN`, clients must supply `Authorization: Bearer <token>` when calling this endpoint. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments.

Server-initiated JSON-RPC requests (for example sampling calls) are forwarded across the WebSocket bridge and relayed back to clients, preserving full protocol semantics.

The new `nfrx-mcp` binary connects a private MCP provider to the public `nfrx`, allowing clients to invoke MCP methods via `POST /api/mcp/id/{id}`. The broker enforces request/response size limits, per-client concurrency caps, and 30s call timeouts; cancellation is not yet implemented. The client negotiates protocol versions and server capabilities, and exposes tunables such as `MCP_PROTOCOL_VERSION`, `MCP_HTTP_TIMEOUT`, and `MCP_MAX_INFLIGHT` for advanced deployments. By default `nfrx-mcp` requires absolute stdio commands and verifies TLS certificates; set `MCP_STDIO_ALLOW_RELATIVE=true` or `MCP_HTTP_INSECURE_SKIP_VERIFY=true` to relax these checks, and `MCP_OAUTH_TOKEN_FILE` to securely cache OAuth tokens on disk.
By default the MCP relay exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s). When enabled, it also probes the MCP provider and remains in a `not_ready` state until the provider becomes reachable.

`nfrx-mcp` reads configuration from a YAML file when `CONFIG_FILE` is set. Values in the file—such as transport order, protocol version preference, or stdio working directory—are used as defaults and can be overridden by environment variables or CLI flags (e.g. `--mcp-http-url`, `--mcp-stdio-workdir`).

For transport configuration, common errors, and developer guidance see [doc/mcpclient.md](doc/mcpclient.md). For a comprehensive list of configuration options, see [doc/env.md](doc/env.md). Sample YAML configuration templates with defaults are available under `examples/config/`.
Server state can be shared across multiple nfrx instances by setting `REDIS_ADDR` to a Redis connection URL (including Sentinel or cluster deployments).
For project direction and future enhancements, see [doc/roadmap.md](doc/roadmap.md).

A typical deployment looks like this:

- **`nfrx`** is deployed to a public or semi-public location (e.g., Azure, GCP, AWS, or a self-hosted server with dynamic DNS).
- **`nfrx-llm`** runs on private machines (e.g., a Mac Studio or personal GPU workstation) alongside an LLM service.
  When a worker connects, its available models are registered with the server and become accessible via the public API.

## Developing Plugins

nfrx exposes a small extension interface so new modules can register routes,
metrics and state. Skeleton implementations for worker-based and relay-based
extensions live under [templates/](templates/).

## macOS Menu Bar App

An early-stage macOS menu bar companion lives under `desktop/macos/nfrx/`. It polls `http://127.0.0.1:4555/status` every two seconds to display live worker status and can manage a per-user LaunchAgent to start or stop a local `nfrx-llm` and toggle launching at login. A simple preferences window lets you edit worker connection settings which are written to `~/Library/Application Support/nfrx/worker.yaml`, and the menu offers quick links to open the config and logs folders, view live logs, copy diagnostics to the Desktop, and check for updates via Sparkle.

### Packaging

The macOS app can be distributed as a signed and notarized DMG. After building the `nfrx` scheme in Release, create the disk image and submit it for notarization:

```bash
# Create a DMG with an /Applications symlink
ci/create-dmg.sh path/to/nfrx.app build/nfrx.dmg

# Notarize (requires AC_API_KEY_ID, AC_API_ISSUER_ID and AC_API_P8)
ci/notarize.sh build/nfrx.dmg
xcrun stapler staple build/nfrx.dmg
```

`AC_API_P8` must contain a base64-encoded App Store Connect API key. Once notarization completes, the DMG can be distributed and will pass Gatekeeper on clean systems.

When using the GitHub Actions workflow, provide the `AC_TEAM_ID` secret with your Apple Developer Team ID so the archive can be signed and exported.

## Windows Tray App

A Windows tray companion lives under `desktop/windows/`. It polls `http://127.0.0.1:4555/status` every two seconds to display worker status.
The tray can start or stop the local `nfrx` Windows service, toggle whether it launches automatically with Windows, edit worker connection settings, open the config and logs folders, view live logs, and collect diagnostics to the Desktop. When the worker exposes lifecycle control endpoints, the tray also provides **Drain**, **Undrain**, and **Shutdown after drain** actions.

The Windows service runs `nfrx-llm` with the `--reconnect` flag and shuts down if the worker process exits, preventing orphaned workers. The worker is attached to a job object so it also terminates if the service process is killed.

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
│                                 nfrx                      │
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
     │      nfrx-llm   │           │      nfrx-llm    │
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
- nfrx API:
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

- **Service isolation**: Debian packages run the daemons as the dedicated `nfrx` user with systemd-managed directories
  (`/var/lib/nfrx`, `/var/cache/nfrx`, `/run/nfrx`) and hardening flags like `NoNewPrivileges=true` and
  `ProtectSystem=full`.

## Monitoring & Observability

- **Prometheus** (`/metrics`, configurable address via `METRICS_PORT` or `--metrics-port`):
  - `nfrx_build_info{component="server",version,sha,date}`
  - `nfrx_model_requests_total{model,outcome}`
  - `nfrx_model_tokens_total{model,kind}`
  - `nfrx_request_duration_seconds{worker_id,model}` (histogram)
  - (Optionally) per-worker gauges/counters if enabled.
- **Worker metrics** (`METRICS_PORT` or `--metrics-port`):
  - Exposes `nfrx_worker_*` series such as
    `nfrx_worker_connected_to_server`,
    `nfrx_worker_connected_to_backend`,
    `nfrx_worker_current_jobs`,
    `nfrx_worker_max_concurrency`,
    `nfrx_worker_jobs_started_total`,
    `nfrx_worker_jobs_succeeded_total`,
    `nfrx_worker_jobs_failed_total`, and
    `nfrx_worker_job_duration_seconds` (histogram).
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
wget https://github.com/gaspardpetit/nfrx/releases/download/v1.3.0/nfrx_1.3.0-1_amd64.deb
sudo dpkg -i nfrx_1.3.0-1_amd64.deb
sudo systemctl status nfrx
```

## Build

On Linux:

```bash
make build
```

On Windows:
```
go build -o .\bin\nfrx.exe .\nfrx-server\cmd\nfrx
go build -o .\bin\nfrx-llm.exe .\nfrx-plugins-llm\cmd\nfrx-llm
```

### Version

Both binaries expose a `--version` flag that prints the build metadata:

```bash
nfrx --version
nfrx-llm --version
```

The output includes the version, git SHA and build date.
The same version information appears at the top of `--help` output.

## Run

### Server

On Linux:

```bash
PORT=8080 CLIENT_KEY=secret API_KEY=test123 go run ./nfrx-server/cmd/nfrx
# or to expose metrics on a different port:
# PORT=8080 METRICS_PORT=9090 CLIENT_KEY=secret API_KEY=test123 go run ./nfrx-server/cmd/nfrx
```

Workers register with the server at `/api/workers/connect`.
`nfrx-mcp` connects to the server at `ws://<server>/api/mcp/connect` and receives a unique id which is used by clients when calling `POST /api/mcp/id/{id}`.

Sending `SIGTERM` to the server stops acceptance of new worker, MCP, and inference requests while allowing in-flight work to complete. The server waits up to `--drain-timeout` (default 5m) before shutting down.

On Windows (CMD)

```
set PORT=8080
set CLIENT_KEY=secret
set API_KEY=test123
go run .\nfrx-server\cmd\nfrx
REM or if you built:
.\bin\nfrx.exe
```

On Windows (Powershell)

```
$env:PORT = "8080"; $env:CLIENT_KEY = "secret"; $env:API_KEY = "test123"
go run .\nfrx-server\cmd\nfrx
# or if you built:
.\bin\nfrx.exe
```


### Worker

On Linux:

```bash
SERVER_URL=ws://localhost:8080/api/workers/connect CLIENT_KEY=secret COMPLETION_BASE_URL=http://127.0.0.1:11434/v1 CLIENT_NAME=Alpha go run ./nfrx-plugins-llm/cmd/nfrx-llm
```
Optionally set `COMPLETION_API_KEY` to forward an API key to the backend. The worker proxies requests to `${COMPLETION_BASE_URL}/chat/completions`.

On Windows (CMD)

```
set SERVER_URL=ws://localhost:8080/api/workers/connect
set CLIENT_KEY=secret
set COMPLETION_BASE_URL=http://127.0.0.1:11434/v1
go run .\nfrx-plugins-llm\cmd\nfrx-llm
REM or if you built:
.\bin\nfrx-llm.exe
```

On Windows (Powershell)

```
$env:SERVER_URL = "ws://localhost:8080/api/workers/connect"
$env:CLIENT_KEY = "secret"
$env:COMPLETION_BASE_URL = "http://127.0.0.1:11434/v1"
$env:CLIENT_NAME = "Alpha"
go run .\nfrx-plugins-llm\cmd\nfrx-llm
# or:
.\bin\nfrx-llm.exe
```

By default the worker exits if the server is unavailable. Add `-r` or `--reconnect` to keep retrying with backoff (1s×3, 5s×3, 15s×3, then every 30s).
When enabled, the worker also retries its Ollama backend and remains connected to the server in a `not_ready` state with zero concurrency until the backend becomes available.



## Run with Docker

Pre-built images are available:

- Server: `ghcr.io/gaspardpetit/nfrx:main`
- Worker: `ghcr.io/gaspardpetit/nfrx-llm:main`

Tagged releases follow semantic versioning (e.g., `v1.2.3`, `v1.2`, `v1`, `latest`); the `main` tag tracks the latest development snapshot.

### Server

```bash
docker run --rm -p 8080:8080 -e CLIENT_KEY=secret -e API_KEY=test123 \
  ghcr.io/gaspardpetit/nfrx:main
```

### Worker

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/api/workers/connect \
  -e CLIENT_KEY=secret \
  -e COMPLETION_BASE_URL=http://host.docker.internal:11434/v1 \
  ghcr.io/gaspardpetit/nfrx-llm:main
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
`/etc/nfrx/server.env` while the worker reads `/etc/nfrx/worker.env`.
Each file contains `KEY=value` pairs, for example:

```bash
# /etc/nfrx/server.env
# API_KEY=your_api_key
# CLIENT_KEY=secret
```

Commented example files are installed to `/etc/nfrx/`; edit these files to configure the services.

When no explicit paths are provided, the worker falls back to OS defaults for
its configuration and logs:

- **Linux:** `/etc/nfrx/worker.yaml`
- **macOS:** `~/Library/Application Support/nfrx/worker.yaml` and
  `~/Library/Logs/nfrx/`
- **Windows:** `%ProgramData%\nfrx\worker.yaml` and
  `%ProgramData%\nfrx\Logs\`

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

The HTML dashboard at `/state` visualizes workers and reports per-worker token totals, embedding totals, and average processing rates for both tokens and embeddings.

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
A WiX-based MSI installer in `desktop/windows/Installer` installs the worker and tray binaries, registers the `nfrx` service, creates `%ProgramData%\nfrx` with a default configuration, and adds a Start Menu shortcut for the tray.
The service wrapper launches `nfrx-llm.exe` installed at
`%ProgramFiles%\nfrx\nfrx-llm.exe` with its working directory set to
`%ProgramData%\nfrx`. Configuration is read from
`%ProgramData%\nfrx\worker.yaml` and log output is written to
`%ProgramData%\nfrx\Logs\worker.log`. The service is registered as
`nfrx` with delayed automatic start. The tray app now polls the local worker
every two seconds and updates its menu and tooltip to reflect the current status.
A details dialog shows connection information, job counts, and any last error. A preferences window can edit the worker configuration and write it back to the YAML file, and the menu offers quick links to open the config and logs folders, view live logs, and collect diagnostics.
The tray app checks the [nfrx GitHub releases](https://github.com/gaspardpetit/nfrx/releases) once per day and notifies when a new version is available.
For manual end-to-end verification on a clean VM, see [desktop/windows/ACCEPTANCE.md](desktop/windows/ACCEPTANCE.md).

## Currently Supported

| Feature | Supported | Notes |
| --- | --- | --- |
| OpenAI-compatible `POST /api/v1/chat/completions` | ✅ | Proxied to workers without payload mutation |
| OpenAI-compatible `POST /api/v1/embeddings` | ✅ | Requests with large input arrays are split and processed in parallel across workers respecting each worker's ideal embedding batch size |
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
| Per-model success/error rates | ✅ | `nfrx_model_requests_total{outcome=...}` |
| Build info (server & worker) | ✅ | Server ldflags; worker-reported version/SHA/date reflected in state |
| Draining | ✅ | Workers can be configured to drain before exiting to avoid interrupting an ongoing request with `--drain-timeout` |
| Linux Deamons | ✅ | Debian packages are provided to install the worker and server as daemons |
| Desktop Trays | In Progress | Windows and macOS tray applications to launch, configure and monitor the worker |
| Server dashboard | ✅ | `/state` HTML page visualizes workers via SSE |
| MCP endpoint bearer auth | ✅ | `nfrx-mcp` requires `Authorization: Bearer <AUTH_TOKEN>` when set |
| Private MCP Endpoints | ✅ | Allow clients to expose an ephemeral MCP server through the `nfrx-mcp` relay |
