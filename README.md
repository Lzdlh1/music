# music

This project is a small service to manage downloading public-domain or user-owned music files and uploading them to a cloud remote using rclone.

Quick start (development):

1. Install dependencies: python3 -m pip install -r requirements.txt
2. Configure environment variables (create `.env`):

   SECRET_KEY=replace-me
   RCLONE_REMOTE=myremote:my/path

3. Run:
   uvicorn src.app.main:app --reload --port 3031

UI is at: http://localhost:3031/index.html (served from container in production)

Docker:
 - Build: docker build -t music-uploader .
 - Run: docker run -e RCLONE_REMOTE=myremote:my/path -p 3031:3031 music-uploader
 - For armv7, use docker buildx: docker buildx build --platform linux/arm/v7 -t music-uploader:armv7 .

Security & Compliance:
 - This service is intended only for publicly-licensed content or files that you own or have permission to transfer.
 - Do NOT use site-scraping or downloading copyrighted content without explicit authorization.

Notes for contributors:
 - Authentication: simple username/password with JWT tokens.
 - Uploads are performed with rclone; configure your remotes in `~/.config/rclone/rclone.conf` or provide a mounted config.
 - Search endpoint is a stub and must be extended only if you have permission or a documented API from the source.

Security note: Storing cookies in localStorage has security and privacy risks — they can be accessed by any script running in the page. Do not store credentials you don't trust here. The backend will not persist cookie values; they are used only for the single request that created the task.

Docker Compose (one-command development) ✅

You can bring up both services (backend and frontend dev server) with docker-compose. Notes:
- Frontend dev server is exposed at http://localhost:12233
- Backend API is exposed at http://localhost:12234 (mapped from the container's 12233)

Quick start:

1. Copy or set env values in your shell (for example):

   export SECRET_KEY=replace-me
   export RCLONE_REMOTE=myremote:my/path

2. Start services:

   docker compose up --build

3. Open the frontend at http://localhost:12233 and use the app. The frontend will call the backend API at http://localhost:12234/api by default.

If you prefer different host ports, edit `docker-compose.yml` accordingly.
