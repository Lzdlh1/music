# 构建后端
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o musicflow ./cmd/server

# 构建前端
FROM node:20-alpine AS frontend
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ .
RUN npm run build

# 运行阶段
FROM alpine:3.19
RUN apk add --no-cache ffmpeg ca-certificates tzdata sqlite-libs
WORKDIR /app
COPY --from=builder /app/musicflow .
COPY --from=frontend /frontend/dist ./web/dist
COPY config.yaml.example ./config.yaml

EXPOSE 8080
VOLUME ["/app/data", "/app/temp"]

CMD ["./musicflow"]
