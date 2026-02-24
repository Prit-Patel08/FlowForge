# FlowForge

FlowForge is a local guardrail for long-running scripts and AI agent jobs.
It supervises a command, detects runaway behavior, intervenes safely, and records why.

## 60-Second Quickstart

```bash
chmod +x scripts/install.sh
./scripts/install.sh --open-browser
```

Build/setup only (no long-running services):

```bash
./scripts/install.sh --no-services --no-demo
```

What you should see:
1. secure API keys generated once in `.flowforge.env`
   - existing legacy keys are auto-migrated to `FLOWFORGE_*`
2. production dashboard build (`next build`) and server startup (`next start`)
3. demo run triggers detection/intervention
4. summary printed:
   - `Runaway detected in X seconds`
   - `CPU peaked at Y%`
   - `Process recovered`
5. dashboard opens at `http://localhost:3001`

## Daily Usage

Supervise your own command:

```bash
./flowforge run -- python3 your_script.py
```

Run policy in dry-run mode (evaluate and log only):

```bash
./flowforge run --shadow-mode -- python3 your_script.py
```

Run policy in canary mode (sampled destructive enforcement):

```bash
./flowforge run --policy-rollout canary --policy-canary-percent 10 -- python3 your_script.py
```

Run demo again:

```bash
./flowforge demo
```

Start/attach local API daemon:

```bash
./flowforge dashboard
```

Direct daemon lifecycle commands:

```bash
./flowforge daemon start
./flowforge daemon status
./flowforge daemon logs --lines 120
./flowforge daemon stop
```

Run API in foreground (script/CI mode):

```bash
./flowforge dashboard --foreground
```

Use baseline profile defaults:

```bash
cp flowforge.yaml.example flowforge.yaml
./flowforge run --profile standard -- python3 your_script.py
```

## How It Works (Mental Model)

1. Supervisor
- starts and watches one child process

2. Decision
- evaluates CPU pressure + output repetition

3. Action
- continue, alert, kill, or restart

4. Evidence
- writes incident/audit/decision records to SQLite and exposes timeline

## Data Flow

```text
process -> monitor -> decision -> action -> DB events -> API -> dashboard
```

## Core Components

- CLI commands: `cmd/run.go`, `cmd/demo.go`, `cmd/dashboard.go`
- Daemon lifecycle: `cmd/daemon.go`, `internal/daemon/runtime.go`
- API server: `internal/api/server.go`
- Runtime state: `internal/state/state.go`
- Persistence: `internal/database/db.go`
- Dashboard UI: `dashboard/pages/index.tsx`
- Installer: `scripts/install.sh`

## Security Defaults

- mutating endpoints require `FLOWFORGE_API_KEY`
- constant-time token comparison
- localhost-only bind (`127.0.0.1` by default)
- strict local CORS allowlist
- auth brute-force/rate limiting on API
- secret redaction before log/state display

## API Endpoints

- `GET /v1/healthz`
- `GET /v1/readyz`
- `GET /v1/stream`
- `GET /v1/incidents`
- `GET /v1/timeline`
- `GET /v1/timeline?incident_id=<id>`
- `GET /v1/worker/lifecycle`
- `GET /v1/metrics`
- `GET /v1/ops/controlplane/replay/history?days=<n>`
- `GET /v1/ops/decisions/replay/{trace_id}`
- `GET /v1/ops/decisions/replay/health?limit=<n>&strict=1`
- `GET /v1/ops/decisions/signals/baseline?limit=<n>&strict=1`
- `GET /v1/ops/requests/{request_id}?limit=<n>`
- `POST /v1/process/kill`
- `POST /v1/process/restart`
- `POST /v1/integrations/workspaces/register`
- `DELETE /v1/integrations/workspaces/{workspace_id}`
- `GET /v1/integrations/workspaces/{workspace_id}/status`
- `POST /v1/integrations/workspaces/{workspace_id}/protection`
- `GET /v1/integrations/workspaces/{workspace_id}/incidents/latest`
- `POST /v1/integrations/workspaces/{workspace_id}/actions`

Legacy non-versioned aliases remain available (`/healthz`, `/readyz`, `/incidents`, `/timeline`, `/worker/lifecycle`, `/metrics`, `/stream`, `/process/*`) for backward compatibility.

