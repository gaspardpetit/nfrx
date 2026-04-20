#!/usr/bin/env python3
"""Secure Jobs API client example for nfrx using CMS/X.509."""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, AsyncGenerator, Dict, Optional, Tuple

import httpx
from httpx_sse import aconnect_sse

from nfrx_sdk.secure_transfer import (
    CMS_CONTENT_TYPE,
    ENVELOPE,
    SCHEME,
    build_header_body_envelope,
    decrypt_cms_der,
    encrypt_cms_der,
    generate_self_signed_identity,
    parse_header_body_envelope,
)
from nfrx_sdk.transfer_client import AuthConfig as TransferAuth, NfrxTransferClient


@dataclass
class AuthConfig:
    api_key: Optional[str] = None


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

    async def create_job(
        self,
        job_type: str,
        metadata: Dict[str, Any],
        worker_id: Optional[str] = None,
        worker_group: Optional[str] = None,
    ) -> Dict[str, Any]:
        payload: Dict[str, Any] = {"type": job_type, "metadata": metadata}
        if worker_id:
            payload["worker_id"] = worker_id
        if worker_group:
            payload["worker_group"] = worker_group
        resp = await self._client.post("/api/jobs", json=payload, headers=self._headers())
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
                payload = sse.json()
                yield event_type, payload


def _read_payload(path: Optional[str]) -> bytes:
    if not path:
        return b"hello world"
    return Path(path).read_bytes()


def _write_result(path: Optional[str], data: bytes) -> None:
    if not path:
        sys.stdout.buffer.write(data)
        sys.stdout.buffer.write(b"\n")
        return
    Path(path).write_bytes(data)


def _validate_payload_properties(payload: Dict[str, Any]) -> str:
    properties = payload.get("properties")
    if not isinstance(properties, dict):
        raise ValueError("payload event missing properties")
    if properties.get("encryption_scheme") != SCHEME:
        raise ValueError(f"unsupported payload encryption scheme: {properties.get('encryption_scheme')!r}")
    if properties.get("envelope") != ENVELOPE:
        raise ValueError(f"unsupported payload envelope: {properties.get('envelope')!r}")
    recipient_cert_pem = properties.get("recipient_cert_pem")
    if not isinstance(recipient_cert_pem, str) or not recipient_cert_pem.strip():
        raise ValueError("payload event missing recipient_cert_pem")
    return recipient_cert_pem


def _validate_result_properties(payload: Dict[str, Any]) -> None:
    properties = payload.get("properties")
    if not isinstance(properties, dict):
        raise ValueError("result event missing properties")
    if properties.get("encryption_scheme") != SCHEME:
        raise ValueError(f"unsupported result encryption scheme: {properties.get('encryption_scheme')!r}")
    if properties.get("envelope") != ENVELOPE:
        raise ValueError(f"unsupported result envelope: {properties.get('envelope')!r}")
    if properties.get("recipient") != "job-metadata-result-cert":
        raise ValueError(f"unexpected result recipient marker: {properties.get('recipient')!r}")


async def run(args: argparse.Namespace) -> int:
    identity = generate_self_signed_identity("nfrx secure client")
    print("generated client certificate (PEM):")
    print(identity.certificate_pem.strip())

    transfer = NfrxTransferClient(args.base_url, TransferAuth(bearer_token=args.api_key), timeout=args.timeout)
    jobs = NfrxJobsClient(args.base_url, AuthConfig(api_key=args.api_key))
    try:
        metadata = json.loads(args.metadata) if args.metadata else {}
        metadata["secure_transfer"] = {
            "supported_schemes": [SCHEME],
            "result_recipient_cert_pem": identity.certificate_pem,
            "envelope": ENVELOPE,
        }
        created = await jobs.create_job(args.job_type, metadata, args.worker_id, args.worker_group)
        job_id = created["job_id"]
        print(f"created secure job: {job_id}")

        async for event_type, payload in jobs.stream_events(job_id):
            if event_type == "status":
                print("status:", payload.get("status"))
                if payload.get("status") in {"completed", "failed", "canceled"}:
                    return 0
                continue

            if event_type == "payload":
                recipient_cert_pem = _validate_payload_properties(payload)
                plaintext = build_header_body_envelope(
                    args.payload_content_type,
                    _read_payload(args.payload_file),
                    "client-payload",
                )
                ciphertext = encrypt_cms_der(plaintext, recipient_cert_pem)
                channel = payload.get("channel_id") or payload.get("url")
                if not channel:
                    raise ValueError("payload event missing transfer channel")
                await transfer.upload(channel, ciphertext, CMS_CONTENT_TYPE)
                print(f"uploaded encrypted payload ({len(ciphertext)} bytes)")
                continue

            if event_type == "result":
                _validate_result_properties(payload)
                channel = payload.get("channel_id") or payload.get("url")
                if not channel:
                    raise ValueError("result event missing transfer channel")
                ciphertext = await transfer.download(channel)
                plaintext = decrypt_cms_der(ciphertext, identity)
                headers, body = parse_header_body_envelope(plaintext)
                print("result headers:", json.dumps(headers, indent=2))
                _write_result(args.result_file, body)
                print(f"received encrypted result ({len(ciphertext)} bytes)")
                continue
    finally:
        await transfer.close()
        await jobs.close()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="nfrx secure jobs API client example")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--api-key", default=None)
    parser.add_argument("--job-type", default="asr.transcribe")
    parser.add_argument("--metadata", default=None, help="optional JSON object merged into job metadata")
    parser.add_argument("--worker-id", default=None)
    parser.add_argument("--worker-group", default=None)
    parser.add_argument("--payload-file", default=None)
    parser.add_argument("--payload-content-type", default="application/octet-stream")
    parser.add_argument("--result-file", default=None)
    parser.add_argument("--timeout", type=float, default=30.0)
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        return asyncio.run(run(args))
    except (ValueError, httpx.HTTPError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
