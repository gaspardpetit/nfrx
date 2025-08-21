```
x-env: &common_env
  API_KEY: "test123"     # client API key for /api
  CLIENT_KEY: "secret"   # client registration key

services:
  ollama:
    container_name: ollama
    image: ollama/ollama:latest
    volumes:
      - ollama_data:/root/.ollama

  server:
    container_name: server
    image: ghcr.io/gaspardpetit/llamapool-server:main
    environment:
      <<: *common_env
      PORT: "8080"
      METRICS_PORT: "9090"
    ports:
      - "8080:8080"   # OpenAI-compatible API + state
      - "9090:9090"   # Prometheus metrics

  worker:
    container_name: worker
    image: ghcr.io/gaspardpetit/llamapool-worker:main
    depends_on: [ollama]
    environment:
      <<: *common_env
      SERVER_URL: "ws://server:8080/api/workers/connect"
      COMPLETION_BASE_URL: "http://ollama:11434/v1"
      CLIENT_NAME: "Alpha"
      STATUS_ADDR: "0.0.0.0:4555"
    ports:
      - "4555:4555"
    command: ["--reconnect","--status-addr","0.0.0.0:4555"]

volumes:
  ollama_data:
```


```
docker compose up -d
```

```
docker exec ollama ollama pull gemma3n:e2b
```

```
docker run --rm -e OPENAI_API_KEY=test123 -e OPENAI_API_BASE=http://host.docker.internal:8080/api/v1/ ghcr.io/tbckr/sgpt:latest -m gemma3n:e2b "Tell me an IT joke about http proxies"
```