`/timeline` now includes `lifecycle` events with structured `evidence` payload for transition forensics.
`decision` timeline/request-trace events now include versioned engine metadata (`decision_engine`, `engine_version`, `decision_contract_version`, `rollout_mode`) for deterministic replay and audits.
`decision` timeline/request-trace events also carry replay metadata (`replay_contract_version`, `replay_digest`) so operators can verify decision determinism.
/readyz returns structured readiness checks and can enforce cloud dependency health when `FLOWFORGE_CLOUD_DEPS_REQUIRED=1`.
Integration write endpoints require `FLOWFORGE_API_KEY`; workspace registration requires absolute `workspace_path`.
Error responses use RFC 7807 Problem Details (`application/problem+json`) with structured `type` URIs, include `request_id`, and keep legacy `error` for compatibility.
API echoes `X-Request-Id` (or generates one) so operators can correlate failed requests with audit evidence.
Use `GET /v1/ops/requests/{request_id}` to retrieve the full correlated event chain for that request id.
Use `GET /v1/ops/decisions/replay/{trace_id}` to recompute and verify deterministic replay digest integrity for a recorded decision trace.
Use `GET /v1/ops/decisions/replay/health` to audit recent replay integrity at fleet level (`strict=1` returns `409` when mismatches, missing digests, or unreplayable traces are detected).
Use `GET /v1/ops/decisions/signals/baseline` to audit signal-history drift by decision-engine bucket (`strict=1` returns `409` when baseline deltas exceed configured thresholds).

`/metrics` now includes lifecycle SLO/latency metrics:
- `flowforge_stop_slo_compliance_ratio`
- `flowforge_restart_slo_compliance_ratio`
- `flowforge_stop_latency_last_seconds`
- `flowforge_restart_latency_last_seconds`
- `flowforge_restart_budget_block_total`
- `flowforge_decision_replay_checked_rows`
- `flowforge_decision_replay_match_rows`
- `flowforge_decision_replay_mismatch_rows`
- `flowforge_decision_replay_missing_digest_rows`
- `flowforge_decision_replay_legacy_fallback_rows`
- `flowforge_decision_replay_unreplayable_rows`
- `flowforge_decision_replay_mismatch_ratio`
- `flowforge_decision_replay_healthiness`
- `flowforge_decision_replay_health_sample_limit`
- `flowforge_decision_replay_stats_error`
- `flowforge_decision_signal_baseline_checked_rows`
- `flowforge_decision_signal_baseline_bucket_count`
- `flowforge_decision_signal_baseline_at_risk_buckets`
- `flowforge_decision_signal_baseline_max_cpu_delta_abs`
- `flowforge_decision_signal_baseline_max_entropy_delta_abs`
- `flowforge_decision_signal_baseline_max_confidence_delta_abs`
- `flowforge_decision_signal_baseline_healthiness`
- `flowforge_decision_signal_baseline_sample_limit`
- `flowforge_decision_signal_baseline_stats_error`

## Detection Benchmark Baseline

Run the fixture baseline + benchmarks:

```bash
go test ./test -run TestDetectionFixtureBaseline -v
go test ./test -bench Detection -benchmem
```

Fixtures:
- runaway logs: `test/fixtures/runaway.txt`
- healthy logs: `test/fixtures/healthy.txt`
- corpus scripts:
  - `test/fixtures/scripts/infinite_looper.py`
  - `test/fixtures/scripts/memory_leaker.py`
  - `test/fixtures/scripts/healthy_spike.py`
  - `test/fixtures/scripts/zombie_spawner.py`

## Build and Validation

New machine bootstrap + first successful gate run:

```bash
make go-tools
make doctor
npm --prefix dashboard ci
make contracts
make precommit
```

Strict toolchain profile (matches CI expectations):

```bash
make doctor-strict
```

Generate a local tooling summary artifact:

```bash
make doctor-summary
```

Generate one operational snapshot artifact bundle:

```bash
make ops-snapshot
```

Generate and verify signed evidence bundle:

```bash
export FLOWFORGE_EVIDENCE_SIGNING_KEY="replace-with-strong-key"
make evidence-bundle
make evidence-verify BUNDLE_DIR=pilot_artifacts/evidence-<timestamp>
```

One-command local gate:

```bash
./scripts/verify_local.sh
```

