#!/usr/bin/env bash

set -euo pipefail

API_PORT="${API_PORT:-8080}"
DASHBOARD_PORT="${DASHBOARD_PORT:-3001}"
ENV_FILE=".flowforge.env"
OPEN_BROWSER=0
RUN_DEMO="${RUN_DEMO:-1}"

for arg in "$@"; do
  case "$arg" in
    --open-browser) OPEN_BROWSER=1 ;;
    --no-demo) RUN_DEMO=0 ;;
  esac
done

echo "== FlowForge production setup =="

command -v go >/dev/null 2>&1 || { echo "Go is required"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "npm is required"; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "curl is required"; exit 1; }

random_hex_32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  fi
}

upsert_env() {
  local key="$1"
  local value="$2"
  local file="$3"

  if grep -q "^${key}=" "$file"; then
    awk -v k="$key" -v v="$value" -F= 'BEGIN { OFS="=" } $1 == k { $0 = k "=" v } { print }' "$file" > "${file}.tmp"
    mv "${file}.tmp" "$file"
  else
    echo "${key}=${value}" >> "$file"
  fi
}

legacy_value() {
  local key_regex="$1"
  local file="$2"
  awk -F= -v key_regex="$key_regex" '
    $1 ~ key_regex && $1 !~ /^FLOWFORGE_/ && $1 !~ /^NEXT_PUBLIC_FLOWFORGE_/ {
      print substr($0, index($0, "=") + 1)
      exit
    }
  ' "$file"
}

wait_for_http() {
  local url="$1"
  local retries="${2:-30}"
  local delay="${3:-1}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  echo "Timed out waiting for $url" >&2
  return 1
}

GENERATED_API_KEY=""

if [[ ! -f "$ENV_FILE" ]]; then
  touch "$ENV_FILE"
  echo "Created $ENV_FILE"
else
  echo "Using existing $ENV_FILE"
fi

set -a
source "$ENV_FILE"
set +a

if [[ -z "${FLOWFORGE_API_KEY:-}" ]]; then
  FLOWFORGE_API_KEY="$(legacy_value '^[A-Z0-9_]+_API_KEY$' "$ENV_FILE" || true)"
  if [[ -z "$FLOWFORGE_API_KEY" ]]; then
    FLOWFORGE_API_KEY="$(random_hex_32)"
    GENERATED_API_KEY="$FLOWFORGE_API_KEY"
  fi
  upsert_env "FLOWFORGE_API_KEY" "$FLOWFORGE_API_KEY" "$ENV_FILE"
fi

if [[ -z "${FLOWFORGE_MASTER_KEY:-}" ]]; then
  FLOWFORGE_MASTER_KEY="$(legacy_value '^[A-Z0-9_]+_MASTER_KEY$' "$ENV_FILE" || true)"
  if [[ -z "$FLOWFORGE_MASTER_KEY" ]]; then
    FLOWFORGE_MASTER_KEY="$(random_hex_32)"
  fi
  upsert_env "FLOWFORGE_MASTER_KEY" "$FLOWFORGE_MASTER_KEY" "$ENV_FILE"
fi

if [[ -z "${FLOWFORGE_ALLOWED_ORIGIN:-}" ]]; then
  FLOWFORGE_ALLOWED_ORIGIN="$(legacy_value '^[A-Z0-9_]+_ALLOWED_ORIGIN$' "$ENV_FILE" || true)"
  if [[ -z "$FLOWFORGE_ALLOWED_ORIGIN" ]]; then
    FLOWFORGE_ALLOWED_ORIGIN="http://localhost:${DASHBOARD_PORT}"
  fi
  upsert_env "FLOWFORGE_ALLOWED_ORIGIN" "$FLOWFORGE_ALLOWED_ORIGIN" "$ENV_FILE"
fi

if [[ -z "${FLOWFORGE_BIND_HOST:-}" ]]; then
  FLOWFORGE_BIND_HOST="$(legacy_value '^[A-Z0-9_]+_BIND_HOST$' "$ENV_FILE" || true)"
  if [[ -z "$FLOWFORGE_BIND_HOST" ]]; then
    FLOWFORGE_BIND_HOST="127.0.0.1"
  fi
  upsert_env "FLOWFORGE_BIND_HOST" "$FLOWFORGE_BIND_HOST" "$ENV_FILE"
fi

if [[ -z "${NEXT_PUBLIC_FLOWFORGE_API_BASE:-}" ]]; then
  NEXT_PUBLIC_FLOWFORGE_API_BASE="$(legacy_value '^NEXT_PUBLIC_[A-Z0-9_]+_API_BASE$' "$ENV_FILE" || true)"
  if [[ -z "$NEXT_PUBLIC_FLOWFORGE_API_BASE" ]]; then
    NEXT_PUBLIC_FLOWFORGE_API_BASE="http://localhost:${API_PORT}"
  fi
  upsert_env "NEXT_PUBLIC_FLOWFORGE_API_BASE" "$NEXT_PUBLIC_FLOWFORGE_API_BASE" "$ENV_FILE"
fi

chmod 600 "$ENV_FILE"

if [[ -n "$GENERATED_API_KEY" ]]; then
  echo "Generated secure runtime API key."
  echo "API key (shown once): $GENERATED_API_KEY"
fi

set -a
source "$ENV_FILE"
set +a

echo "Building backend..."
go mod download
go build -o flowforge .

echo "Ensuring dashboard dependencies..."
pushd dashboard >/dev/null
if [[ ! -d "node_modules" ]]; then
  npm ci
fi
echo "Building dashboard (production)..."
NEXT_PUBLIC_FLOWFORGE_API_BASE="http://localhost:${API_PORT}" npm run build
popd >/dev/null

if [[ "$RUN_DEMO" == "1" ]]; then
  echo "🎬 Running 60-second value demo..."
  FLOWFORGE_DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}" ./flowforge demo || true
fi

if command -v lsof >/dev/null 2>&1; then
  echo "🧹 Clearing ports ${API_PORT} and ${DASHBOARD_PORT}..."
  lsof -t -i :"${API_PORT}" -i :"${DASHBOARD_PORT}" | xargs kill -9 2>/dev/null || true
fi

API_PID=""
DASHBOARD_PID=""

cleanup() {
  echo "Stopping services..."
  if [[ -n "$API_PID" ]]; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  if [[ -n "$DASHBOARD_PID" ]]; then
    kill "$DASHBOARD_PID" 2>/dev/null || true
    wait "$DASHBOARD_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "🚀 Starting services..."

echo "Starting API..."
./flowforge dashboard &
API_PID=$!
wait_for_http "http://127.0.0.1:${API_PORT}/healthz" 30 1

echo "Starting dashboard server (production)..."
(
  cd dashboard
  NEXT_PUBLIC_FLOWFORGE_API_BASE="http://localhost:${API_PORT}" npm run start -- -p "${DASHBOARD_PORT}"
) &
DASHBOARD_PID=$!
wait_for_http "http://127.0.0.1:${DASHBOARD_PORT}" 30 1

echo "------------------------------------------------"
echo "✅ API:       http://localhost:${API_PORT}/healthz"
echo "✅ Dashboard: http://localhost:${DASHBOARD_PORT}"
echo "------------------------------------------------"

if [[ "$OPEN_BROWSER" == "1" ]]; then
  if command -v open >/dev/null 2>&1; then
    open "http://localhost:${DASHBOARD_PORT}" || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "http://localhost:${DASHBOARD_PORT}" || true
  fi
fi

echo "Services are running. Press Ctrl+C to stop."
wait
