# Operations Guide

## Health and Readiness

- Liveness: `GET /healthz`
- Readiness: `GET /readyz`
- Metrics: `GET /metrics` (Prometheus text format)
- Timeline: `GET /timeline` (incident + audit + decision trace feed)

## Hardened Container Run

```bash
docker run --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  -e FLOWFORGE_API_KEY="$(openssl rand -hex 32)" \
  -e FLOWFORGE_MASTER_KEY="$(openssl rand -hex 32)" \
  -p 8080:8080 \
  flowforge
```

## Crash Recovery Model

- Supervisor sends termination signals to subprocess process-groups.
- On shutdown, process groups are first terminated gracefully then force-killed after timeout.
- Incident records remain in `flowforge.db` for post-mortem review.
- Audit events include actor, action, reason, and timestamp for kill/restart operations.
- Decision traces capture CPU score, entropy score, and confidence score for intervention transparency.

## Demo Mode

```bash
./flowforge demo
```

Expected summary:
- `Runaway detected in X seconds`
- `CPU peaked at Y%`
- `Process recovered`

## Performance Validation

Run benchmark suite:

```bash
go test ./test -bench . -benchmem -run '^$'
```

Run race detector:

```bash
go test ./... -race -v
```

## Week 1 Reliability Pack

```bash
./scripts/week1_pilot.sh
./scripts/tune_detection.sh
./scripts/recovery_drill.sh
./scripts/release_checkpoint.sh
```

## Week 2 Real-Workload Pilot

1. Copy and edit command list:

```bash
cp scripts/pilot_commands.example.txt pilot_commands.txt
```

2. Replace commands with your real workloads.
  - quoted arguments are supported in `pilot_commands.txt` (shell-style quoting).

3. Run pilot:

```bash
./scripts/week2_real_pilot.sh pilot_commands.txt
```

Optional strict mode (non-zero exit if expectations fail):

```bash
PILOT_FAIL_ON_MISMATCH=1 ./scripts/week2_real_pilot.sh pilot_commands.txt
```

4. Review:
- `pilot_artifacts/.../summary.md`
- `pilot_artifacts/.../incidents_snapshot.txt`
- `pilot_artifacts/.../week2-pilot.db`
- `pilot_artifacts/.../results.csv` (`exit_reason` and `decision_reason` columns come from DB-correlated incidents)
