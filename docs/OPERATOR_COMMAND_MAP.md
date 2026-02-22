# Operator Command Map

Use this file as the single command reference for day-to-day operation.

## 1) Machine Readiness

| Goal | Command |
|---|---|
| Toolchain diagnostics (warn profile) | `make doctor` |
| Toolchain diagnostics (strict profile) | `make doctor-strict` |
| Tooling summary artifact | `make doctor-summary` |

## 2) Local Quality Gates

| Goal | Command |
|---|---|
| Full local verifier | `./scripts/verify_local.sh` |
| Strict local verifier | `./scripts/verify_local.sh --strict` |
| Pre-commit gate | `make precommit` |
| Contract test suite | `make contracts` |

## 3) Git Hook Workflow

| Goal | Command |
|---|---|
| Install managed pre-commit hook | `make hook` |
| Install strict pre-commit hook | `make hook-strict` |

## 4) Cloud-Ready Local Ops

| Goal | Command |
|---|---|
| Start cloud stack | `./scripts/cloud_dev_stack.sh up` |
| Cloud stack status | `./scripts/cloud_dev_stack.sh status` |
| Cloud readiness smoke (deps + `/readyz`) | `./scripts/cloud_ready_smoke.sh` |
| Cloud readiness smoke (deps only) | `./scripts/cloud_ready_smoke.sh --skip-readyz` |

## 5) Snapshot and Evidence

| Goal | Command |
|---|---|
| Generate ops status snapshot | `./scripts/ops_status_snapshot.sh` |
| Strict snapshot (doctor strict) | `./scripts/ops_status_snapshot.sh --strict-doctor` |
| Export signed evidence bundle | `go run . evidence export --out-dir pilot_artifacts/evidence-<timestamp>` |
| Verify signed evidence bundle | `go run . evidence verify --bundle-dir pilot_artifacts/evidence-<timestamp>` |

## 6) Release Workflow

| Goal | Command |
|---|---|
| Release checkpoint | `./scripts/release_checkpoint.sh` |
| Release checkpoint (replay + retention strict) | `FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1 FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION=1 ./scripts/release_checkpoint.sh` |
| Release checkpoint (weekly SLO GREEN strict) | `FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 FLOWFORGE_SLO_REVIEW_DAYS=7 FLOWFORGE_SLO_REPLAY_MAX_ROWS=50000 ./scripts/release_checkpoint.sh` |
| Weekly SLO report | `./scripts/slo_weekly_review.sh --days 7` |
| Control-plane replay drill | `./scripts/controlplane_replay_drill.sh` |
| Control-plane replay retention prune | `./scripts/controlplane_replay_retention.sh --retention-days 30 --max-rows 50000` |
