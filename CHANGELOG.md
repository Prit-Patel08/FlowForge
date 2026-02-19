# Changelog

All notable changes to this project are documented in this file.

## v0.2.0-stable - 2026-02-19

### Added
- Incident drilldown UX with incident grouping, shareable incident links, and per-event filtering.
- API contract snapshot tests for `/timeline` and `/timeline?incident_id=...`.
- CI smoke gate plus Docker runtime validation job (non-root and healthcheck assertions).
- Local smoke script for release checks: build, demo, API, dashboard, health/metrics/timeline probes.
- Installer build-only mode (`--no-services`) and clearer post-install summary output.

### Changed
- Installer now uses production dashboard flow (`next build` + `next start`) and safer startup/cleanup handling.
- Benchmark corpus gate now includes runtime regression thresholds and artifact uploads.
- End-to-end incident correlation improved via `incident_id` drilldown path.
- Policy decider now uses progress-aware guard (`RawDiversity`, `ProgressLike`) to reduce healthy-spike false positives.

### Security
- Continued hardening of auth/rate limiting workflows and CI verification pipeline.

### Validation
- GitHub Actions checks green: `backend`, `dashboard`, `smoke`, `docker`, `sbom`.

## v0.1.0 - 2026-02-18

### Added
- Initial local-first supervision engine with Go backend and Next.js dashboard.
- API key auth, health probes, Prometheus metrics, process monitoring/restart logic.
