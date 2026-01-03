#!/usr/bin/env bash
set -euo pipefail

echo "Starting services with docker compose..."
docker compose up --build
