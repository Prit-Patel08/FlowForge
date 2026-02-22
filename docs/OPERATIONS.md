# Operations Guide

## Health and Readiness

- Liveness: `GET /healthz`
- Readiness: `GET /readyz`
- Metrics: `GET /metrics` (Prometheus text format)
- Timeline: `GET /timeline` (incident + audit + decision trace feed)
- Replay history: `GET /v1/ops/controlplane/replay/history?days=7` (daily replay/conflict trend + ledger stats)

## Hardened Container Run

```bash
docker run --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -e FLOWFORGE_API_KEY="$(openssl rand -hex 32)" \
  -e FLOWFORGE_MASTER_KEY="$(openssl rand -hex 32)" \
  -p 8080:8080 \
  flowforge
```

## Crash Recovery Model

- Supervisor sends termination signals to subprocess process-groups.
- On shutdown, process groups are first terminated gracefully then force-killed after timeout.
- Incident records remain in `flowforge.db` for post-mortem review.
- Audit events include actor, action, reason, and timestamp for kill/restart operations.
- Decision traces capture CPU score, entropy score, and confidence score for intervention transparency.

## Demo Mode

```bash
./flowforge demo
```

Expected summary:
- `Runaway detected in X seconds`
- `CPU peaked at Y%`
- `Process recovered`

## Performance Validation

Run benchmark suite:

```bash
go test ./test -bench . -benchmem -run '^$'
```

Run race detector:

```bash
go test ./... -race -v
```

Run strict local verification before release actions:

```bash
./scripts/verify_local.sh --strict
```

Optional local commit gate:

```bash
./scripts/tooling_doctor.sh
./scripts/precommit_checks.sh
./scripts/install_git_hook.sh
./scripts/install_git_hook.sh --strict
```

Equivalent make shortcuts:

```bash
make go-tools
make doctor
make doctor-summary
make contracts
make precommit
make ops-snapshot
make evidence-bundle
```

`make contracts` runs:
- `scripts/tooling_doctor_contract_test.sh`
- `scripts/release_checkpoint_contract_test.sh`
- `scripts/controlplane_replay_retention_contract_test.sh`
- `scripts/slo_weekly_review_contract_test.sh`
- `scripts/install_git_hook_contract_test.sh`

Cloud readiness smoke:

```bash
./scripts/cloud_ready_smoke.sh
```

Signed evidence bundle workflow:

```bash
export FLOWFORGE_EVIDENCE_SIGNING_KEY="replace-with-strong-key"
go run . evidence export --out-dir pilot_artifacts/evidence-$(date +%Y%m%d-%H%M%S)
go run . evidence verify --bundle-dir pilot_artifacts/evidence-<timestamp>
```

Notes:
- strict mode fails if `staticcheck`/`govulncheck` are missing
- if `govulncheck` reports Go stdlib advisories, upgrade local Go patch version to match CI (`1.25.7`)

## Week 1 Reliability Pack

```bash
./scripts/week1_pilot.sh
./scripts/tune_detection.sh
./scripts/recovery_drill.sh
./scripts/controlplane_replay_drill.sh
./scripts/controlplane_replay_retention.sh
./scripts/release_checkpoint.sh
```

Published drill evidence:
- `docs/CHAOS_DRILL_2026-02-21.md`
- `docs/CONTROLPLANE_REPLAY_DRILL_2026-02-22.md`

## Weekly SLO Dashboard Ritual

Generate the weekly SLO report and error-budget decision artifact:

```bash
./scripts/slo_weekly_review.sh --days 7
```

Optional spike threshold tuning:

```bash
./scripts/slo_weekly_review.sh \
  --days 7 \
  --replay-spike-yellow 5 --replay-spike-red 10 \
  --conflict-spike-yellow 2 --conflict-spike-red 5
```

Run control-plane idempotency replay drill evidence:

```bash
./scripts/controlplane_replay_drill.sh
```

Prune persisted replay ledger rows (retention + cap):

```bash
./scripts/controlplane_replay_retention.sh --retention-days 30 --max-rows 50000
```

