# Release Checklist

Use this checklist before tagging any release.

## 1. Pre-Release Gate

- [ ] `./scripts/verify_local.sh --strict` passes.
- [ ] `./scripts/smoke_local.sh` passes.
- [ ] `./scripts/release_checkpoint.sh` passes.
- [ ] If `FLOWFORGE_CLOUD_DEPS_REQUIRED=1`, API `/readyz` is healthy and reports `cloud_dependencies_required=true`.
- [ ] If `FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1`, replay drill artifact shows `overall_status=PASS`.
- [ ] If `FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION=1`, retention artifact status is `PASS` or `SKIPPED`.
- [ ] If `FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1`, weekly SLO artifact summary shows `error_budget_status=GREEN`.
- [ ] If `FLOWFORGE_REQUIRE_MVP_PHASE1_EXIT_GATE=1`, MVP exit-gate artifact summary shows `overall_status=PASS`.
- [ ] If `FLOWFORGE_REQUIRE_DAEMON_SMOKE=1`, daemon smoke artifact summary shows `overall_status=PASS`.
- [ ] Signed evidence bundle exported: `go run . evidence export --out-dir <release-artifact-dir>/evidence`.
- [ ] Signed evidence bundle verified: `go run . evidence verify --bundle-dir <release-artifact-dir>/evidence`.
- [ ] CI checks on `main` are green (`shellcheck`, `release-checkpoint-contract`, `backend` [build/test/race/vet/staticcheck/govulncheck], `dashboard`, `smoke`, `replay-drill`, `docker`, `sbom`).
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
