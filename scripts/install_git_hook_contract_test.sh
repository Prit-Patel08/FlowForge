#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

make_repo() {
  local repo_dir="$1"
  mkdir -p "$repo_dir/scripts"
  (
    cd "$repo_dir"
    git init -q
  )
  cp "$ROOT_DIR/scripts/install_git_hook.sh" "$repo_dir/scripts/install_git_hook.sh"
  chmod +x "$repo_dir/scripts/install_git_hook.sh"
}

assert_file_contains() {
  local file_path="$1"
  local pattern="$2"
  if ! grep -Eq -- "$pattern" "$file_path"; then
    echo "assertion failed: expected pattern '$pattern' in $file_path" >&2
    exit 1
  fi
}

assert_file_not_contains() {
  local file_path="$1"
  local pattern="$2"
  if grep -Eq -- "$pattern" "$file_path"; then
    echo "assertion failed: unexpected pattern '$pattern' in $file_path" >&2
    exit 1
  fi
}

run_default_install_case() {
  local repo="$tmp_dir/repo-default"
  make_repo "$repo"
  (
    cd "$repo"

    ./scripts/install_git_hook.sh --dry-run >/dev/null
    if [[ -f ".git/hooks/pre-commit" ]]; then
      echo "dry-run unexpectedly created pre-commit hook" >&2
      exit 1
    fi

    ./scripts/install_git_hook.sh >/dev/null
    [[ -x ".git/hooks/pre-commit" ]]
    assert_file_contains ".git/hooks/pre-commit" "flowforge-managed-precommit"
    assert_file_not_contains ".git/hooks/pre-commit" "--strict"

    local before_sum
    before_sum="$(shasum ".git/hooks/pre-commit" | awk '{print $1}')"
    local second_run_output
    second_run_output="$(./scripts/install_git_hook.sh)"
    local after_sum
    after_sum="$(shasum ".git/hooks/pre-commit" | awk '{print $1}')"

    [[ "$before_sum" == "$after_sum" ]]
    echo "$second_run_output" | grep -Eq "already up to date"

    ./scripts/install_git_hook.sh --strict >/dev/null
    assert_file_contains ".git/hooks/pre-commit" "--strict"
  )
}

run_backup_case() {
  local repo="$tmp_dir/repo-backup"
  make_repo "$repo"
  (
    cd "$repo"

    cat > ".git/hooks/pre-commit" <<'EOF'
#!/usr/bin/env bash
echo custom-hook
EOF
    chmod +x ".git/hooks/pre-commit"

    ./scripts/install_git_hook.sh >/dev/null
    assert_file_contains ".git/hooks/pre-commit" "flowforge-managed-precommit"

    shopt -s nullglob
    local backups=(.git/hooks/pre-commit.backup.*)
    if (( ${#backups[@]} == 0 )); then
      echo "expected backup pre-commit hook to be created" >&2
      exit 1
    fi
  )
}

run_custom_hooks_path_case() {
  local repo="$tmp_dir/repo-custom-hooks"
  make_repo "$repo"
  (
    cd "$repo"
    git config core.hooksPath .githooks
    ./scripts/install_git_hook.sh >/dev/null
    [[ -x ".githooks/pre-commit" ]]
    assert_file_contains ".githooks/pre-commit" "flowforge-managed-precommit"
  )
}

run_default_install_case
run_backup_case
run_custom_hooks_path_case

echo "install git hook contract tests passed"