Artifact output:
- `pilot_artifacts/slo-weekly-<timestamp>/slo_weekly_report.md`
- `pilot_artifacts/slo-weekly-<timestamp>/summary.tsv`
- `pilot_artifacts/slo-weekly-<timestamp>/replay_daily_trend.tsv`
- `pilot_artifacts/slo-weekly-<timestamp>/replay_history.json`

Canonical process and policy:
- `docs/SLO_OPERATIONS.md`

## Release and Rollback Checklists

- Release procedure: `docs/RELEASE_CHECKLIST.md`
- Rollback procedure: `docs/ROLLBACK_CHECKLIST.md`

Before tagging, complete:
- release checklist
- release checkpoint (`./scripts/release_checkpoint.sh`)

Optional strict replay gate during release checkpoint:

```bash
FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1 ./scripts/release_checkpoint.sh
```

Optional retention prune during release checkpoint:

```bash
FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION=1 \
FLOWFORGE_CONTROLPLANE_REPLAY_RETENTION_DAYS=30 \
FLOWFORGE_CONTROLPLANE_REPLAY_MAX_ROWS=50000 \
./scripts/release_checkpoint.sh
```

Optional weekly SLO green gate during release checkpoint:

```bash
FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 \
FLOWFORGE_SLO_REVIEW_DAYS=7 \
FLOWFORGE_SLO_REPLAY_MAX_ROWS=50000 \
./scripts/release_checkpoint.sh
```

Optional weekly SLO spike-threshold pass-through for the release gate:

```bash
FLOWFORGE_REQUIRE_WEEKLY_SLO_GREEN=1 \
FLOWFORGE_SLO_REPLAY_SPIKE_YELLOW=5 \
FLOWFORGE_SLO_REPLAY_SPIKE_RED=10 \
FLOWFORGE_SLO_CONFLICT_SPIKE_YELLOW=2 \
FLOWFORGE_SLO_CONFLICT_SPIKE_RED=5 \
./scripts/release_checkpoint.sh
```

## Issue Intake and Postmortem Templates

Use GitHub issue forms:

- Issue intake: `.github/ISSUE_TEMPLATE/issue_intake.yml`
- Incident postmortem: `.github/ISSUE_TEMPLATE/incident_postmortem.yml`

Guideline:
- file normal bugs/reliability/security problems with the issue intake form
- file all P0/P1 incidents and any production-impacting event with the postmortem form

## Week 2 Real-Workload Pilot

1. Copy and edit command list:

```bash
cp scripts/pilot_commands.example.txt pilot_commands.txt
```

2. Replace commands with your real workloads.
  - quoted arguments are supported in `pilot_commands.txt` (shell-style quoting).

3. Run pilot:

```bash
./scripts/week2_real_pilot.sh pilot_commands.txt
```

Optional strict mode (non-zero exit if expectations fail):

```bash
PILOT_FAIL_ON_MISMATCH=1 ./scripts/week2_real_pilot.sh pilot_commands.txt
```

4. Review:
- `pilot_artifacts/.../summary.md`
- `pilot_artifacts/.../incidents_snapshot.txt`
- `pilot_artifacts/.../week2-pilot.db`
- `pilot_artifacts/.../results.csv` (`exit_reason` and `decision_reason` columns come from DB-correlated incidents)

## Onboarding Usability Verification

Internal tooling dry-run:

```bash
./scripts/onboarding_usability_test.sh --mode internal
```

External validation run:

```bash
./scripts/onboarding_usability_test.sh --mode external --tester-name "First Last" --tester-role "Developer"
```

Review:
- `pilot_artifacts/onboarding-<timestamp>/report.md`
- `pilot_artifacts/onboarding-<timestamp>/logs/`
- `pilot_artifacts/onboarding-<timestamp>/summary.tsv`
- `pilot_artifacts/onboarding-<timestamp>/external_feedback.md`
- `pilot_artifacts/onboarding-<timestamp>/observer_notes.md`

Procedure for a true new-developer run:
- `docs/ONBOARDING_USABILITY_TEST.md`

Published external validation evidence:
- `docs/ONBOARDING_EXTERNAL_VALIDATION_2026-02-21.md`
