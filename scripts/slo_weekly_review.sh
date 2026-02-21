#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DAYS=7
API_BASE="${API_BASE:-http://127.0.0.1:8080}"
DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}"
OUT_DIR=""
FAIL_ON_BREACH=0

usage() {
  cat <<EOF
Usage: ./scripts/slo_weekly_review.sh [options]

Options:
  --days N             Window size in days (default: 7).
  --api-base URL       FlowForge API base URL (default: http://127.0.0.1:8080).
  --db PATH            SQLite DB path (default: FLOWFORGE_DB_PATH or flowforge.db).
  --out DIR            Output directory (default: pilot_artifacts/slo-weekly-<timestamp>).
  --fail-on-breach     Exit non-zero if error budget status is RED.
  -h, --help           Show help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --days)
      DAYS="${2:-}"
      shift 2
      ;;
    --api-base)
      API_BASE="${2:-}"
      shift 2
      ;;
    --db)
      DB_PATH="${2:-}"
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --fail-on-breach)
      FAIL_ON_BREACH=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ ! "$DAYS" =~ ^[0-9]+$ ]] || [[ "$DAYS" -le 0 ]]; then
  echo "--days must be a positive integer (got: $DAYS)" >&2
  exit 1
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/slo-weekly-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"

for cmd in sqlite3 curl awk; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
done

if [[ ! -f "$DB_PATH" ]]; then
  echo "Database file not found: $DB_PATH" >&2
  echo "Set FLOWFORGE_DB_PATH or pass --db PATH." >&2
  exit 1
fi

WINDOW="-${DAYS} day"
RUN_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

sql_scalar() {
  local query="$1"
  sqlite3 -batch -noheader "$DB_PATH" "$query"
}

EVENT_TIME_COL="created_at"
if [[ "$(sql_scalar "SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='created_at';")" -eq 0 ]]; then
  EVENT_TIME_COL="timestamp"
fi

num_le() {
  local left="$1"
  local right="$2"
  awk -v a="$left" -v b="$right" 'BEGIN { exit !(a <= b) }'
}

probe_failures=0

probe() {
  local endpoint="$1"
  local outfile="$2"
  if curl -fsS --max-time 4 "${API_BASE}${endpoint}" >"$outfile" 2>"$outfile.err"; then
    rm -f "$outfile.err"
    return 0
  fi
  probe_failures=$((probe_failures + 1))
  {
    echo "{\"error\":\"probe failed\",\"endpoint\":\"${endpoint}\"}"
    cat "$outfile.err" 2>/dev/null || true
  } >"$outfile"
  rm -f "$outfile.err"
  return 1
}

probe "/healthz" "$OUT_DIR/healthz.json" || true
probe "/readyz" "$OUT_DIR/readyz.json" || true
probe "/metrics" "$OUT_DIR/metrics.prom" || true
probe "/timeline" "$OUT_DIR/timeline.json" || true

TOTAL_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}');")"
INCIDENT_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='incident';")"
DECISION_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='decision';")"
AUDIT_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='audit';")"
POLICY_DRY_RUN_EVENTS="$(sql_scalar "SELECT COUNT(*) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND event_type='policy_dry_run';")"
DISTINCT_INCIDENTS="$(sql_scalar "SELECT COUNT(DISTINCT incident_id) FROM events WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}') AND incident_id IS NOT NULL AND incident_id != '';")"

DESTRUCTIVE_ACTIONS="$(sql_scalar "
SELECT COUNT(*) FROM events
WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND event_type='audit'
  AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE');
")"

ACTION_INCIDENTS="$(sql_scalar "
WITH action_incidents AS (
  SELECT DISTINCT incident_id
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND incident_id IS NOT NULL
    AND incident_id != ''
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE')
)
SELECT COUNT(*) FROM action_incidents;
")"

LOW_CONFIDENCE_ACTION_INCIDENTS="$(sql_scalar "
WITH action_incidents AS (
  SELECT DISTINCT incident_id
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND incident_id IS NOT NULL
    AND incident_id != ''
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE')
)
SELECT COUNT(*)
FROM events e
WHERE e.${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
  AND e.event_type='incident'
  AND e.incident_id IN (SELECT incident_id FROM action_incidents)
  AND e.confidence_score < 0.60;
")"

LOW_CONFIDENCE_RATIO="N/A"
if [[ "$ACTION_INCIDENTS" -gt 0 ]]; then
  LOW_CONFIDENCE_RATIO="$(awk -v low="$LOW_CONFIDENCE_ACTION_INCIDENTS" -v total="$ACTION_INCIDENTS" 'BEGIN { printf "%.4f", low/total }')"
fi

LATENCY_SAMPLE_COUNT="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
)
SELECT COUNT(*) FROM latencies;
")"

