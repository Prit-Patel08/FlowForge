#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmp_dir="$(mktemp -d)"
stop_mock_ready_server() {
  if [[ -n "${server_pid:-}" ]]; then
    kill "$server_pid" >/dev/null 2>&1 || true
    wait "$server_pid" 2>/dev/null || true
    unset server_pid
  fi
}

cleanup() {
  stop_mock_ready_server
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

copy_repo_script() {
  mkdir -p "$tmp_dir/scripts" "$tmp_dir/docs"
  cp "$ROOT_DIR/scripts/release_checkpoint.sh" "$tmp_dir/scripts/release_checkpoint.sh"
  chmod +x "$tmp_dir/scripts/release_checkpoint.sh"
}

write_stub_verify_local() {
  cat > "$tmp_dir/scripts/verify_local.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "stub verify_local pass"
EOF
  chmod +x "$tmp_dir/scripts/verify_local.sh"
}

write_stub_controlplane_replay_drill() {
  cat > "$tmp_dir/scripts/controlplane_replay_drill.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
out_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out_dir="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$out_dir" ]]; then
  echo "missing --out" >&2
  exit 1
fi
mkdir -p "$out_dir"
cat > "$out_dir/summary.tsv" <<'TSV'
overall_status	PASS
TSV
echo "stub replay drill pass"
EOF
  chmod +x "$tmp_dir/scripts/controlplane_replay_drill.sh"
}

write_stub_controlplane_replay_retention() {
  cat > "$tmp_dir/scripts/controlplane_replay_retention.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
out_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out_dir="${2:-}"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done
if [[ -z "$out_dir" ]]; then
  echo "missing --out" >&2
  exit 1
fi
mkdir -p "$out_dir"
cat > "$out_dir/summary.tsv" <<'TSV'
status	PASS
TSV
echo "stub replay retention pass"
EOF
  chmod +x "$tmp_dir/scripts/controlplane_replay_retention.sh"
}

write_stub_slo_weekly_review() {
  cat > "$tmp_dir/scripts/slo_weekly_review.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
out_dir=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out_dir="${2:-}"
      shift 2
      ;;
    --days|--api-base|--db|--replay-max-rows|--replay-spike-yellow|--replay-spike-red|--conflict-spike-yellow|--conflict-spike-red)
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [[ -z "$out_dir" ]]; then
  echo "missing --out" >&2
  exit 1
fi

if [[ "${FLOWFORGE_TEST_SLO_REVIEW_EXIT_CODE:-0}" != "0" ]]; then
  exit "${FLOWFORGE_TEST_SLO_REVIEW_EXIT_CODE}"
fi

mkdir -p "$out_dir"
status="${FLOWFORGE_TEST_SLO_ERROR_BUDGET_STATUS:-GREEN}"
cat > "$out_dir/summary.tsv" <<TSV
error_budget_status	${status}
TSV
echo "stub slo weekly review ${status}"
EOF
  chmod +x "$tmp_dir/scripts/slo_weekly_review.sh"
}

write_required_docs() {
  for doc in RUNBOOK.md WEEK1_PILOT.md RELEASE_CHECKLIST.md ROLLBACK_CHECKLIST.md; do
    echo "# ${doc}" > "$tmp_dir/docs/$doc"
  done
  cat > "$tmp_dir/README.md" <<'EOF'
# FlowForge
EOF
}

init_temp_git_repo() {
  (
    cd "$tmp_dir"
    git init -q
    git add .
  )
}

run_case_no_cloud_required() {
  (
    cd "$tmp_dir"
    ./scripts/release_checkpoint.sh "$tmp_dir/out-case-1" >/dev/null
  )
  rg -q "Cloud dependency readiness gate: SKIPPED" "$tmp_dir/out-case-1/checkpoint.md"
}

run_case_replay_required_missing_key_fails() {
  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1 ./scripts/release_checkpoint.sh "$tmp_dir/out-case-replay-missing-key"
  ) >"$tmp_dir/case_replay_missing_key.stdout.log" 2>"$tmp_dir/case_replay_missing_key.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "replay-missing-key case expected failure but passed" >&2
    exit 1
  fi
  rg -q "replay drill gate requires FLOWFORGE_API_KEY" "$tmp_dir/case_replay_missing_key.stderr.log"
}

run_case_replay_required_passes() {
  (
    cd "$tmp_dir"
    FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1 FLOWFORGE_API_KEY="test-key" \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-replay-pass" >/dev/null
  )
  rg -q "Control-plane replay drill gate: PASS" "$tmp_dir/out-case-replay-pass/checkpoint.md"
  test -f "$tmp_dir/out-case-replay-pass/controlplane-replay-drill/summary.tsv"
}

run_case_replay_retention_enabled_passes() {
  (
    cd "$tmp_dir"
    FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION=1 \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-retention-pass" >/dev/null
  )
  rg -q "Control-plane replay retention prune: PASS" "$tmp_dir/out-case-retention-pass/checkpoint.md"
  test -f "$tmp_dir/out-case-retention-pass/controlplane-replay-retention/summary.tsv"
}

