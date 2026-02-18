#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-/tmp/agent-gocache}"

echo "== Detection fixture baseline =="
go test ./test -run TestDetectionFixtureBaseline -v

echo
echo "== Detection benchmarks =="
go test ./test -bench Detection -benchmem
