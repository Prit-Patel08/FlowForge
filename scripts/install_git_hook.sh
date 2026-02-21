#!/usr/bin/env bash

set -euo pipefail

DRY_RUN=0
FORCE=0

usage() {
  cat <<EOF
Usage: ./scripts/install_git_hook.sh [options]

Options:
  --dry-run   Print actions without writing files.
  --force     Overwrite existing pre-commit hook without backup.
  -h, --help  Show this help text.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --force) FORCE=1 ;;
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

if ! git_root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  echo "ERROR: not inside a git repository." >&2
  exit 1
fi

hook_path="$git_root/.git/hooks/pre-commit"
marker="# flowforge-managed-precommit"

hook_payload="$(cat <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
# flowforge-managed-precommit
ROOT_DIR="$(git rev-parse --show-toplevel)"
exec "$ROOT_DIR/scripts/precommit_checks.sh"
EOF
)"

echo "Git root: $git_root"
echo "Hook path: $hook_path"
echo "Dry run: $DRY_RUN"

if [[ -f "$hook_path" ]] && ! rg -q "$marker" "$hook_path"; then
  if [[ "$FORCE" == "1" ]]; then
    echo "Existing non-FlowForge pre-commit hook will be overwritten (--force)."
  else
    backup_path="$hook_path.backup.$(date +%Y%m%d-%H%M%S)"
    echo "Backing up existing pre-commit hook to: $backup_path"
    if [[ "$DRY_RUN" == "0" ]]; then
      cp "$hook_path" "$backup_path"
    fi
  fi
fi

echo "Installing FlowForge pre-commit hook."
if [[ "$DRY_RUN" == "0" ]]; then
  printf "%s\n" "$hook_payload" > "$hook_path"
  chmod +x "$hook_path"
fi

echo "✅ Pre-commit hook installed."
