# AGENTS.md

## Project Overview
Llamapool provides a minimal Ollama-compatible server with a pool of workers that proxy
requests to local Ollama instances. The repository contains two binaries:
- `llamapool-server`: hosts the public HTTP API and coordinates workers over WebSocket
- `llamapool-worker`: connects to the server and forwards requests to a local Ollama

## Build & Commands
- Build server and worker: `make build`
- Run tests with race detector: `make test`
- Lint (requires golangci-lint): `make lint`

## Code Style
- Version of Go used is 1.23
- Use standard Go formatting via `gofmt -w` or `go fmt`
- Prefer clarity over cleverness; keep functions small and well named
- Default to the patterns already present in the `internal/` packages

## Testing Guidelines
- Unit tests live alongside the code using `*_test.go` files
- End-to-end tests are in the `test/` directory
- Always run `make build` and `make test` before submitting a change
- If the Dockerfiles under `deploy/` are updated, ensure they still build:
  ```bash
  docker build -f deploy/Dockerfile.server .
  docker build -f deploy/Dockerfile.worker .
  ```

## Further Reading
- @README.md for project usage and examples
