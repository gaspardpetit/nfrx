[![ci](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml)

# llamapool

Llamapool is a minimal worker pool that exposes an Ollama-compatible HTTP API. The
`llamapool-server` binary accepts client requests and dispatches them to connected
`llamapool-worker` processes over WebSocket. Workers authenticate using a shared
key provided via the `WORKER_KEY` environment variable. Client HTTP requests can
be protected with an `API_KEY` passed in the `Authorization` header. The server
also proxies OpenAI-style `POST /v1/chat/completions` requests to workers without
modifying the JSON payloads.

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

## Run

### Server

On Linux:

```bash
PORT=8080 WORKER_KEY=secret API_KEY=test123 go run ./cmd/llamapool-server
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
- Client: `ghcr.io/gaspardpetit/llamapool-client:main`

### Server

```bash
docker run --rm -p 8080:8080 -e WORKER_KEY=secret -e API_KEY=test123 \
  ghcr.io/gaspardpetit/llamapool-server:main
```

### Client

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/workers/connect \
  -e WORKER_KEY=secret \
  -e OLLAMA_BASE_URL=http://host.docker.internal:11434 \
  ghcr.io/gaspardpetit/llamapool-client:main
```

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
```

The server also exposes OpenAI-style model listing endpoints:

```bash
curl -H "Authorization: Bearer test123" http://localhost:8080/v1/models
curl -H "Authorization: Bearer test123" http://localhost:8080/v1/models/llama3:8b
```

## Testing

On Linux:

```bash
make test
```

On Windows:

```
go test ./...
```

