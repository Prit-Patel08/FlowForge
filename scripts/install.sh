#!/usr/bin/env bash

set -euo pipefail

API_PORT="${API_PORT:-8080}"
DASHBOARD_PORT="${DASHBOARD_PORT:-3001}"
ENV_FILE=".sentry.env"
OPEN_BROWSER=0
RUN_DEMO="${RUN_DEMO:-1}"

for arg in "$@"; do
  case "$arg" in
    --open-browser) OPEN_BROWSER=1 ;;
    --no-demo) RUN_DEMO=0 ;;
  esac
done

echo "== Agent-Sentry production setup =="

command -v go >/dev/null 2>&1 || { echo "Go is required"; exit 1; }
command -v npm >/dev/null 2>&1 || { echo "npm is required"; exit 1; }

random_hex_32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  fi
}

if [[ ! -f "$ENV_FILE" ]]; then
  API_KEY="$(random_hex_32)"
  MASTER_KEY="$(random_hex_32)"
  cat > "$ENV_FILE" <<EOF
SENTRY_API_KEY=$API_KEY
SENTRY_MASTER_KEY=$MASTER_KEY
SENTRY_ALLOWED_ORIGIN=http://localhost:${DASHBOARD_PORT}
SENTRY_BIND_HOST=127.0.0.1
NEXT_PUBLIC_SENTRY_API_BASE=http://localhost:${API_PORT}
EOF
  chmod 600 "$ENV_FILE"
  echo "Generated secure runtime secrets."
  echo "API key (shown once): $API_KEY"
else
  echo "Using existing $ENV_FILE"
fi

set -a
source "$ENV_FILE"
set +a

echo "Building backend..."
go mod download
go build -o sentry .

echo "Building dashboard (production)..."
pushd dashboard >/dev/null
npm ci
npm run build
popd >/dev/null

if command -v lsof >/dev/null 2>&1; then
  lsof -t -i :"${API_PORT}" -i :"${DASHBOARD_PORT}" | xargs kill -9 2>/dev/null || true
fi

if [[ "$RUN_DEMO" == "1" ]]; then
  echo "Running 60-second value demo..."
  ./sentry demo || true
fi

cleanup() {
  pkill -f "./sentry dashboard" 2>/dev/null || true
  pkill -f "next start -p ${DASHBOARD_PORT}" 2>/dev/null || true
}
trap cleanup EXIT

echo "Starting API..."
./sentry dashboard &
sleep 1

echo "Starting dashboard server..."
(
  cd dashboard
  NEXT_PUBLIC_SENTRY_API_BASE="http://localhost:${API_PORT}" npm run start -- -p "${DASHBOARD_PORT}"
) &

echo "API:       http://localhost:${API_PORT}/healthz"
echo "Dashboard: http://localhost:${DASHBOARD_PORT}"
echo "Metrics:   http://localhost:${API_PORT}/metrics"

if [[ "$OPEN_BROWSER" == "1" ]]; then
  if command -v open >/dev/null 2>&1; then
    open "http://localhost:${DASHBOARD_PORT}" || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "http://localhost:${DASHBOARD_PORT}" || true
  fi
fi

wait
