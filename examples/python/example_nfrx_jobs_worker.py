#!/usr/bin/env python3
"""Async Jobs API worker example for nfrx.

This worker claims jobs, pulls payload bytes via transfer channel,
passes them to a delegate for processing, then posts result bytes.
"""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from dataclasses import dataclass
from typing import Any, Awaitable, Callable, Dict, Optional, Tuple, TypedDict

import httpx


@dataclass
class AuthConfig:
    client_key: Optional[str] = None


class JobClaim(TypedDict):
    job_id: str
    type: str
    metadata: Dict[str, Any]


ProcessHandler = Callable[[JobClaim, bytes], Awaitable[Tuple[bytes, Optional[str]]]]
StatusHandler = Callable[[str, Optional[Dict[str, Any]]], Awaitable[None]]


class NfrxJobsWorker:
    def __init__(self, base_url: str, auth: AuthConfig) -> None:
        self._base_url = base_url.rstrip("/")
        self._auth = auth
        self._client = httpx.AsyncClient(base_url=self._base_url, timeout=60.0)

    async def close(self) -> None:
        await self._client.aclose()

    def _headers(self) -> Dict[str, str]:
        headers: Dict[str, str] = {}
        if self._auth.client_key:
            headers["Authorization"] = f"Bearer {self._auth.client_key}"
        return headers

    async def claim_job(
        self, types: Optional[list[str]] = None, max_wait_seconds: Optional[int] = None
    ) -> Optional[JobClaim]:
        payload: Dict[str, Any] = {}
        if types:
            payload["types"] = types
        if max_wait_seconds is not None:
            payload["max_wait_seconds"] = max_wait_seconds
        resp = await self._client.post("/api/jobs/claim", json=payload, headers=self._headers())
        if resp.status_code == 204:
            return None
        resp.raise_for_status()
        return resp.json()

    async def request_payload_channel(self, job_id: str) -> Dict[str, Any]:
        resp = await self._client.post(f"/api/jobs/{job_id}/payload", headers=self._headers())
        resp.raise_for_status()
        return resp.json()

    async def request_result_channel(self, job_id: str) -> Dict[str, Any]:
        resp = await self._client.post(f"/api/jobs/{job_id}/result", headers=self._headers())
        resp.raise_for_status()
        return resp.json()

    async def update_status(self, job_id: str, state: str, payload: Optional[Dict[str, Any]] = None) -> None:
        body = {"state": state}
        if payload:
            body.update(payload)
        resp = await self._client.post(
            f"/api/jobs/{job_id}/status", json=body, headers=self._headers()
        )
        resp.raise_for_status()

    async def read_payload(self, url: str) -> bytes:
        resp = await self._client.get(url, headers=self._headers())
        resp.raise_for_status()
        return resp.content

    async def write_result(self, url: str, data: bytes) -> None:
        resp = await self._client.post(url, content=data, headers=self._headers())
        resp.raise_for_status()

    async def run_once(
        self,
        handler: ProcessHandler,
        types: Optional[list[str]],
        max_wait_seconds: int,
        on_status: Optional[StatusHandler] = None,
    ) -> bool:
        job = await self.claim_job(types=types, max_wait_seconds=max_wait_seconds)
        if not job:
            print("no job available")
            return False
        job_id = job.get("job_id") or job.get("id")
        print(f"claimed job: {job_id}")
        try:
            await self.update_status(job_id, "claimed")
            if on_status:
                await on_status("claimed", None)

            payload_channel = await self.request_payload_channel(job_id)
            payload_url = payload_channel.get("reader_url") or payload_channel.get("url")
            if on_status:
                await on_status("awaiting_payload", payload_channel)
            payload_bytes = await self.read_payload(payload_url)

            await self.update_status(job_id, "running")
            if on_status:
                await on_status("running", None)
            result_bytes, error_message = await handler(job, payload_bytes)

            if error_message:
                await self.update_status(
                    job_id,
                    "failed",
                    payload={"error": {"code": "handler_error", "message": error_message}},
                )
                if on_status:
                    await on_status("failed", {"error": {"code": "handler_error", "message": error_message}})
                return True

            result_channel = await self.request_result_channel(job_id)
            result_url = result_channel.get("writer_url") or result_channel.get("url")
            if on_status:
                await on_status("awaiting_result", result_channel)
            await self.write_result(result_url, result_bytes)

            await self.update_status(job_id, "completed")
            if on_status:
                await on_status("completed", None)
            return True
        except httpx.HTTPError as exc:
            await self.update_status(
                job_id,
                "failed",
                payload={"error": {"code": "http_error", "message": str(exc)}},
            )
            if on_status:
                await on_status("failed", {"error": {"code": "http_error", "message": str(exc)}})
            return True


async def echo_handler(job: JobClaim, payload: bytes) -> Tuple[bytes, Optional[str]]:
    _ = job
    return payload, None


async def run(args: argparse.Namespace) -> int:
    auth = AuthConfig(client_key=args.client_key)
    worker = NfrxJobsWorker(args.base_url, auth)
    try:
        types = args.types.split(",") if args.types else None
        while True:
            handled = await worker.run_once(
                handler=echo_handler,
                types=types,
                max_wait_seconds=args.max_wait_seconds,
            )
            if handled and args.once:
                return 0
    finally:
        await worker.close()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="nfrx jobs API worker example")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--client-key", default=None)
    parser.add_argument("--types", default=None, help="comma-separated job types")
    parser.add_argument("--max-wait-seconds", type=int, default=30)
    parser.add_argument("--once", action="store_true", help="exit if no job claimed")
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
