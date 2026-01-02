import asyncio
import tempfile
from pathlib import Path
import pytest
from httpx import AsyncClient
from fastapi import FastAPI, Response

from src.app.downloaders import download_to_path

@pytest.mark.asyncio
async def test_download_to_path():
    # Create a small ASGI app to serve a test file
    app = FastAPI()

    @app.get('/file')
    async def file():
        return Response(content=b'hello world', media_type='application/octet-stream')

    async with AsyncClient(app=app, base_url='http://test') as client:
        # use httpx client URL
        url = 'http://test/file'
        with tempfile.TemporaryDirectory() as td:
            dest = Path(td) / 'hello.bin'
            await download_to_path(url, dest)
            assert dest.exists()
            data = dest.read_bytes()
            assert data == b'hello world'
