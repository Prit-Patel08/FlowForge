#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

API_PORT="${API_PORT:-8080}"
DASHBOARD_PORT="${DASHBOARD_PORT:-3001}"
API_BASE="http://127.0.0.1:${API_PORT}"
ARTIFACT_DIR="${1:-smoke_artifacts/$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$ARTIFACT_DIR"
SMOKE_DB_PATH="$ARTIFACT_DIR/flowforge-smoke.db"
SMOKE_MASTER_KEY="${FLOWFORGE_SMOKE_MASTER_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"

DEMO_LOG="$ARTIFACT_DIR/demo.log"
API_LOG="$ARTIFACT_DIR/api.log"
DASHBOARD_LOG="$ARTIFACT_DIR/dashboard.log"
TIMELINE_JSON="$ARTIFACT_DIR/timeline.json"

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_cmd go
require_cmd npm
require_cmd curl

if command -v lsof >/dev/null 2>&1; then
  if lsof -t -i :"$API_PORT" >/dev/null 2>&1 || lsof -t -i :"$DASHBOARD_PORT" >/dev/null 2>&1; then
    echo "Ports $API_PORT or $DASHBOARD_PORT are already in use. Stop existing services and retry." >&2
    exit 1
  fi
fi

cleanup() {
  if [[ -n "${API_PID:-}" ]]; then
    kill "$API_PID" 2>/dev/null || true
    wait "$API_PID" 2>/dev/null || true
  fi
  if [[ -n "${DASHBOARD_PID:-}" ]]; then
    kill "$DASHBOARD_PID" 2>/dev/null || true
    wait "$DASHBOARD_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

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

echo "[1/5] Building backend binary..."
go build -o flowforge .

echo "[2/5] Building dashboard production assets..."
(
  cd dashboard
  npm ci
  npm run build
) >"$ARTIFACT_DIR/dashboard-build.log" 2>&1

echo "[3/5] Running demo for incident generation..."
FLOWFORGE_DB_PATH="$SMOKE_DB_PATH" FLOWFORGE_MASTER_KEY="$SMOKE_MASTER_KEY" ./flowforge demo >"$DEMO_LOG" 2>&1

grep -q "Runaway detected in" "$DEMO_LOG"
grep -q "CPU peaked at" "$DEMO_LOG"
grep -q "Process recovered" "$DEMO_LOG"

echo "[4/5] Starting API + dashboard..."
FLOWFORGE_DB_PATH="$SMOKE_DB_PATH" FLOWFORGE_MASTER_KEY="$SMOKE_MASTER_KEY" FLOWFORGE_BIND_HOST=127.0.0.1 ./flowforge dashboard --foreground >"$API_LOG" 2>&1 &
API_PID=$!
wait_for_http "$API_BASE/healthz" 30 1

(
  cd dashboard
  NEXT_PUBLIC_FLOWFORGE_API_BASE="$API_BASE" npm run start -- -p "$DASHBOARD_PORT"
) >"$DASHBOARD_LOG" 2>&1 &
DASHBOARD_PID=$!
wait_for_http "http://127.0.0.1:${DASHBOARD_PORT}" 30 1

echo "[5/5] Probing service endpoints..."
health_payload="$(curl -fsS --max-time 3 "$API_BASE/healthz")"
metrics_payload="$(curl -fsS --max-time 3 "$API_BASE/metrics")"
curl -fsS --max-time 3 "$API_BASE/timeline" >"$TIMELINE_JSON"

echo "$health_payload" | grep -q '"status":"ok"'
echo "$metrics_payload" | grep -q "flowforge_uptime_seconds"
grep -q '^\[' "$TIMELINE_JSON"

echo ""
echo "Smoke check passed."
echo "Artifacts: $ARTIFACT_DIR"
echo "- db: $SMOKE_DB_PATH"
echo "- demo log: $DEMO_LOG"
echo "- api log: $API_LOG"
echo "- dashboard log: $DASHBOARD_LOG"
echo "- timeline payload: $TIMELINE_JSON"
