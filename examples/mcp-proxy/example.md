# MCP Proxy Example

This example shows how to bridge a Python [fastmcp](https://pypi.org/project/fastmcp/) server through `llamapool-mcp` so that calls sent to `llamapool-server` are forwarded to a private MCP provider.


## 1. Docker compose

Define the following `docker-compose.yaml`:

```
x-env: &common_env
  API_KEY: "test123"     # client API key for /v1 and /api
  WORKER_KEY: "secret"   # worker registration key
  CLIENT_ID: mcp-1234    # a unique identifier for this mcp server
  
services:
  clock:
    image: python:3.12-slim
    container_name: clock
    command: |
      sh -c "pip install --no-cache-dir fastmcp &&
             echo '
      from datetime import datetime
      from fastmcp import FastMCP

      app = FastMCP(\"clock\", stateless_http=True, json_response=True)

      @app.tool(\"time/now\")
      def now() -> str:
          return datetime.now().isoformat()

      if __name__ == \"__main__\":
          app.run(\"http\", host=\"0.0.0.0\", port=7777)
      ' > /app/app.py &&
                  python /app/app.py"
    working_dir: /app
    ports:
      - "7777:7777"
    restart: unless-stopped
  server:
    container_name: server
    image: ghcr.io/gaspardpetit/llamapool-server:main
    environment:
      <<: *common_env
      PORT: "8080"
      METRICS_PORT: "9090"
    ports:
      - "8080:8080"   # MCP public endpoint
      - "9090:9090"   # Prometheus metrics

  worker:
    container_name: mcp
    image: ghcr.io/gaspardpetit/llamapool-mcp-worker:main
    environment:
      <<: *common_env
      SERVER_URL: "ws://server:8080/api/mcp/connect"
      WORKER_NAME: "Alpha"
      PROVIDER_URL: http://clock:7777/mcp/
    ports:
      - "4555:4555"
    command: [--reconnect]
```

Run the following command

```
docker compose up
```

## 2. Confirm status on server

```
curl http://localhost:8080/api/state \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer test123' | jq
```

```
{
  "server": {
    "now": "2025-08-21T02:08:19.044622409Z",
    "version": "3572acb",
    "build_sha": "3572acb",
    "build_date": "2025-08-21T01:34:31Z",
    "uptime_s": 5,
    "jobs_inflight_total": 0,
    "jobs_completed_total": 0,
    "jobs_failed_total": 0,
    "scheduler_queue_len": 0
  },
  "workers_summary": {
    "connected": 0,
    "working": 0,
    "idle": 0,
    "not_ready": 0,
    "gone": 0
  },
  "models": [],
  "workers": [],
  "mcp": {
    "clients": [
      {
        "id": "mcp-1234",
        "status": "idle",
        "inflight": 0,
        "functions": {}
      }
    ],
    "sessions": null
  }
}
```

And notice that the client is registered (`mcp-1234` in this example).

## 3. Run MCP commands across the llamapool-server 

Regular MCP commands should now be available under `http://localhost:8080/api/mcp/id/<id>`

**List MCP Tools**

```
curl -s -X POST http://localhost:8080/api/mcp/id/mcp-1234 \
   -H 'Content-Type: application/json' \
   -d '{
        "jsonrpc": "2.0",
        "id": 2,
        "method": "tools/list",
        "params": {}
      }' | jq
```

```
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "time/now",
        "inputSchema": {
          "properties": {},
          "type": "object"
        },
        "outputSchema": {
          "properties": {
            "result": {
              "title": "Result",
              "type": "string"
            }
          },
          "required": [
            "result"
          ],
          "title": "_WrappedResult",
          "type": "object",
          "x-fastmcp-wrap-result": true
        },
        "_meta": {
          "_fastmcp": {
            "tags": []
          }
        }
      }
    ]
  }
}
```


**Call MCP Tools**

```
curl -s -X POST http://localhost:8080/api/mcp/id/mcp-1234 \
   -H 'Content-Type: application/json' \
   -d '{
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": { "name": "time/now", "arguments": {} }
      }' | jq
```

```
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "2025-08-21T02:13:53.798406"
      }
    ],
    "structuredContent": {
      "result": "2025-08-21T02:13:53.798406"
    },
    "isError": false
  }
}
```
