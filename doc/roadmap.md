# Roadmap

## Current State

### Architecture
- **nfrx** – central HTTP API and coordination point. Accepts OpenAI-compatible requests, maintains registry of connected workers and MCP relays over WebSocket, and dispatches jobs using a least‑busy scheduler.
- **nfrx-llm** – runs near an LLM runtime (Ollama, vLLM, etc.). Workers register available models, forward generation/embedding jobs, track health, and expose status/metrics endpoints.
- **nfrx-mcp** – bridges Model Context Protocol providers to the public server via WebSocket, enforcing size limits and auth while relaying JSON‑RPC calls.

### Resilience
- Workers and MCP relays reconnect with bounded backoff.
- Server and workers support graceful draining on shutdown signals.
- Basic backend probing and status reporting keep the pool aware of worker health.

### Security
- API key auth for HTTP clients and a separate client key for workers/MCP relays.
- Connections occur over HTTPS/WSS; tokens are shared secrets.
- No role separation, rate limiting, or fine‑grained audit trails yet.

## Proposed Improvements

### 1. Technical Improvements
- **Increase unit/integration test coverage and enable CI race detection.**
  - Reach: high – all contributors.
  - Impact: high – reduces regressions and aids refactors.
  - Confidence: high.
  - Effort: medium.
- **Refactor configuration handling for hot‑reload and validation.**
  - Reach: medium – operators with dynamic environments.
  - Impact: medium – fewer restarts, fewer misconfigurations.
  - Confidence: medium.
  - Effort: medium.
- **Adopt context-based cancellation and timeouts across worker HTTP calls.**
  - Reach: high – all runtime requests.
  - Impact: medium – prevents resource leaks.
  - Confidence: medium.
  - Effort: low.

### 2. Ergonomic Improvements
- **Provide a `docker-compose` quick‑start including server, worker, and example backend.**
  - Reach: high – new users evaluating the project.
  - Impact: high – lowers barrier to entry.
  - Confidence: high.
  - Effort: low.
- **Expand CLI help and samples for MCP relay and advanced worker options.**
  - Reach: medium.
  - Impact: medium.
  - Confidence: high.
  - Effort: low.
- **Add web dashboard actions (drain, shutdown, config links).**
  - Reach: medium – administrators.
  - Impact: medium.
  - Confidence: medium.
  - Effort: medium.

### 3. Feature Improvements
- **RAG integration with pluggable vector stores.**
  - Reach: medium – users building retrieval‑augmented chat.
  - Impact: high – unlocks new use cases.
  - Confidence: low.
  - Effort: high.
- **Job prioritization and queueing for multi‑tenant fairness.**
  - Reach: medium.
  - Impact: high.
  - Confidence: medium.
  - Effort: high.
- **Support additional OpenAI endpoints (images, audio, tools).**
  - Reach: medium.
  - Impact: medium.
  - Confidence: medium.
  - Effort: medium.

### 4. Safety & Security
- **Replace shared secrets with OIDC/OAuth2 authentication and mTLS options.**
  - Reach: high – all deployments.
  - Impact: high – stronger auth story.
  - Confidence: medium.
  - Effort: high.
- **Introduce rate limiting and per‑API quotas.**
  - Reach: high.
  - Impact: medium – mitigates abuse.
  - Confidence: high.
  - Effort: medium.
- **Add content‑filter hooks before forwarding requests.**
  - Reach: medium – regulated environments.
  - Impact: medium.
  - Confidence: low.
  - Effort: medium.

### 5. Traceability
- **Integrate OpenTelemetry tracing and propagate request IDs to workers.**
  - Reach: high.
  - Impact: high – simplifies debugging and latency analysis.
  - Confidence: medium.
  - Effort: medium.
- **Centralize structured logs with log levels and JSON output.**
  - Reach: high.
  - Impact: medium.
  - Confidence: high.
  - Effort: low.
- **Expose detailed metrics for queue lengths, per‑client usage, and MCP call stats.**
  - Reach: medium.
  - Impact: medium.
  - Confidence: medium.
  - Effort: medium.

### 6. Scaling & Availability
- **Distribute worker registry via Redis/etcd for multi‑server deployments.**
  - Reach: high – enables horizontal scaling.
  - Impact: high.
  - Confidence: medium.
  - Effort: high.
- **Sharding workers by model or tenant with smart routing.**
  - Reach: medium.
  - Impact: medium.
  - Confidence: low.
  - Effort: high.
- **Graceful failover with heartbeat replication and client‑side retry helpers.**
  - Reach: medium.
  - Impact: medium.
  - Confidence: medium.
  - Effort: medium.

### 7. Additional Ideas
- **Plugin system for worker backends (e.g., REST, gRPC, custom runtimes).**
  - Reach: medium.
  - Impact: medium.
  - Confidence: low.
  - Effort: high.
- **Packaging: provide Homebrew/Scoop formulas and container images with minimal base.**
  - Reach: high.
  - Impact: low.
  - Confidence: high.
  - Effort: low.

## Recommendations & Next Steps
1. Ship a `docker-compose` quick start and improved samples to grow adoption.
2. Implement OpenTelemetry traces and better metrics for easier debugging.
3. Plan distributed registry (Redis/etcd) to enable multi‑server scaling.
4. Explore stronger authentication (OIDC) and rate limiting for production use.
5. Evaluate RAG integration once core stability and observability improve.
