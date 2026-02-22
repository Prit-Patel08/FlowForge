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

run_missing_table_case() {
  local db_path="$tmp_dir/missing-table.db"
  local out_dir="$tmp_dir/out-missing-table"
  sqlite3 "$db_path" 'CREATE TABLE IF NOT EXISTS something_else(id INTEGER PRIMARY KEY);'

  ./scripts/controlplane_replay_retention.sh --db "$db_path" --out "$out_dir" >/dev/null

  test -f "$out_dir/summary.tsv"
  assert_file_contains "$out_dir/summary.tsv" '^status	SKIPPED$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_before	0$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_after	0$'
}

run_retention_and_cap_case() {
  local db_path="$tmp_dir/with-table.db"
  local out_dir="$tmp_dir/out-with-table"

  sqlite3 "$db_path" <<'SQL'
CREATE TABLE control_plane_replays (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  idempotency_key TEXT NOT NULL,
  endpoint TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  response_status INTEGER NOT NULL,
  response_body TEXT NOT NULL,
  replay_count INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO control_plane_replays(idempotency_key, endpoint, request_hash, response_status, response_body, last_seen_at)
VALUES
  ('k1','POST /x','h1',200,'{}',datetime('now','-45 day')),
  ('k2','POST /x','h2',200,'{}',datetime('now','-15 day')),
  ('k3','POST /x','h3',200,'{}',datetime('now','-1 day')),
  ('k4','POST /x','h4',200,'{}',datetime('now'));
SQL

  ./scripts/controlplane_replay_retention.sh \
    --db "$db_path" \
    --retention-days 30 \
    --max-rows 2 \
    --out "$out_dir" >/dev/null

  test -f "$out_dir/summary.tsv"
  assert_file_contains "$out_dir/summary.tsv" '^status	PASS$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_before	4$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_after	2$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_deleted_by_age	1$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_deleted_by_cap	1$'
  assert_file_contains "$out_dir/summary.tsv" '^rows_deleted	2$'
}

run_invalid_arg_case() {
  local out_file="$tmp_dir/invalid.out.log"
  local err_file="$tmp_dir/invalid.err.log"

  set +e
  ./scripts/controlplane_replay_retention.sh --retention-days nope >"$out_file" 2>"$err_file"
  local rc=$?
  set -e
  assert_nonzero_exit "$rc" "invalid retention-days"
  assert_file_contains "$err_file" '--retention-days must be an integer'
}

run_missing_db_case() {
  local out_file="$tmp_dir/missing-db.out.log"
  local err_file="$tmp_dir/missing-db.err.log"

  set +e
  ./scripts/controlplane_replay_retention.sh --db "$tmp_dir/does-not-exist.db" >"$out_file" 2>"$err_file"
  local rc=$?
  set -e
  assert_nonzero_exit "$rc" "missing db"
  assert_file_contains "$err_file" '^Database file not found: '
}

run_missing_table_case
run_retention_and_cap_case
run_invalid_arg_case
run_missing_db_case

echo "control-plane replay retention contract tests passed"
