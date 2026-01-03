FROM golang:1.24 AS build
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go env -w GOPROXY=https://proxy.golang.org
RUN apt-get update && apt-get install -y --no-install-recommends git gcc libc6-dev build-essential pkg-config
COPY . .
ENV CGO_ENABLED=1
RUN go build -o music-backend ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates rclone && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /app/music-backend /app/music-backend
COPY .env.example /app/.env.example
EXPOSE 12233
ENTRYPOINT ["/app/music-backend"]
