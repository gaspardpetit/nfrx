# Codebase Layers and Dependencies

This document describes the intended package layering and dependency rules in the repo. The goal is to keep extensions and agents thin, reusable, and decoupled from server internals while sharing utilities and contracts where it makes sense.

## Goals
- Keep SDK contracts (interfaces and DTOs) dependency‑free so server, extensions, and agents can interoperate.
- Centralize low‑level utilities in a small, import‑safe location.
- Provide light scaffolding for extensions without dragging server internals.
- Avoid circular or accidental imports from server internals into agents/extensions.

## Layers

- core/
  - Purpose: low‑level utilities for building apps (server, agents, and extensions).
  - Examples: logging, config/env helpers, retry/backoff, serialization, small HTTP helpers.
  - Dependencies: standard library only. Must not import sdk/ or server/.

- sdk/api/
  - Purpose: public, no‑dep contracts/interfaces and protocol types.
  - Examples: SPI plugin/router/metrics interfaces, control protocol (LLM agent), MCP protocol DTOs.
  - Dependencies: standard library only. Must not import core/ or server/.

- sdk/base/
  - Purpose: light, reusable helpers for extensions built on top of sdk/api.
  - Examples: descriptor validation, option binding helpers for PluginOptions, state‑view helpers.
  - Dependencies: may depend on core/ and sdk/api/. Must not import server/.

- modules/<type>/common
  - Purpose: type‑specific reuse that is heavier than sdk/base/ but not generic enough for core/.
  - Examples: OpenAI surface helpers for LLM, broker/bridge helpers for MCP that are shared between their extension and agent.
  - Dependencies: may depend on core/ and sdk/api/. Must not import server/.

- modules/<type>/ext
  - Purpose: concrete extension implementation registered with the server.
  - Examples: LLM extension (OpenAI surface), MCP extension (broker endpoints).
  - Dependencies: sdk/api/ (+ optionally sdk/base/ and modules/<type>/common). Must not import server/.

- modules/<type>/agent
  - Purpose: the agent binaries that connect to the server and perform work or relay.
  - Dependencies: core/, sdk/api/, modules/<type>/common. Must not import server/.

- server/internal/*
  - Purpose: server‑only packages (HTTP router, plugin loader, metrics registry wiring, dashboard, etc.).
  - Dependencies: may depend on core/ and sdk/api/. Must not be imported by modules/* or sdk/*.

## Import Direction (allowed)
- server → core, sdk/api, sdk/base, modules/<type>/common
- extensions → core, sdk/api, sdk/base, modules/<type>/common
- agents → core, sdk/api, modules/<type>/common
- sdk/api → stdlib only
- sdk/base → sdk/api, core
- core → stdlib only

## Notes
- Defaults and CLI/ENV/YAML binding for extensions are described via descriptors (sdk/api), with binding helpers living in sdk/base if needed.
- Worker‑style and tunnel‑style scaffolding should live under modules/<type>/common or sdk/base if they can stay server‑agnostic. Do not move server internal control‑plane code under core/.
- Keep server routes and registry under server/internal; agents and extensions should not import server.

