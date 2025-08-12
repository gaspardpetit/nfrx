[![ci](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml)

# llamapool

Llamapool is a minimal worker pool that exposes an Ollama-compatible HTTP API. The
`llamapool-server` binary accepts client requests and dispatches them to connected
`llamapool-worker` processes over WebSocket. Workers authenticate using a shared
bearer token provided via the `TOKEN` environment variable.

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
PORT=8080 WORKER_TOKEN=secret go run ./cmd/llamapool-server
```

On Windows (CMD)

```
set PORT=8080
set WORKER_TOKEN=secret
go run .\cmd\llamapool-server
REM or if you built:
.\bin\llamapool-server.exe
```

On Windows (Powershell)

```
$env:PORT = "8080"; $env:WORKER_TOKEN = "secret"
go run .\cmd\llamapool-server
# or if you built:
.\bin\llamapool-server.exe
```


### Worker

On Linux:

```bash
SERVER_URL=ws://localhost:8080/workers/connect TOKEN=secret OLLAMA_URL=http://127.0.0.1:11434 WORKER_NAME=Alpha go run ./cmd/llamapool-worker
```

On Windows (CMD)

```
set SERVER_URL=ws://localhost:8080/workers/connect
set TOKEN=secret
set OLLAMA_URL=http://127.0.0.1:11434
go run .\cmd\llamapool-worker
REM or if you built:
.\bin\llamapool-worker.exe
```

On Windows (Powershell)

```
$env:SERVER_URL = "ws://localhost:8080/workers/connect"
$env:TOKEN = "secret"
$env:OLLAMA_URL = "http://127.0.0.1:11434"
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
docker run --rm -p 8080:8080 -e WORKER_TOKEN=secret \
  ghcr.io/gaspardpetit/llamapool-server:main
```

### Client

```bash
docker run --rm \
  -e SERVER_URL=ws://localhost:8080/workers/connect \
  -e TOKEN=secret \
  -e OLLAMA_URL=http://host.docker.internal:11434 \
  ghcr.io/gaspardpetit/llamapool-client:main
```

## Example request

On Linux:

```bash
curl -N -X POST http://localhost:8080/api/generate \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3","prompt":"Hello","stream":true}'
```

On Windows (CMD):

```
curl -N -X POST "http://localhost:8080/api/generate" ^
  -H "Content-Type: application/json" ^
  -d "{ \"model\": \"llama3\", \"prompt\": \"Hello\", \"stream\": true }"
```

On Windows (Powershell):

```
curl -N -X POST http://localhost:8080/api/generate `
  -H "Content-Type: application/json" `
  -d '{ "model": "llama3", "prompt": "Hello", "stream": true }'
```

The server also exposes a basic health check:

```bash
curl http://localhost:8080/healthz
```

The server also exposes OpenAI-style model listing endpoints:

```bash
curl http://localhost:8080/v1/models
curl http://localhost:8080/v1/models/llama3:8b
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

