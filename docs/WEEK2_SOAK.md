# Week 2 Soak Plan (7 Days)

Goal: prove FlowForge is stable on repeated real workloads before `v0.1.0`.

## Daily Command

```bash
./scripts/week2_soak_check.sh pilot_commands.txt
```

This runs:
1. real-workload pilot
2. release checkpoint
3. false-positive / false-negative check

## Pass Criteria

- False positives: `0`
- False negatives: `0`
- Release checkpoint: pass every day
- No unexplained crash/lockup in operator logs
- `week2_soak_check.sh` exits non-zero on FAIL status

## What To Review Each Day

- `pilot_artifacts/soak-<run-id>/summary.md`
- `pilot_artifacts/soak-<run-id>/real-pilot/summary.md`
- `pilot_artifacts/soak-<run-id>/real-pilot/week2-pilot.db`
- `pilot_artifacts/soak-<run-id>/release-checkpoint/checkpoint.md`

## Promotion Rule

Promote to `v0.1.0` only after 7 consecutive daily PASS checks.
