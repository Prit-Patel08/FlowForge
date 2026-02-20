# FlowForge Alignment Audit (Blueprint vs Current Build)

Date: 2026-02-20  
Scope: verify alignment against the 10-domain infrastructure company blueprint.

## Verdict

Current direction is broadly aligned with your blueprint for domains 1, 2, 3, 4, 8, and 9 foundations.

Main gaps are in:
- full event-model unification
- policy canary lifecycle
- formal SLO/error-budget operations
- integration productization (domain 10)
- cost intelligence (domain 6)

## What Is Aligned and Kept

1. Domain 1 (Execution Runtime Guard)
- process-group teardown, signal trap, and crash-loop protection baseline are implemented.

2. Domain 2 (Decision Intelligence)
- multi-signal scoring and reason output are in production path.
- shadow-mode policy behavior exists.

3. Domain 3 (Policy and Governance)
- policy dry-run/shadow mode exists.
- policy controls are present but still early.

4. Domain 4 (Evidence and Audit)
- append-only events baseline exists with timeline query support.
- audit/event recording exists and is test-covered.

5. Domain 8 (Security and Trust)
- API key auth, constant-time compare, local bind defaults, redaction baseline.
- CI includes vulnerability checks and SBOM artifact.

6. Domain 9 (Reliability Engineering)
- release checkpoint gate exists.
- soak checks and recovery drill workflows exist.
- branch protection + required checks are active.

## What Is Partially Aligned (Realign Required)

1. Evidence model still has legacy overlap
- `incidents`, `audit_events`, `decision_traces`, and `events` coexist.
- action: converge to one canonical event envelope over time.

2. Decision engine versioning
- reason outputs exist, but formal `engine_version` contract is not yet first-class.
- action: add decision engine metadata contract in schema and API payloads.

3. Policy lifecycle governance
- shadow mode exists; canary rollout and approval workflow are still planned.
- action: add policy rollout states and policy-change gates.

4. SLO ritual maturity
- checks exist technically, but reliability governance rituals are not fully formalized.
- action: add weekly SLO dashboard + error budget policy doc + owner cadence.

## What Was Unnecessary and Removed

1. `trigger.txt` was removed from the repository.
2. `trigger.txt` was added to `.gitignore` to avoid accidental reintroduction.
3. `FLOWFORGE_DOCS.md` was removed (redundant/unreferenced top-level doc dump).
4. `demo/stuck.py` was removed (unused duplicate of root `stuck.py` demo script).
5. `pilot_commands.txt` was removed from tracking (local user file; `scripts/pilot_commands.example.txt` remains canonical).
6. Root `Dockerfile` is the single canonical runtime image spec (duplicate internal path removed).

## What We Intentionally Did Not Remove

1. Existing runbooks/checklists/docs that support current operations.
2. Current runtime and API architecture while still in staged hardening.
3. Generated CLI reference docs in `docs/` (kept for documentation-as-product consistency).

## Alignment Plan (Short-Term)

1. Implement unified event schema design + migration plan doc and execution PR.
2. Implement policy canary workflow (shadow -> canary -> enforce).
3. Formalize SLO and error-budget operating ritual.
4. Run first chaos drill and publish findings.

## Alignment Rule Going Forward

Any new work is accepted only if it maps to a blueprint domain and has:
- measurable success criteria
- rollback plan
- explainability preservation
- release checkpoint compatibility
