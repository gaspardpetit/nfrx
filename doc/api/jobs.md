# Jobs API

This document describes the Jobs API for submitting work, claiming it from workers, and transferring payloads/results using transfer channels. The API is intentionally small and language-agnostic so you can implement clients and workers in any stack.

## Overview

- **Client** creates a job and listens for updates (SSE or polling).
- **Worker** claims jobs, requests payload/result transfer channels, and posts status updates.
- **Payloads/results** are streamed via `/api/transfer/{channel_id}` (no server persistence).
- Jobs are stored **in memory** (not durable).

## Authentication

Jobs endpoints are split into **client** and **worker** operations:

Client operations (`/api/jobs`, `/api/jobs/{id}`, `/events`, `/cancel`) authorize with:
- `Authorization: Bearer <API_KEY>`
- `X-User-Roles` containing any configured API role

Worker operations (`/api/jobs/claim`, `/payload`, `/result`, `/status`) authorize with:
- `Authorization: Bearer <CLIENT_KEY>`
- `X-User-Roles` containing any configured client role

If no auth is configured, the endpoints are open.

## Job lifecycle (typical)

1) Client creates a job → status `queued`.
2) Worker claims the job → status `claimed`.
3) Worker requests payload channel → status `awaiting_payload`.
4) Client POSTs payload to transfer channel.
5) Worker processes job → status `running` (optional progress).
6) Worker requests result channel → status `awaiting_result`.
7) Worker POSTs result to transfer channel; client GETs it.
8) Worker marks job `completed` (or `failed`).

## Endpoints

### Create job (client)

`POST /api/jobs`

Request body:

```json
{
  "type": "asr.transcribe",
  "metadata": {"filename": "sample.wav"},
  "worker_group": "asr-cache-a"
}
```

Response:

```json
{
  "job_id": "<uuid>",
  "status": "queued"
}
```

curl:

```bash
curl -s -X POST http://localhost:8080/api/jobs \
  -H "Content-Type: application/json" \
  -d '{"type":"asr.transcribe","metadata":{"filename":"sample.wav"}}'
```

Authorization (client):

```
Authorization: Bearer <API_KEY>
```

or

```
X-User-Roles: <api_role>
```

---

### Poll job status (client)

`GET /api/jobs/{job_id}`

Response (example):

```json
{
  "id": "<uuid>",
  "type": "asr.transcribe",
  "status": "awaiting_payload",
  "metadata": {"filename": "sample.wav"},
  "worker_group": "asr-cache-a",
  "claimed_worker_id": "worker-17",
  "claimed_worker_group": "asr-cache-a",
  "payloads": {
    "payload": {
      "key": "payload",
      "channel_id": "<uuid>",
      "method": "POST",
      "url": "/api/transfer/<uuid>",
      "expires_at": "2026-01-31T01:00:00Z"
    }
  },
  "queue_position": 1,
  "created_at": "2026-01-31T01:00:00Z",
  "updated_at": "2026-01-31T01:00:10Z"
}
```

curl:

```bash
curl http://localhost:8080/api/jobs/<job_id>
```

Authorization (client):

```
Authorization: Bearer <API_KEY>
```

or

```
X-User-Roles: <api_role>
```

---

### Stream job events (client SSE)

`GET /api/jobs/{job_id}/events`

SSE event types:

- `status` — full job snapshot (JobView), including `progress` and `error` when present
- `payload` — transfer info for **client to write** payload (POST); includes optional `key`
- `result` — transfer info for **client to read** result (GET); includes optional `key`

SSE format notes:

- Each event is separated by a blank line.
- `event: <type>` is optional; when present it labels the event type.
- `data: <json>` contains a single JSON object.
- The stream is plain UTF-8 text; there is no binary payload in SSE.

Example SSE stream:

```
event: status
data: {"id":"...","status":"queued",...}


event: payload
data: {"key":"payload","channel_id":"...","method":"POST","url":"/api/transfer/...","expires_at":"...","properties":{"protocol":"demo-v1"}}


event: result
data: {"key":"result","channel_id":"...","method":"GET","url":"/api/transfer/...","expires_at":"...","properties":{"protocol":"demo-v1"}}


event: status
data: {"id":"...","status":"completed",...}
```

curl:

```bash
curl -N http://localhost:8080/api/jobs/<job_id>/events
```

Authorization (client):

```
Authorization: Bearer <API_KEY>
```

or

```
X-User-Roles: <api_role>
```

---

### Cancel job (client)

`POST /api/jobs/{job_id}/cancel`

Response:

```json
{"status":"canceled"}
```

If the job is already terminal, cancel remains a successful no-op and returns the
current terminal status (`completed`, `failed`, or `canceled`).