Release-grade local gate (fails if `staticcheck`/`govulncheck` are missing):

```bash
./scripts/verify_local.sh --strict
```

`verify_local.sh` uses explicit Go package targets (`.`, `./cmd/...`, `./internal/...`, `./test`) to avoid scanning non-Go trees (for example `dashboard/node_modules`).
CI backend gates use the same explicit package target set for build/test/race/vet/staticcheck/govulncheck.
For faster local reruns, use `./scripts/verify_local.sh --skip-npm-install` to skip dashboard `npm ci` and run only dashboard build (requires `dashboard/node_modules` to exist).

Release smoke gate:

```bash
./scripts/smoke_local.sh
```

MVP Phase-1 exit gate artifact:

```bash
./scripts/mvp_phase1_exit_gate.sh
```

Release checkpoint (`./scripts/release_checkpoint.sh`) runs `verify_local.sh --strict`.
If `FLOWFORGE_CLOUD_DEPS_REQUIRED=1`, it also enforces `/readyz` health (HTTP 200 + `status=ready` + `cloud_dependencies_required=true`).
If `FLOWFORGE_REQUIRE_CONTROLPLANE_REPLAY_DRILL=1`, it enforces a passing `controlplane_replay_drill.sh` run against the live API.
If `FLOWFORGE_RUN_CONTROLPLANE_REPLAY_RETENTION=1`, it runs replay-ledger retention prune and records artifact output in release report.
If `FLOWFORGE_REQUIRE_DAEMON_SMOKE=1`, it enforces a passing `daemon_smoke.sh` artifact (`overall_status=PASS`).
If `govulncheck` reports Go standard library advisories, upgrade your local Go patch version (CI uses Go `1.25.7`).
Release checkpoint contract tests: `./scripts/release_checkpoint_contract_test.sh`.
Daemon smoke contract tests: `./scripts/daemon_smoke_contract_test.sh`.
Replay retention contract tests: `./scripts/controlplane_replay_retention_contract_test.sh`.
Tooling doctor contract tests: `./scripts/tooling_doctor_contract_test.sh`.
CI also enforces `shellcheck` for `scripts/*.sh`.
CI also runs `tooling_doctor.sh --strict`.
CI runs a live replay drill gate (`replay-drill`) using `controlplane_replay_drill.sh` and uploads drill artifacts.
CI uploads tooling doctor summary artifact (`tooling-doctor/summary.tsv`).
ShellCheck policy is pinned in repo at `.shellcheckrc`.
Run local toolchain diagnostics: `./scripts/tooling_doctor.sh` (or `--strict`).
Fast local pre-commit checks: `./scripts/precommit_checks.sh`.
Install a managed git pre-commit hook: `./scripts/install_git_hook.sh`.
Install strict hook mode: `./scripts/install_git_hook.sh --strict`.
Git hook installer contract tests: `./scripts/install_git_hook_contract_test.sh`.
Cloud dependency + readyz smoke: `./scripts/cloud_ready_smoke.sh`.
Ops status snapshot artifact: `./scripts/ops_status_snapshot.sh`.
Signed evidence export: `go run . evidence export`.
Signed evidence verification: `go run . evidence verify --bundle-dir <path>`.
Control-plane replay retention cleanup: `./scripts/controlplane_replay_retention.sh`.
Daemon lifecycle smoke artifact: `./scripts/daemon_smoke.sh`.

Expected smoke output:
- `Runaway detected in ...`
- `CPU peaked at ...`
- `Process recovered`
- health/metrics/timeline probes succeed

Backend:

```bash
go build ./...
go test ./... -v
go test ./... -race -v
go vet ./...
```

Dashboard:

```bash
cd dashboard
npm ci
npm run build
```

Repo-root equivalent (avoids `cd` mistakes):

```bash
npm --prefix dashboard ci
npm --prefix dashboard run build
NEXT_PUBLIC_FLOWFORGE_API_BASE=http://127.0.0.1:8080 npm --prefix dashboard run start -- -p 3001
```

Cloud-capable local dependency stack (Postgres + Redis + NATS + MinIO):

```bash
./scripts/cloud_dev_stack.sh up
./scripts/cloud_dev_stack.sh status
```

Enable strict cloud dependency readiness checks in API:

