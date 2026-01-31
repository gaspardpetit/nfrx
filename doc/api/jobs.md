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
  "metadata": {"filename": "sample.wav"}
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
  "payload": {
    "channel_id": "<uuid>",
    "method": "POST",
    "url": "/api/transfer/<uuid>",
    "expires_at": "2026-01-31T01:00:00Z"
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
- `payload` — transfer info for **client to write** payload (POST)
- `result` — transfer info for **client to read** result (GET)

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
data: {"channel_id":"...","method":"POST","url":"/api/transfer/...","expires_at":"..."}


event: result
data: {"channel_id":"...","method":"GET","url":"/api/transfer/...","expires_at":"..."}


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
  "max_wait_seconds": 30
}
```

- `types` filters which job types the worker will accept. Omit to claim any type.
- `max_wait_seconds` controls long-poll wait; `0` returns immediately.

Response (200):

```json
{
  "job_id": "<uuid>",
  "type": "asr.transcribe",
  "metadata": {"filename": "sample.wav"}
}
```

Response (204): no jobs available.

curl:

```bash
curl -s -X POST http://localhost:8080/api/jobs/claim \
  -H "Content-Type: application/json" \
  -d '{"types":["asr.transcribe"],"max_wait_seconds":30}'
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

Response:

```json
{
  "channel_id": "<uuid>",
  "reader_url": "/api/transfer/<uuid>",
  "expires_at": "2026-01-31T01:00:00Z"
}
```

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

Response:

```json
{
  "channel_id": "<uuid>",
  "writer_url": "/api/transfer/<uuid>",
  "expires_at": "2026-01-31T01:00:00Z"
}
```

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

Transfer channels are created by `/payload` and `/result` or directly via `/api/transfer`:

- **One‑time**: exactly one reader and one writer can attach.
- **Time‑limited**: channels expire if the other side does not connect in time.
- **Streaming**: payloads are streamed; no server persistence.
- **Methods**: client uses the URL and method in the `payload`/`result` event (`POST` for payload, `GET` for result).
## Notes

- Transfer channels are one‑time, time‑limited, and in‑memory only.
- Jobs are in‑memory; a server restart clears the queue.
- For clients without SSE, poll `GET /api/jobs/{job_id}` and look for `payload` / `result` fields.
