# Agent-Sentry

Agent-Sentry is a local guardrail for long-running scripts and AI agent jobs.
It supervises a command, detects runaway behavior, intervenes safely, and records why.

## 60-Second Quickstart

```bash
cd /Users/pritpatel/Desktop/agent-sentry
chmod +x scripts/install.sh
./scripts/install.sh --open-browser
```

What you should see:
1. secure API keys generated once in `.sentry.env`
2. demo run triggers detection/intervention
3. summary printed:
   - `Runaway detected in X seconds`
   - `CPU peaked at Y%`
   - `Process recovered`
4. dashboard opens at `http://localhost:3001`

## Daily Usage

Supervise your own command:

```bash
./sentry run -- python3 your_script.py
```

Run demo again:

```bash
./sentry demo
```

Start API only:

```bash
./sentry dashboard
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

- mutating endpoints require `SENTRY_API_KEY`
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

## Build and Validation

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
- ensure `NEXT_PUBLIC_SENTRY_API_BASE` is correct

2. Kill/Restart returns unauthorized
- set `SENTRY_API_KEY` and provide `Authorization: Bearer <key>`

3. Demo doesn’t trigger quickly
- run `./sentry demo --max-cpu 30`

## Docs

- operations: `docs/OPERATIONS.md`
- threat model: `docs/THREAT_MODEL.md`
- security policy: `SECURITY.md`
