#!/usr/bin/env python3
"""Secure Jobs API worker example for nfrx using CMS/X.509."""

from __future__ import annotations

import argparse
import asyncio
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Optional

import httpx

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


@dataclass
class AuthConfig:
    client_key: Optional[str] = None


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
        self,
        types: Optional[list[str]] = None,
        max_wait_seconds: int = 30,
        worker_id: Optional[str] = None,
        worker_group: Optional[str] = None,
    ) -> Optional[Dict[str, Any]]:
        payload: Dict[str, Any] = {"max_wait_seconds": max_wait_seconds}
        if types:
            payload["types"] = types
        if worker_id:
            payload["worker_id"] = worker_id
        if worker_group:
            payload["worker_group"] = worker_group
        resp = await self._client.post("/api/jobs/claim", json=payload, headers=self._headers())
        if resp.status_code == 204:
            return None
        resp.raise_for_status()
        return resp.json()

    async def request_payload_channel(self, job_id: str, properties: Dict[str, Any]) -> Dict[str, Any]:
        resp = await self._client.post(
            f"/api/jobs/{job_id}/payload",
            json={"properties": properties},
            headers=self._headers(),
        )
        resp.raise_for_status()
        return resp.json()

    async def request_result_channel(self, job_id: str, properties: Dict[str, Any]) -> Dict[str, Any]:
        resp = await self._client.post(
            f"/api/jobs/{job_id}/result",
            json={"properties": properties},
            headers=self._headers(),
        )
        resp.raise_for_status()
        return resp.json()

    async def update_status(self, job_id: str, state: str, payload: Optional[Dict[str, Any]] = None) -> None:
        body = {"state": state}
        if payload:
            body.update(payload)
        resp = await self._client.post(f"/api/jobs/{job_id}/status", json=body, headers=self._headers())
        resp.raise_for_status()

    async def read_payload(self, url: str) -> bytes:
        resp = await self._client.get(url, headers=self._headers())
        resp.raise_for_status()
        return resp.content

    async def write_result(self, url: str, data: bytes) -> None:
        resp = await self._client.post(
            url,
            content=data,
            headers={**self._headers(), "Content-Type": CMS_CONTENT_TYPE},
        )
        resp.raise_for_status()


def _secure_metadata(job: Dict[str, Any]) -> Dict[str, Any]:
    metadata = job.get("metadata")
    if not isinstance(metadata, dict):
        raise ValueError("job metadata missing")
    secure = metadata.get("secure_transfer")
    if not isinstance(secure, dict):
        raise ValueError("job metadata missing secure_transfer section")
    schemes = secure.get("supported_schemes")
    if not isinstance(schemes, list) or SCHEME not in schemes:
        raise ValueError(f"client did not advertise {SCHEME!r}")
    if secure.get("envelope") != ENVELOPE:
        raise ValueError(f"client advertised unsupported envelope: {secure.get('envelope')!r}")
    recipient = secure.get("result_recipient_cert_pem")
    if not isinstance(recipient, str) or not recipient.strip():
        raise ValueError("job metadata missing result_recipient_cert_pem")
    return secure


def _write_result(path: Optional[str], data: bytes) -> None:
    if not path:
        return
    Path(path).write_bytes(data)


async def run(args: argparse.Namespace) -> int:
    identity = generate_self_signed_identity("nfrx secure worker")
    print("generated worker certificate (PEM):")
    print(identity.certificate_pem.strip())

    worker = NfrxJobsWorker(args.base_url, AuthConfig(client_key=args.client_key))
    try:
        types = args.types.split(",") if args.types else None
        while True:
            job = await worker.claim_job(types, args.max_wait_seconds, args.worker_id, args.worker_group)
            if not job:
                print("no job available")
                if args.once:
                    return 0
                continue

            job_id = job["job_id"]
            print(f"claimed secure job: {job_id}")
            try:
                secure = _secure_metadata(job)
                await worker.update_status(job_id, "claimed")

                payload_properties = {
                    "encryption_scheme": SCHEME,
                    "envelope": ENVELOPE,
                    "recipient_cert_pem": identity.certificate_pem,
                }
                payload_channel = await worker.request_payload_channel(job_id, payload_properties)
                payload_url = payload_channel.get("reader_url") or payload_channel.get("url")
                if not payload_url:
                    raise ValueError("payload channel missing reader_url")
                ciphertext = await worker.read_payload(payload_url)
                plaintext = decrypt_cms_der(ciphertext, identity)
                headers, body = parse_header_body_envelope(plaintext)
                print("payload headers:", json.dumps(headers, indent=2))

                await worker.update_status(job_id, "running")

                result_envelope = build_header_body_envelope(
                    headers["content-type"],
                    body,
                    "worker-result",
                )
                result_ciphertext = encrypt_cms_der(result_envelope, secure["result_recipient_cert_pem"])
                result_properties = {
                    "encryption_scheme": SCHEME,
                    "envelope": ENVELOPE,
                    "recipient": "job-metadata-result-cert",
                }
                result_channel = await worker.request_result_channel(job_id, result_properties)
                result_url = result_channel.get("writer_url") or result_channel.get("url")
                if not result_url:
                    raise ValueError("result channel missing writer_url")
                await worker.write_result(result_url, result_ciphertext)
                _write_result(args.debug_result_file, body)
                await worker.update_status(job_id, "completed")
                print(f"completed secure job: {job_id}")
                return 0
            except Exception as exc:
                await worker.update_status(
                    job_id,
                    "failed",
                    payload={"error": {"code": "secure_transfer_error", "message": str(exc)}},
                )
                raise
    finally:
        await worker.close()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="nfrx secure jobs API worker example")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--client-key", default=None)
    parser.add_argument("--types", default="asr.transcribe")
    parser.add_argument("--worker-id", default=None)
    parser.add_argument("--worker-group", default=None)
    parser.add_argument("--max-wait-seconds", type=int, default=30)
    parser.add_argument("--debug-result-file", default=None, help="optional plaintext result output")
    parser.add_argument("--once", action="store_true")
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
