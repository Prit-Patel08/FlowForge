#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR=""
FLOWFORGE_BIN="${FLOWFORGE_BIN:-./flowforge}"
API_PORT="${API_PORT:-8080}"
BUILD_BINARY=1
SKIP_HTTP_PROBES=0

usage() {
  cat <<'USAGE'
Usage: ./scripts/daemon_smoke.sh [options] [out_dir]

Options:
  --out DIR             Output artifact directory (default: smoke_artifacts/daemon-<timestamp>)
  --bin PATH            FlowForge binary path (default: ./flowforge or FLOWFORGE_BIN env)
  --api-port PORT       API port to probe (default: 8080 or API_PORT env)
  --skip-build          Skip `go build -o flowforge .`
  --skip-http-probes    Skip /healthz up/down probes (for contract tests only)
  -h, --help            Show help text
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --bin)
      FLOWFORGE_BIN="${2:-}"
      shift 2
      ;;
    --api-port)
      API_PORT="${2:-}"
      shift 2
      ;;
    --skip-build)
      BUILD_BINARY=0
      shift
      ;;
    --skip-http-probes)
      SKIP_HTTP_PROBES=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      if [[ -z "$OUT_DIR" ]]; then
        OUT_DIR="$1"
      else
        echo "Unexpected positional argument: $1" >&2
        usage >&2
        exit 1
      fi
      shift
      ;;
  esac
done

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="smoke_artifacts/daemon-$(date +%Y%m%d-%H%M%S)"
fi

LOG_DIR="$OUT_DIR/logs"
SUMMARY_TSV="$OUT_DIR/summary.tsv"
SUMMARY_MD="$OUT_DIR/summary.md"
mkdir -p "$LOG_DIR"

API_BASE="http://127.0.0.1:${API_PORT}"
OVERALL_STATUS="PASS"
INITIAL_STATUS="unknown"
RESTORE_DAEMON=0
FG_PID=""

declare -a STEP_NAMES=()
declare -a STEP_STATUS=()
declare -a STEP_LOGS=()

record_step() {
  local name="$1"
  local status="$2"
  local log_file="$3"
  STEP_NAMES+=("$name")
  STEP_STATUS+=("$status")
  STEP_LOGS+=("$log_file")
  if [[ "$status" == "FAIL" ]]; then
    OVERALL_STATUS="FAIL"
  fi
}

run_step() {
  local step="$1"
  shift
  local slug
  slug="$(echo "$step" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9' '_')"
  local log_file="$LOG_DIR/${slug}.log"
  if "$@" >"$log_file" 2>&1; then
    record_step "$step" "PASS" "$log_file"
    return 0
  fi
  record_step "$step" "FAIL" "$log_file"
  return 1
}

write_summary() {
  {
    echo -e "overall_status\t${OVERALL_STATUS}"
    echo -e "api_port\t${API_PORT}"
    echo -e "api_base\t${API_BASE}"
    echo -e "flowforge_bin\t${FLOWFORGE_BIN}"
    echo -e "skip_http_probes\t${SKIP_HTTP_PROBES}"
    echo -e "initial_status\t${INITIAL_STATUS}"
    local i
    for i in "${!STEP_NAMES[@]}"; do
      echo -e "step\t${STEP_NAMES[$i]}\t${STEP_STATUS[$i]}\t${STEP_LOGS[$i]}"
    done
  } >"$SUMMARY_TSV"

  {
    echo "# Daemon Smoke Summary"
    echo ""
    echo "- Overall status: ${OVERALL_STATUS}"
    echo "- API base: \`${API_BASE}\`"
    echo "- FlowForge binary: \`${FLOWFORGE_BIN}\`"
    echo "- Initial daemon status: \`${INITIAL_STATUS}\`"
    echo "- Skip HTTP probes: \`${SKIP_HTTP_PROBES}\`"
    echo ""
    echo "## Steps"
    local i
    for i in "${!STEP_NAMES[@]}"; do
      echo "- ${STEP_NAMES[$i]}: ${STEP_STATUS[$i]} (\`${STEP_LOGS[$i]}\`)"
    done
  } >"$SUMMARY_MD"
}

