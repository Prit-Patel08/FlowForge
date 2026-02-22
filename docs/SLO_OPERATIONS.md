# FlowForge SLO Operations and Weekly Ritual

Date: 2026-02-21  
Owner: Reliability Lead (with Engineering + Product)  
Status: Active

## Purpose

Formalize weekly SLO dashboard operations and error-budget decisions so reliability governance is consistent and auditable.

This document is the canonical process for:
1. weekly SLO review
2. error-budget status decisions
3. reliability action ownership

## Inputs

Required:
1. local API endpoints: `/healthz`, `/readyz`, `/metrics`, `/timeline`
2. SQLite events database (`FLOWFORGE_DB_PATH`, default `flowforge.db`)
3. weekly report script:

```bash
./scripts/slo_weekly_review.sh --days 7
```

Output directory default:
- `pilot_artifacts/slo-weekly-<timestamp>/`

## SLO Groups and Weekly Targets

### Group A: Detection and Action

1. Detection->Action latency p95:
- target: `<= 15s`
- source: incident-linked `decision` and `audit` events

2. Low-confidence destructive actions ratio:
- target: `<= 0.10`
- definition: incidents with destructive action (`AUTO_KILL`, `AUTO_RESTART`, `KILL`, `RESTART`, `TERMINATE`) where incident confidence `< 0.60`

### Group B: Runtime Stability

1. Restart-storm violations (10-minute buckets):
- target: `0`
- definition: any run with `AUTO_RESTART > 3` in the same 10-minute bucket

2. Restart budget blocks (manual/API):
- target: `<= 2` per week
- source: `/metrics` counter `flowforge_restart_budget_block_total` (weekly delta)

### Group C: API and Dashboard Operations

1. API probe failures (`/healthz`, `/readyz`, `/metrics`, `/timeline`):
- target: `0`

2. Latest event age (data freshness):
- target: `<= 300s`

## Weekly Ritual (Mandatory)

Cadence:
- Once per week (same weekday/time), before roadmap planning.

Procedure:
1. Ensure FlowForge API is running:

```bash
./flowforge dashboard
```

2. Generate weekly report:

```bash
./scripts/slo_weekly_review.sh --days 7
```

3. Review `slo_weekly_report.md` in meeting.
4. Assign top 3 reliability actions with owner + due date.
5. Publish decision (`GREEN`, `YELLOW`, `RED`) in the weekly notes.
6. Link output artifact path in tracking docs/issues.

## Error-Budget Decision Policy

Based on current SLO scoreboard fail count:

1. `GREEN` (0 failed checks):
- continue planned roadmap execution

2. `YELLOW` (1 failed check):
- prioritize reliability fixes
- defer non-critical expansion for current week

3. `RED` (2+ failed checks):
- feature freeze
- start reliability sprint until SLO state recovers

Optional CI/automation gate:

```bash
./scripts/slo_weekly_review.sh --days 7 --fail-on-breach
```

This exits non-zero when status is `RED`.

## Meeting Output Contract

Every weekly review must produce:
1. generated report path
2. chosen error-budget status
3. top 3 risks
4. assigned owners and due dates
5. rollback/readiness impact note (if `YELLOW`/`RED`)

## Artifact Contract

Each run includes:
1. `slo_weekly_report.md`
2. `summary.tsv`
3. API probe outputs:
- `healthz.json`
- `readyz.json`
- `metrics.prom`
- `timeline.json`

Retention:
- keep the latest 8 weekly artifact directories.
