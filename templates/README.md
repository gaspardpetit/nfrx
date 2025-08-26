# Extension Templates

These packages provide skeleton implementations of the extension interfaces used
by nfrx.  Copy and adapt them when authoring new modules:

- `workerextension`: starting point for load-balanced worker providers.
- `relayextension`: starting point for per-client relay providers.

Each template compiles on its own and documents the methods that need to be
filled out.
