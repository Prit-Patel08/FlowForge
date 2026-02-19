#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

COMMANDS_FILE="${1:-pilot_commands.txt}"
RUN_ID="${2:-day-$(date +%Y%m%d-%H%M%S)}"
OUT_DIR="pilot_artifacts/soak-${RUN_ID}"

if [[ ! -f "$COMMANDS_FILE" ]]; then
  echo "Missing commands file: $COMMANDS_FILE"
  echo "Use scripts/pilot_commands.example.txt as a template."
  exit 1
fi

mkdir -p "$OUT_DIR"

echo "== Week 2 Soak Check =="
echo "Run ID: $RUN_ID"
echo "Output: $OUT_DIR"

./scripts/week2_real_pilot.sh "$COMMANDS_FILE" "$OUT_DIR/real-pilot"
./scripts/release_checkpoint.sh "$OUT_DIR/release-checkpoint"

RESULTS="$OUT_DIR/real-pilot/results.csv"

false_positive_count=0
false_negative_count=0
total_cases=0

while IFS=, read -r name threshold exit_code detected expected; do
  [[ "$name" == "name" ]] && continue
  total_cases=$((total_cases + 1))

  if [[ "$expected" == "no intervention" && "$detected" == "yes" ]]; then
    false_positive_count=$((false_positive_count + 1))
  fi

  if [[ "$expected" == "loop detection + termination" && "$detected" != "yes" ]]; then
    false_negative_count=$((false_negative_count + 1))
  fi
done < "$RESULTS"

status="PASS"
if [[ "$false_positive_count" -gt 0 || "$false_negative_count" -gt 0 ]]; then
  status="FAIL"
fi

cat > "$OUT_DIR/summary.md" <<EOF
# Week 2 Soak Check

- Run ID: \`$RUN_ID\`
- Total cases: $total_cases
- False positives: $false_positive_count
- False negatives: $false_negative_count
- Status: **$status**

Artifacts:
- Pilot summary: \`$OUT_DIR/real-pilot/summary.md\`
- Pilot raw results: \`$OUT_DIR/real-pilot/results.csv\`
- Release checkpoint: \`$OUT_DIR/release-checkpoint/checkpoint.md\`
EOF

echo "Soak check complete: $OUT_DIR/summary.md"
