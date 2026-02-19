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

if [[ ! -f "docs/RUNBOOK.md" || ! -f "docs/WEEK1_PILOT.md" ]]; then
  echo "Blocked: runbook or pilot docs missing." >&2
  exit 1
fi

cat > "$REPORT_FILE" <<EOF
# Release Checkpoint

Date: $(date -u +"%Y-%m-%dT%H:%M:%SZ")

## Status

- Local verification gate: PASS
- Tracked secret/runtime legacy artifacts: PASS
- Legacy naming scan: PASS
- Operator docs present: PASS

## Ready

Release checkpoint is complete.
EOF

echo "Release checkpoint complete: $REPORT_FILE"
