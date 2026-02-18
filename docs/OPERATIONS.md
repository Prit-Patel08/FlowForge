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
  -e SENTRY_API_KEY="$(openssl rand -hex 32)" \
  -e SENTRY_MASTER_KEY="$(openssl rand -hex 32)" \
  -p 8080:8080 \
  agent-sentry
```

## Crash Recovery Model

- Supervisor sends termination signals to subprocess process-groups.
- On shutdown, process groups are first terminated gracefully then force-killed after timeout.
- Incident records remain in `sentry.db` for post-mortem review.
- Audit events include actor, action, reason, and timestamp for kill/restart operations.
- Decision traces capture CPU score, entropy score, and confidence score for intervention transparency.

## Demo Mode

```bash
./sentry demo
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
