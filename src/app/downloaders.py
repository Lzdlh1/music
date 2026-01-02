import httpx
import aiofiles
from pathlib import Path
from typing import Callable

async def download_to_path(url: str, dest_path: Path, progress_callback: Callable[[int, int], None] = None):
    """Stream download a file to dest_path using httpx."""
    async with httpx.AsyncClient(timeout=60) as client:
        r = await client.get(url, follow_redirects=True)
        r.raise_for_status()
        total = int(r.headers.get("content-length") or 0)
        os_parent = dest_path.parent
        os_parent.mkdir(parents=True, exist_ok=True)
        downloaded = 0
        async with aiofiles.open(dest_path, "wb") as f:
            async for chunk in r.aiter_bytes():
                if not chunk:
                    continue
                await f.write(chunk)
                downloaded += len(chunk)
                if progress_callback:
                    progress_callback(downloaded, total)
    return dest_path

# Adapter template for site-specific scraping - DO NOT implement unauthorized scraping here.
# If GDStudio API is configured, use the adapter; otherwise return a safe stub.
async def gdstudio_search_stub(query: str, source: str = "netease", count: int = 20, pages: int = 1):
    """Return results from configured gdstudio API or a stub if not configured."""
    try:
        from .adapters.gdstudio import GdStudioAdapter
        adapter = GdStudioAdapter()
        return await adapter.search(query, source=source, count=count, pages=pages)
    except Exception:
        # conservative fallback: return a harmless stub that points to a generic sample
        return [{"title": f"Stub result for {query}", "url": "https://example.com/sample.mp3", "id": "0", "source": source}]
