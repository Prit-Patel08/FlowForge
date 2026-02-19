#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export GOCACHE="${GOCACHE:-/tmp/agent-gocache}"
STRICT_MODE="${VERIFY_STRICT:-0}"
SKIP_DASHBOARD=0

usage() {
  cat <<EOF
Usage: ./scripts/verify_local.sh [options]

Options:
  --strict          Require all checks (including staticcheck/govulncheck) to be present.
  --skip-dashboard  Skip dashboard npm install/build step.
  -h, --help        Show this help text.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --strict) STRICT_MODE=1 ;;
    --skip-dashboard) SKIP_DASHBOARD=1 ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

resolve_tool() {
  local name="$1"
  if command -v "$name" >/dev/null 2>&1; then
    command -v "$name"
    return 0
  fi

  local gobin
  gobin="$(go env GOPATH)/bin/${name}"
  if [[ -x "$gobin" ]]; then
    echo "$gobin"
    return 0
  fi

  return 1
}

run_optional_tool() {
  local name="$1"
  local module="$2"
  shift
  shift
  local tool_path
  if tool_path="$(resolve_tool "$name")"; then
    "$tool_path" "$@"
    return 0
  fi

  if [[ "$STRICT_MODE" == "1" ]]; then
    echo "ERROR: $name is required in strict mode but not installed." >&2
    echo "Install with: go install ${module}@latest (or add it to PATH)" >&2
    return 1
  fi

  echo "WARN: $name not installed; skipping (run with --strict to fail on missing tools)"
  return 0
}

echo "== FlowForge local verification =="
echo "Root: $ROOT_DIR"
echo "GOCACHE: $GOCACHE"
echo "Strict mode: $STRICT_MODE"

cd "$ROOT_DIR"

echo "[1/7] go build ./..."
go build ./...

echo "[2/7] go test ./... -v"
go test ./... -v

echo "[3/7] go test ./... -race -v"
go test ./... -race -v

echo "[4/7] go vet ./..."
go vet ./...

echo "[5/7] staticcheck ./..."
if ! run_optional_tool "staticcheck" "honnef.co/go/tools/cmd/staticcheck" ./...; then
  echo "ERROR: staticcheck failed." >&2
  exit 1
fi

echo "[6/7] govulncheck ./..."
if ! run_optional_tool "govulncheck" "golang.org/x/vuln/cmd/govulncheck" ./...; then
  echo "ERROR: govulncheck failed." >&2
  echo "If this is a Go standard library advisory, upgrade your local Go patch toolchain (e.g. 1.25.7+)." >&2
  exit 1
fi

echo "[7/7] dashboard build"
if [[ "$SKIP_DASHBOARD" == "1" ]]; then
  echo "Skipping dashboard build (--skip-dashboard)."
else
  (
    cd dashboard
    npm ci
    npm run build
  )
fi

echo "✅ Local verification passed"
