# Architecture

llamapool is composed of three cooperating binaries:

- **llamapool-server** – accepts OpenAI-compatible HTTP requests and routes them
  to workers or MCP relays over WebSocket.
- **llamapool-worker** – runs alongside an LLM runtime and proxies requests from
  the server to the runtime.
- **llamapool-mcp** – connects a private MCP provider to the server so clients
  can invoke MCP methods through the public API.

Workers and MCP relays establish outbound WebSocket connections to the server.
The server keeps a registry of available models and chooses the least-busy worker
that can satisfy each request. Responses flow back to the client unchanged.

The server exposes `/state` and `/metrics` endpoints for monitoring and can be
scaled horizontally behind a load balancer. Workers can join or leave at any
time; the server updates its model catalog in real time.

For additional details see the individual component guides:
[server](server.md), [worker](worker.md), and [MCP relay](mcp.md).
