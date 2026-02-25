## SECTION 1 — Architecture Audit

1. **[P1] Monolithic boundaries are already limiting change velocity.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:293` (route graph + handlers + metrics + replay logic in one file), `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:174` (schema + migrations + query layer + event mapping in one file), `/Users/pritpatel/Desktop/FlowForge/cmd/run.go:365` (process orchestration + policy + persistence + API bootstrap).  
Refactor: split into `domain` + `application` + `adapters` (hexagonal).  
Files to add: `/Users/pritpatel/Desktop/FlowForge/internal/domain/*`, `/Users/pritpatel/Desktop/FlowForge/internal/app/*`, `/Users/pritpatel/Desktop/FlowForge/internal/adapters/{sqlite,http,cli}/*`.  
Result: lower coupling, testability, easier cloud control-plane evolution.

2. **[P1] Layering violations are frequent.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/cmd/run.go:6-15` imports API, DB, state, policy, supervisor directly; `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:1670` reads lifecycle state and DB in handlers directly.  
Refactor: command layer should call application services only; API should call use-cases, not DB/state globals.

3. **[P1] Global singleton state creates hidden coupling and nondeterministic behavior under concurrency/tests.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:18` (`var db *sql.DB`), `/Users/pritpatel/Desktop/FlowForge/internal/api/worker_lifecycle.go:621` (`var workerControl`), `/Users/pritpatel/Desktop/FlowForge/internal/state/state.go:27` (`currentState`).  
Refactor: constructor-injected dependencies (`Server{Repo, LifecycleManager, Metrics}`), no mutable package globals outside `main` composition.

4. **[P2] API route surface is mixed legacy + v1 without deprecation protocol.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:293-332`.  
Refactor: create explicit route groups: `/v1/*` primary, legacy routes behind compatibility middleware with deprecation headers and removal date.

5. **[P2] Architecture docs and runtime behavior are drifting.**  
Evidence: docs claim SQLite WAL strategy but runtime does not set WAL/busy timeout (`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:174-475`, no PRAGMA).  
Refactor: enforce architecture assertions in startup checks and CI contract tests.

---

## SECTION 2 — Backend (Go) Deep Review

1. **[P0] Auth logic bug allows unauthenticated non-POST mutations when `FLOWFORGE_API_KEY` is unset.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:123-131` blocks only `POST`; `DELETE /v1/integrations/workspaces/{id}` uses `requireAuth` (`/Users/pritpatel/Desktop/FlowForge/internal/api/integration.go:183-190`).  
Live verification: POST register returned `403`; DELETE unregister without auth reached business logic and returned `404`.  
Fix: in `requireAuth`, treat all non-safe methods (`POST|PUT|PATCH|DELETE`) as mutating when key unset.

2. **[P1] Data race risk in runtime monitor path.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/cmd/run.go:447` declares `maxObservedCpu`; monitor goroutine mutates it (`:531`), signal/main flow reads it (`:858`, `:897`).  
Fix: use `atomic.Uint64` via `math.Float64bits` or protect shared run state with mutex.

3. **[P1] `state.GetState()` leaks internal mutable slice reference.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/state/state.go:67-72` returns struct copy containing `Args []string` header referencing internal slice.  
Fix: deep-copy slice on read path before return.

4. **[P1] DB initialization is not concurrency-safe and runs in request path.**  
Evidence: repeated `if database.GetDB()==nil { InitDB() }` in handlers (`/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:417`, `:1697`, `:1743`), no `sync.Once` in DB package.  
Fix: initialize DB once at server start; use `sync.Once` + explicit readiness failure state.

5. **[P1] `context.Context` usage is shallow for DB operations.**  
Evidence: DB access uses `Query/Exec/QueryRow` extensively (`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go`) instead of `QueryContext/ExecContext`.  
Fix: pass request context through repository methods to bound latency and support cancellation.

6. **[P1] Error handling is inconsistent and loses root cause under load.**  
Evidence: many ignored errors (`_ = database.Log...` at `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:1801`, `:1864`; migration `db.Exec(...)` ignored at `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:212-221`, `:340-346`).  
Fix: promote structured error policy: `errors.Wrap`, explicit failure classes, mandatory handling in migrations and audit/event writes.

7. **[P1] Logging is not production-grade structured telemetry.**  
Evidence: `fmt.Printf`/`fmt.Println` in runtime paths (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:372`, `:676`, `:811`), `log.Printf` in API (`/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:1905`).  
Fix: use structured logger (`slog`/`zap`) with fields: `request_id`, `incident_id`, `action`, `pid`, `workspace_id`.

8. **[P2] Observability has metrics but no tracing and no log correlation sink.**  
Good: metrics coverage for lifecycle/idempotency is strong (`/Users/pritpatel/Desktop/FlowForge/internal/metrics/metrics.go`).  
Gap: no OpenTelemetry spans across API -> DB -> lifecycle actions.  
Fix: add OTel HTTP middleware + DB spans + request_id baggage.

9. **[P1] API coverage is too low for risky paths.**  
Measured: `internal/api` coverage is `2.7%` (`go test ... -cover`).  
Fix: prioritize table-driven tests for auth, idempotency conflicts, lifecycle transitions, integration endpoints.

10. **[P1] Hot path performance inefficiency: regex compiled per call.**  
Evidence: `NormalizeLog` compiles regex each invocation (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:178-193`).  
Fix: precompile regex at package scope.

11. **[P1] Unbounded incident reads and polling can become O(N) every few seconds.**  
Evidence: `GetAllIncidents()` returns full set (`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:630-660`), dashboard polls `/v1/incidents` every 6s (`/Users/pritpatel/Desktop/FlowForge/dashboard/pages/index.tsx:138-143`).  
Fix: cursor pagination (`?after_id=&limit=`), delta endpoints, and server-side caps.

12. **Dependency review outcome.**  
Good: `govulncheck ./...` reports `No vulnerabilities found`; CI includes `govulncheck` and `staticcheck` (`/Users/pritpatel/Desktop/FlowForge/.github/workflows/test.yml`).  
Risk: `github.com/mattn/go-sqlite3` + CGO dependency conflicts with Docker build strategy (see Section 6).

---

## SECTION 3 — Task Supervision & Safety Logic

1. **Good core reliability primitive: process-group teardown is correct and deterministic.**  
Evidence: `Setpgid=true` and TERM->KILL escalation in `/Users/pritpatel/Desktop/FlowForge/internal/supervisor/supervisor.go:50-160`.  
Why good: prevents orphan subprocess trees and gives bounded stop semantics.

2. **[P1] Crash recovery is partial and mostly in-memory.**  
Evidence: lifecycle state held in singleton (`/Users/pritpatel/Desktop/FlowForge/internal/api/worker_lifecycle.go:621`), not durable across daemon restart except minimal PID/state files for daemon itself.  
Fix: persist lifecycle state machine events to DB and replay on startup.

3. **[P1] Restart policy lacks backoff/jitter and restart cause classification.**  
Evidence: restart budget exists (`/Users/pritpatel/Desktop/FlowForge/internal/api/worker_lifecycle.go:280-315`), but no exponential backoff or failure buckets.  
Fix: add restart policy object `{max, window, backoff_curve, classify(exit_reason)}`.

4. **[P1] Resource limits are advisory, not enforced isolation.**  
Evidence: memory/token checks in monitor loop (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:803-845`) but no RLIMIT/cgroup constraints.  
Fix: add optional cgroup v2 or container sandbox mode with hard quotas.

5. **[P1] No sandbox boundary for untrusted command execution.**  
Evidence: child command runs as same OS user with inherited environment (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:407`).  
Fix: add isolated execution adapters: rootless container, user namespace jail, seccomp profile, env allowlist.

6. **[P1] Secret redaction is not comprehensive enough for command-level leakage.**  
Evidence: redaction only applied to log lines (`/Users/pritpatel/Desktop/FlowForge/internal/redact/redact.go:13-20` + `/Users/pritpatel/Desktop/FlowForge/cmd/run.go:131`), but command/args are persisted raw (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:381`, `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:586`).  
Fix: add command sanitizer before persistence and UI exposure.

---

## SECTION 4 — CLI & API Design

1. **[P1] REST semantics violation: GET endpoint mutates server state.**  
Evidence: `/v1/integrations/workspaces/{id}/status` updates `active_pid` in DB (`/Users/pritpatel/Desktop/FlowForge/internal/api/integration.go:265`).  
Fix: make GET read-only; move updates to worker lifecycle event ingestion.

2. **[P0] Auth contract inconsistency on mutating verbs (POST blocked, DELETE not blocked when key unset).**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:123-131`.  
Fix: enforce auth on all unsafe methods, then add regression tests.

3. **Good: idempotency implementation is strong for POST control-plane mutations.**  
Evidence: key parsing, request hashing, replay/conflict audit in `/Users/pritpatel/Desktop/FlowForge/internal/api/idempotency.go:27-90`.  
Why good: protects duplicate operator actions and creates audit-grade traces.

4. **[P1] Versioning strategy is implicit and lacks deprecation governance.**  
Evidence: dual legacy/v1 routes in same mux (`/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:293-332`).  
Fix: publish deprecation header `Sunset`, create removal schedule, track usage metrics by route prefix.

5. **[P1] Pagination/filtering are inconsistent.**  
Evidence: incidents endpoint returns full dataset (`/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:1729-1759`, DB call full set), timeline hardcoded limit 100 (`:1717`).  
Fix: consistent query contract: `limit`, `cursor`, `sort`, `filters`; response envelope with `next_cursor`.

6. **Good: RFC7807 problem details and request correlation are implemented.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:1944-1988`.  
Why good: supports supportability and external integration troubleshooting.

7. **OpenAPI recommendation (must implement).**  
Files to add: `/Users/pritpatel/Desktop/FlowForge/api/openapi/v1.yaml`, `/Users/pritpatel/Desktop/FlowForge/scripts/validate_openapi.sh`.  
CI gate: validate schema + contract tests against OpenAPI examples.

8. **[P1] CORS method list is incomplete for actual API surface.**  
Evidence: `Access-Control-Allow-Methods: GET, POST, OPTIONS` in `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:90`; but DELETE endpoint exists.  
Fix: include DELETE (and any other supported unsafe verbs).

---

## SECTION 5 — Dashboard (Next.js)

1. **[P1] Polling architecture causes redundant fetch load and timing skew.**  
Evidence: seven independent polling hooks in `/Users/pritpatel/Desktop/FlowForge/dashboard/pages/index.tsx:138-181`.  
Fix: use unified query client (SWR/React Query) with deduping, stale-time, and shared refresh cadence.

2. **[P1] Control actions do not send idempotency keys.**  
Evidence: kill/restart POSTs in `/Users/pritpatel/Desktop/FlowForge/dashboard/components/dashboard/LiveMonitorPanel.tsx:28-37`.  
Fix: add `Idempotency-Key: <uuid>` per action request.

3. **[P1] CSP and font loading policy conflict.**  
Evidence: CSP allows `style-src 'self'` only (`/Users/pritpatel/Desktop/FlowForge/dashboard/next.config.ts:16`) while CSS imports Google Fonts (`/Users/pritpatel/Desktop/FlowForge/dashboard/styles/globals.css:1`).  
Fix: self-host fonts via `next/font/local` or expand CSP explicitly and intentionally.

4. **[P2] No dashboard test/lint pipeline beyond build.**  
Evidence: `dashboard/package.json` has only `dev/build/start` scripts, no lint/test.  
Fix: add `eslint`, `typecheck`, component tests (Vitest + Testing Library), Playwright smoke.

5. **[P2] Full CSR is acceptable for local ops console, but SSR fallback for first paint is missing.**  
Evidence: page uses client polling hooks only (`/Users/pritpatel/Desktop/FlowForge/dashboard/pages/index.tsx`).  
Fix: server-side bootstrap snapshot (optional) to reduce blank/empty initial state.

6. **Good: request trace panel safely encodes request ID and bounds trace size.**  
Evidence: encodeURIComponent + `limit=200` in `/Users/pritpatel/Desktop/FlowForge/dashboard/components/dashboard/RequestTracePanel.tsx:40`.  
Why good: avoids path injection and runaway payloads.

7. **[P2] Unused dependency indicates stale dependency hygiene.**  
Evidence: `swr` listed but not used (`/Users/pritpatel/Desktop/FlowForge/dashboard/package.json`, no imports found).  
Fix: either adopt SWR centrally or remove it.

---

## SECTION 6 — DevOps & CI/CD

1. **Good: CI breadth is above typical early-stage projects.**  
Evidence: shellcheck, contract tests, race, staticcheck, govulncheck, smoke, replay drill, docker runtime, sbom in `/Users/pritpatel/Desktop/FlowForge/.github/workflows/test.yml`.  
Why good: catches broad regression classes.

2. **[P0] Docker runtime image is incompatible with SQLite driver strategy.**  
Evidence: build disables CGO (`/Users/pritpatel/Desktop/FlowForge/Dockerfile:7`) while DB uses `mattn/go-sqlite3` (`/Users/pritpatel/Desktop/FlowForge/go.mod:7`).  
Observed behavior: CGO-disabled binary served health but `/v1/incidents` returned 500 “Database not initialized”.  
Fix path A: enable CGO in runtime image and include required libc.  
Fix path B: migrate to pure-Go sqlite driver (`modernc.org/sqlite`) for static builds.

3. **[P1] Docker health smoke is too shallow.**  
Evidence: workflow checks health endpoint only (`Runtime health smoke` in `/Users/pritpatel/Desktop/FlowForge/.github/workflows/test.yml`).  
Fix: add DB-dependent probe (`/v1/incidents` with seeded DB) to catch CGO/driver failures.

4. **[P1] Missing security CI gates for app+container.**  
Needed: CodeQL, Trivy/Grype image scan, npm audit, secret scan (gitleaks), SLSA provenance attestation.

5. **[P1] Release workflow lacks promotion model.**  
Evidence: only manual checkpoint workflow (`/Users/pritpatel/Desktop/FlowForge/.github/workflows/release-checkpoint.yml`).  
Fix: add release pipeline with environments (`staging` -> `prod`), approval gate, changelog generation, signed artifacts.

6. **[P2] Semantic versioning exists but is not automated by release branches/tags.**  
Evidence: tags exist, but no automated semantic release process.  
Fix: conventional commits + release-please or semantic-release-go + signed tags.

---

## SECTION 7 — Security Audit

1. **Threat model (current attack surface).**  
Local attacker process on same host, malicious local web origin, compromised CI dependency, leaked API key, untrusted supervised command with inherited env.

2. **[P0] Auth bypass on DELETE when API key unset.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:123-131`.  
Mitigation: block all mutating verbs without key, add test for `DELETE /v1/integrations/workspaces/{id}` no-key path.

3. **[P0] Encryption fail-open for sensitive incident fields.**  
Evidence: encryption errors ignored and plaintext fallback in `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:569-578`; decrypt fallback returns raw string (`:1397-1404`).  
Mitigation: fail-closed writes when encryption required, or explicit plaintext mode flag with telemetry and policy denial in secure mode.

4. **[P0] Command-line secrets can be persisted and exposed.**  
Evidence: raw command persisted via `fullCommand` (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:381`, `/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:586`).  
Mitigation: sanitize command args with deterministic redaction policy before state/database writes.

5. **[P1] API key model is single static bearer, no rotation/scope/expiry.**  
Mitigation: introduce key IDs, scoped permissions, expiry metadata, and key revocation.

6. **[P1] Idempotency keys stored as plaintext.**  
Evidence: schema stores `idempotency_key TEXT` (`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:423`).  
Mitigation: store keyed hash, not raw key material.

7. **[P1] Local privilege escalation risk in supervised workload execution.**  
Evidence: process runs as current user with full inherited environment (`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:407`).  
Mitigation: optional reduced-privilege execution profile with env allowlist and filesystem/network restrictions.

8. **Supply chain posture.**  
Good: SBOM generation and govulncheck in CI.  
Gap: no provenance signature, no image vulnerability gate, no dependency policy enforcement (license/security allowlist).

---

## SECTION 8 — Testing Strategy

1. **Current baseline.**  
Measured coverage: `cmd 14.7%`, `internal/api 2.7%`, `internal/database 50.3%`, `internal/supervisor 70.1%`.  
Interpretation: high-risk HTTP/auth/control-plane logic under-tested.

2. **[P0] Add auth regression tests immediately.**  
Add cases in `/Users/pritpatel/Desktop/FlowForge/test/api_test.go`:  
`no API key + DELETE unregister -> 403`; `no API key + POST kill/restart -> 403`; `GET remains allowed`.

3. **[P0] Add Docker/CGO compatibility test.**  
New test script under `/Users/pritpatel/Desktop/FlowForge/scripts/` validating runtime container can serve `/v1/incidents` not just `/healthz`.

4. **[P1] Property-based tests for deterministic replay and normalization.**  
Targets: replay digest stability in `/Users/pritpatel/Desktop/FlowForge/internal/policy/*`; log normalization in `/Users/pritpatel/Desktop/FlowForge/cmd/run.go` should be idempotent and monotonic under random inputs.

5. **[P1] Failure simulation tests.**  
Add chaos-style tests for abrupt child exit, hung stop, zombie tree, DB lock contention, SIGTERM during restart.

6. **[P1] Load and soak plan.**  
Add k6/vegeta profile for API (`incidents/timeline/request-trace/lifecycle`) and long-run worker soak with synthetic logs.

7. **[P2] Frontend test suite.**  
Add component tests for action controls, retry/error states, and request trace panel parsing; add Playwright E2E against local daemon.

8. **Deterministic reproducibility.**  
Pin seeds for fuzz/property tests, capture fixtures under `/Users/pritpatel/Desktop/FlowForge/test/fixtures`, and enforce deterministic CI locale/timezone.

---

## SECTION 9 — Documentation & DX

1. **[P1] Missing legal/community baseline docs.**  
No `LICENSE`, no `CONTRIBUTING.md`, no `CODEOWNERS`.  
Add these before external contributor growth.

2. **[P1] README is comprehensive but overloaded for new operators.**  
Evidence: single large operations-heavy file (`/Users/pritpatel/Desktop/FlowForge/README.md`).  
Refactor into: Quickstart, Operator Runbook, API Contract, Architecture, Security, Troubleshooting.

3. **[P1] Missing formal API contract docs.**  
No OpenAPI spec present.  
Add `/Users/pritpatel/Desktop/FlowForge/api/openapi/v1.yaml` and generated markdown docs.

4. **[P2] Onboarding friction around dashboard startup context.**  
Users repeatedly run npm from wrong directory; docs should standardize commands with `npm --prefix dashboard ...` examples.

5. **Good: operational scripts and doctor tooling are strong for local reproducibility.**  
Evidence: `/Users/pritpatel/Desktop/FlowForge/Makefile`, tooling/contract scripts in `/Users/pritpatel/Desktop/FlowForge/scripts`.

---

## SECTION 10 — Scaling Strategy

1. **Single-node to distributed path (recommended).**  
Phase A: keep local agent + local SQLite for edge resilience.  
Phase B: emit signed event stream to cloud ingest API (outbox + retry).  
Phase C: cloud control-plane persists in Postgres, analytics in columnar/time-series.

2. **Multi-tenant design path.**  
Introduce `tenant_id` in all event/audit/decision records and API auth context.  
Use tenant-scoped keys and policy bundles.

3. **Cloud control plane integration.**  
Add agent registration, heartbeat, command channel, event ingestion with idempotency and exactly-once semantics via outbox table.

4. **Database scaling.**  
Local mode: SQLite with WAL + busy timeout + periodic compaction job.  
Cloud mode: Postgres partitioned tables (`events` by day/tenant), async rollups for dashboard KPIs.

5. **Horizontal scaling.**  
Stateless API instances behind LB; lifecycle controls routed by workspace/agent identity; message bus for action fanout and replay.

---

## SECTION 11 — Monetization Readiness

1. **Enterprise blockers today.**  
No RBAC, no tenant model, no scoped keys, no immutable compliance-grade audit retention policy.

2. **Audit readiness.**  
Good: append-only `events` trigger exists (`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:373-385`).  
Gap: audit export signatures use HMAC shared secret (`/Users/pritpatel/Desktop/FlowForge/internal/evidence/bundle.go:195-200`), which is weaker for non-repudiation than asymmetric signatures.

3. **Compliance (SOC2-like) missing controls.**  
Need access reviews, key rotation policy, secure SDLC evidence, incident response runbook, backup/restore attestations, environment separation.

4. **Billing hooks missing.**  
No metering primitives for seats, protected runtime-minutes, action volume, data retention tier.

5. **API rate limiting model is not enterprise-grade.**  
Current limiter is in-memory per process/IP (`/Users/pritpatel/Desktop/FlowForge/internal/api/ratelimit.go`).  
Need distributed, tenant-scoped, policy-driven limits.

---

## SECTION 12 — Final Verdict

1. **Strengths.**  
Process supervision teardown logic is robust and production-oriented; idempotency replay/conflict handling is well designed; CI breadth is unusually strong; RFC7807 + request-id correlation is correctly implemented; deterministic decision replay primitives are a strong differentiator.

2. **Weaknesses.**  
Security-critical auth gap on DELETE, encryption fail-open, command secret persistence risk, monolithic architecture hotspots, low API coverage, and Docker runtime/database incompatibility.

3. **Primary risk areas.**  
Authentication correctness, data confidentiality, runtime portability, and long-term maintainability under feature growth.

4. **30-day improvement roadmap (must execute).**  
Week 1: fix auth verb policy, add regression tests, patch CORS method list, add idempotency keys from dashboard controls.  
Week 2: fix encryption fail-open, add explicit secure-mode behavior, sanitize command/args before persistence.  
Week 3: resolve Docker SQLite compatibility, add DB-dependent runtime smoke in CI, enable SQLite WAL/busy timeout.  
Week 4: split `internal/api/server.go` by domain handlers, introduce repository interfaces, raise `internal/api` coverage from `2.7%` to at least `25%`.

5. **90-day roadmap (scale + enterprise foundation).**  
Month 2: implement OpenAPI contract + generated SDK stubs, cursor pagination, structured logging, OTel traces, and frontend test suite.  
Month 3: introduce tenant-aware auth model (scoped keys), RBAC skeleton, cloud event ingest outbox design, staged release pipeline with signed artifacts/provenance.

6. **Must-fix-before-release list (hard gate).**  
`/Users/pritpatel/Desktop/FlowForge/internal/api/server.go:123` auth bypass on non-POST mutations.  
`/Users/pritpatel/Desktop/FlowForge/internal/database/db.go:569-578` encryption fail-open.  
`/Users/pritpatel/Desktop/FlowForge/cmd/run.go:381` raw command persistence with possible secrets.  
`/Users/pritpatel/Desktop/FlowForge/Dockerfile:7` CGO-disabled build incompatible with current SQLite driver behavior.  
`/Users/pritpatel/Desktop/FlowForge/internal/state/state.go:67-72` state snapshot mutability leak.  
`/Users/pritpatel/Desktop/FlowForge/internal/api/integration.go:265` GET endpoint side-effect.
