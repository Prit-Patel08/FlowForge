#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR=""
GO_TEST_TIMEOUT="${GO_TEST_TIMEOUT:-10m}"
RUN_DASHBOARD_BUILD=1

usage() {
  cat <<'EOF'
Usage: ./scripts/mvp_phase1_exit_gate.sh [options]

Validates Phase-1 MVP exit criteria with deterministic artifact output.

Options:
  --out DIR                Output directory (default: pilot_artifacts/mvp-phase1-exit-<timestamp>)
  --go-test-timeout VALUE  go test timeout (default: 10m or GO_TEST_TIMEOUT env)
  --skip-dashboard-build   Skip production dashboard build check
  -h, --help               Show this help text
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --go-test-timeout)
      GO_TEST_TIMEOUT="${2:-}"
      shift 2
      ;;
    --skip-dashboard-build)
      RUN_DASHBOARD_BUILD=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/mvp-phase1-exit-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"
LOG_DIR="$OUT_DIR/logs"
mkdir -p "$LOG_DIR"

if ! command -v go >/dev/null 2>&1; then
  echo "Missing required command: go" >&2
  exit 1
fi

if [[ "$RUN_DASHBOARD_BUILD" -eq 1 ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "Missing required command: npm (needed for dashboard build gate)." >&2
    exit 1
  fi
fi

run_ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
overall_status="PASS"

run_test_group() {
  local regex="$1"
  local logfile="$2"
  set +e
  go test ./test -count=1 -timeout "$GO_TEST_TIMEOUT" -run "$regex" -v >"$logfile" 2>&1
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "PASS"
  else
    overall_status="FAIL"
    echo "FAIL"
  fi
}

kill_restart_log="$LOG_DIR/kill_restart_correctness.log"
evidence_chain_log="$LOG_DIR/evidence_chain_determinism.log"
request_trace_log="$LOG_DIR/request_trace_correlation.log"
dashboard_build_log="$LOG_DIR/dashboard_build.log"

kill_restart_regex='TestKillEndpointAcknowledgesAndTerminatesWorker|TestRestartEndpointUpdatesRuntimeState|TestRestartEndpointRejectsWhileProcessRunning|TestKillAndRestartConflictDuringStop|TestRestartEndpointEnforcesRestartBudget'
evidence_chain_regex='TestTimelineEndpointSnapshotContract|TestTimelineIncidentEndpointSnapshotContract|TestTimelineDecisionEngineVersionContract|TestCorrelatedIncidentChainFromLoggingHelpers|TestIncidentTimelineQueryReturnsChronologicalEvents'
request_trace_regex='TestRequestTraceEndpointContract|TestRequestTraceEndpointIncludesDecisionEngineVersionContract|TestRequestTraceEndpointRejectsInvalidParams|TestProcessKillIdempotencyReplayAndConflict'

kill_restart_status="$(run_test_group "$kill_restart_regex" "$kill_restart_log")"
evidence_chain_status="$(run_test_group "$evidence_chain_regex" "$evidence_chain_log")"
request_trace_status="$(run_test_group "$request_trace_regex" "$request_trace_log")"

dashboard_build_status="SKIPPED"
if [[ "$RUN_DASHBOARD_BUILD" -eq 1 ]]; then
  set +e
  npm --prefix dashboard run build >"$dashboard_build_log" 2>&1
  dashboard_rc=$?
  set -e
  if [[ "$dashboard_rc" -eq 0 ]]; then
    dashboard_build_status="PASS"
  else
    dashboard_build_status="FAIL"
    overall_status="FAIL"
  fi
fi

cat >"$OUT_DIR/summary.tsv" <<EOF
run_timestamp	${run_ts}
go_test_timeout	${GO_TEST_TIMEOUT}
kill_restart_correctness	${kill_restart_status}
evidence_chain_determinism	${evidence_chain_status}
request_trace_correlation	${request_trace_status}
dashboard_build	${dashboard_build_status}
overall_status	${overall_status}
EOF

cat >"$OUT_DIR/summary.md" <<EOF
# MVP Phase-1 Exit Gate Summary

- Generated: \`${run_ts}\`
- Overall status: **${overall_status}**
- Go test timeout: \`${GO_TEST_TIMEOUT}\`

## Gate Results

1. Kill/restart correctness: ${kill_restart_status}
2. Deterministic evidence chain: ${evidence_chain_status}
3. Request-trace correlation: ${request_trace_status}
4. Dashboard production build: ${dashboard_build_status}

## Artifacts

- \`${OUT_DIR}/summary.tsv\`
- \`${kill_restart_log}\`
- \`${evidence_chain_log}\`
- \`${request_trace_log}\`
- \`${dashboard_build_log}\`
EOF

echo "MVP Phase-1 exit gate complete: $OUT_DIR/summary.md"
echo "Overall status: $overall_status"

if [[ "$overall_status" != "PASS" ]]; then
  exit 1
fi
