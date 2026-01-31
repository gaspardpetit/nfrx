# Server Endpoints

Endpoints are grouped by functional area.

## System & Documentation

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /healthz` | – | Basic health check. | Public |
| `GET /metrics` | – | Prometheus metrics (`METRICS_PORT` env or `--metrics-port` flag for separate address). | Public |
| `GET /api/client/openapi.json` | – | OpenAPI schema. | Public |
| `GET /api/client/*` | – | Swagger UI. | Public |

## State API

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /state` | – | HTML status dashboard (prompts for API key). | API key |
| `GET /api/state` | – | Server state snapshot (JSON envelope). | API key |
| `GET /api/state/stream` | – | Server state stream (SSE of JSON envelope). | API key |
| `GET /api/state/view/{id}.html` | Path `{id}` | Returns plugin-provided HTML fragment for state view. | API key |

JSON envelope shape:

```
{
  "plugins": {
    "llm": { ... plugin-defined state ... },
    "mcp": { ... plugin-defined state ... },
    ...
  }
}
```

- Each plugin contributes its own state under its ID. The server does not enforce a schema for plugin state.
- The dashboard loads plugin-specific HTML fragments via `/api/state/view/{id}.html` and renders using the streamed envelope. If a plugin provides no view, it will not have a dedicated section in the dashboard.

Notes:
- The LLM plugin’s state includes server status (`ready`, `not_ready`, `draining`), workers, models, and aggregates. Other plugins (e.g., MCP) expose their own structures.

## Inference API

These endpoints are present when the `llm` plugin is enabled.

### Worker Registration

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/llm/connect` (WS) | Initial message `{ type: "register", client_key?: string, worker_id?: string, worker_name?: string, models?: [string], max_concurrency?: int, embedding_batch_size?: int }` | Worker connects to server. | Client key |

### Client Usage

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `POST /api/llm/v1/chat/completions` | Body `{ model: string, messages: [{role: string, content: string}], stream?: bool, ... }` | Proxy OpenAI chat completions. | API key |
| `POST /api/llm/v1/embeddings` | Body `{ model: string, input: any, ... }` | Proxy OpenAI embeddings; large input arrays are automatically batched per worker. | API key |
| `GET /api/llm/v1/models` | – | List models. | API key |
| `GET /api/llm/v1/models/{id}` | Path `{id}` | Get model details. | API key |

## Audio Transcription API

These endpoints are present when the `asr` plugin is enabled.

### Worker Registration

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/asr/connect` (WS) | Initial message `{ type: "register", client_key?: string, worker_id?: string, worker_name?: string, models?: [string], max_concurrency?: int }` | Worker connects to server. | Client key |

### Client Usage

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/asr/v1/models` | – | List models. | API key |
| `GET /api/asr/v1/models/{id}` | Path `{id}` | Get model details. | API key |
| `POST /api/asr/v1/audio/transcriptions` | Multipart form data: `file`, `model`, optional fields; `stream=true` for SSE | Proxy audio transcription requests. | API key |

## MCP API

These endpoints are present when the `mcp` plugin is enabled.

### MCP Registration

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/mcp/connect` (WS) | Initial message `{ id?: string, client_name?: string, client_key?: string }` | MCP relay connects and receives an id. | Client key |

### Client (LLM) Usage

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `POST /api/mcp/id/{id}` | Path `{id}`; Body `{ jsonrpc: "2.0", id: number\|string, method: string, params?: object }` | Forward MCP JSON-RPC request to relay. | MCP token (optional) |

### Authentication schemes
- **Public** – No authentication required.
- **API key** – `Authorization: Bearer <API_KEY>`.
- **Client key** – WebSocket `register` message must include `client_key` matching server configuration. Providing a key when the server is configured without one results in an immediate failure.
- **MCP token** – Optional `Authorization: Bearer <AUTH_TOKEN>` forwarded to the MCP relay. The server neither validates nor requires this header; if the relay is configured with a token it will reject missing or invalid tokens. Future improvements may allow the relay to signal this requirement so the server can reject unauthenticated requests early.

## Transfer API

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `POST /api/transfer` | – | Create a new transfer channel; returns `channel_id` and `expires_at`. | API key or client key or roles |
| `GET /api/transfer/{channel_id}` | Path `{channel_id}` | Initiate downstream transfer (reader). | API key or client key or roles |
| `POST /api/transfer/{channel_id}` | Path `{channel_id}` | Initiate upstream transfer (writer). | API key or client key or roles |

Notes:
- Channels are in-memory and time-limited; they expire if the other side does not connect before the TTL.
- Only one reader and one writer can attach to a channel; reuse returns `409`.
