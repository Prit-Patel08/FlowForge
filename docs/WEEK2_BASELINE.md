# Week 2 Baseline Decision

## Locked Default

- Default `max-cpu` threshold: **60.0**
- Profile: `standard`

## Why 60.0

From local tuning runs:
- runaway workload consistently detected at 30, 40, 50, 60
- bursty workload did not trigger false loop detection at 30, 40, 50, 60

Artifacts:
- `pilot_artifacts/tuning-latest/summary.md`
- `pilot_artifacts/tuning-latest/results.csv`

## How to Use

1. Copy baseline config:

```bash
cp flowforge.yaml.example flowforge.yaml
```

2. Start with standard profile:

```bash
./flowforge run --profile standard -- python3 your_worker.py
```

3. If workload is latency-sensitive and noisy:
- try `--profile light` first.

4. If workload is strict-risk / high-cost:
- try `--profile heavy`.
