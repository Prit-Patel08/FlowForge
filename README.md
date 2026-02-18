# Agent-Sentry

Autonomous supervision and security layer for AI agent subprocesses.

## First Value In 60 Seconds

```bash
chmod +x scripts/install.sh
./scripts/install.sh --open-browser
```

This now runs a real demo flow:
- launches a runaway process,
- detects runaway behavior,
- terminates it automatically,
- restarts a healthy worker,
- prints: detection time, peak CPU, and recovery outcome.

## Security and Reliability Highlights

- Zero-shell execution path (`exec.Command` with structured args)
- Local-only API binding (`127.0.0.1` / `localhost`)
- Constant-time API key checks
- API request throttling + auth brute-force protection
- In-memory runtime state guarded by `sync.RWMutex`
- Secret redaction before dashboard/state exposure
- Graceful process-group shutdown with forced fallback
- Structured audit logs (`kill`/`restart` actor + reason + timestamp)
- Decision traces (CPU score, entropy score, confidence score)

## API Endpoints

- `GET /incidents`
- `GET /stream`
- `POST /process/kill`
- `POST /process/restart`
- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /timeline`

## Configuration

- Config file: `sentry.yaml` (or `--config`)
- Environment:
  - `SENTRY_API_KEY` for mutating API endpoints
  - `SENTRY_MASTER_KEY` for DB field encryption (64 hex chars)
  - `SENTRY_ALLOWED_ORIGIN` for CORS allow-list (default `http://localhost:3000`)
  - `SENTRY_BIND_HOST` (`127.0.0.1`/`localhost` only)
  - `NEXT_PUBLIC_SENTRY_API_BASE` for dashboard base URL

## Demo Mode

Run an explicit value demo without full installer:

```bash
./sentry demo
```

Output summary:
- `Runaway detected in X seconds`
- `CPU peaked at Y%`
- `Process recovered`

## Installer Behavior

`scripts/install.sh` now:
- auto-generates secure API/master keys (printed once),
- stores them in `.sentry.env`,
- runs production dashboard build (`next build`) and server (`next start`),
- optionally opens browser (`--open-browser`),
- defaults to localhost binding and sensible zero-config values.

```bash
./scripts/install.sh --open-browser
```

---

## Development

Dashboard:

```bash
cd dashboard
npm ci
npm run build
```

## Security Documentation

- Threat model: `docs/THREAT_MODEL.md`
- Operations guide: `docs/OPERATIONS.md`
- Security policy: `SECURITY.md`
