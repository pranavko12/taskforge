#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

COMPOSE_FILE="$ROOT/docker-compose.integration.yml"
export POSTGRES_DSN="${POSTGRES_DSN:-postgres://taskforge:taskforge@localhost:5432/taskforge?sslmode=disable}"
export REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
export QUEUE_NAME="${QUEUE_NAME:-jobs:ready}"

docker compose -f "$COMPOSE_FILE" up -d

cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v
}
trap cleanup EXIT

echo "Waiting for services..."
for i in {1..40}; do
  if docker compose -f "$COMPOSE_FILE" ps --status running | grep -q postgres && \
     docker compose -f "$COMPOSE_FILE" ps --status running | grep -q redis; then
    break
  fi
  sleep 2
done

go test -tags=integration ./internal/integration -count=1
