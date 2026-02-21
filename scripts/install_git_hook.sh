#!/usr/bin/env bash

set -euo pipefail

DRY_RUN=0
FORCE=0
STRICT_HOOK=0

usage() {
  cat <<EOF
Usage: ./scripts/install_git_hook.sh [options]

Options:
  --dry-run     Print actions without writing files.
  --force       Overwrite existing pre-commit hook without backup.
  --strict      Install hook that runs precommit checks with --strict.
  -h, --help    Show this help text.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --force) FORCE=1 ;;
    --strict) STRICT_HOOK=1 ;;
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

marker="# flowforge-managed-precommit"
hooks_path_cfg="$(git config --get core.hooksPath || true)"
if [[ -z "$hooks_path_cfg" ]]; then
  hooks_dir="$git_root/.git/hooks"
elif [[ "$hooks_path_cfg" = /* ]]; then
  hooks_dir="$hooks_path_cfg"
else
  hooks_dir="$git_root/$hooks_path_cfg"
fi
hook_path="$hooks_dir/pre-commit"

strict_suffix=""
if [[ "$STRICT_HOOK" == "1" ]]; then
  strict_suffix=" --strict"
fi

hook_payload="$(cat <<EOF
#!/usr/bin/env bash
set -euo pipefail
# flowforge-managed-precommit
ROOT_DIR="\$(git rev-parse --show-toplevel)"
exec "\$ROOT_DIR/scripts/precommit_checks.sh"${strict_suffix}
EOF
)"

echo "Git root: $git_root"
echo "Hooks dir: $hooks_dir"
echo "Hook path: $hook_path"
echo "Dry run: $DRY_RUN"
echo "Strict hook mode: $STRICT_HOOK"

if [[ -f "$hook_path" ]]; then
  existing_payload="$(cat "$hook_path")"
  if [[ "$existing_payload" == "$hook_payload" ]]; then
    echo "Pre-commit hook is already up to date."
    exit 0
  fi
fi

if [[ "$DRY_RUN" == "0" ]]; then
  mkdir -p "$hooks_dir"
else
  echo "Dry run: would ensure hooks directory exists."
fi

if [[ -f "$hook_path" ]] && ! grep -q "$marker" "$hook_path"; then
  if [[ "$FORCE" == "1" ]]; then
    echo "Existing non-FlowForge pre-commit hook will be overwritten (--force)."
  else
    backup_path="$hook_path.backup.$(date +%Y%m%d-%H%M%S)"
    echo "Backing up existing pre-commit hook to: $backup_path"
    if [[ "$DRY_RUN" == "0" ]]; then
      cp "$hook_path" "$backup_path"
    else
      echo "Dry run: would create backup copy."
    fi
  fi
fi

echo "Installing FlowForge pre-commit hook."
if [[ "$DRY_RUN" == "0" ]]; then
  printf "%s\n" "$hook_payload" > "$hook_path"
  chmod +x "$hook_path"
  echo "✅ Pre-commit hook installed."
else
  echo "Dry run complete. No files were modified."
fi
