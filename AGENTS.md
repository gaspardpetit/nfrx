# AGENTS.md

## Project Overview
infx provides a minimal Ollama-compatible server with a pool of workers that proxy
requests to local Ollama instances. The repository contains three binaries:
- `infx`: hosts the public HTTP API and coordinates workers over WebSocket
- `infx-llm`: connects to the server and forwards requests to a local Ollama
- `infx-mcp`: bridges private MCP providers to the public server

## Build & Commands
- Build all binaries: `make build`
- Run tests with race detector: `make test`
- Lint (requires golangci-lint): `make lint`

## Code Style
- Version of Go used is 1.23
- Use standard Go formatting via `gofmt -w` or `go fmt`
- Prefer clarity over cleverness; keep functions small and well named
- Default to the patterns already present in the `internal/` packages
- Use lowercase `infx` in documentation and text unless referring to binaries or package names

## Logging Policy
- Use structured logging via `internal/logx`.
- `Info` logs capture normal lifecycle events such as connections, disconnections, draining, and job dispatch/completion.
- `Warn` logs report expected failures (e.g., model not found, no worker available, worker busy, draining rejections).
- `Error` logs report unexpected failures that require investigation (e.g., socket errors, serialization failures).
- `Fatal` logs are reserved for unrecoverable errors that terminate the service.
- Classify failures by impact:
  - Business-case issues (e.g., invalid model requests) should log at **Warn**.
  - Unexpected failures outside normal flow (e.g., backend timeouts or authentication rejections) should log at **Error**.
  - Failures that will terminate or corrupt the service (e.g., OOM) should log at **Fatal**.

## Testing Guidelines
- Unit tests live alongside the code using `*_test.go` files
- End-to-end tests are in the `test/` directory
- Always run `make lint`, `make build`, and `make test` before submitting a change
- Fix any lint errors before completing your task
- If the Dockerfiles under `deploy/` are updated, ensure they still build:
  ```bash
  docker build -f deploy/Dockerfile.server .
  docker build -f deploy/Dockerfile.worker .
  ```

## Documentation
- Keep `doc/env.md` updated whenever environment variables, command line flags, or configuration file options change.
- Keep `doc/server-endpoints.md` current whenever HTTP or WebSocket endpoints change in any component.

## Further Reading
- @README.md for project usage and examples
