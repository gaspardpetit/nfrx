# ADR 0001: Transport-agnostic MCP client connector

## Context
infero-mcp must connect to arbitrary third-party Model Context Protocol (MCP) servers using different transports. Compatibility and sane defaults are required without forcing legacy behaviours.

## Decision
We introduce a `mcpclient` package that defines a transport-agnostic `Connector` interface and an `Orchestrator` that attempts connection using multiple transports in order:

1. **stdio** – spawns a local MCP server process.
2. **streamable HTTP** – POST requests that return JSON or SSE.
3. **OAuth HTTP** – same as HTTP but obtains tokens when required.
4. **legacy SSE** – optional, guarded by a feature flag and disabled by default.

Each transport is wrapped by a common connector implementing `Start`, `Initialize`, `DoRPC`, and `Close`. The orchestrator tries transports sequentially with escalating timeouts and stops on the first successful initialization.

Configuration specifies transport order, timeouts, auth settings, and feature gates. Legacy SSE remains opt-in to keep the default path modern.

## Consequences
- infero-mcp gains fallback behaviour and can negotiate protocol versions and capabilities.
- New transports can be added by implementing the `Connector` interface and registering a factory with the orchestrator.
- Legacy transports do not affect default behaviour unless explicitly enabled.
