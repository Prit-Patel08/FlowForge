# Agent-Sentry

**Autonomous Supervision & Security Layer for AI Agents.**

## Hardened Architecture (v2.0)

This version represents a production-hardened refactor focusing on Zero-Shell execution, strict network isolation, and high-performance monitoring.

### Key Features
- **Zero-Shell Restarts**: Structured argument execution eliminates command injection risks.
- **Native Monitoring**: `gopsutil` integration replaces legacy `lsof`/`ps` shelling.
- **In-Memory State**: Thread-safe state management with `sync.RWMutex`.
- **API Security**: 127.0.0.1 binding and constant-time API key comparisons.

## Production Deployment

### 1. Docker (Hardened)

```bash
docker build -t agent-sentry .
docker run -p 8080:8080 \
  -e SENTRY_API_KEY=$(openssl rand -hex 32) \
  -e SENTRY_MASTER_KEY=$(openssl rand -hex 32) \
  agent-sentry
```

### 2. Configuration

Set the following environment variables:
- `SENTRY_API_KEY`: Required for Kill/Restart API actions.
- `SENTRY_MASTER_KEY`: 64-char hex string for DB encryption.

---

## Development

```bash
# Build
go build ./...

# Test with Race Detector
go test -v -race ./...
```

See [SECURITY.md](SECURITY.md) for more hardening details.
