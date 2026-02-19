#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMMANDS_FILE="${1:-pilot_commands.txt}"
ARTIFACT_DIR="${2:-pilot_artifacts/week2-real-$(date +%Y%m%d-%H%M%S)}"
FAIL_ON_MISMATCH="${PILOT_FAIL_ON_MISMATCH:-0}"

if [[ ! -f "$COMMANDS_FILE" ]]; then
  echo "Commands file not found: $COMMANDS_FILE"
  echo "Use scripts/pilot_commands.example.txt as template."
  exit 1
fi

mkdir -p "$ARTIFACT_DIR"
PILOT_DB="$ARTIFACT_DIR/week2-pilot.db"
export FLOWFORGE_DB_PATH="$PILOT_DB"
rm -f "$PILOT_DB"
RESULTS_CSV="$ARTIFACT_DIR/results.csv"
echo "name,max_cpu,exit_code,loop_detected,expected,exit_reason,duration_s,decision_reason" > "$RESULTS_CSV"

parse_command_text() {
  local raw="$1"
  python3 - "$raw" <<'PY'
import shlex
import sys

raw = sys.argv[1]
parts = shlex.split(raw)
for part in parts:
    sys.stdout.buffer.write(part.encode("utf-8"))
    sys.stdout.buffer.write(b"\0")
PY
}

incident_max_id() {
  python3 - <<'PY'
import os
import sqlite3

db = os.getenv("FLOWFORGE_DB_PATH", "flowforge.db")
if not os.path.exists(db):
    print(0)
    raise SystemExit(0)

conn = sqlite3.connect(db)
cur = conn.cursor()
cur.execute("SELECT COALESCE(MAX(id), 0) FROM incidents")
row = cur.fetchone()
print(row[0] if row else 0)
conn.close()
PY
}

incident_assessment_after_id() {
  local after_id="$1"
  python3 - "$after_id" <<'PY'
import os
import sqlite3
import sys

after_id = int(sys.argv[1])
db = os.getenv("FLOWFORGE_DB_PATH", "flowforge.db")
if not os.path.exists(db):
    print("no||")
    raise SystemExit(0)

conn = sqlite3.connect(db)
cur = conn.cursor()
cur.execute(
    """
    SELECT id, COALESCE(exit_reason, ''), COALESCE(reason, '')
    FROM incidents
    WHERE id > ?
    ORDER BY id ASC
    """,
    (after_id,),
)
rows = cur.fetchall()
if not rows:
    print("no||")
    conn.close()
    raise SystemExit(0)

intervention = {"LOOP_DETECTED", "RESTART_TRIGGERED", "SAFETY_LIMIT_EXCEEDED"}
detected = "no"
best_exit = ""
best_reason = ""

for _id, exit_reason, reason in rows:
    if exit_reason in intervention:
        detected = "yes"
        best_exit = exit_reason
        best_reason = reason

if detected == "no":
    _id, exit_reason, reason = rows[-1]
    best_exit = exit_reason
    best_reason = reason

safe_reason = (best_reason or "").replace("|", "/")
print(f"{detected}|{best_exit}|{safe_reason}")
conn.close()
PY
}

run_case() {
  local name="$1"
  local threshold="$2"
  local expected="$3"
  local command_text="$4"
  local log_file="$ARTIFACT_DIR/${name}.log"
  local before_id
  local after_info
  local detected="no"
  local exit_reason=""
  local decision_reason=""
  local started_at
  local ended_at
  local duration_s

  echo "== Real case: $name =="
  echo "Command: $command_text"

  local -a cmd_arr=()
  while IFS= read -r -d '' token; do
    cmd_arr+=("$token")
  done < <(parse_command_text "$command_text")
  if [[ ${#cmd_arr[@]} -eq 0 ]]; then
    echo "Skipping empty command for case '$name'"
    return
  fi

  before_id="$(incident_max_id)"
  started_at="$(date +%s)"
  set +e
  ./flowforge run --max-cpu "$threshold" -- "${cmd_arr[@]}" >"$log_file" 2>&1
  local code=$?
  set -e
  ended_at="$(date +%s)"
  duration_s=$((ended_at - started_at))

  after_info="$(incident_assessment_after_id "$before_id")"
  if [[ -n "$after_info" ]]; then
    IFS='|' read -r detected exit_reason decision_reason <<< "$after_info"
  fi

  if [[ "$detected" == "no" ]] && rg -q "LOOP DETECTED|AUTO_KILL|RESTART_TRIGGERED|SAFETY CHOKE" "$log_file"; then
    detected="yes"
  fi

  echo "${name},${threshold},${code},${detected},${expected},${exit_reason},${duration_s},${decision_reason}" >> "$RESULTS_CSV"
}

while IFS='|' read -r name threshold expected command_text; do
  [[ -z "${name// }" ]] && continue
  [[ "$name" =~ ^# ]] && continue
  run_case "$name" "$threshold" "$expected" "$command_text"
done < "$COMMANDS_FILE"

false_positive_count=0
false_negative_count=0
total_cases=0

while IFS=, read -r name _threshold _exit_code detected expected _exit_reason _duration_s _decision_reason; do
  [[ "$name" == "name" ]] && continue
  total_cases=$((total_cases + 1))

  if [[ "$expected" == "no intervention" && "$detected" == "yes" ]]; then
    false_positive_count=$((false_positive_count + 1))
  fi

  if [[ "$expected" == "loop detection + termination" && "$detected" != "yes" ]]; then
    false_negative_count=$((false_negative_count + 1))
  fi
done < "$RESULTS_CSV"

status="PASS"
if [[ "$false_positive_count" -gt 0 || "$false_negative_count" -gt 0 ]]; then
  status="FAIL"
fi

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
  echo "| Case | Max CPU | Exit Code | Loop Detected | Exit Reason | Duration (s) | Expected |"
  echo "|---|---:|---:|---|---|---:|---|"
  tail -n +2 "$RESULTS_CSV" | while IFS=, read -r name threshold code detected expected exit_reason duration_s decision_reason; do
    echo "| $name | $threshold | $code | $detected | ${exit_reason:-none} | ${duration_s:-0} | $expected |"
  done
  echo
  echo "Result:"
  echo "- total cases: $total_cases"
  echo "- false positives: $false_positive_count"
  echo "- false negatives: $false_negative_count"
  echo "- status: **$status**"
  echo
  echo "Artifacts:"
  echo "- results: \`$RESULTS_CSV\`"
  echo "- pilot db: \`$PILOT_DB\`"
  echo "- incident snapshot: \`$ARTIFACT_DIR/incidents_snapshot.txt\`"
} > "$ARTIFACT_DIR/summary.md"

echo "Week 2 real pilot complete: $ARTIFACT_DIR/summary.md"

if [[ "$status" != "PASS" && "$FAIL_ON_MISMATCH" == "1" ]]; then
  echo "Week 2 real pilot failed expectation checks (status=$status)." >&2
  exit 1
fi