LATENCY_P95="N/A"
LATENCY_AVG="N/A"
if [[ "$LATENCY_SAMPLE_COUNT" -gt 0 ]]; then
  LATENCY_P95="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
),
ranked AS (
  SELECT latency_s,
         ROW_NUMBER() OVER (ORDER BY latency_s) AS rn,
         COUNT(*) OVER () AS n
  FROM latencies
)
SELECT printf('%.2f', latency_s)
FROM ranked
WHERE rn = ((n * 95 + 99) / 100)
LIMIT 1;
")"

  LATENCY_AVG="$(sql_scalar "
WITH period AS (
  SELECT incident_id, event_type, title, ${EVENT_TIME_COL} AS ts
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND incident_id IS NOT NULL
    AND incident_id != ''
),
first_decision AS (
  SELECT incident_id, MIN(ts) AS decision_at
  FROM period
  WHERE event_type='decision'
  GROUP BY incident_id
),
first_action AS (
  SELECT incident_id, MIN(ts) AS action_at
  FROM period
  WHERE event_type='audit'
    AND UPPER(title) IN ('AUTO_KILL','AUTO_RESTART','KILL','RESTART','TERMINATE','WATCHDOG_ALERT','WATCHDOG_WARN','WATCHDOG_CRITICAL')
  GROUP BY incident_id
),
latencies AS (
  SELECT (julianday(fa.action_at) - julianday(fd.decision_at)) * 86400.0 AS latency_s
  FROM first_decision fd
  JOIN first_action fa USING(incident_id)
  WHERE julianday(fa.action_at) >= julianday(fd.decision_at)
)
SELECT printf('%.2f', AVG(latency_s)) FROM latencies;
")"
fi

RESTART_STORM_VIOLATIONS="$(sql_scalar "
WITH restarts AS (
  SELECT
    run_id,
    strftime('%Y-%m-%d %H:', ${EVENT_TIME_COL}) || printf('%02d', (CAST(strftime('%M', ${EVENT_TIME_COL}) AS INTEGER) / 10) * 10) AS bucket_10m,
    COUNT(*) AS restart_count
  FROM events
  WHERE ${EVENT_TIME_COL} >= datetime('now', '${WINDOW}')
    AND event_type='audit'
    AND UPPER(title) = 'AUTO_RESTART'
  GROUP BY run_id, bucket_10m
)
SELECT COUNT(*) FROM restarts WHERE restart_count > 3;
")"

LATEST_EVENT_AGE_SECONDS="$(sql_scalar "
SELECT COALESCE(CAST((julianday('now') - julianday(MAX(${EVENT_TIME_COL}))) * 86400 AS INTEGER), -1)
FROM events;
")"