cleanup() {
  if [[ -n "$FG_PID" ]]; then
    stop_foreground_pid "$FG_PID" >/dev/null 2>&1 || true
    FG_PID=""
  fi
  "$FLOWFORGE_BIN" daemon stop --port "$API_PORT" >/dev/null 2>&1 || true
  if [[ "$RESTORE_DAEMON" == "1" ]]; then
    "$FLOWFORGE_BIN" daemon start --port "$API_PORT" --wait-seconds 10 >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

wait_for_http() {
  local url="$1"
  local retries="${2:-40}"
  local delay="${3:-0.25}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

wait_for_http_down() {
  local url="$1"
  local retries="${2:-40}"
  local delay="${3:-0.25}"
  local i
  for ((i = 1; i <= retries; i++)); do
    if ! curl -fsS --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$delay"
  done
  return 1
}

probe_up() {
  if [[ "$SKIP_HTTP_PROBES" == "1" ]]; then
    return 0
  fi
  wait_for_http "${API_BASE}/healthz" 40 0.25
}

probe_down() {
  if [[ "$SKIP_HTTP_PROBES" == "1" ]]; then
    return 0
  fi
  wait_for_http_down "${API_BASE}/healthz" 40 0.25
}

stop_foreground_pid() {
  local pid="$1"
  if [[ -z "$pid" ]]; then
    return 0
  fi
  if ! kill -0 "$pid" >/dev/null 2>&1; then
    wait "$pid" >/dev/null 2>&1 || true
    return 0
  fi

  pkill -TERM -P "$pid" >/dev/null 2>&1 || true
  kill "$pid" >/dev/null 2>&1 || true

  local i
  for ((i = 0; i < 20; i++)); do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      wait "$pid" >/dev/null 2>&1 || true
      return 0
    fi
    sleep 0.1
  done

  pkill -KILL -P "$pid" >/dev/null 2>&1 || true
  kill -KILL "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
  return 0
}

status_from_json() {
  local json_file="$1"
  python3 - "$json_file" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
print(payload.get("status", "unknown"))
PY
}

api_healthy_from_json() {
  local json_file="$1"
  python3 - "$json_file" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    payload = json.load(f)
print("true" if payload.get("api_healthy") else "false")
PY
}

cmd_build_binary() {
  go build -o flowforge .
}

cmd_capture_initial_status() {
  "$FLOWFORGE_BIN" daemon status --port "$API_PORT" --json >"$OUT_DIR/status_before.json"
  INITIAL_STATUS="$(status_from_json "$OUT_DIR/status_before.json")"
  if [[ "$INITIAL_STATUS" == "running" || "$INITIAL_STATUS" == "degraded" ]]; then
    RESTORE_DAEMON=1
  fi
  if [[ "$INITIAL_STATUS" == "external" ]]; then
    echo "existing API process is running without daemon pid ownership (status=external)." >&2
    echo "stop that process before running daemon smoke." >&2
    return 1
  fi
  return 0
}

cmd_daemon_stop() {
  "$FLOWFORGE_BIN" daemon stop --port "$API_PORT"
}

cmd_daemon_start() {
  "$FLOWFORGE_BIN" daemon start --port "$API_PORT" --wait-seconds 10
}

cmd_daemon_status_running() {
  "$FLOWFORGE_BIN" daemon status --port "$API_PORT" --json >"$OUT_DIR/status_running.json"
  local daemon_status
  daemon_status="$(status_from_json "$OUT_DIR/status_running.json")"
  local api_healthy
  api_healthy="$(api_healthy_from_json "$OUT_DIR/status_running.json")"
  if [[ "$daemon_status" != "running" ]]; then
    echo "expected daemon status running, got: $daemon_status" >&2
    return 1
  fi
  if [[ "$api_healthy" != "true" ]]; then
    echo "expected api_healthy=true in daemon status" >&2
    return 1
  fi
  return 0
}

cmd_daemon_logs_tail() {
  "$FLOWFORGE_BIN" daemon logs --lines 40 >"$OUT_DIR/daemon_logs_tail.txt"
}

cmd_daemon_start_again() {
  "$FLOWFORGE_BIN" daemon start --port "$API_PORT" --wait-seconds 10 >"$OUT_DIR/daemon_start_again.txt"
}

cmd_dashboard_auto_attach() {
  "$FLOWFORGE_BIN" dashboard --port "$API_PORT" --wait-seconds 10 >"$OUT_DIR/dashboard_auto_attach.txt"
}

cmd_dashboard_foreground_lifecycle() {
  local fg_log="$OUT_DIR/dashboard_foreground_runtime.log"
  "$FLOWFORGE_BIN" dashboard --port "$API_PORT" --foreground >"$fg_log" 2>&1 &
  FG_PID=$!

  if ! probe_up; then
    stop_foreground_pid "$FG_PID"
    FG_PID=""
    echo "foreground dashboard did not become healthy" >&2
    return 1
  fi

  stop_foreground_pid "$FG_PID"
  FG_PID=""

  if ! probe_down; then
    echo "API remained healthy after foreground process exit" >&2
    return 1
  fi
  return 0
}

require_cmd curl
require_cmd python3

if [[ "$BUILD_BINARY" == "1" ]]; then
  run_step "Build FlowForge binary" cmd_build_binary || true
else
  record_step "Build FlowForge binary" "SKIPPED" "-"
fi

run_step "Capture initial daemon status" cmd_capture_initial_status || true
run_step "Stop daemon baseline" cmd_daemon_stop || true
run_step "Start daemon" cmd_daemon_start || true
run_step "Health probe after daemon start" probe_up || true
run_step "Verify daemon status running" cmd_daemon_status_running || true
run_step "Tail daemon logs" cmd_daemon_logs_tail || true
run_step "Idempotent daemon start" cmd_daemon_start_again || true
run_step "Stop daemon before dashboard auto-attach" cmd_daemon_stop || true
run_step "Dashboard auto-attach" cmd_dashboard_auto_attach || true
run_step "Health probe after dashboard auto-attach" probe_up || true
run_step "Stop daemon before dashboard foreground test" cmd_daemon_stop || true
run_step "Dashboard foreground lifecycle" cmd_dashboard_foreground_lifecycle || true
run_step "Final daemon stop" cmd_daemon_stop || true

write_summary
echo "Daemon smoke complete: $SUMMARY_MD"

if [[ "$OVERALL_STATUS" != "PASS" ]]; then
  exit 1
fi
