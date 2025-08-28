# Worker Plugin Template

This package provides a starting point for plugins that expose load-balanced
workers.  Copy the directory and implement your own logic:

1. Update `ID()` with a unique identifier.
2. Register any HTTP routes and WebSocket endpoints in `RegisterRoutes`.
3. Add Prometheus collectors via `RegisterMetrics`.
4. Publish state elements in `RegisterState`.
5. Provide a scheduler that dispatches tasks in `Scheduler`.

The server will automatically detect the `WorkerProvider` capability when the
plugin is loaded.
