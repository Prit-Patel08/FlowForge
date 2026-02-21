#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

REPORT_DIR="${1:-pilot_artifacts/release-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$REPORT_DIR"
REPORT_FILE="$REPORT_DIR/checkpoint.md"

check() {
  local name="$1"
  shift
  echo "[$name] $*"
  "$@"
}

check "verify-local" ./scripts/verify_local.sh --strict

if git ls-files | rg -q "flowforge\\.key|\\.flowforge_live|flowforge\\.db|sentry\\.key"; then
  echo "Blocked: secret/runtime artifacts still tracked." >&2
  exit 1
fi

if rg -n -i --hidden "quenvor|agent-sentry " -g '!.git/*' -g '!scripts/release_checkpoint.sh' -g '!README.md' | rg -q .; then
  echo "Blocked: legacy brand references found." >&2
  exit 1
fi

required_docs=(
  "docs/RUNBOOK.md"
  "docs/WEEK1_PILOT.md"
  "docs/RELEASE_CHECKLIST.md"
  "docs/ROLLBACK_CHECKLIST.md"
)

missing_docs=()
for doc in "${required_docs[@]}"; do
  if [[ ! -f "$doc" ]]; then
    missing_docs+=("$doc")
  fi
done

if (( ${#missing_docs[@]} > 0 )); then
  echo "Blocked: required operator docs missing:" >&2
  for doc in "${missing_docs[@]}"; do
    echo "  - $doc" >&2
  done
  exit 1
fi

is_truthy() {
  local v="${1:-}"
  v="$(echo "$v" | tr '[:upper:]' '[:lower:]')"
  case "$v" in
    1|true|yes|on)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

cloud_readiness_status="SKIPPED (FLOWFORGE_CLOUD_DEPS_REQUIRED not enabled)"
readyz_artifact=""
readyz_url="${FLOWFORGE_API_BASE:-http://127.0.0.1:8080}/readyz"

if is_truthy "${FLOWFORGE_CLOUD_DEPS_REQUIRED:-0}"; then
  readyz_artifact="$REPORT_DIR/readyz.json"
  curl_exit=0
  readyz_http_code="$(curl -sS --max-time 5 -o "$readyz_artifact" -w "%{http_code}" "$readyz_url")" || curl_exit=$?
  if (( curl_exit != 0 )); then
    echo "Blocked: strict cloud readiness is enabled, but /readyz probe failed (curl exit $curl_exit)." >&2
    echo "  URL: $readyz_url" >&2
    exit 1
  fi

  compact_readyz="$(tr -d '\n\r\t ' < "$readyz_artifact")"
  if [[ "$readyz_http_code" != "200" || "$compact_readyz" != *'"status":"ready"'* ]]; then
    echo "Blocked: strict cloud readiness is enabled, but /readyz is not healthy." >&2
    echo "  URL: $readyz_url" >&2
    echo "  HTTP: $readyz_http_code" >&2
    exit 1
  fi

  if [[ "$compact_readyz" != *'"cloud_dependencies_required":true'* ]]; then
    echo "Blocked: release env has FLOWFORGE_CLOUD_DEPS_REQUIRED=1, but API /readyz does not report cloud_dependencies_required=true." >&2
    echo "  Start API with matching env before running checkpoint." >&2
    exit 1
  fi

  cloud_readiness_status="PASS"
fi

cat > "$REPORT_FILE" <<EOF
# Release Checkpoint

Date: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

## Status

- Local verification gate: PASS
- Tracked secret/runtime legacy artifacts: PASS
- Legacy naming scan: PASS
- Operator docs present (runbook/pilot/release/rollback): PASS
- Cloud dependency readiness gate: $cloud_readiness_status

## Readiness Evidence

- Ready endpoint: $readyz_url
- Ready payload artifact: ${readyz_artifact:-N/A}

## Ready

Release checkpoint is complete.
EOF

echo "Release checkpoint complete: $REPORT_FILE"
