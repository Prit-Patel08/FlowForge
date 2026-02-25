#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC_PATH="${1:-api/openapi/v1.yaml}"

if [[ "$SPEC_PATH" != /* ]]; then
  SPEC_PATH="$ROOT_DIR/$SPEC_PATH"
fi

if [[ ! -f "$SPEC_PATH" ]]; then
  echo "ERROR: OpenAPI spec not found: $SPEC_PATH" >&2
  exit 1
fi

echo "Validating OpenAPI spec: $SPEC_PATH"
go run github.com/getkin/kin-openapi/cmd/validate@v0.126.0 "$SPEC_PATH"
echo "OpenAPI validation passed."
