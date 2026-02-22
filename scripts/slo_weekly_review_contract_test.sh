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
  if ! grep -Eq -- "$pattern" "$file_path"; then
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

write_curl_stub() {
  mkdir -p "$tmp_dir/bin"
  cat >"$tmp_dir/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

url=""
for arg in "$@"; do
  case "$arg" in
    http://*|https://*)
      url="$arg"
      ;;
  esac
done

if [[ -z "$url" ]]; then
  echo "curl stub missing URL" >&2
  exit 2
fi

if [[ -n "${MOCK_CURL_FAIL_CONTAINS:-}" ]] && [[ "$url" == *"${MOCK_CURL_FAIL_CONTAINS}"* ]]; then
  echo "mock curl failure for ${url}" >&2
  exit 22
fi

case "$url" in
  */healthz)
    printf '{"status":"ok"}\n'
    ;;
  */readyz)
    printf '{"status":"ready"}\n'
    ;;
  */metrics)
    cat <<'METRICS'
flowforge_uptime_seconds 7200
flowforge_process_kill_total 3
flowforge_process_restart_total 1
flowforge_controlplane_idempotent_replay_total 9
flowforge_controlplane_idempotency_conflict_total 0
flowforge_controlplane_replay_rows 9
flowforge_controlplane_replay_oldest_age_seconds 1800
METRICS
    ;;
  */timeline)
    printf '[]\n'
    ;;
  */v1/ops/controlplane/replay/history*)
    printf '{"days":7,"points":[]}\n'
    ;;
  *)
    echo "unexpected url: $url" >&2
    exit 22
    ;;
esac
EOF
  chmod +x "$tmp_dir/bin/curl"
}

write_base_events_schema() {
  local db_path="$1"
  sqlite3 "$db_path" <<'SQL'
CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL UNIQUE,
  run_id TEXT NOT NULL,
  incident_id TEXT,
  event_type TEXT NOT NULL,
  title TEXT NOT NULL,
  confidence_score REAL NOT NULL DEFAULT 1.0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
SQL
}

seed_replay_events() {
  local db_path="$1"
  local count="$2"
  local offset="$3"
  for _ in $(seq 1 "$count"); do
    sqlite3 "$db_path" "
INSERT INTO events(event_id, run_id, incident_id, event_type, title, confidence_score, created_at, timestamp)
VALUES(lower(hex(randomblob(16))), 'run-contract', '', 'audit', 'IDEMPOTENT_REPLAY', 1.0, datetime('now', '${offset}'), datetime('now', '${offset}'));
"
  done
}

run_yellow_spike_case() {
  local db_path="$tmp_dir/yellow.db"
  local out_dir="$tmp_dir/out-yellow"
  write_base_events_schema "$db_path"
  seed_replay_events "$db_path" 4 "-1 day"
  seed_replay_events "$db_path" 6 "-0 day"

  PATH="$tmp_dir/bin:$PATH" ./scripts/slo_weekly_review.sh --db "$db_path" --out "$out_dir" --days 7 >/dev/null

  test -f "$out_dir/summary.tsv"
  test -f "$out_dir/slo_weekly_report.md"
  test -f "$out_dir/replay_daily_trend.tsv"
  test -f "$out_dir/replay_history.json"

  assert_file_contains "$out_dir/summary.tsv" $'^replay_spike_budget_trigger\tYELLOW$'
  assert_file_contains "$out_dir/summary.tsv" $'^slo_c_replay_spike_status\tWARN$'
  assert_file_contains "$out_dir/summary.tsv" $'^error_budget_status\tYELLOW$'
  assert_file_contains "$out_dir/summary.tsv" $'^latest_replay_events\t6$'
  assert_file_contains "$out_dir/replay_daily_trend.tsv" $'^day\treplay_events\tconflict_events$'
  assert_file_contains "$out_dir/slo_weekly_report.md" '^## Replay Trend Analysis$'
  assert_file_contains "$out_dir/slo_weekly_report.md" 'Replay/conflict daily spike'
}

run_red_spike_case() {
  local db_path="$tmp_dir/red.db"
  local out_dir="$tmp_dir/out-red"
  local out_log="$tmp_dir/red.stdout.log"
  local err_log="$tmp_dir/red.stderr.log"
  write_base_events_schema "$db_path"
  seed_replay_events "$db_path" 1 "-1 day"
  seed_replay_events "$db_path" 12 "-0 day"

  set +e
  PATH="$tmp_dir/bin:$PATH" ./scripts/slo_weekly_review.sh --db "$db_path" --out "$out_dir" --days 7 --fail-on-breach >"$out_log" 2>"$err_log"
  local rc=$?
  set -e
  assert_nonzero_exit "$rc" "red spike fail-on-breach"

  assert_file_contains "$out_dir/summary.tsv" $'^replay_spike_budget_trigger\tRED$'
  assert_file_contains "$out_dir/summary.tsv" $'^slo_c_replay_spike_status\tFAIL$'
  assert_file_contains "$out_dir/summary.tsv" $'^error_budget_status\tRED$'
  assert_file_contains "$err_log" 'Error budget status is RED'
}

run_invalid_threshold_case() {
  local db_path="$tmp_dir/invalid.db"
  local out_log="$tmp_dir/invalid.stdout.log"
  local err_log="$tmp_dir/invalid.stderr.log"
  write_base_events_schema "$db_path"

  set +e
  PATH="$tmp_dir/bin:$PATH" ./scripts/slo_weekly_review.sh --db "$db_path" --replay-spike-yellow 11 --replay-spike-red 10 >"$out_log" 2>"$err_log"
  local rc=$?
  set -e
  assert_nonzero_exit "$rc" "invalid spike threshold ordering"
  assert_file_contains "$err_log" '--replay-spike-yellow must be <= --replay-spike-red.'
}

write_curl_stub
run_yellow_spike_case
run_red_spike_case
run_invalid_threshold_case

echo "slo weekly review contract tests passed"
