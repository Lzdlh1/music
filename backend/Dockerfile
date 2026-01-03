FROM golang:1.24-alpine AS build
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go env -w GOPROXY=https://proxy.golang.org
RUN apk add --no-cache git
COPY . .
RUN go build -o music-backend ./cmd/server

FROM alpine:3.18
RUN apk add --no-cache ca-certificates rclone
WORKDIR /app
COPY --from=build /app/music-backend /app/music-backend
COPY .env.example /app/.env.example
EXPOSE 12233
ENTRYPOINT ["/app/music-backend"]
