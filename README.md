# music

This project is a small service to manage downloading public-domain or user-owned music files and uploading them to a cloud remote using rclone.

Quick start (development):

1. Install dependencies: python3 -m pip install -r requirements.txt
2. Configure environment variables (create `.env`):

   SECRET_KEY=replace-me
   RCLONE_REMOTE=myremote:my/path

3. Run:
   uvicorn src.app.main:app --reload --port 8080

UI is at: http://localhost:8080/index.html (served from container in production)

Docker:
 - Build: docker build -t music-uploader .
 - Run: docker run -e RCLONE_REMOTE=myremote:my/path -p 8080:8080 music-uploader
 - For armv7, use docker buildx: docker buildx build --platform linux/arm/v7 -t music-uploader:armv7 .

Security & Compliance:
 - This service is intended only for publicly-licensed content or files that you own or have permission to transfer.
 - Do NOT use site-scraping or downloading copyrighted content without explicit authorization.

Notes for contributors:
 - Authentication: simple username/password with JWT tokens.
 - Uploads are performed with rclone; configure your remotes in `~/.config/rclone/rclone.conf` or provide a mounted config.
 - Search endpoint is a stub and must be extended only if you have permission or a documented API from the source.
