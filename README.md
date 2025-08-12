# llamapool

[![ci](https://github.com/you/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/you/llamapool/actions/workflows/ci.yml)

Llamapool is a minimal worker pool that exposes an Ollama-compatible HTTP API. The
`llamapool-server` binary accepts client requests and dispatches them to connected
`llamapool-worker` processes over WebSocket.

## Build

```bash
make build
```

## Run

### Server

```bash
PORT=8080 WORKER_TOKEN=secret go run ./cmd/llamapool-server
```

### Worker

```bash
SERVER_URL=ws://localhost:8080/workers/connect TOKEN=secret OLLAMA_URL=http://127.0.0.1:11434 go run ./cmd/llamapool-worker
```

## Example request

```bash
curl -N -X POST http://localhost:8080/api/generate \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3","prompt":"Hello","stream":true}'
```

## Testing

```bash
make test
```