UPTIME_SECONDS="N/A"
KILL_TOTAL="N/A"
RESTART_TOTAL="N/A"
if [[ -f "$OUT_DIR/metrics.prom" ]]; then
  UPTIME_SECONDS="$(awk '/^flowforge_uptime_seconds /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  KILL_TOTAL="$(awk '/^flowforge_process_kill_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  RESTART_TOTAL="$(awk '/^flowforge_process_restart_total /{print $2}' "$OUT_DIR/metrics.prom" | tail -n1)"
  UPTIME_SECONDS="${UPTIME_SECONDS:-N/A}"
  KILL_TOTAL="${KILL_TOTAL:-N/A}"
  RESTART_TOTAL="${RESTART_TOTAL:-N/A}"
fi

slo_a_latency_status="NO_DATA"
slo_a_precision_status="NO_DATA"
slo_b_storm_status="PASS"
slo_c_api_status="FAIL"
slo_c_freshness_status="NO_DATA"

if [[ "$LATENCY_SAMPLE_COUNT" -gt 0 ]]; then
  if num_le "$LATENCY_P95" "15"; then
    slo_a_latency_status="PASS"
  else
    slo_a_latency_status="FAIL"
  fi
fi

if [[ "$ACTION_INCIDENTS" -gt 0 ]]; then
  if num_le "$LOW_CONFIDENCE_RATIO" "0.10"; then
    slo_a_precision_status="PASS"
  else
    slo_a_precision_status="FAIL"
  fi
fi

if [[ "$RESTART_STORM_VIOLATIONS" -gt 0 ]]; then
  slo_b_storm_status="FAIL"
fi

if [[ "$probe_failures" -eq 0 ]]; then
  slo_c_api_status="PASS"
fi

if [[ "$LATEST_EVENT_AGE_SECONDS" -ge 0 ]]; then
  if num_le "$LATEST_EVENT_AGE_SECONDS" "300"; then
    slo_c_freshness_status="PASS"
  else
    slo_c_freshness_status="FAIL"
  fi
fi

fail_count=0
for st in \
  "$slo_a_latency_status" \
  "$slo_a_precision_status" \
  "$slo_b_storm_status" \
  "$slo_c_api_status" \
  "$slo_c_freshness_status"; do
  if [[ "$st" == "FAIL" ]]; then
    fail_count=$((fail_count + 1))
  fi
done

ERROR_BUDGET_STATUS="GREEN"
if [[ "$fail_count" -eq 1 ]]; then
  ERROR_BUDGET_STATUS="YELLOW"
elif [[ "$fail_count" -ge 2 ]]; then
  ERROR_BUDGET_STATUS="RED"
fi

cat >"$OUT_DIR/summary.tsv" <<EOF
run_timestamp	${RUN_TS}
window_days	${DAYS}
db_path	${DB_PATH}
api_base	${API_BASE}
total_events	${TOTAL_EVENTS}
incident_events	${INCIDENT_EVENTS}
decision_events	${DECISION_EVENTS}
audit_events	${AUDIT_EVENTS}
policy_dry_run_events	${POLICY_DRY_RUN_EVENTS}
distinct_incidents	${DISTINCT_INCIDENTS}
destructive_actions	${DESTRUCTIVE_ACTIONS}
action_incidents	${ACTION_INCIDENTS}
low_confidence_action_incidents	${LOW_CONFIDENCE_ACTION_INCIDENTS}
low_confidence_ratio	${LOW_CONFIDENCE_RATIO}
latency_sample_count	${LATENCY_SAMPLE_COUNT}
latency_p95_seconds	${LATENCY_P95}
latency_avg_seconds	${LATENCY_AVG}
restart_storm_violations	${RESTART_STORM_VIOLATIONS}
latest_event_age_seconds	${LATEST_EVENT_AGE_SECONDS}
probe_failures	${probe_failures}
error_budget_status	${ERROR_BUDGET_STATUS}
EOF

cat >"$OUT_DIR/slo_weekly_report.md" <<EOF
# FlowForge Weekly SLO Review

- Generated: \`${RUN_TS}\`
- Window: last **${DAYS} day(s)**
- Database: \`${DB_PATH}\`
- API base: \`${API_BASE}\`

## SLO Scoreboard

| SLO Objective | Target | Observed | Status |
|---|---:|---:|---|
| Detection->Action latency p95 | <= 15s | ${LATENCY_P95} | ${slo_a_latency_status} |
| Low-confidence destructive actions ratio | <= 0.10 | ${LOW_CONFIDENCE_RATIO} | ${slo_a_precision_status} |
| Restart-storm 10m bucket violations | 0 | ${RESTART_STORM_VIOLATIONS} | ${slo_b_storm_status} |
| API probe failures (\`/healthz,/readyz,/metrics,/timeline\`) | 0 | ${probe_failures} | ${slo_c_api_status} |
| Latest event age | <= 300s | ${LATEST_EVENT_AGE_SECONDS}s | ${slo_c_freshness_status} |

## Error Budget Decision

- Current status: **${ERROR_BUDGET_STATUS}**

Policy:
- GREEN: continue planned roadmap work.
- YELLOW: prioritize reliability fixes; defer non-critical expansion this week.
- RED: feature freeze and start reliability sprint until status returns to GREEN.

## Activity Snapshot

- Total events: ${TOTAL_EVENTS}
- Distinct incidents: ${DISTINCT_INCIDENTS}
- Incident events: ${INCIDENT_EVENTS}
- Decision events: ${DECISION_EVENTS}
- Audit events: ${AUDIT_EVENTS}
- Policy dry-run events: ${POLICY_DRY_RUN_EVENTS}
- Destructive actions: ${DESTRUCTIVE_ACTIONS}
- Detection latency samples: ${LATENCY_SAMPLE_COUNT}
- Detection latency avg: ${LATENCY_AVG}
- FlowForge uptime (probe time): ${UPTIME_SECONDS}
- Process kill total (probe time): ${KILL_TOTAL}
- Process restart total (probe time): ${RESTART_TOTAL}

## Weekly Ritual Checklist

- [ ] Reliability lead reviewed this report in weekly ops meeting.
- [ ] Top 3 reliability risks identified and owners assigned.
- [ ] Error budget decision communicated to product/engineering.
- [ ] Corrective tasks added with due dates.
- [ ] Follow-up evidence linked in next week's report.

## Artifacts

- Summary TSV: \`${OUT_DIR}/summary.tsv\`
- API probes:
  - \`${OUT_DIR}/healthz.json\`
  - \`${OUT_DIR}/readyz.json\`
  - \`${OUT_DIR}/metrics.prom\`
  - \`${OUT_DIR}/timeline.json\`
EOF

echo "Weekly SLO review written:"
echo "- $OUT_DIR/slo_weekly_report.md"
echo "- $OUT_DIR/summary.tsv"

if [[ "$FAIL_ON_BREACH" -eq 1 ]] && [[ "$ERROR_BUDGET_STATUS" == "RED" ]]; then
  echo "Error budget status is RED (fail-on-breach enabled)." >&2
  exit 1
fi
