"""Shared transfer client for nfrx examples."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional

import httpx


@dataclass
class AuthConfig:
    bearer_token: Optional[str] = None


class NfrxTransferClient:
    def __init__(self, base_url: str, auth: AuthConfig, timeout: float) -> None:
        self._base_url = base_url.rstrip("/")
        self._auth = auth
        self._client = httpx.AsyncClient(base_url=self._base_url, timeout=timeout)

    async def close(self) -> None:
        await self._client.aclose()

    def _headers(self) -> dict[str, str]:
        headers: dict[str, str] = {}
        if self._auth.bearer_token:
            headers["Authorization"] = f"Bearer {self._auth.bearer_token}"
        return headers

    def _resolve_url(self, url_or_path_or_id: str) -> str:
        if url_or_path_or_id.startswith("http://") or url_or_path_or_id.startswith("https://"):
            return url_or_path_or_id
        if "/" not in url_or_path_or_id:
            return f"{self._base_url}/api/transfer/{url_or_path_or_id}"
        return f"{self._base_url}{url_or_path_or_id}"

    async def upload(self, url_or_path_or_id: str, data: bytes, content_type: Optional[str]) -> None:
        url = self._resolve_url(url_or_path_or_id)
        headers = self._headers()
        if content_type:
            headers["Content-Type"] = content_type
        resp = await self._client.post(url, content=data, headers=headers)
        resp.raise_for_status()

    async def download(self, url_or_path_or_id: str) -> bytes:
        url = self._resolve_url(url_or_path_or_id)
        resp = await self._client.get(url, headers=self._headers())
        resp.raise_for_status()
        return resp.content

    async def create_channel(self) -> dict[str, str]:
        resp = await self._client.post("/api/transfer", headers=self._headers())
        resp.raise_for_status()
        return resp.json()
