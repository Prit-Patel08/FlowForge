# Operations Guide

## Health and Readiness

- Liveness: `GET /healthz`
- Readiness: `GET /readyz`
- Metrics: `GET /metrics` (Prometheus text format)
- Timeline: `GET /timeline` (incident + audit + decision trace feed)

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
./scripts/precommit_checks.sh
./scripts/install_git_hook.sh
```

Notes:
- strict mode fails if `staticcheck`/`govulncheck` are missing
- if `govulncheck` reports Go stdlib advisories, upgrade local Go patch version to match CI (`1.25.7`)

## Week 1 Reliability Pack

```bash
./scripts/week1_pilot.sh
./scripts/tune_detection.sh
./scripts/recovery_drill.sh
./scripts/release_checkpoint.sh
```

Published drill evidence:
- `docs/CHAOS_DRILL_2026-02-21.md`

## Weekly SLO Dashboard Ritual

Generate the weekly SLO report and error-budget decision artifact:

```bash
./scripts/slo_weekly_review.sh --days 7
```

Artifact output:
- `pilot_artifacts/slo-weekly-<timestamp>/slo_weekly_report.md`
- `pilot_artifacts/slo-weekly-<timestamp>/summary.tsv`

Canonical process and policy:
- `docs/SLO_OPERATIONS.md`

## Release and Rollback Checklists

- Release procedure: `docs/RELEASE_CHECKLIST.md`
- Rollback procedure: `docs/ROLLBACK_CHECKLIST.md`

Before tagging, complete:
- release checklist
- release checkpoint (`./scripts/release_checkpoint.sh`)

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
