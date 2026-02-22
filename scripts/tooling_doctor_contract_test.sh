#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

assert_file_contains() {
  local file_path="$1"
  local pattern="$2"
  if ! grep -Eq -- "$pattern" "$file_path"; then
    echo "assertion failed: expected pattern '$pattern' in $file_path" >&2
    exit 1
  fi
}

assert_exit_nonzero() {
  local rc="$1"
  local label="$2"
  if [[ "$rc" -eq 0 ]]; then
    echo "assertion failed: expected non-zero exit for $label" >&2
    exit 1
  fi
}

setup_fake_toolchain() {
  local bin_dir="$tmp_dir/bin"
  local fake_gopath="$tmp_dir/gopath"
  mkdir -p "$bin_dir" "$fake_gopath/bin"

  # Keep PATH deterministic for contract testing. We vend a minimal utility set
  # into the fake bin so host-installed optional tools (for example docker on
  # CI runners) do not affect assertions.
  local util
  local util_path
  for util in bash grep rm cat chmod mkdir dirname head tr mktemp; do
    util_path="$(command -v "$util")"
    if [[ -z "$util_path" ]]; then
      echo "missing required utility for contract test: $util" >&2
      exit 1
    fi
    ln -sf "$util_path" "$bin_dir/$util"
  done

  cat > "$bin_dir/go" <<EOF
#!/usr/bin/env bash
set -euo pipefail
if [[ "\${1:-}" == "env" && "\${2:-}" == "GOPATH" ]]; then
  echo "$fake_gopath"
  exit 0
fi
if [[ "\${1:-}" == "version" ]]; then
  echo "go version go1.25.7 linux/amd64"
  exit 0
fi
echo "unsupported go args: \$*" >&2
exit 1
EOF

  cat > "$bin_dir/node" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "v20.12.0"
EOF

  cat > "$bin_dir/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "10.9.2"
EOF

  cat > "$bin_dir/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "Docker version 27.0.0, build test"
EOF

  cat > "$bin_dir/shellcheck" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "ShellCheck - shell script analysis tool"
EOF

  cat > "$fake_gopath/bin/staticcheck" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "staticcheck 2025.1.1 (0.6.1)"
EOF

  cat > "$fake_gopath/bin/govulncheck" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "Go: go1.25.7"
EOF

  chmod +x "$bin_dir/go" "$bin_dir/node" "$bin_dir/npm" "$bin_dir/docker" "$bin_dir/shellcheck"
  chmod +x "$fake_gopath/bin/staticcheck" "$fake_gopath/bin/govulncheck"

  export PATH="$bin_dir"
}

run_all_tools_present_case() {
  local summary="$tmp_dir/summary-all.tsv"
  ./scripts/tooling_doctor.sh --strict --summary-file "$summary" >/dev/null
  # Keep compatibility with both historical ("detail") and newer ("details")
  # header spellings so contract tests do not flap on this non-breaking rename.
  assert_file_contains "$summary" $'^tool\tstatus\tdetails?$'
  assert_file_contains "$summary" $'^go\tPASS\t'
  assert_file_contains "$summary" $'^docker\tPASS\t'
  assert_file_contains "$summary" $'^shellcheck\tPASS\t'
  assert_file_contains "$summary" $'^staticcheck\tPASS\t'
  assert_file_contains "$summary" $'^govulncheck\tPASS\t'
}

run_optional_missing_warn_case() {
  local summary="$tmp_dir/summary-warn.tsv"
  rm -f "$tmp_dir/bin/docker"
  ./scripts/tooling_doctor.sh --summary-file "$summary" >/dev/null
  assert_file_contains "$summary" $'^docker\tWARN\t'
}

run_optional_missing_strict_fail_case() {
  local summary="$tmp_dir/summary-fail.tsv"
  local strict_out="$tmp_dir/doctor.strict.out"
  local strict_err="$tmp_dir/doctor.strict.err"
  set +e
  ./scripts/tooling_doctor.sh --strict --summary-file "$summary" >"$strict_out" 2>"$strict_err"
  local rc=$?
  set -e
  assert_exit_nonzero "$rc" "strict mode with missing optional tools"
  assert_file_contains "$summary" $'^docker\tFAIL\t'
}

run_unknown_arg_case() {
  local arg_out="$tmp_dir/doctor.arg.out"
  local arg_err="$tmp_dir/doctor.arg.err"
  set +e
  ./scripts/tooling_doctor.sh --nope >"$arg_out" 2>"$arg_err"
  local rc=$?
  set -e
  assert_exit_nonzero "$rc" "unknown argument handling"
  assert_file_contains "$arg_err" "^Unknown argument: --nope$"
}

cd "$ROOT_DIR"
setup_fake_toolchain
run_all_tools_present_case
run_optional_missing_warn_case
run_optional_missing_strict_fail_case
run_unknown_arg_case

echo "tooling doctor contract tests passed"
