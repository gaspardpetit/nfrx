# Getting Started

This quick start runs one `llamapool-server` and a single `llamapool-worker`
serving a model from an OpenAI-compatible runtime.

## Prerequisites

- Go 1.23 or later
- An LLM runtime on the worker machine that supports the OpenAI
  chat/completions API (e.g.,
  [Ollama](https://github.com/ollama/ollama) or
  [vLLM](https://github.com/vllm-project/vllm))

## Build

```bash
git clone https://github.com/gaspardpetit/llamapool.git
cd llamapool
make build
```

## Run the server

```bash
API_KEY=test123 CLIENT_KEY=worker456 ./llamapool-server
```

## Run a worker

On the machine with the LLM runtime:

```bash
CLIENT_KEY=worker456 SERVER_URL=wss://your.server.example/api/ws ./llamapool-worker
```

The worker registers models available in the runtime and begins accepting requests.

## Test the API

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/completions \
  -H 'Authorization: Bearer test123' \
  -H 'Content-Type: application/json' \
  -d '{"model":"llama3","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

## Next steps

- Read more about the [server](server.md), [worker](worker.md), and [MCP relay](mcp.md)
- Explore sample configuration files in `examples/config/`
- See [env.md](env.md) for all configuration options