curl:

```bash
curl -X POST http://localhost:8080/api/jobs/<job_id>/cancel
```

Authorization (client):

```
Authorization: Bearer <API_KEY>
```

or

```
X-User-Roles: <api_role>
```

---

### Claim a job (worker)

`POST /api/jobs/claim`

Request body (optional):

```json
{
  "types": ["asr.transcribe", "doc.convert"],
  "max_wait_seconds": 30,
  "worker_id": "worker-17",
  "worker_group": "asr-cache-a"
}
```

- `types` filters which job types the worker will accept. Omit to claim any type.
- `max_wait_seconds` controls long-poll wait; `0` returns immediately.
- `worker_id` identifies the claiming worker for traceability and exact-targeted jobs.
- `worker_group` identifies the affinity pool for group-targeted jobs.

Response (200):

```json
{
  "job_id": "<uuid>",
  "type": "asr.transcribe",
  "metadata": {"filename": "sample.wav"},
  "worker_group": "asr-cache-a",
  "claimed_worker_id": "worker-17",
  "claimed_worker_group": "asr-cache-a"
}
```

Response (204): no jobs available.

curl:

```bash
curl -s -X POST http://localhost:8080/api/jobs/claim \
  -H "Content-Type: application/json" \
  -d '{"types":["asr.transcribe"],"max_wait_seconds":30,"worker_id":"worker-17","worker_group":"asr-cache-a"}'
```

Authorization (worker):

```
Authorization: Bearer <CLIENT_KEY>
```

or

```
X-User-Roles: <client_role>
```

---

### Stream compatible jobs (worker SSE)

`GET /api/jobs/stream`

Query parameters:

- `types` optional comma-separated job types
- `worker_id` optional claiming worker identity
- `worker_group` optional claiming worker affinity group

The server applies the same compatibility rules as `POST /api/jobs/claim`. Each emitted `job` event claims the job immediately for that worker identity.

Example:

```text
event: job
data: {"job_id":"...","type":"asr.transcribe","worker_group":"asr-cache-a","claimed_worker_id":"worker-17","claimed_worker_group":"asr-cache-a"}
```

curl:

```bash
curl -N "http://localhost:8080/api/jobs/stream?types=asr.transcribe&worker_id=worker-17&worker_group=asr-cache-a"
```

Authorization (worker):

```
Authorization: Bearer <CLIENT_KEY>
```

or

```
X-User-Roles: <client_role>
```

---

### Request payload transfer (worker)

`POST /api/jobs/{job_id}/payload`

Request body (optional):

```json
{
  "key": "payload",
  "properties": {
    "protocol": "demo-v1",
    "options": {
      "mode": "header-body",
      "note": "opaque to nfrx"
    }
  }
}
```

Response:

```json
{
  "key": "payload",
  "channel_id": "<uuid>",
  "reader_url": "/api/transfer/<uuid>",
  "expires_at": "2026-01-31T01:00:00Z",
  "properties": {
    "protocol": "demo-v1",
    "options": {
      "mode": "header-body",
      "note": "opaque to nfrx"
    }
  }
}
```

- `key` is optional; if omitted the server defaults to `"payload"`. Explicit empty is allowed.
- `properties` is optional opaque worker-client metadata; nfrx stores and relays it without interpretation.
- Worker should **GET** `reader_url` to receive payload.
- Client receives a `payload` event with a POST URL to send bytes.

curl (worker opens reader):

```bash
curl -v http://localhost:8080/api/transfer/<channel_id> -o /tmp/payload.bin
```

Authorization (worker):

```
Authorization: Bearer <CLIENT_KEY>
```

or

```
X-User-Roles: <client_role>
```

---

### Update job status (worker)

`POST /api/jobs/{job_id}/status`

Progress update:

```json
{
  "state": "running",
  "progress": {"percent": 42, "message": "decoding"}
}
```

Fail:

```json
{
  "state": "failed",
  "error": {"code": "upstream_error", "message": "timeout"}
}
```

Complete:

```json
{ "state": "completed" }
```

curl:

```bash
curl -X POST http://localhost:8080/api/jobs/<job_id>/status \
  -H "Content-Type: application/json" \
  -d '{"state":"running","progress":{"percent":42}}'
```

Authorization (worker):

```
Authorization: Bearer <CLIENT_KEY>
```

or

```
X-User-Roles: <client_role>
```

---

### Request result transfer (worker)

`POST /api/jobs/{job_id}/result`

Request body (optional):

```json
{
  "key": "result",
  "properties": {
    "protocol": "demo-v1",
    "options": {
      "mode": "header-body",
      "note": "opaque to nfrx"
    }
  }
}
```