```bash
export FLOWFORGE_CLOUD_DEPS_REQUIRED=1
export FLOWFORGE_CLOUD_POSTGRES_ADDR=127.0.0.1:15432
export FLOWFORGE_CLOUD_REDIS_ADDR=127.0.0.1:16379
export FLOWFORGE_CLOUD_NATS_HEALTH_URL=http://127.0.0.1:18222/healthz
export FLOWFORGE_CLOUD_MINIO_HEALTH_URL=http://127.0.0.1:19000/minio/health/live
export FLOWFORGE_CLOUD_PROBE_TIMEOUT_MS=800
curl -s http://127.0.0.1:8080/readyz
```

Optional restart storm guard for manual/API restarts:

```bash
export FLOWFORGE_RESTART_BUDGET_MAX=3
export FLOWFORGE_RESTART_BUDGET_WINDOW_SECONDS=300
```

Optional container health probe override (for custom ports/sidecar layouts):

```bash
export FLOWFORGE_HEALTHCHECK_URL=http://127.0.0.1:8080/healthz
```

## Troubleshooting

1. Dashboard cannot connect
- ensure API is running on `http://localhost:8080`
- ensure `NEXT_PUBLIC_FLOWFORGE_API_BASE` is correct

2. Kill/Restart returns unauthorized
- set `FLOWFORGE_API_KEY` and provide `Authorization: Bearer <key>`

3. Restart returns `429 restart budget exceeded`
- either wait for the configured budget window, or raise `FLOWFORGE_RESTART_BUDGET_MAX` for your environment
- API includes `Retry-After` header and `retry_after_seconds` field for operator retry timing

4. Evidence export fails with signing-key error
- set `FLOWFORGE_EVIDENCE_SIGNING_KEY` (or `FLOWFORGE_MASTER_KEY`) before running `flowforge evidence export`

5. Demo doesn’t trigger quickly
- run `./flowforge demo --max-cpu 30`

## Week 1 Ops

- run pilot pack: `./scripts/week1_pilot.sh`
- run threshold tuning: `./scripts/tune_detection.sh`
- run recovery drill: `./scripts/recovery_drill.sh`
- run control-plane replay drill: `./scripts/controlplane_replay_drill.sh`
- run daemon lifecycle smoke: `./scripts/daemon_smoke.sh`
- run release checkpoint: `./scripts/release_checkpoint.sh`

## Week 2 Ops

- baseline decision: `docs/WEEK2_BASELINE.md` (`max-cpu: 60.0`)
- run real-workload pilot: `./scripts/week2_real_pilot.sh scripts/pilot_commands.example.txt`
- replace sample commands with your own workload commands before final run
- run daily soak check: `./scripts/week2_soak_check.sh pilot_commands.txt`
- run release checkpoint again before tagging

## Onboarding Usability Test

Run end-to-end onboarding verification:

```bash
./scripts/onboarding_usability_test.sh
```

Report output:
- `pilot_artifacts/onboarding-<timestamp>/report.md`

## Docs

- master company plan: `plan.md`
- company execution playbook: `docs/COMPANY_EXECUTION.md`
- blueprint alignment audit: `docs/ALIGNMENT_AUDIT.md`
- CLI reference: `docs/reference/cli/flowforge.md`
- operations: `docs/OPERATIONS.md`
- branch protection: `docs/BRANCH_PROTECTION.md`
- local daemon RFC (P1): `docs/DAEMON_RFC.md`
- integration API contract (P1): `docs/INTEGRATION_API_CONTRACT.md`
- issue templates: `.github/ISSUE_TEMPLATE/`
- threat model: `docs/THREAT_MODEL.md`
- runbook: `docs/RUNBOOK.md`
- cloud-dev dependency stack: `infra/local-cloud/README.md`
- operator command map: `docs/OPERATOR_COMMAND_MAP.md`
- onboarding usability test: `docs/ONBOARDING_USABILITY_TEST.md`
- week 1 checklist: `docs/WEEK1_PILOT.md`
- week 2 baseline: `docs/WEEK2_BASELINE.md`
- week 2 soak: `docs/WEEK2_SOAK.md`
- release checklist: `docs/RELEASE_CHECKLIST.md`
- rollback checklist: `docs/ROLLBACK_CHECKLIST.md`
- security policy: `SECURITY.md`
