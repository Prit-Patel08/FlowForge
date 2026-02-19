#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

OUT_DIR="${1:-pilot_artifacts/corpus-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$OUT_DIR"

run_case() {
  local name="$1"
  local expected_exit="$2"
  local min_duration="$3"
  local max_duration="$4"
  shift
  shift
  shift
  shift
  local log_file="$OUT_DIR/${name}.log"
  local started_at
  local ended_at
  local duration_s
  echo "== Fixture: ${name} =="
  started_at="$(date +%s)"
  set +e
  python3 "$@" >"$log_file" 2>&1
  local code=$?
  set -e
  ended_at="$(date +%s)"
  duration_s=$((ended_at - started_at))
  echo "${name},${code},${duration_s},${expected_exit},${min_duration},${max_duration}" | tee -a "$OUT_DIR/results.csv"
}

assert_exit_shape() {
  local name="$1"
  local expected="$2"
  local actual
  actual="$(awk -F',' -v target="$name" '$1==target {print $2}' "$OUT_DIR/results.csv" | tail -n1)"
  if [[ -z "$actual" ]]; then
    echo "Missing result for ${name}" >&2
    return 1
  fi
  if [[ "$expected" == "zero" && "$actual" != "0" ]]; then
    echo "Expected ${name} to exit 0, got ${actual}" >&2
    return 1
  fi
  if [[ "$expected" == "nonzero" && "$actual" == "0" ]]; then
    echo "Expected ${name} to exit non-zero, got 0" >&2
    return 1
  fi
}

assert_runtime_window() {
  local name="$1"
  local min_expected="$2"
  local max_expected="$3"
  local actual
  actual="$(awk -F',' -v target="$name" '$1==target {print $3}' "$OUT_DIR/results.csv" | tail -n1)"
  if [[ -z "$actual" ]]; then
    echo "Missing duration for ${name}" >&2
    return 1
  fi
  if (( actual < min_expected || actual > max_expected )); then
    echo "Runtime regression for ${name}: duration ${actual}s is outside ${min_expected}s..${max_expected}s" >&2
    return 1
  fi
}

echo "name,exit_code,duration_s,expected_exit,min_duration_s,max_duration_s" > "$OUT_DIR/results.csv"

run_case "infinite_looper" "nonzero" "1" "15" test/fixtures/scripts/infinite_looper.py --timeout 2
run_case "memory_leaker" "nonzero" "1" "15" test/fixtures/scripts/memory_leaker.py --timeout 2
run_case "healthy_spike" "zero" "1" "30" test/fixtures/scripts/healthy_spike.py --timeout 20 --spike-seconds 2
run_case "zombie_spawner" "nonzero" "0" "15" test/fixtures/scripts/zombie_spawner.py --timeout 2

assert_exit_shape "infinite_looper" "nonzero"
assert_exit_shape "memory_leaker" "nonzero"
assert_exit_shape "healthy_spike" "zero"
assert_exit_shape "zombie_spawner" "nonzero"
assert_runtime_window "infinite_looper" 1 15
assert_runtime_window "memory_leaker" 1 15
assert_runtime_window "healthy_spike" 1 30
assert_runtime_window "zombie_spawner" 0 15

cat > "$OUT_DIR/summary.md" <<EOF
# Benchmark Corpus Run

- Output directory: \`$OUT_DIR\`
- Results: \`$OUT_DIR/results.csv\`
- Logs: \`$OUT_DIR/*.log\`

Expected shape:
- \`infinite_looper\`: non-zero (self-timeout)
- \`memory_leaker\`: non-zero (self-timeout)
- \`healthy_spike\`: zero exit (clean completion)
- \`zombie_spawner\`: non-zero (intentional parent crash)

Runtime regression thresholds (seconds):
- \`infinite_looper\`: 1-15
- \`memory_leaker\`: 1-15
- \`healthy_spike\`: 1-30
- \`zombie_spawner\`: 0-15
EOF

echo "Corpus benchmark complete: $OUT_DIR/summary.md"