run_case_weekly_slo_green_gate_passes() {
  (
    cd "$tmp_dir"
    FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 FLOWFORGE_TEST_SLO_ERROR_BUDGET_STATUS=GREEN \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-slo-green-pass" >/dev/null
  )
  rg -q "Weekly SLO GREEN gate: PASS" "$tmp_dir/out-case-slo-green-pass/checkpoint.md"
  rg -q "Weekly SLO error budget status: GREEN" "$tmp_dir/out-case-slo-green-pass/checkpoint.md"
  test -f "$tmp_dir/out-case-slo-green-pass/slo-weekly-review/summary.tsv"
}

run_case_weekly_slo_green_gate_yellow_fails() {
  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 FLOWFORGE_TEST_SLO_ERROR_BUDGET_STATUS=YELLOW \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-slo-yellow-fail"
  ) >"$tmp_dir/case_slo_yellow.stdout.log" 2>"$tmp_dir/case_slo_yellow.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "slo-yellow case expected failure but passed" >&2
    exit 1
  fi
  rg -Fq "weekly SLO gate requires error_budget_status=GREEN" "$tmp_dir/case_slo_yellow.stderr.log"
}

run_case_weekly_slo_green_gate_script_failure_fails() {
  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 FLOWFORGE_TEST_SLO_REVIEW_EXIT_CODE=11 \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-slo-script-fail"
  ) >"$tmp_dir/case_slo_script_fail.stdout.log" 2>"$tmp_dir/case_slo_script_fail.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "slo-script-fail case expected failure but passed" >&2
    exit 1
  fi
  rg -Fq "weekly SLO review script failed while evaluating release gate" "$tmp_dir/case_slo_script_fail.stderr.log"
}

run_case_cloud_required_unreachable_fails() {
  local unreachable_port
  unreachable_port="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"

  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE="http://127.0.0.1:${unreachable_port}" \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-2"
  ) >"$tmp_dir/case2.stdout.log" 2>"$tmp_dir/case2.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "case2 expected failure but passed" >&2
    exit 1
  fi
  rg -q "Blocked: strict cloud readiness is enabled" "$tmp_dir/case2.stderr.log"
}

start_mock_ready_server() {
  local body="$1"
  local port_file="$tmp_dir/mock_readyz_port.txt"
  rm -f "$port_file"

  cat > "$tmp_dir/mock_readyz_server.py" <<EOF
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

BODY = os.environ["RESPONSE_BODY"].encode("utf-8")
PORT_FILE = os.environ["PORT_FILE"]

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/readyz":
            self.send_response(404)
            self.end_headers()
            return
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(BODY)))
        self.end_headers()
        self.wfile.write(BODY)

    def log_message(self, format, *args):
        return

server = HTTPServer(("127.0.0.1", 0), Handler)
with open(PORT_FILE, "w", encoding="utf-8") as f:
    f.write(str(server.server_port))
server.serve_forever()
EOF
  PORT_FILE="$port_file" RESPONSE_BODY="$body" python3 "$tmp_dir/mock_readyz_server.py" >/dev/null 2>&1 &
  server_pid=$!

  server_port=""
  for _ in $(seq 1 40); do
    if [[ -s "$port_file" ]]; then
      server_port="$(cat "$port_file")"
      break
    fi
    sleep 0.05
  done

  if [[ -z "$server_port" ]]; then
    echo "failed to start mock readyz server" >&2
    stop_mock_ready_server
    exit 1
  fi
}

run_case_cloud_required_passes() {
  start_mock_ready_server '{"status":"ready","cloud_dependencies_required":true,"checks":{"database":{"healthy":true}}}'

  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE="http://127.0.0.1:${server_port}" \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-3" >/dev/null
  )
  rg -q "Cloud dependency readiness gate: PASS" "$tmp_dir/out-case-3/checkpoint.md"
  rg -q '"status":"ready"' "$tmp_dir/out-case-3/readyz.json"

  stop_mock_ready_server
}

run_case_cloud_required_mismatch_fails() {
  start_mock_ready_server '{"status":"ready","cloud_dependencies_required":false,"checks":{"database":{"healthy":true}}}'

  set +e
  (
    cd "$tmp_dir"
    FLOWFORGE_CLOUD_DEPS_REQUIRED=1 FLOWFORGE_API_BASE="http://127.0.0.1:${server_port}" \
      ./scripts/release_checkpoint.sh "$tmp_dir/out-case-4"
  ) >"$tmp_dir/case4.stdout.log" 2>"$tmp_dir/case4.stderr.log"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "case4 expected failure but passed" >&2
    exit 1
  fi
  rg -q "cloud_dependencies_required=true" "$tmp_dir/case4.stderr.log"

  stop_mock_ready_server
}

copy_repo_script
write_stub_verify_local
write_stub_controlplane_replay_drill
write_stub_controlplane_replay_retention
write_stub_slo_weekly_review
write_required_docs
init_temp_git_repo

run_case_no_cloud_required
run_case_replay_required_missing_key_fails
run_case_replay_required_passes
run_case_replay_retention_enabled_passes
run_case_weekly_slo_green_gate_passes
run_case_weekly_slo_green_gate_yellow_fails
run_case_weekly_slo_green_gate_script_failure_fails
run_case_cloud_required_unreachable_fails
run_case_cloud_required_passes
run_case_cloud_required_mismatch_fails

echo "release checkpoint contract tests passed"
