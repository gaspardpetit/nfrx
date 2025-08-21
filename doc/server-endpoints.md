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
| `GET /api/state` | – | Server state snapshot (JSON). | API key |
| `GET /api/state/stream` | – | Server state stream (SSE). | API key |

## Inference API

### Worker Registration

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/workers/connect` (WS) | Initial message `{ type: "register", client_key?: string, worker_id?: string, worker_name?: string, models?: [string], max_concurrency?: int }` | Worker connects to server. | Client key |

### Client Usage

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `POST /api/v1/chat/completions` | Body `{ model: string, messages: [{role: string, content: string}], stream?: bool, ... }` | Proxy OpenAI chat completions. | API key |
| `POST /api/v1/embeddings` | Body `{ model: string, input: any, ... }` | Proxy OpenAI embeddings. | API key |
| `GET /api/v1/models` | – | List models. | API key |
| `GET /api/v1/models/{id}` | Path `{id}` | Get model details. | API key |

## MCP API

### MCP Registration

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `GET /api/mcp/connect` (WS) | Initial message `{ id?: string, client_key?: string }` | MCP relay connects and receives an id. | Client key |

### Client (LLM) Usage

| Verb & Endpoint | Parameters | Description | Auth |
| --- | --- | --- | --- |
| `POST /api/mcp/id/{id}` | Path `{id}`; Body `{ jsonrpc: "2.0", id: number\|string, method: string, params?: object }` | Forward MCP JSON-RPC request to relay. | MCP token (optional) |

### Authentication schemes
- **Public** – No authentication required.
- **API key** – `Authorization: Bearer <API_KEY>`.
- **Client key** – WebSocket `register` message must include `client_key` matching server configuration. Providing a key when the server is configured without one results in an immediate failure.
- **MCP token** – Optional `Authorization: Bearer <AUTH_TOKEN>` forwarded to the MCP relay. The server neither validates nor requires this header; if the relay is configured with a token it will reject missing or invalid tokens. Future improvements may allow the relay to signal this requirement so the server can reject unauthenticated requests early.

