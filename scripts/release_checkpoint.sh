#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REPORT_DIR="${1:-pilot_artifacts/release-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$REPORT_DIR"
REPORT_FILE="$REPORT_DIR/checkpoint.md"

check() {
  local name="$1"
  shift
  echo "[$name] $*"
  "$@"
}

check "verify-local" ./scripts/verify_local.sh --strict

if git ls-files | rg -q "flowforge\\.key|\\.flowforge_live|flowforge\\.db|sentry\\.key"; then
  echo "Blocked: secret/runtime artifacts still tracked." >&2
  exit 1
fi

if rg -n -i --hidden "quenvor|agent-sentry " -g '!.git/*' -g '!scripts/release_checkpoint.sh' -g '!README.md' | rg -q .; then
  echo "Blocked: legacy brand references found." >&2
  exit 1
fi

required_docs=(
  "docs/RUNBOOK.md"
  "docs/WEEK1_PILOT.md"
  "docs/RELEASE_CHECKLIST.md"
  "docs/ROLLBACK_CHECKLIST.md"
)

missing_docs=()
for doc in "${required_docs[@]}"; do
  if [[ ! -f "$doc" ]]; then
    missing_docs+=("$doc")
  fi
done

if (( ${#missing_docs[@]} > 0 )); then
  echo "Blocked: required operator docs missing:" >&2
  for doc in "${missing_docs[@]}"; do
    echo "  - $doc" >&2
  done
  exit 1
fi

is_truthy() {
  local v="${1:-}"
  v="$(echo "$v" | tr '[:upper:]' '[:lower:]')"
  case "$v" in
    1|true|yes|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

cloud_readiness_status="SKIPPED (FLOWFORGE_CLOUD_DEPS_REQUIRED not enabled)"
readyz_artifact=""
readyz_url="${FLOWFORGE_API_BASE:-http://127.0.0.1:8080}/readyz"
evidence_bundle_status="SKIPPED (FLOWFORGE_REQUIRE_EVIDENCE_BUNDLE not enabled)"
evidence_bundle_dir=""
controlplane_replay_drill_status="SKIPPED (FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL not enabled)"
controlplane_replay_drill_dir=""
controlplane_replay_retention_status="SKIPPED (FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION not enabled)"
controlplane_replay_retention_dir=""
weekly_slo_green_gate_status="SKIPPED (FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN not enabled)"
weekly_slo_review_dir=""
weekly_slo_error_budget_status="N/A"
mvp_phase1_exit_gate_status="SKIPPED (FLOWFORGE_REQUIRE_MVP_PHASE1_EXIT_GATE not enabled)"
mvp_phase1_exit_gate_dir=""
daemon_smoke_gate_status="SKIPPED (FLOWFORGE_REQUIRE_DAEMON_SMOKE not enabled)"
daemon_smoke_gate_dir=""

if is_truthy "${FLOWFORGE_CLOUD_DEPS_REQUIRED:-0}"; then
  readyz_artifact="$REPORT_DIR/readyz.json"
  curl_exit=0
  readyz_http_code="$(curl -sS --max-time 5 -o "$readyz_artifact" -w "%{http_code}" "$readyz_url")" || curl_exit=$?
  if (( curl_exit != 0 )); then
    echo "Blocked: strict cloud readiness is enabled, but /readyz probe failed (curl exit $curl_exit)." >&2
    echo "  URL: $readyz_url" >&2
    exit 1
  fi

  compact_readyz="$(tr -d '\n\r\t ' < "$readyz_artifact")"
  if [[ "$readyz_http_code" != "200" || "$compact_readyz" != *'"status":"ready"'* ]]; then
    echo "Blocked: strict cloud readiness is enabled, but /readyz is not healthy." >&2
    echo "  URL: $readyz_url" >&2
    echo "  HTTP: $readyz_http_code" >&2
    exit 1
  fi

  if [[ "$compact_readyz" != *'"cloud_dependencies_required":true'* ]]; then
    echo "Blocked: release env has FLOWFORGE_CLOUD_DEPS_REQUIRED=1, but API /readyz does not report cloud_dependencies_required=true." >&2
    echo "  Start API with matching env before running checkpoint." >&2
    exit 1
  fi

  cloud_readiness_status="PASS"
fi

if is_truthy "${FLOWFORGE_REQUIRE_EVIDENCE_BUNDLE:-0}"; then
  if [[ -z "${FLOWFORGE_EVIDENCE_SIGNING_KEY:-}" && -z "${FLOWFORGE_MASTER_KEY:-}" ]]; then
    echo "Blocked: evidence bundle signing key is missing. Set FLOWFORGE_EVIDENCE_SIGNING_KEY (or FLOWFORGE_MASTER_KEY)." >&2
    exit 1
  fi

  cli_cmd=()
  if [[ -x "./flowforge" ]]; then
    cli_cmd=(./flowforge)
  elif command -v go >/dev/null 2>&1 && [[ -f "./main.go" ]]; then
    cli_cmd=(go run .)
  else
    echo "Blocked: cannot execute flowforge CLI for evidence bundle generation." >&2
    exit 1
  fi

  evidence_bundle_dir="$REPORT_DIR/evidence-bundle"
  export_cmd=("${cli_cmd[@]}" evidence export --out-dir "$evidence_bundle_dir")
  if [[ -n "${FLOWFORGE_EVIDENCE_INCIDENT_ID:-}" ]]; then
    export_cmd+=(--incident-id "${FLOWFORGE_EVIDENCE_INCIDENT_ID}")
  fi
  "${export_cmd[@]}" >/dev/null
  "${cli_cmd[@]}" evidence verify --bundle-dir "$evidence_bundle_dir" >/dev/null
  evidence_bundle_status="PASS"
fi

if is_truthy "${FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL:-0}"; then
  if [[ -z "${FLOWFORGE_API_KEY:-}" ]]; then
    echo "Blocked: replay drill gate requires FLOWFORGE_API_KEY." >&2
    exit 1
  fi
  if [[ ! -x "./scripts/controlplane_replay_drill.sh" ]]; then
    echo "Blocked: replay drill script missing or not executable (scripts/controlplane_replay_drill.sh)." >&2
    exit 1
  fi

  controlplane_replay_drill_dir="$REPORT_DIR/controlplane-replay-drill"
  ./scripts/controlplane_replay_drill.sh \
    --api-base "${FLOWFORGE_API_BASE:-http://127.0.0.1:8080}" \
    --api-key "${FLOWFORGE_API_KEY}" \
    --out "$controlplane_replay_drill_dir" >/dev/null

  summary_tsv="$controlplane_replay_drill_dir/summary.tsv"
  if [[ ! -f "$summary_tsv" ]] || ! grep -q $'^overall_status\tPASS$' "$summary_tsv"; then
    echo "Blocked: replay drill did not produce PASS summary." >&2
    exit 1
  fi

  controlplane_replay_drill_status="PASS"
fi

if is_truthy "${FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION:-0}"; then
  if [[ ! -x "./scripts/controlplane_replay_retention.sh" ]]; then
    echo "Blocked: replay retention script missing or not executable (scripts/controlplane_replay_retention.sh)." >&2
    exit 1
  fi

  retention_days="${FLOWFORGE_CONTROLPLANE_REPLAY_RETENTION_DAYS:-30}"
  max_rows="${FLOWFORGE_CONTROLPLANE_REPLAY_MAX_ROWS:-50000}"
  controlplane_replay_retention_dir="$REPORT_DIR/controlplane-replay-retention"

  retention_cmd=(
    ./scripts/controlplane_replay_retention.sh
    --retention-days "$retention_days"
    --max-rows "$max_rows"
    --out "$controlplane_replay_retention_dir"
  )
  if [[ -n "${FLOWFORGE_DB_PATH:-}" ]]; then
    retention_cmd+=(--db "${FLOWFORGE_DB_PATH}")
  fi
  "${retention_cmd[@]}" >/dev/null

  retention_summary="$controlplane_replay_retention_dir/summary.tsv"
  if [[ ! -f "$retention_summary" ]]; then
    echo "Blocked: replay retention script did not produce summary.tsv." >&2
    exit 1
  fi
  if grep -q $'^status\tPASS$' "$retention_summary"; then
    controlplane_replay_retention_status="PASS"
  elif grep -q $'^status\tSKIPPED$' "$retention_summary"; then
    controlplane_replay_retention_status="SKIPPED"
  else
    echo "Blocked: replay retention script status was neither PASS nor SKIPPED." >&2
    exit 1
  fi
fi

if is_truthy "${FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN:-0}"; then
  if [[ ! -x "./scripts/slo_weekly_review.sh" ]]; then
    echo "Blocked: weekly SLO gate requires scripts/slo_weekly_review.sh." >&2
    exit 1
  fi

  weekly_slo_review_days="${FLOWFORGE_SLO_REVIEW_DAYS:-7}"
  weekly_slo_review_dir="$REPORT_DIR/slo-weekly-review"

  slo_review_cmd=(
    ./scripts/slo_weekly_review.sh
    --days "$weekly_slo_review_days"
    --out "$weekly_slo_review_dir"
  )
  if [[ -n "${FLOWFORGE_API_BASE:-}" ]]; then
    slo_review_cmd+=(--api-base "${FLOWFORGE_API_BASE}")
  fi
  if [[ -n "${FLOWFORGE_DB_PATH:-}" ]]; then
    slo_review_cmd+=(--db "${FLOWFORGE_DB_PATH}")
  fi
  if [[ -n "${FLOWFORGE_SLO_REPLAY_MAX_ROWS:-}" ]]; then
    slo_review_cmd+=(--replay-max-rows "${FLOWFORGE_SLO_REPLAY_MAX_ROWS}")
  fi
  if [[ -n "${FLOWFORGE_SLO_REPLAY_SPIKE_YELLOW:-}" ]]; then
    slo_review_cmd+=(--replay-spike-yellow "${FLOWFORGE_SLO_REPLAY_SPIKE_YELLOW}")
  fi
  if [[ -n "${FLOWFORGE_SLO_REPLAY_SPIKE_RED:-}" ]]; then
    slo_review_cmd+=(--replay-spike-red "${FLOWFORGE_SLO_REPLAY_SPIKE_RED}")
  fi
  if [[ -n "${FLOWFORGE_SLO_CONFLICT_SPIKE_YELLOW:-}" ]]; then
    slo_review_cmd+=(--conflict-spike-yellow "${FLOWFORGE_SLO_CONFLICT_SPIKE_YELLOW}")
  fi
  if [[ -n "${FLOWFORGE_SLO_CONFLICT_SPIKE_RED:-}" ]]; then
    slo_review_cmd+=(--conflict-spike-red "${FLOWFORGE_SLO_CONFLICT_SPIKE_RED}")
  fi
  if ! "${slo_review_cmd[@]}" >/dev/null; then
    echo "Blocked: weekly SLO review script failed while evaluating release gate." >&2
    exit 1
  fi

  weekly_slo_summary="$weekly_slo_review_dir/summary.tsv"
  if [[ ! -f "$weekly_slo_summary" ]]; then
    echo "Blocked: weekly SLO review did not produce summary.tsv." >&2
    exit 1
  fi
  weekly_slo_error_budget_status="$(awk -F '\t' '$1=="error_budget_status"{print $2; exit}' "$weekly_slo_summary")"
  if [[ -z "$weekly_slo_error_budget_status" ]]; then
    echo "Blocked: weekly SLO summary missing error_budget_status." >&2
    exit 1
  fi
  if [[ "$weekly_slo_error_budget_status" != "GREEN" ]]; then
    echo "Blocked: weekly SLO gate requires error_budget_status=GREEN (found: $weekly_slo_error_budget_status)." >&2
    exit 1
  fi

  weekly_slo_green_gate_status="PASS"
fi

if is_truthy "${FLOWFORGE_REQUIRE_MVP_PHASE1_EXIT_GATE:-0}"; then
  if [[ ! -x "./scripts/mvp_phase1_exit_gate.sh" ]]; then
    echo "Blocked: MVP Phase-1 exit gate script missing or not executable (scripts/mvp_phase1_exit_gate.sh)." >&2
    exit 1
  fi

  mvp_phase1_exit_gate_dir="$REPORT_DIR/mvp-phase1-exit-gate"
  mvp_gate_cmd=(
    ./scripts/mvp_phase1_exit_gate.sh
    --out "$mvp_phase1_exit_gate_dir"
  )
  if [[ -n "${FLOWFORGE_MVP_EXIT_GO_TEST_TIMEOUT:-}" ]]; then
    mvp_gate_cmd+=(--go-test-timeout "${FLOWFORGE_MVP_EXIT_GO_TEST_TIMEOUT}")
  fi
  if is_truthy "${FLOWFORGE_MVP_EXIT_SKIP_DASHBOARD_BUILD:-0}"; then
    mvp_gate_cmd+=(--skip-dashboard-build)
  fi
  if ! "${mvp_gate_cmd[@]}" >/dev/null; then
    echo "Blocked: MVP Phase-1 exit gate failed while evaluating release readiness." >&2
    exit 1
  fi

  mvp_summary="$mvp_phase1_exit_gate_dir/summary.tsv"
  if [[ ! -f "$mvp_summary" ]] || ! grep -q $'^overall_status\tPASS$' "$mvp_summary"; then
    echo "Blocked: MVP Phase-1 exit gate did not report PASS summary status." >&2
    exit 1
  fi

  mvp_phase1_exit_gate_status="PASS"
fi

if is_truthy "${FLOWFORGE_REQUIRE_DAEMON_SMOKE:-0}"; then
  if [[ ! -x "./scripts/daemon_smoke.sh" ]]; then
    echo "Blocked: daemon smoke script missing or not executable (scripts/daemon_smoke.sh)." >&2
    exit 1
  fi

  daemon_smoke_gate_dir="$REPORT_DIR/daemon-smoke"
  daemon_smoke_cmd=(
    ./scripts/daemon_smoke.sh
    --out "$daemon_smoke_gate_dir"
  )
  if is_truthy "${FLOWFORGE_DAEMON_SMOKE_SKIP_BUILD:-0}"; then
    daemon_smoke_cmd+=(--skip-build)
  fi
  if is_truthy "${FLOWFORGE_DAEMON_SMOKE_SKIP_HTTP_PROBES:-0}"; then
    daemon_smoke_cmd+=(--skip-http-probes)
  fi

  if ! "${daemon_smoke_cmd[@]}" >/dev/null; then
    echo "Blocked: daemon smoke gate failed while evaluating release readiness." >&2
    exit 1
  fi

  daemon_summary="$daemon_smoke_gate_dir/summary.tsv"
  if [[ ! -f "$daemon_summary" ]] || ! grep -q $'^overall_status\tPASS$' "$daemon_summary"; then
    echo "Blocked: daemon smoke gate did not report PASS summary status." >&2
    exit 1
  fi

  daemon_smoke_gate_status="PASS"
fi

cat > "$REPORT_FILE" <<EOF
# Release Checkpoint

Date: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

## Status

- Local verification gate: PASS
- Tracked secret/runtime legacy artifacts: PASS
- Legacy naming scan: PASS
- Operator docs present (runbook/pilot/release/rollback): PASS
- Cloud dependency readiness gate: $cloud_readiness_status
- Signed evidence bundle gate: $evidence_bundle_status
- Control-plane replay drill gate: $controlplane_replay_drill_status
- Control-plane replay retention prune: $controlplane_replay_retention_status
- Weekly SLO GREEN gate: $weekly_slo_green_gate_status
- MVP Phase-1 exit gate: $mvp_phase1_exit_gate_status
- Daemon smoke gate: $daemon_smoke_gate_status

## Readiness Evidence

- Ready endpoint: $readyz_url
- Ready payload artifact: ${readyz_artifact:-N/A}
- Evidence bundle artifact: ${evidence_bundle_dir:-N/A}
- Control-plane replay drill artifact: ${controlplane_replay_drill_dir:-N/A}
- Control-plane replay retention artifact: ${controlplane_replay_retention_dir:-N/A}
- Weekly SLO artifact: ${weekly_slo_review_dir:-N/A}
- Weekly SLO error budget status: ${weekly_slo_error_budget_status:-N/A}
- MVP Phase-1 exit gate artifact: ${mvp_phase1_exit_gate_dir:-N/A}
- Daemon smoke artifact: ${daemon_smoke_gate_dir:-N/A}

## Ready

Release checkpoint is complete.
EOF

echo "Release checkpoint complete: $REPORT_FILE"
