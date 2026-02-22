#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DB_PATH="${FLOWFORGE_DB_PATH:-flowforge.db}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
MAX_ROWS="${MAX_ROWS:-50000}"
OUT_DIR=""

usage() {
  cat <<EOF
Usage: ./scripts/controlplane_replay_retention.sh [options]

Options:
  --db PATH              SQLite DB path (default: FLOWFORGE_DB_PATH or flowforge.db).
  --retention-days N     Delete rows older than N days (default: 30, <=0 disables age purge).
  --max-rows N           Keep newest N rows (default: 50000, <=0 disables row-cap purge).
  --out DIR              Output directory (default: pilot_artifacts/controlplane-retention-<timestamp>).
  -h, --help             Show help text.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --db)
      DB_PATH="${2:-}"
      shift 2
      ;;
    --retention-days)
      RETENTION_DAYS="${2:-}"
      shift 2
      ;;
    --max-rows)
      MAX_ROWS="${2:-}"
      shift 2
      ;;
    --out)
      OUT_DIR="${2:-}"
      shift 2
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

if [[ ! "$RETENTION_DAYS" =~ ^-?[0-9]+$ ]]; then
  echo "--retention-days must be an integer (got: $RETENTION_DAYS)" >&2
  exit 1
fi
if [[ ! "$MAX_ROWS" =~ ^-?[0-9]+$ ]]; then
  echo "--max-rows must be an integer (got: $MAX_ROWS)" >&2
  exit 1
fi

if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="pilot_artifacts/controlplane-retention-$(date +%Y%m%d-%H%M%S)"
fi
mkdir -p "$OUT_DIR"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "Missing required command: sqlite3" >&2
  exit 1
fi

if [[ ! -f "$DB_PATH" ]]; then
  echo "Database file not found: $DB_PATH" >&2
  exit 1
fi

table_exists="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='control_plane_replays';")"
if [[ "$table_exists" != "1" ]]; then
  echo "Table control_plane_replays not found in $DB_PATH; nothing to prune." >"$OUT_DIR/summary.md"
  cat >"$OUT_DIR/summary.tsv" <<EOF
db_path	${DB_PATH}
retention_days	${RETENTION_DAYS}
max_rows	${MAX_ROWS}
rows_before	0
rows_after	0
rows_deleted	0
status	SKIPPED
EOF
  echo "Control-plane replay retention skipped: $OUT_DIR/summary.md"
  exit 0
fi

rows_before="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
deleted_by_age=0
deleted_by_cap=0

if [[ "$RETENTION_DAYS" -gt 0 ]]; then
  before_age="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
  sqlite3 -batch "$DB_PATH" "DELETE FROM control_plane_replays WHERE last_seen_at < datetime('now', '-${RETENTION_DAYS} day');"
  after_age="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
  deleted_by_age=$((before_age - after_age))
fi

if [[ "$MAX_ROWS" -gt 0 ]]; then
  before_cap="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
  sqlite3 -batch "$DB_PATH" "DELETE FROM control_plane_replays WHERE id IN (SELECT id FROM control_plane_replays ORDER BY last_seen_at DESC, id DESC LIMIT -1 OFFSET ${MAX_ROWS});"
  after_cap="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
  deleted_by_cap=$((before_cap - after_cap))
fi

rows_after="$(sqlite3 -batch -noheader "$DB_PATH" "SELECT COUNT(*) FROM control_plane_replays;")"
rows_deleted=$((rows_before - rows_after))

cat >"$OUT_DIR/summary.tsv" <<EOF
db_path	${DB_PATH}
retention_days	${RETENTION_DAYS}
max_rows	${MAX_ROWS}
rows_before	${rows_before}
rows_after	${rows_after}
rows_deleted_by_age	${deleted_by_age}
rows_deleted_by_cap	${deleted_by_cap}
rows_deleted	${rows_deleted}
status	PASS
EOF

cat >"$OUT_DIR/summary.md" <<EOF
# Control-Plane Replay Retention Summary

- DB path: \`${DB_PATH}\`
- Retention days: \`${RETENTION_DAYS}\`
- Max rows: \`${MAX_ROWS}\`

## Result

- Rows before: ${rows_before}
- Rows after: ${rows_after}
- Deleted by age: ${deleted_by_age}
- Deleted by row cap: ${deleted_by_cap}
- Total deleted: ${rows_deleted}

## Artifact

- \`${OUT_DIR}/summary.tsv\`
EOF

echo "Control-plane replay retention complete: $OUT_DIR/summary.md"
