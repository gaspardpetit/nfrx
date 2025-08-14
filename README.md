[![Build](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml)
[![Docker](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml)
[![.deb](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml)


# llamapool

<div align="center">
  <img alt="llamapool" width="240" src="https://github.com/gaspardpetit/llamapool/blob/888f4e74e32c752adb75662813438d2da16513a4/doc/img/llamapool-logo-3.png">
</div>

## Overview

**llamapool** is a lightweight, distributed worker pool that exposes an OpenAI-compatible `chat/completions` API, forwarding requests to one or more connected **LLM workers**.  
It sits in front of existing LLM runtimes such as [Ollama](https://github.com/ollama/ollama), [vLLM](https://github.com/vllm-project/vllm), or [Open-WebUI](https://github.com/open-webui/open-webui), allowing you to scale, load-balance, and securely access them from anywhere.

A typical deployment looks like this:

- **`llamapool-server`** is deployed to a public or semi-public location (e.g., Azure, GCP, AWS, or a self-hosted server with dynamic DNS).
- **`llamapool-worker`** runs on private machines (e.g., a Mac Studio or personal GPU workstation) alongside an LLM service.
  When a worker connects, its available models are registered with the server and become accessible via the public API.

## macOS Menu Bar App

An early-stage macOS menu bar companion lives under `desktop/macos/llamapool/`. It polls `http://127.0.0.1:4555/status` every two seconds to display live worker status and can manage a per-user LaunchAgent to start or stop a local `llamapool-worker` and toggle launching at login.
The app icon is stored as a base64 file (`AppIcon.png.b64`); decode it to `AppIcon.png` before building.

### Key features
- **Dynamic worker discovery** – Workers can connect and disconnect at any time; the server updates the available model list in real-time.
- **Least-busy routing** – If multiple workers support the same model, the server dispatches requests to the one with the lowest current load.
- **Alias-based model fallback** – Requests for a missing quantization fall back to workers serving the same base model.
- **Security by design** –
  - Separate authentication keys for clients (`API_KEY`) and workers (`WORKER_KEY`).
  - Workers typically run behind firewalls and connect outbound over HTTPS/WSS.  
  - All traffic is encrypted end-to-end.
- **Protocol compatibility** – Accepts and forwards OpenAI-style `POST /v1/chat/completions` without altering JSON payloads.

### How it works
- The **server** accepts incoming HTTP requests from clients, authenticates them, and routes them to workers via WebSocket connections.
- **Workers** authenticate using a shared `WORKER_KEY` and advertise the models they can serve.
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
│  │  /v1/chat/completions    │                     │  /metrics     │   │
│  │  /v1/models (+/{id})     │                     └───────────────┘   │
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
(WORKER_KEY) │    | REQUEST          (WORKER_KEY) │           | REQUEST
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
  - `GET /v1/models`
  - `GET /v1/models/{id}`
- OpenAI Chat Completions: `POST /v1/chat/completions`
- llamapool API:
  - **State (JSON):** `GET /api/v1/state`
  - **State (SSE):** `GET /api/v1/state/stream`
- Prometheus metrics: `GET /metrics` (can run on a separate port via `--metrics-port`)


## Security

- **Client authentication**: `API_KEY` required for `/api` and `/v1` routes via `Authorization: Bearer <API_KEY>`.
- **Worker authentication**: `WORKER_KEY` required for worker WebSocket registration.
- **Transport**: run behind TLS (HTTPS/WSS) via reverse proxy or terminate TLS in-process.

- **Service isolation**: Debian packages run the daemons as the dedicated `llamapool` user with systemd-managed directories
  (`/var/lib/llamapool`, `/var/cache/llamapool`, `/run/llamapool`) and hardening flags like `NoNewPrivileges=true` and
  `ProtectSystem=full`.

## Monitoring & Observability

- **Prometheus** (`/metrics`, configurable port via `--metrics-port`):
  - `llamapool_build_info{component="server",version,sha,date}`
  - `llamapool_model_requests_total{model,outcome}`
  - `llamapool_model_tokens_total{model,kind}`
  - `llamapool_request_duration_seconds{worker_id,model}` (histogram)
  - (Optionally) per-worker gauges/counters if enabled.
- **Worker metrics** (`--metrics-addr`):
  - Exposes `llamapool_worker_*` series such as
    `llamapool_worker_connected_to_server`,
    `llamapool_worker_connected_to_ollama`,
    `llamapool_worker_current_jobs`,
    `llamapool_worker_max_concurrency`,
    `llamapool_worker_jobs_started_total`,
    `llamapool_worker_jobs_succeeded_total`,
    `llamapool_worker_jobs_failed_total`, and
    `llamapool_worker_job_duration_seconds` (histogram).

- **JSON/SSE State** (`/api/v1/state`, `/api/v1/state/stream`): suitable for custom dashboards showing:
  - worker list and status (connected/working/idle/gone)
  - per-worker totals (processed, inflight, failures, avg duration)
  - per-model availability (how many workers support each model)
  - versions/build info for server & workers


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
PORT=8080 WORKER_KEY=secret API_KEY=test123 go run ./cmd/llamapool-server
# or to expose metrics on a different port:
# PORT=8080 METRICS_PORT=9090 WORKER_KEY=secret API_KEY=test123 go run ./cmd/llamapool-server
```

On Windows (CMD)

```
set PORT=8080
set WORKER_KEY=secret
set API_KEY=test123
go run .\cmd\llamapool-server
REM or if you built:
.\bin\llamapool-server.exe
```

On Windows (Powershell)

```
$env:PORT = "8080"; $env:WORKER_KEY = "secret"; $env:API_KEY = "test123"
go run .\cmd\llamapool-server
# or if you built:
.\bin\llamapool-server.exe
```


### Worker

On Linux:

```bash
SERVER_URL=ws://localhost:8080/workers/connect WORKER_KEY=secret OLLAMA_BASE_URL=http://127.0.0.1:11434 WORKER_NAME=Alpha go run ./cmd/llamapool-worker
```
Optionally set `OLLAMA_API_KEY` to forward an API key to the local Ollama instance. The worker proxies requests to `${OLLAMA_BASE_URL}/v1/chat/completions`.

On Windows (CMD)

```
set SERVER_URL=ws://localhost:8080/workers/connect
set WORKER_KEY=secret
set OLLAMA_BASE_URL=http://127.0.0.1:11434
go run .\cmd\llamapool-worker
REM or if you built:
.\bin\llamapool-worker.exe
```

On Windows (Powershell)

```
$env:SERVER_URL = "ws://localhost:8080/workers/connect"
$env:WORKER_KEY = "secret"
$env:OLLAMA_BASE_URL = "http://127.0.0.1:11434"
$env:WORKER_NAME = "Alpha"
go run .\cmd\llamapool-worker
# or:
.\bin\llamapool-worker.exe
```



## Run with Docker

Pre-built images are available:

- Server: `ghcr.io/gaspardpetit/llamapool-server:main`
- Worker: `ghcr.io/gaspardpetit/llamapool-worker:main`

Tagged releases follow semantic versioning (e.g., `v1.2.3`, `v1.2`, `v1`, `latest`); the `main` tag tracks the latest development snapshot.

### Server

```bash
docker run --rm -p 8080:8080 -e WORKER_KEY=secret -e API_KEY=test123 \
  ghcr.io/gaspardpetit/llamapool-server:main
```

### Worker

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/workers/connect \
  -e WORKER_KEY=secret \
  -e OLLAMA_BASE_URL=http://host.docker.internal:11434 \
  ghcr.io/gaspardpetit/llamapool-worker:main
```

When started with `--status-addr <addr>`, the worker serves local endpoints:

- `GET /status` returns the current worker state.
- `GET /version` returns build information.

Sending `SIGTERM` causes the worker to stop accepting new jobs and wait up to
`--drain-timeout` (default 1m) for in-flight work to finish before exiting.
Send `SIGTERM` again to terminate immediately. Set `--drain-timeout=0` to exit
without waiting or `--drain-timeout=-1` to wait indefinitely.

The worker periodically checks the local Ollama instance so that
`connected_to_ollama` and `models` stay current in the `/status` output.

## Configuration

When running as a systemd service, both components read optional configuration
files managed by systemd. The server loads variables from
`/etc/llamapool/server.env` while the worker reads `/etc/llamapool/worker.env`.
Each file contains `KEY=value` pairs, for example:

```bash
# /etc/llamapool/server.env
# API_KEY=your_api_key
# WORKER_KEY=secret
```

Commented example files are installed to `/etc/llamapool/`; edit these files to configure the services.

When no explicit paths are provided, the worker falls back to OS defaults for
its configuration and logs:

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
curl -N -X POST http://localhost:8080/api/generate \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer test123' \
  -d '{"model":"llama3","prompt":"Hello","stream":true}'
```

On Windows (CMD):

```
curl -N -X POST "http://localhost:8080/api/generate" ^
  -H "Content-Type: application/json" ^
  -H "Authorization: Bearer test123" ^
  -d "{ \"model\": \"llama3\", \"prompt\": \"Hello\", \"stream\": true }"
```

On Windows (Powershell):

```
curl -N -X POST http://localhost:8080/api/generate `
  -H "Content-Type: application/json" `
  -H "Authorization: Bearer test123" `
  -d '{ "model": "llama3", "prompt": "Hello", "stream": true }'
```

The server also exposes a basic health check:

```bash
curl http://localhost:8080/healthz
```

For server administration and monitoring:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/api/v1/state
curl -H "Authorization: Bearer test123" http://localhost:8080/api/v1/state/stream
# Prometheus metrics
curl http://localhost:8080/metrics
# or if `--metrics-port` is set:
curl http://localhost:9090/metrics
```

The server also exposes OpenAI-style model listing endpoints:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/v1/models
curl -H "Authorization: Bearer test123" http://localhost:8080/v1/models/llama3:8b
```

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
The service wrapper launches `llamapool-worker.exe` installed at
`%ProgramFiles%\llamapool\llamapool-worker.exe` with its working directory set to
`%ProgramData%\llamapool`. Configuration is read from
`%ProgramData%\llamapool\worker.yaml` and log output is written to
`%ProgramData%\llamapool\Logs\worker.log`. The service is registered as
`llamapool` with delayed automatic start. The tray app currently hosts a tray icon
with placeholder menu items for controlling the worker.

## Currently Supported

| Feature | Supported | Notes |
| --- | --- | --- |
| OpenAI-compatible `POST /v1/chat/completions` | ✅ | Proxied to workers without payload mutation |
| Multiple worker registration | ✅ | Workers can join/leave dynamically; models registered on connect |
| Model-based routing (least-busy) | ✅ | `LeastBusyScheduler` selects worker by current load |
| Model alias fallback | ✅ | Falls back to base model when exact quantization not available |
| API key authentication for clients | ✅ | `Authorization: Bearer <API_KEY>` for `/api` and `/v1` routes |
| Worker key authentication | ✅ | Workers authenticate over WebSocket using `WORKER_KEY` |
| Dynamic model discovery | ✅ | Workers advertise supported models; server aggregates |
| HTTPS/WSS transport | ✅ | Use TLS terminator or run behind reverse proxy; WS path configurable |
| Prometheus metrics endpoint | ✅ | `/metrics`; includes build info, per-model counters, histograms; supports separate `--metrics-port` |
| Real-time state API (JSON) | ✅ | `GET /api/v1/state` returns full server/worker snapshot |
| Real-time state stream (SSE) | ✅ | `GET /api/v1/state/stream` for dashboards |
| Token usage tracking | ✅ | Per-model and per-worker token totals (in/out) |
| Per-model success/error rates | ✅ | `llamapool_model_requests_total{outcome=...}` |
| Build info (server & worker) | ✅ | Server ldflags; worker-reported version/SHA/date reflected in state |
| Private MCP Endpoints | Planned | Allow clients to expose an ephemeral MCP server through the llamapool-server |
