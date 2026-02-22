#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

assert_file_contains() {
  local file_path="$1"
  local pattern="$2"
  if ! rg -q -- "$pattern" "$file_path"; then
    echo "assertion failed: expected pattern '$pattern' in $file_path" >&2
    exit 1
  fi
}

assert_nonzero_exit() {
  local rc="$1"
  local label="$2"
  if [[ "$rc" -eq 0 ]]; then
    echo "assertion failed: expected non-zero exit for ${label}" >&2
    exit 1
  fi
}

write_flowforge_stub() {
  mkdir -p "$tmp_dir/bin"
  cat >"$tmp_dir/bin/flowforge-stub" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_file="${FLOWFORGE_STUB_STATE_FILE:?missing FLOWFORGE_STUB_STATE_FILE}"

if [[ ! -f "$state_file" ]]; then
  echo "stopped" >"$state_file"
fi

read_state() {
  cat "$state_file"
}

write_state() {
  local next="$1"
  printf '%s\n' "$next" >"$state_file"
}

daemon_status_json() {
  local st
  st="$(read_state)"
  if [[ "$st" == "running" ]]; then
    cat <<JSON
{
  "status": "running",
  "pid": 99999,
  "api_healthy": true,
  "port": "8080",
  "runtime_dir": "/tmp/flowforge-daemon",
  "pid_file": "/tmp/flowforge-daemon/flowforge-daemon.pid",
  "log_file": "/tmp/flowforge-daemon/flowforge-daemon.log",
  "state_present": true,
  "started_at": "2026-02-22T00:00:00Z"
}
JSON
  else
    cat <<JSON
{
  "status": "stopped",
  "pid": 0,
  "api_healthy": false,
  "port": "8080",
  "runtime_dir": "/tmp/flowforge-daemon",
  "pid_file": "/tmp/flowforge-daemon/flowforge-daemon.pid",
  "log_file": "/tmp/flowforge-daemon/flowforge-daemon.log",
  "state_present": true
}
JSON
  fi
}

cmd="${1:-}"
case "$cmd" in
  daemon)
    sub="${2:-}"
    shift 2
    case "$sub" in
      start)
        if [[ "${FLOWFORGE_STUB_FAIL_START:-0}" == "1" ]]; then
          echo "stub start failure" >&2
          exit 1
        fi
        if [[ "$(read_state)" == "running" ]]; then
          echo "FlowForge daemon already running (pid=99999) on http://127.0.0.1:8080"
        else
          write_state "running"
          echo "FlowForge daemon started (pid=99999) on http://127.0.0.1:8080"
        fi
        ;;
      stop)
        write_state "stopped"
        echo "FlowForge daemon stopped."
        ;;
      status)
        if [[ " $* " == *" --json "* ]]; then
          daemon_status_json
        else
          echo "Status: $(read_state)"
        fi
        ;;
      logs)
        echo "stub daemon log line"
        ;;
      *)
        echo "unsupported daemon subcommand: $sub" >&2
        exit 2
        ;;
    esac
    ;;
  dashboard)
    shift
    foreground=0
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --foreground)
          foreground=1
          ;;
      esac
      shift
    done

    if [[ "$foreground" == "1" ]]; then
      trap 'exit 0' TERM INT
      while true; do
        sleep 1
      done
    fi

    if [[ "$(read_state)" == "running" ]]; then
      echo "FlowForge daemon already running (pid=99999)"
    else
      write_state "running"
      echo "FlowForge daemon started (pid=99999)"
    fi
    ;;
  *)
    echo "unsupported command: $cmd" >&2
    exit 2
    ;;
esac
EOF
  chmod +x "$tmp_dir/bin/flowforge-stub"
}

run_success_case() {
  local out_dir="$tmp_dir/out-pass"
  echo "stopped" >"$tmp_dir/state"
  FLOWFORGE_STUB_STATE_FILE="$tmp_dir/state" \
    FLOWFORGE_BIN="$tmp_dir/bin/flowforge-stub" \
    ./scripts/daemon_smoke.sh \
      --out "$out_dir" \
      --skip-build \
      --skip-http-probes >/dev/null

  test -f "$out_dir/summary.tsv"
  test -f "$out_dir/summary.md"
  assert_file_contains "$out_dir/summary.tsv" '^overall_status	PASS$'
  assert_file_contains "$out_dir/summary.tsv" '^skip_http_probes	1$'
  assert_file_contains "$out_dir/summary.tsv" '^step	Build FlowForge binary	SKIPPED	-$'
  assert_file_contains "$out_dir/summary.tsv" '^step	Start daemon	PASS	'
}

run_start_failure_case() {
  local out_dir="$tmp_dir/out-fail-start"
  local out_log="$tmp_dir/fail-start.stdout.log"
  local err_log="$tmp_dir/fail-start.stderr.log"
  echo "stopped" >"$tmp_dir/state"

  set +e
  FLOWFORGE_STUB_STATE_FILE="$tmp_dir/state" \
    FLOWFORGE_STUB_FAIL_START=1 \
    FLOWFORGE_BIN="$tmp_dir/bin/flowforge-stub" \
    ./scripts/daemon_smoke.sh \
      --out "$out_dir" \
      --skip-build \
      --skip-http-probes >"$out_log" 2>"$err_log"
  local rc=$?
  set -e

  assert_nonzero_exit "$rc" "daemon-smoke start failure"
  test -f "$out_dir/summary.tsv"
  assert_file_contains "$out_dir/summary.tsv" '^overall_status	FAIL$'
  assert_file_contains "$out_dir/summary.tsv" '^step	Start daemon	FAIL	'
}

write_flowforge_stub
run_success_case
run_start_failure_case

echo "daemon smoke contract tests passed"
