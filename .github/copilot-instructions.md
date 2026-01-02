# GitHub Copilot instructions for contributors

**Short summary:** This repository now contains a minimal working scaffold: a Python FastAPI backend, a simple frontend (`web/index.html`), rclone-based upload support, basic JWT auth, and a test for the download routine.

## What I discovered 🔎
- Language & stack: Python (FastAPI), uses `sqlmodel` (SQLite), `httpx` for downloads, and `rclone` for uploads.
- Key locations:
  - `src/app/main.py` — API endpoints, auth, and a search stub.
  - `src/app/models.py` — `User` and `Task` SQL models.
  - `src/app/downloaders.py` — download helper and site-adapter stub (do NOT implement scraping unless authorized).
  - `src/app/uploaders.py` — rclone invocation wrapper (expects `RCLONE_REMOTE` env).
  - `web/index.html` — minimal single-file web UI for login and task management.
  - `Dockerfile`, `docker-compose.yml` — container + compose templates; notes for `armv7` build with `docker buildx`.
- Current limitations: search is a stub; site-specific adapters are templates (no scraping implemented).

## How to be productive here ✅
- Local dev:
  1. Create `.env` (or use `.env.example`) with `SECRET_KEY` and `RCLONE_REMOTE`.
  2. Install: `python -m pip install -r requirements.txt`.
  3. Run: `uvicorn src.app.main:app --reload --port 8080`.
- To run in Docker:
  - `docker build -t music-uploader .`
  - `docker run -e RCLONE_REMOTE=myremote:my/path -p 8080:8080 music-uploader`
  - For ARMv7: use `docker buildx build --platform linux/arm/v7 -t music-uploader:armv7 .`
- rclone integration: mount or provide `~/.config/rclone/rclone.conf` via volume and set `RCLONE_REMOTE` env to the desired remote target.

## Project-specific rules & safety ⚠️
- Copyright compliance: Only download public-domain or user-owned files. **Do not implement or run scraping for third-party sites unless you have explicit permission or an API.** The repo contains a `gdstudio` adapter stub—use it only if you obtain the site's authorization.
- Upload behavior: uploads are performed by invoking the `rclone` binary; the service assumes `rclone` is pre-installed in the runtime or available in the container.
- Authentication: simple username/password + JWT for API access; all task creation and listing endpoints require auth.

## Testing / CI
- Tests: `tests/test_download.py` exercises the download helper using an ASGI test server.
- Add a GitHub Actions workflow that runs `pytest` and optionally builds the Docker image for CI validations.

## Useful examples & patterns 🔧
- Creating a task (server-side): POST `/tasks` with `{ "url": "https://...", "title": "..." }` (requires auth). The server downloads the file to `DOWNLOAD_DIR` and then calls `rclone copy` to `RCLONE_REMOTE`.
- Implementing a new site adapter: add an async function to `src/app/downloaders.py` and wire it into `/search` or into an adapter registry; **only** add site-specific scraping if you have explicit permission.

## When in doubt
- Ask one clear question (authorization for a source, credentials required, or which features to prioritize). If a source requires credentials or non-public API access, get written permission before implementing.

---

If you'd like, I can:
- add CI (`.github/workflows/ci.yml`) that runs `pytest` and builds cross-arch docker images; or
- implement a concrete site adapter for `https://music.gdstudio.org/` only after you confirm you have permission to access and download its content.
