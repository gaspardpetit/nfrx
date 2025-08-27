# Relay Plugin Template

This package is a minimal scaffold for plugins that proxy requests to private
per-client services (similar to the MCP relay).

Steps to adapt:

1. Change `ID()` to a unique identifier.
2. Register HTTP routes and WebSocket endpoints in `RegisterRoutes`.
3. Add Prometheus collectors via `RegisterMetrics`.
4. Publish state elements in `RegisterState`.
