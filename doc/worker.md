# llamapool-worker

## Overview
`llamapool-worker` connects to a `llamapool-server` and forwards requests to a
local LLM runtime that implements the
[OpenAI chat/completions API](https://platform.openai.com/docs/api-reference/chat/create),
such as [Ollama](https://github.com/ollama/ollama) or
[vLLM](https://github.com/vllm-project/vllm).

## Setup
Ensure the LLM runtime is running and the desired models are installed. Workers
communicate with the server over WebSocket and register the models they can
serve.

Start a worker:

```bash
CLIENT_KEY=worker456 SERVER_URL=wss://your.server.example/api/ws ./llamapool-worker
```

The default server URL is `ws://localhost:8080/api/ws`.

## Configuration
Workers can be configured via environment variables or a YAML file. Common
settings:

- `SERVER_URL` – WebSocket URL of the server
- `WORKER_NAME` – optional label shown in dashboards
- `DRAIN_TIMEOUT` – graceful shutdown delay

See [env.md](env.md) and `examples/config/worker.yaml` for all options.

Use a YAML file:

```bash
CONFIG_FILE=/path/to/worker.yaml ./llamapool-worker
```

## Lifecycle
Workers report status to the server and can be drained to finish in-flight
requests before exiting. The macOS and Windows tray apps under `desktop/`
provide simple GUIs for managing a local worker.

## Further reading
- [Server guide](server.md)
- [Getting Started](getting-started.md)
- [Architecture](architecture.md)
