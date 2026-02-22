#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${1:-pilot_artifacts/tuning-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$ARTIFACT_DIR"

THRESHOLDS=(30 40 50 60)

run_one() {
  local profile="$1"
  local threshold="$2"
  local script_path="$3"
  local outfile="$ARTIFACT_DIR/${profile}_cpu${threshold}.log"

  set +e
  ./flowforge run --max-cpu "$threshold" -- python3 "$script_path" >"$outfile" 2>&1
  local code=$?
  set -e

  local loop_detected="no"
  if rg -q "LOOP DETECTED" "$outfile"; then
    loop_detected="yes"
  fi

  printf "%s,%s,%s,%s\n" "$profile" "$threshold" "$code" "$loop_detected" >> "$ARTIFACT_DIR/results.csv"
}

echo "profile,threshold,exit_code,loop_detected" > "$ARTIFACT_DIR/results.csv"

for t in "${THRESHOLDS[@]}"; do
  run_one "runaway" "$t" "demo/pilot/runaway_worker.py"
done

for t in "${THRESHOLDS[@]}"; do
  run_one "bursty" "$t" "demo/pilot/bursty_worker.py"
done

{
  echo "# Detection Tuning Results"
  echo
  echo "| Profile | Threshold | Exit Code | Loop Detected |"
  echo "|---|---:|---:|---|"
  tail -n +2 "$ARTIFACT_DIR/results.csv" | while IFS=, read -r profile threshold code detected; do
    echo "| $profile | $threshold | $code | $detected |"
  done
  echo
  echo "Target:"
  echo "- Runaway should be detected (\`loop_detected=yes\`) at chosen threshold."
  echo "- Bursty should not be detected (\`loop_detected=no\`) at chosen threshold."
} > "$ARTIFACT_DIR/summary.md"

echo "Tuning complete: $ARTIFACT_DIR/summary.md"
