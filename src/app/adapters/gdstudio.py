import os
import httpx
from typing import List, Dict, Optional

class GdStudioAdapter:
    """Adapter for GD Studio's public API.

    Configure with env var:
      - GDSTUDIO_API_BASE (e.g. https://music-api.gdstudio.xyz/api.php)

    This adapter implements the documented endpoints:
      - types=search  -> returns list or results
      - types=url     -> returns streaming URL for a track id
      - types=pic     -> returns picture URL
      - types=lyric   -> returns lyric text

    Note: the public API does not require an API key according to the docs; the
    adapter is tolerant and will raise only if GDSTUDIO_API_BASE is missing.
    """

    def __init__(self, base: Optional[str] = None):
        self.base = base or os.getenv("GDSTUDIO_API_BASE")
        if not self.base:
            raise RuntimeError("GDStudio adapter not configured: set GDSTUDIO_API_BASE")

    async def search(self, query: str, source: str = "netease", count: int = 20, pages: int = 1) -> List[Dict]:
        params = {"types": "search", "source": source, "name": query, "count": count, "pages": pages}
        async with httpx.AsyncClient(timeout=30) as client:
            r = await client.get(self.base, params=params)
            r.raise_for_status()
            data = r.json()
        items = data.get("result") if isinstance(data, dict) and "result" in data else (data.get("results") if isinstance(data, dict) and "results" in data else (data if isinstance(data, list) else []))
        out = []
        for item in items:
            title = item.get("name") or item.get("title") or item.get("song")
            track_id = item.get("id") or item.get("track_id")
            artist = item.get("artist")
            album = item.get("album")
            pic_id = item.get("pic_id")
            lyric_id = item.get("lyric_id") or item.get("id")
            if title and track_id:
                out.append({"title": title, "id": track_id, "artist": artist, "album": album, "pic_id": pic_id, "lyric_id": lyric_id, "source": item.get("source")})
        return out

    async def get_track_url(self, source: str, track_id: str, br: int = 320) -> Dict:
        params = {"types": "url", "source": source, "id": track_id, "br": br}
        async with httpx.AsyncClient(timeout=30) as client:
            r = await client.get(self.base, params=params)
            r.raise_for_status()
            data = r.json()
        # Expect data to contain url/br/size
        return data

    async def get_pic_url(self, source: str, pic_id: str, size: int = 300) -> Optional[str]:
        params = {"types": "pic", "source": source, "id": pic_id, "size": size}
        async with httpx.AsyncClient(timeout=30) as client:
            r = await client.get(self.base, params=params)
            r.raise_for_status()
            data = r.json()
        if isinstance(data, dict) and "url" in data:
            return data["url"]
        return None

    async def get_lyric(self, source: str, lyric_id: str) -> Optional[str]:
        params = {"types": "lyric", "source": source, "id": lyric_id}
        async with httpx.AsyncClient(timeout=30) as client:
            r = await client.get(self.base, params=params)
            r.raise_for_status()
            data = r.json()
        # expects {"lyric": "...", "tlyric": "..."} or similar
        if isinstance(data, dict) and "lyric" in data:
            return data.get("lyric")
        return None
