#!/usr/bin/env python3
"""Async transfer channel example for nfrx.

Download (channel id optional):
  python examples/python/example_nfrx_transfer.py -d [<id>] [--base-url http://localhost:8080]

Upload (channel id optional):
  python examples/python/example_nfrx_transfer.py -u [<id>] [--base-url http://localhost:8080] <file>
"""

from __future__ import annotations

import argparse
import asyncio
import sys
from typing import Optional

import httpx

from nfrx_sdk.transfer_client import AuthConfig, NfrxTransferClient


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="nfrx transfer API async example")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--bearer", default=None, help="Authorization bearer token")
    parser.add_argument(
        "-d",
        "--download",
        nargs="?",
        const="",
        default=None,
        metavar="CHANNEL_ID",
        help="download from channel (optional id; creates if omitted)",
    )
    parser.add_argument(
        "-u",
        "--upload",
        nargs="?",
        const="",
        default=None,
        metavar="CHANNEL_ID",
        help="upload to channel (optional id; creates if omitted)",
    )
    parser.add_argument("file", nargs="?", help="file to upload")
    parser.add_argument("-o", "--output", default=None, help="output file for GET (defaults to stdout)")
    parser.add_argument("--content-type", default=None, help="Content-Type for POST")
    parser.add_argument("--timeout", type=float, default=60.0)
    return parser


def read_input(path: Optional[str]) -> bytes:
    if not path:
        raise ValueError("Provide a file to upload")
    with open(path, "rb") as handle:
        return handle.read()


def write_output(path: Optional[str], data: bytes) -> None:
    if path:
        with open(path, "wb") as handle:
            handle.write(data)
        return
    sys.stdout.buffer.write(data)


async def run(args: argparse.Namespace) -> int:
    auth = AuthConfig(bearer_token=args.bearer)
    client = NfrxTransferClient(args.base_url, auth, args.timeout)
    try:
        if args.download is not None and args.upload is not None:
            raise ValueError("Choose only one of --download or --upload")
        if args.download is None and args.upload is None:
            args.upload = ""

        if args.download is not None:
            channel_id = args.download or None
            if not channel_id:
                channel = await client.create_channel()
                channel_id = channel.get("channel_id")
                if not channel_id:
                    raise ValueError("transfer channel missing channel_id")
                print(f"channel_id: {channel_id}")
                expires = channel.get("expires_at")
                if expires:
                    print(f"expires_at: {expires}")
            data = await client.download(channel_id)
            write_output(args.output, data)
            return 0

        channel_id = args.upload or None
        if not channel_id:
            channel = await client.create_channel()
            channel_id = channel.get("channel_id")
            if not channel_id:
                raise ValueError("transfer channel missing channel_id")
            print(f"channel_id: {channel_id}")
            expires = channel.get("expires_at")
            if expires:
                print(f"expires_at: {expires}")
        payload = read_input(args.file)
        await client.upload(channel_id, payload, args.content_type)
        print("upload complete")
        return 0
    finally:
        await client.close()


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
