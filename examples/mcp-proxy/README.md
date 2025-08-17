# MCP Proxy Example

This example shows how to bridge a Python [fastmcp](https://pypi.org/project/fastmcp/) server through `llamapool-mcp` so that calls sent to `llamapool-server` are forwarded to a private MCP provider.

## 1. Start the FastMCP provider

```bash
python examples/mcp-proxy/time_server.py
```

The server exposes a single tool `time/now` that returns the current ISO timestamp over HTTP on `http://127.0.0.1:7777/mcp`.

## 2. Run the llamapool components

In separate terminals:

```bash
# Start the public server
env BROKER_ACCEPTED_CLIENTS=time-client BROKER_RELAY_TOKEN=secret API_KEY=test123 ./llamapool-server

# Connect the MCP relay
env BROKER_WS_URL=ws://localhost:8080/ws/relay \
    CLIENT_ID=time-client \
    PROVIDER_URL=http://127.0.0.1:7777/mcp \
    RELAY_AUTH_TOKEN=secret \
    ./llamapool-mcp
```

## 3. Invoke the tool via HTTP

```bash
curl -s -X POST http://localhost:8080/mcp/time-client \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"time/now","arguments":{}}}'
```

The response contains the current time string returned by the FastMCP server.
