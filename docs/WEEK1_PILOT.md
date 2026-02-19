# Week 1 Execution Checklist

This checklist completes Day 3 to Day 7 of Week 1.

## Day 3: Real Workload Pilot

Run:

```bash
./scripts/week1_pilot.sh
```

Expected outcomes:
- `healthy` profile completes without intervention.
- `bursty` profile completes without intervention.
- `runaway` profile is detected and terminated.

Review:
- `pilot_artifacts/.../summary.md`
- `pilot_artifacts/.../incidents_snapshot.txt`

## Day 4: Detection Quality Tuning

Run:

```bash
./scripts/tune_detection.sh
```

Pick threshold by evidence:
- keep `loop_detected=yes` for runaway
- keep `loop_detected=no` for bursty

Save selected threshold in your deployment notes.

## Day 5: Recovery Reliability

Run:

```bash
./scripts/recovery_drill.sh
```

Must pass:
- no orphan process after parent shutdown
- no orphan process after API kill

## Day 6: Clarity + Runbook

Required docs:
- `docs/RUNBOOK.md`
- `README.md` quick path + local verification gate

Operator can answer in < 5 minutes:
- what triggered action
- why it triggered
- how to recover manually

## Day 7: Release Checkpoint

Run:

```bash
./scripts/release_checkpoint.sh
```

Pass criteria:
- strict local verification gate passes (`./scripts/verify_local.sh --strict`)
- no tracked secret/runtime artifacts
- no legacy naming
- runbook and pilot docs present
