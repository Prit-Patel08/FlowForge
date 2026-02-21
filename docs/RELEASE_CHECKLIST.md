# Release Checklist

Use this checklist before tagging any release.

## 1. Pre-Release Gate

- [ ] `./scripts/verify_local.sh --strict` passes.
- [ ] `./scripts/smoke_local.sh` passes.
- [ ] `./scripts/release_checkpoint.sh` passes.
- [ ] If `FLOWFORGE_CLOUD_DEPS_REQUIRED=1`, API `/readyz` is healthy and reports `cloud_dependencies_required=true`.
- [ ] CI checks on `main` are green (`backend`, `dashboard`, `smoke`, `docker`, `sbom`).
- [ ] No tracked secret/runtime artifacts in git index.
- [ ] No unresolved high-severity security findings.

## 2. Release Preparation

- [ ] Confirm version/tag to publish.
- [ ] Update user-facing release notes (behavior changes and caveats).
- [ ] Confirm docs are up to date for any changed behavior.
- [ ] Confirm upgrade impact and safe rollback path.

## 3. Release Execution

- [ ] Create annotated tag for release version.
- [ ] Push tag to origin.
- [ ] Publish release notes/changelog entry.

## 4. Post-Release Verification

- [ ] Re-run health checks (`/healthz`, `/readyz`, `/metrics`).
- [ ] Validate dashboard can load timeline and incidents.
- [ ] Validate one supervised demo/real command works end-to-end.
- [ ] Confirm no immediate regression in incident/action signals.

## 5. Exit Criteria

Release is considered complete only when all checklist items above are done.
