# Plugin Templates

These packages provide skeleton implementations of the plugin interfaces used
by nfrx.  Copy and adapt them when authoring new modules:

- `workerplugin`: starting point for load-balanced worker providers.
- `relayplugin`: starting point for per-client relay providers.

Each template compiles on its own and documents the methods that need to be
filled out.
