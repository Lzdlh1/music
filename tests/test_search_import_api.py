import pytest
from fastapi import FastAPI
from fastapi.responses import JSONResponse, Response
from httpx import AsyncClient
import os

from src.app.adapters.gdstudio import GdStudioAdapter
from src.app.main import app

@pytest.mark.asyncio
async def test_search_endpoint(monkeypatch):
    # Fake upstream API
    upstream = FastAPI()

    @upstream.get('/')
    async def root(types: str, source: str = 'netease', name: str = '', count: int = 20, pages: int = 1):
        if types == 'search':
            return JSONResponse({'result': [{'id': '123', 'name': 'Test Song', 'artist': 'Artist', 'pic_id': '999'}]})
        if types == 'url':
            return JSONResponse({'url': 'http://files/test.mp3', 'br': 320, 'size': 1234})
        return Response(status_code=404)

    async with AsyncClient(app=upstream, base_url='http://test') as client:
        monkeypatch.setenv('GDSTUDIO_API_BASE', 'http://test/')
        # call our API
        async with AsyncClient(app=app, base_url='http://app') as ac:
            # We need to register a test user
            res = await ac.post('/auth/register', params={'username': 'u', 'password': 'p'})
            assert res.status_code == 200
            res = await ac.post('/auth/login', data={'username': 'u', 'password': 'p'})
            token = res.json()['access_token']

            # search
            res = await ac.get('/search', params={'q': 'test', 'source': 'netease'}, headers={'Authorization': f'Bearer {token}'})
            assert res.status_code == 200
            data = res.json()
            assert isinstance(data, list)
            assert data[0]['id'] == '123'

            # import
            res = await ac.post('/import', json={'source': 'netease', 'id': '123', 'br': 320}, headers={'Authorization': f'Bearer {token}'})
            assert res.status_code == 200
            j = res.json()
            assert 'id' in j and 'resolved_br' in j
