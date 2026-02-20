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

Run demo again:

```bash
./flowforge demo
```

Start API only:

```bash
./flowforge dashboard
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

- `GET /healthz`
- `GET /readyz`
- `GET /stream`
- `GET /incidents`
- `GET /timeline`
- `GET /timeline?incident_id=<id>`
- `GET /metrics`
- `POST /process/kill`
- `POST /process/restart`

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

One-command local gate:

```bash
./scripts/verify_local.sh
```

Release-grade local gate (fails if `staticcheck`/`govulncheck` are missing):

```bash
./scripts/verify_local.sh --strict
```

Release smoke gate:

```bash
./scripts/smoke_local.sh
```

Release checkpoint (`./scripts/release_checkpoint.sh`) runs `verify_local.sh --strict`.
If `govulncheck` reports Go standard library advisories, upgrade your local Go patch version (CI uses Go `1.25.7`).

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

## Troubleshooting

1. Dashboard cannot connect
- ensure API is running on `http://localhost:8080`
- ensure `NEXT_PUBLIC_FLOWFORGE_API_BASE` is correct

2. Kill/Restart returns unauthorized
- set `FLOWFORGE_API_KEY` and provide `Authorization: Bearer <key>`

3. Demo doesn’t trigger quickly
- run `./flowforge demo --max-cpu 30`

## Week 1 Ops

- run pilot pack: `./scripts/week1_pilot.sh`
- run threshold tuning: `./scripts/tune_detection.sh`
- run recovery drill: `./scripts/recovery_drill.sh`
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

- operations: `docs/OPERATIONS.md`
- branch protection: `docs/BRANCH_PROTECTION.md`
- local daemon RFC (P1): `docs/DAEMON_RFC.md`
- integration API contract (P1): `docs/INTEGRATION_API_CONTRACT.md`
- issue templates: `.github/ISSUE_TEMPLATE/`
- threat model: `docs/THREAT_MODEL.md`
- runbook: `docs/RUNBOOK.md`
- onboarding usability test: `docs/ONBOARDING_USABILITY_TEST.md`
- week 1 checklist: `docs/WEEK1_PILOT.md`
- week 2 baseline: `docs/WEEK2_BASELINE.md`
- week 2 soak: `docs/WEEK2_SOAK.md`
- release checklist: `docs/RELEASE_CHECKLIST.md`
- rollback checklist: `docs/ROLLBACK_CHECKLIST.md`
- security policy: `SECURITY.md`
