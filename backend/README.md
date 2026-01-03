# Music Backend (Go + Gin)

This service is a replacement for the previous Python backend. It provides:

- Gin HTTP server
- GORM + SQLite
- JWT auth
- Download-to-temp + rclone upload flow

Important:
- **Cookies for China Mobile 云盘 and similar must be stored on the frontend (localStorage).** They may be sent to backend *only* when creating a task and will not be stored in the database.
- Ensure `RCLONE_REMOTE` is provided via env if you expect uploads to work.

Run locally:
1. Copy `.env.example` to `.env` and edit.
2. `go run ./cmd/server` (or `go build` then run binary). The server listens on port `12233` by default.

Docker:
- Build: `docker build -t music-backend:local .`
- Run: `docker run -e RCLONE_REMOTE=myremote:my/path -p 12233:12233 music-backend:local`

Docker Compose:
- When using the provided `docker-compose.yml`, the backend container still listens on `12233` inside the container, but the compose file maps it to host port `12234` (to avoid conflicting with the frontend dev server which uses `12233`).
