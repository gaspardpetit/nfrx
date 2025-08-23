# ADR 0002: MCP ingress bridge frame schema

## Context
The public `infero` will accept MCP client traffic over Streamable HTTP and relay it to `infero-mcp` via WebSocket. To keep the relay transport-agnostic and opaque, we need a well-defined frame format and correlation rules for JSON-RPC messages.

## Decision
Define a WebSocket frame schema used between the ingress and the downstream bridge:

```json
{
  "type": "request | response | notification | server_request | server_response | stream_event",
  "id": "<corrId>",
  "sessionId": "<mcp-session-id>",
  "payload": { "...opaque json-rpc..." },
  "meta": { "optional tracing or timestamps" }
}
```

* Every client JSON-RPC request is assigned an internal correlation ID (`corrId`).
* The mapper stores the original JSON-RPC `id` as raw bytes keyed by `corrId`.
* Downstream responses and streamed events use the same `corrId` in the WebSocket frame.
* When forwarding back to the client, the mapper restores the original JSON-RPC `id` and removes the mapping.

No attempt is made to parse or validate the JSON-RPC payloads; they remain opaque.

## Consequences
- Enables transparent, bidirectional forwarding between HTTP clients and the internal WebSocket bridge.
- Keeps future enhancements (logging, tracing, quotas) decoupled from JSON-RPC semantics.
- Simple ID mapper guarantees correct correlation while avoiding payload interpretation.

