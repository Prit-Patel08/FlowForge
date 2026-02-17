# Security Policy

## Supported Versions

Only the latest release of `agent-sentry` is supported for security updates.

## Reporting a Vulnerability

We take security seriously. If you find a vulnerability, please do NOT open a public issue. Instead, send a detailed report to `security@agent-sentry.io`.

### Disclosure Policy

- Provide detailed steps to reproduce.
- Allow 30 days for a patch before public disclosure.
- Do not perform destructive actions on public infrastructure.

## Security Hardening Features

This repository implements:
- **Zero-Shell Execution**: No use of `sh -c`. Arguments are passed as structured slices.
- **Strict Network Binding**: Dashboard API binds to `127.0.0.1` by default.
- **Constant-Time Auth**: API key comparisons use `subtle.ConstantTimeCompare`.
- **In-Memory State**: Runtime data is kept in memory with `sync.RWMutex`, not written to shared files.
