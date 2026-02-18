#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMMANDS_FILE="${1:-pilot_commands.txt}"
ARTIFACT_DIR="${2:-pilot_artifacts/week2-real-$(date +%Y%m%d-%H%M%S)}"

if [[ ! -f "$COMMANDS_FILE" ]]; then
  echo "Commands file not found: $COMMANDS_FILE"
  echo "Use scripts/pilot_commands.example.txt as template."
  exit 1
fi

mkdir -p "$ARTIFACT_DIR"
RESULTS_CSV="$ARTIFACT_DIR/results.csv"
echo "name,max_cpu,exit_code,loop_detected,expected" > "$RESULTS_CSV"

run_case() {
  local name="$1"
  local threshold="$2"
  local expected="$3"
  local command_text="$4"
  local log_file="$ARTIFACT_DIR/${name}.log"

  echo "== Real case: $name =="
  echo "Command: $command_text"

  # Split command on spaces; keep commands in template simple and explicit.
  read -r -a cmd_arr <<< "$command_text"
  if [[ ${#cmd_arr[@]} -eq 0 ]]; then
    echo "Skipping empty command for case '$name'"
    return
  fi

  set +e
  ./flowforge run --max-cpu "$threshold" -- "${cmd_arr[@]}" >"$log_file" 2>&1
  local code=$?
  set -e

  local detected="no"
  if rg -q "LOOP DETECTED" "$log_file"; then
    detected="yes"
  fi

  echo "${name},${threshold},${code},${detected},${expected}" >> "$RESULTS_CSV"
}

while IFS='|' read -r name threshold expected command_text; do
  [[ -z "${name// }" ]] && continue
  [[ "$name" =~ ^# ]] && continue
  run_case "$name" "$threshold" "$expected" "$command_text"
done < "$COMMANDS_FILE"

FLOWFORGE_DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}" python3 - <<'PY' > "$ARTIFACT_DIR/incidents_snapshot.txt"
import os
import sqlite3

db = os.getenv("FLOWFORGE_DB_PATH", "flowforge.db")
conn = sqlite3.connect(db)
cur = conn.cursor()
cur.execute(
    """
    SELECT id, timestamp, exit_reason, max_cpu, reason
    FROM incidents
    ORDER BY id DESC
    LIMIT 20
    """
)
rows = cur.fetchall()
print("id | timestamp | exit_reason | max_cpu | reason")
for r in rows:
    print(f"{r[0]} | {r[1]} | {r[2]} | {r[3]:.1f} | {r[4] or ''}")
conn.close()
PY

{
  echo "# Week 2 Real Workload Pilot"
  echo
  echo "| Case | Max CPU | Exit Code | Loop Detected | Expected |"
  echo "|---|---:|---:|---|---|"
  tail -n +2 "$RESULTS_CSV" | while IFS=, read -r name threshold code detected expected; do
    echo "| $name | $threshold | $code | $detected | $expected |"
  done
  echo
  echo "Artifacts:"
  echo "- results: \`$RESULTS_CSV\`"
  echo "- incident snapshot: \`$ARTIFACT_DIR/incidents_snapshot.txt\`"
} > "$ARTIFACT_DIR/summary.md"

echo "Week 2 real pilot complete: $ARTIFACT_DIR/summary.md"
