#!/usr/bin/env python3
"""Async Jobs API client example for nfrx.

Usage examples:
  python examples/python/example_nfrx_jobs_client.py --job-type asr.transcribe --metadata '{"filename":"sample.wav"}'
  python examples/python/example_nfrx_jobs_client.py --sse --timeout 10
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from dataclasses import dataclass
from typing import Any, AsyncGenerator, Awaitable, Callable, Dict, Optional, Tuple, TypedDict

import httpx
from httpx_sse import aconnect_sse

from nfrx_sdk.transfer_client import AuthConfig as TransferAuth, NfrxTransferClient

@dataclass
class AuthConfig:
    api_key: Optional[str] = None


class JobCreateResponse(TypedDict):
    job_id: str
    status: str


@dataclass
class JobSession:
    job_id: str
    payload_provider: PayloadProvider
    result_consumer: ResultConsumer


StatusHandler = Callable[[Dict[str, Any]], Awaitable[None]]
PayloadHandler = Callable[[Dict[str, Any]], Awaitable[None]]
ResultHandler = Callable[[Dict[str, Any]], Awaitable[None]]
PayloadData = Tuple[bytes, Optional[str]]
PayloadProvider = Callable[[str, Dict[str, Any]], Awaitable[Optional[PayloadData]]]
ResultConsumer = Callable[[str, bytes, Dict[str, Any]], Awaitable[None]]


class NfrxJobsClient:
    def __init__(self, base_url: str, auth: AuthConfig) -> None:
        self._base_url = base_url.rstrip("/")
        self._auth = auth
        self._client = httpx.AsyncClient(base_url=self._base_url, timeout=30.0)

    async def close(self) -> None:
        await self._client.aclose()

    def _headers(self) -> Dict[str, str]:
        headers: Dict[str, str] = {}
        if self._auth.api_key:
            headers["Authorization"] = f"Bearer {self._auth.api_key}"
        return headers

    async def create_job(self, job_type: str, metadata: Optional[Dict[str, Any]] = None) -> JobCreateResponse:
        payload = {"type": job_type, "metadata": metadata or {}}
        resp = await self._client.post("/api/jobs", json=payload, headers=self._headers())
        resp.raise_for_status()
        return resp.json()

    async def get_job(self, job_id: str) -> Dict[str, Any]:
        resp = await self._client.get(f"/api/jobs/{job_id}", headers=self._headers())
        resp.raise_for_status()
        return resp.json()

    async def cancel_job(self, job_id: str) -> Dict[str, Any]:
        resp = await self._client.post(f"/api/jobs/{job_id}/cancel", headers=self._headers())
        resp.raise_for_status()
        return resp.json()

    async def stream_events(self, job_id: str) -> AsyncGenerator[Tuple[str, Dict[str, Any]], None]:
        async with aconnect_sse(
            self._client,
            "GET",
            f"/api/jobs/{job_id}/events",
            headers=self._headers(),
        ) as event_source:
            event_source.response.raise_for_status()
            async for sse in event_source.aiter_sse():
                event_type = sse.event or "message"
                try:
                    payload = sse.json()
                except ValueError:
                    payload = {"raw": sse.data}
                yield event_type, payload

    async def run_with_handlers(
        self,
        job_id: str,
        on_status: Optional[StatusHandler] = None,
        on_payload: Optional[PayloadHandler] = None,
        on_result: Optional[ResultHandler] = None,
    ) -> None:
        async for event_type, payload in self.stream_events(job_id):
            if event_type == "status" and on_status:
                await on_status(payload)
            elif event_type == "payload" and on_payload:
                await on_payload(payload)
            elif event_type == "result" and on_result:
                await on_result(payload)
            if event_type == "status" and isinstance(payload, dict):
                status = payload.get("status")
                if status in {"completed", "failed", "canceled"}:
                    return


class NfrxJobsRunner:
    def __init__(self, base_url: str, api_key: Optional[str], timeout: float) -> None:
        self._jobs = NfrxJobsClient(base_url, AuthConfig(api_key=api_key))
        self._transfer = NfrxTransferClient(base_url, TransferAuth(bearer_token=api_key), timeout=timeout)

    async def close(self) -> None:
        await self._transfer.close()
        await self._jobs.close()

    async def create_job_session(
        self,
        job_type: str,
        metadata: Optional[Dict[str, Any]],
        payload_provider: PayloadProvider,
        result_consumer: ResultConsumer,
    ) -> JobSession:
        created = await self._jobs.create_job(job_type, metadata)
        return JobSession(
            job_id=created["job_id"],
            payload_provider=payload_provider,
            result_consumer=result_consumer,
        )

    async def run_session(
        self,
        session: JobSession,
        on_status: Optional[StatusHandler] = None,
        timeout: Optional[float] = None,
    ) -> None:
        await asyncio.wait_for(
            self._jobs.run_with_handlers(
                session.job_id,
                on_status=on_status,
                on_payload=lambda payload: _handle_payload(
                    self._transfer,
                    payload,
                    session.payload_provider,
                ),
                on_result=lambda payload: _handle_result(
                    self._transfer,
                    payload,
                    session.result_consumer,
                ),
            ),
            timeout=timeout,
        )

    async def get_job(self, job_id: str) -> Dict[str, Any]:
        return await self._jobs.get_job(job_id)


async def _print_event(prefix: str, payload: Dict[str, Any]) -> None:
    print(f"{prefix}: {json.dumps(payload)}")


def _read_file(path: Optional[str]) -> Optional[bytes]:
    if not path:
        return b"hello world"
    with open(path, "rb") as handle:
        return handle.read()


def _write_file(path: Optional[str], data: bytes) -> None:
    if not path:
        return
    with open(path, "wb") as handle:
        handle.write(data)


def _default_payload_provider(
    payload_file: Optional[str], content_type: Optional[str]
) -> PayloadProvider:
    async def provider(key: str, payload: Dict[str, Any]) -> Optional[PayloadData]:
        _ = key
        _ = payload
        data = _read_file(payload_file)
        if data is None:
            return None
        return data, content_type

    return provider


def _default_result_consumer(result_file: Optional[str]) -> ResultConsumer:
    async def consumer(key: str, data: bytes, payload: Dict[str, Any]) -> None:
        _ = key
        _ = payload
        if result_file:
            _write_file(result_file, data)
            print(f"result saved to {result_file}")
            return
        print("result received (no --result-file provided)")

    return consumer


async def _handle_payload(
    transfer: NfrxTransferClient,
    payload: Dict[str, Any],
    provider: PayloadProvider,
) -> None:
    key = payload.get("key", "payload")
    provided = await provider(key, payload)
    if provided is None:
        print("payload event: provider returned no data")
        return
    data, content_type = provided
    channel = payload.get("channel_id") or payload.get("url")
    if not channel:
        print("payload event: missing channel info")
        return
    await transfer.upload(channel, data, content_type)
    print("payload upload complete")


async def _handle_result(
    transfer: NfrxTransferClient,
    payload: Dict[str, Any],
    consumer: ResultConsumer,
) -> None:
    key = payload.get("key", "result")
    channel = payload.get("channel_id") or payload.get("url")
    if not channel:
        print("result event: missing channel info")
        return
    data = await transfer.download(channel)
    await consumer(key, data, payload)


async def run(args: argparse.Namespace) -> int:
    runner = NfrxJobsRunner(args.base_url, args.api_key, args.timeout)
    try:
        metadata = json.loads(args.metadata) if args.metadata else None
        payload_provider = _default_payload_provider(args.payload_file, args.payload_content_type)
        result_consumer = _default_result_consumer(args.result_file)
        session = await runner.create_job_session(
            args.job_type,
            metadata,
            payload_provider,
            result_consumer,
        )
        print(f"created job: {session.job_id}")

        if args.poll:
            job = await runner.get_job(session.job_id)
            print(json.dumps(job, indent=2))
            return 0

        if args.sse:
            try:
                await runner.run_session(
                    session,
                    on_status=lambda payload: _print_event("status", payload),
                    timeout=args.timeout,
                )
            except asyncio.TimeoutError:
                print("SSE stream timed out")
            return 0

        job = await runner.get_job(session.job_id)
        print(json.dumps(job, indent=2))
        return 0
    finally:
        await runner.close()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="nfrx jobs API client example")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--api-key", default=None)
    parser.add_argument("--job-type", default="asr.transcribe")
    parser.add_argument("--metadata", default=None, help="JSON string for metadata")
    parser.add_argument("--poll", action="store_true", help="poll job once after creation")
    parser.add_argument("--sse", action="store_true", help="stream job events (SSE)")
    parser.add_argument("--timeout", type=float, default=10.0, help="SSE timeout in seconds")
    parser.add_argument("--payload-file", default=None, help="file to upload on payload event")
    parser.add_argument("--payload-content-type", default=None, help="Content-Type for payload upload")
    parser.add_argument("--result-file", default=None, help="file to write on result event")
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        return asyncio.run(run(args))
    except httpx.HTTPError as exc:
        print(f"HTTP error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
