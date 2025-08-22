[![Build](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/ci.yml)
[![Docker](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/docker-publish.yml)
[![.deb](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/release-deb.yml)
[![macOS Build](https://github.com/gaspardpetit/llamapool/actions/workflows/macos.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/macos.yml)
[![Windows Build](https://github.com/gaspardpetit/llamapool/actions/workflows/windows.yml/badge.svg)](https://github.com/gaspardpetit/llamapool/actions/workflows/windows.yml)

# llamapool

llamapool provides a minimal Ollama-compatible server with a pool of workers
that proxy requests to local LLM runtimes. It exposes an OpenAI-compatible
`chat/completions` API and can relay [Model Context Protocol](https://github.com/modelcontextprotocol)
requests.

## Table of Contents
- [Getting Started](#getting-started)
- [Components](#components)
- [Architecture](#architecture)
- [Testing](#testing)
- [Contributing](#contributing)
- [Deployment](#deployment)
- [Troubleshooting](#troubleshooting)
- [License](#license)
- [Credits](#credits)
- [Further Documentation](#further-documentation)

## Getting Started
See [doc/getting-started.md](doc/getting-started.md) to build the binaries,
launch a server, connect a worker, and issue your first request.

## Components
### llamapool-server
Hosts the public HTTP API and dispatches requests to workers or MCP relays.
[Read more](doc/server.md).

### llamapool-worker
Connects to the server and forwards requests to a local LLM runtime.
[Read more](doc/worker.md).

### llamapool-mcp
Relays calls to private MCP providers through the server.
[Read more](doc/mcp.md).

## Architecture
A high-level overview of how the pieces fit together is available in
[doc/architecture.md](doc/architecture.md).

## Testing
Run the standard checks before committing changes:

```bash
make lint
make build
make test
```

## Contributing
Guidelines for contributing, coding style, and logging policy are in
[doc/contributing.md](doc/contributing.md).

## Deployment
Dockerfiles live under `deploy/` and `.deb` packages are produced for Linux.
Example docker-compose setups are in `examples/`.

## Troubleshooting
Common configuration options are listed in [doc/env.md](doc/env.md). Endpoint
details and expected responses are documented in
[doc/server-endpoints.md](doc/server-endpoints.md). For MCP-specific issues see
[doc/mcp.md](doc/mcp.md).

## License
This project is licensed under the terms of the [MIT License](LICENSE).

## Credits
llamapool builds on several open source projects:

- [Ollama](https://github.com/ollama/ollama) – local LLM runtime whose API llamapool mirrors.
- [chi](https://github.com/go-chi/chi) – HTTP router that serves the REST API.
- [coder/websocket](https://github.com/coder/websocket) – WebSocket library used for worker communication.
- [zerolog](https://github.com/rs/zerolog) – structured logging across components.
- [Prometheus client_golang](https://github.com/prometheus/client_golang) – metrics collection and exposition.
- [mcp-go](https://github.com/mark3labs/mcp-go) – Model Context Protocol relay support.
- [kin-openapi](https://github.com/getkin/kin-openapi) and [oapi-codegen/runtime](https://github.com/oapi-codegen/runtime) – OpenAPI tooling for request and response validation.

## Further Documentation
Additional documents live under [doc/](doc) including the
[roadmap](doc/roadmap.md) and architectural decision records in `doc/adr/`.
