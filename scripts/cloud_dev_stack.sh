#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
STACK_DIR="$ROOT_DIR/infra/local-cloud"
COMPOSE_FILE="$STACK_DIR/docker-compose.yml"
ENV_FILE="$STACK_DIR/.env"
ENV_EXAMPLE="$STACK_DIR/.env.example"

usage() {
  cat <<'EOF'
Usage: ./scripts/cloud_dev_stack.sh <command>

Commands:
  up      Start cloud-dev dependencies and wait for readiness checks.
  down    Stop stack (keep volumes).
  reset   Stop stack and remove volumes.
  status  Show stack service status.
  logs    Tail stack logs.
  config  Validate resolved docker-compose config.
EOF
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_cmd docker

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose v2 is required (docker compose ...)." >&2
  exit 1
fi

if [[ ! -f "$ENV_FILE" ]]; then
  cp "$ENV_EXAMPLE" "$ENV_FILE"
  echo "Created $ENV_FILE from template."
fi

# shellcheck disable=SC1090
source "$ENV_FILE"

FF_POSTGRES_PORT="${FF_POSTGRES_PORT:-15432}"
FF_REDIS_PORT="${FF_REDIS_PORT:-16379}"
FF_NATS_MONITOR_PORT="${FF_NATS_MONITOR_PORT:-18222}"
FF_MINIO_PORT="${FF_MINIO_PORT:-19000}"

wait_for_tcp() {
  local host="$1"
  local port="$2"
  local label="$3"
  local retries="${4:-45}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if command -v nc >/dev/null 2>&1; then
      if nc -z "$host" "$port" >/dev/null 2>&1; then
        echo "Ready: $label ($host:$port)"
        return 0
      fi
    elif (exec 3<>"/dev/tcp/${host}/${port}") >/dev/null 2>&1; then
      exec 3<&-
      exec 3>&-
      echo "Ready: $label ($host:$port)"
      return 0
    fi
    sleep 1
  done
  echo "Timed out waiting for $label ($host:$port)" >&2
  return 1
}

wait_for_http() {
  local url="$1"
  local label="$2"
  local retries="${3:-60}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      echo "Ready: $label ($url)"
      return 0
    fi
    sleep 1
  done
  echo "Timed out waiting for $label ($url)" >&2
  return 1
}

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

command="${1:-}"
case "$command" in
  up)
    require_cmd curl
    compose up -d
    wait_for_tcp 127.0.0.1 "$FF_POSTGRES_PORT" "Postgres"
    wait_for_tcp 127.0.0.1 "$FF_REDIS_PORT" "Redis"
    wait_for_http "http://127.0.0.1:${FF_NATS_MONITOR_PORT}/healthz" "NATS monitor"
    wait_for_http "http://127.0.0.1:${FF_MINIO_PORT}/minio/health/live" "MinIO"
    echo
    compose ps
    ;;
  down)
    compose down
    ;;
  reset)
    compose down -v --remove-orphans
    ;;
  status)
    compose ps
    ;;
  logs)
    compose logs -f --tail=200
    ;;
  config)
    compose config
    ;;
  *)
    usage
    exit 1
    ;;
esac
