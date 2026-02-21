# Chaos Drill Evidence (2026-02-21)

Date (UTC): 2026-02-21  
Owner: Reliability  
Drill command:

```bash
./scripts/recovery_drill.sh pilot_artifacts/chaos-drill-20260221-published
```

Artifact directory:
- `pilot_artifacts/chaos-drill-20260221-published/`

## Scope

Two controlled failure scenarios were exercised:
1. parent supervisor receives `SIGTERM` and must not orphan worker processes
2. API kill path (`POST /process/kill`) must terminate active worker workload

## Results

### Drill A: Parent SIGTERM cleanup

Status: **PASS**

Evidence:
- `pilot_artifacts/chaos-drill-20260221-published/drill_sigterm.log`
- log line: `PASS: no orphan worker after parent SIGTERM`

### Drill B: API kill

Status: **PASS (with known transport race)**

Evidence:
- `pilot_artifacts/chaos-drill-20260221-published/drill_api_kill.log`
- log line: `WARN: /process/kill returned HTTP 000; treating as pass because worker exited (expected during shutdown race).`

Interpretation:
- The worker did terminate as expected.
- `HTTP 000` occurred because the supervising process can exit during shutdown before curl receives a full response.
- This is currently treated as acceptable for drill success when worker termination is confirmed.

## Findings

1. Process-group cleanup behavior works for operator `SIGTERM`.
2. API-driven kill action successfully removes the supervised workload.
3. API transport response reliability during kill is not deterministic (`200` vs `000`) due process-exit timing.

## Follow-up Actions

1. Short-term: keep drill acceptance rule as “worker exited” even when API kill transport code is `000`.
2. Medium-term: make `/process/kill` response deterministic (send acknowledgment before shutdown path completes).
3. Medium-term: remove startup warning `Failed to initialize database: no such column: created_at` by hardening legacy DB migration compatibility.

Update (2026-02-21):
- Follow-up action 2 is now implemented in API handler logic and covered by regression test `TestKillEndpointAcknowledgesAndTerminatesWorker`.
- Recovery drill gate now treats non-`200` `/process/kill` responses as failures.

## Conclusion

First chaos drill is completed and evidence is published.  
Core reliability objective passed: no orphan survivors and successful worker termination under injected failure paths.
