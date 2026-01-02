import pytest
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from httpx import AsyncClient

from src.app.adapters.gdstudio import GdStudioAdapter

@pytest.mark.asyncio
async def test_gdstudio_adapter_parses_list(monkeypatch):
    # Create a fake upstream API
    app = FastAPI()

    @app.get('/search')
    async def search(q: str):
        return JSONResponse({"results": [{"title": "Song A", "file_url": "http://files/testa.mp3"}, {"name": "Song B", "download_url": "http://files/testb.mp3"}]})

    async with AsyncClient(app=app, base_url='http://test') as client:
        # monkeypatch adapter base and key
        monkeypatch.setenv('GDSTUDIO_API_BASE', 'http://test')
        monkeypatch.setenv('GDSTUDIO_API_KEY', 'fake')
        adapter = GdStudioAdapter()
        res = await adapter.search('foo')
        assert isinstance(res, list)
        assert any(r['title'] == 'Song A' and r['url'].endswith('testa.mp3') for r in res)
        assert any(r['title'] == 'Song B' and r['url'].endswith('testb.mp3') for r in res)
