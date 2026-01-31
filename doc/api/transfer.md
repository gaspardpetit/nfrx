# Transfer API

This document describes the transfer API for streaming payloads/results over one-time channels.

## Python client usage (minimal)

To use the Python transfer client in your own project, copy this example file into your codebase:

- `examples/python/nfrx_sdk/transfer_client.py`

Install dependencies:

```bash
pip install httpx
```

Minimal usage:

```python
import asyncio

from nfrx_sdk.transfer_client import AuthConfig, NfrxTransferClient


async def main() -> None:
    client = NfrxTransferClient("http://localhost:8080", AuthConfig(), timeout=60)
    try:
        channel = await client.create_channel()
        channel_id = channel["channel_id"]
        print("channel_id:", channel_id)

        # download (reader)
        data = await client.download(channel_id)
        print("got bytes:", len(data))

        # upload (writer)
        await client.upload(channel_id, b"hello world", "application/octet-stream")
    finally:
        await client.close()


asyncio.run(main())
```