Response:

```json
{
  "key": "result",
  "channel_id": "<uuid>",
  "writer_url": "/api/transfer/<uuid>",
  "expires_at": "2026-01-31T01:00:00Z",
  "properties": {
    "protocol": "demo-v1",
    "options": {
      "mode": "header-body",
      "note": "opaque to nfrx"
    }
  }
}
```

- `key` is optional; if omitted the server defaults to `"result"`. Explicit empty is allowed.
- `properties` is optional opaque worker-client metadata; nfrx stores and relays it without interpretation.
- Worker should **POST** bytes to `writer_url`.
- Client receives a `result` event with a GET URL to read bytes.

curl (worker writes result):

```bash
curl -v -X POST http://localhost:8080/api/transfer/<channel_id> --data-binary @/tmp/result.bin
```

Authorization (worker):

```
Authorization: Bearer <CLIENT_KEY>
```

or

```
X-User-Roles: <client_role>
```

---

## Status values

Workers should use these job states when calling `/status`:

- `queued` — created, waiting to be claimed
- `claimed` — claimed by a worker
- `awaiting_payload` — worker requested payload channel
- `running` — job is being processed (progress updates may be sent)
- `awaiting_result` — worker requested result channel
- `completed` — job completed successfully
- `failed` — job failed (include `error`)
- `canceled` — job canceled (include `error` optionally)

Clients should expect the server to emit `status` events with these values.

## Transfer channel behavior

Transfer channels are created by `/payload` and `/result` (optional `key`) or directly via `/api/transfer`:

- **One‑time**: exactly one reader and one writer can attach.
- **Time‑limited**: channels expire if the other side does not connect in time.
- **Streaming**: payloads are streamed; no server persistence.
- **Methods**: client uses the URL and method in the `payload`/`result` event (`POST` for payload, `GET` for result).
- **Properties**: jobs-side `/payload` and `/result` requests may attach opaque `properties`, which are relayed via job events and views but are not part of direct `/api/transfer` channels.
## Notes

- Direct `/api/transfer` channels do not carry jobs-side `properties`; those are available only through `/payload`, `/result`, and job events/views.

- Transfer channels are one‑time, time‑limited, and in‑memory only.
- Jobs are in‑memory; a server restart clears the queue.
- Jobs may optionally target a `worker_id`, a `worker_group`, or both. A worker claim is compatible only when it satisfies all requested affinity fields.
- Claimed jobs record `claimed_worker_id` and `claimed_worker_group` for traceability.
- For clients without SSE, poll `GET /api/jobs/{job_id}` and look for `payloads` / `results` fields.
- When no SSE client is connected, jobs are canceled after `JOBS_CLIENT_TTL` (default `30s`).

## Python client usage (minimal)

To use the Python client in your own project, copy these example files into your codebase:

- `examples/python/example_nfrx_jobs_client.py`
- `examples/python/nfrx_sdk/transfer_client.py`

Install dependencies:

```bash
pip install httpx httpx-sse
```

For the secure CMS/X.509 reference examples, also install:

```bash
pip install cryptography
```

Secure example reference implementations:

- `examples/python/example_nfrx_jobs_client_secure.py`
- `examples/python/example_nfrx_jobs_worker_secure.py`
- `examples/dotnet/example-nfrx-jobs-client-secure/Program.cs`
- `examples/dotnet/example-nfrx-jobs-worker-secure/Program.cs`

These examples demonstrate one possible client-worker protocol built on top of opaque job `metadata` and transfer `properties`. They are not built-in `nfrx` protocol features.

Minimal usage with provider/consumer delegates:

```python
import asyncio
from typing import Dict, Any, Optional

from example_nfrx_jobs_client import NfrxJobsRunner


async def payload_provider(key: str, payload: Dict[str, Any]) -> Optional[tuple[bytes, str | None]]:
    _ = key, payload
    return b"hello world", "application/octet-stream"


async def result_consumer(key: str, data: bytes, payload: Dict[str, Any]) -> None:
    _ = key, payload
    print("result:", data.decode("utf-8", errors="replace"))


async def on_status(status: Dict[str, Any]) -> None:
    print("status:", status.get("status"))


async def main() -> None:
    runner = NfrxJobsRunner("http://localhost:8080", api_key=None, timeout=30)
    try:
        session = await runner.create_job_session(
            job_type="asr.transcribe",
            metadata={"language": "en"},
            worker_id=None,
            worker_group=None,
            payload_provider=payload_provider,
            result_consumer=result_consumer,
        )
        await runner.run_session(session, on_status=on_status, timeout=30)
    finally:
        await runner.close()


asyncio.run(main())
```
