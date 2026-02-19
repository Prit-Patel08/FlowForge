# FlowForge Operator Runbook

## 1. Start Service

```bash
./scripts/install.sh --open-browser
```

Or backend only:

```bash
./flowforge dashboard
```

## 2. Smoke Check (Release Gate)

```bash
./scripts/smoke_local.sh
```

Expected output:
- demo summary includes:
  - `Runaway detected in ...`
  - `CPU peaked at ...`
  - `Process recovered`
- endpoint probes pass:
  - `GET /healthz` returns `{"status":"ok"}`
  - `GET /metrics` includes `flowforge_uptime_seconds`
  - `GET /timeline` returns JSON array payload

## 3. Daily Supervision

```bash
./flowforge run -- python3 your_worker.py
```

Recommended starting thresholds:
- `--max-cpu 85` for general workers
- `--max-cpu 70` for stricter runaway control

## 4. Incident Triage

1. Open dashboard timeline (`/timeline`) and select an incident group.
2. Inspect the drilldown chain loaded from `/timeline?incident_id=<id>`.
3. Check reason text and scores (CPU, entropy, confidence).
4. Confirm whether the action was expected:
- Expected: repeated output + sustained high CPU.
- Unexpected: short burst, startup compile, one-time spikes.

## 5. Manual Actions

Use API key protected endpoints:

```bash
curl -X POST \
  -H "Authorization: Bearer $FLOWFORGE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"reason":"operator manual restart"}' \
  http://127.0.0.1:8080/process/restart
```

```bash
curl -X POST \
  -H "Authorization: Bearer $FLOWFORGE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"reason":"operator emergency stop"}' \
  http://127.0.0.1:8080/process/kill
```

## 6. Recovery Checks

Run reliability drills:

```bash
./scripts/recovery_drill.sh
```

The drill must confirm:
- parent SIGTERM leaves no child orphan
- API kill removes active process

## 7. Detection Tuning

Run pilot and threshold tuning:

```bash
./scripts/week1_pilot.sh
./scripts/tune_detection.sh
```

Choose a threshold where:
- runaway profiles are terminated
- bursty/healthy profiles are not terminated

## 8. What FlowForge Does Not Do

- It does not sandbox untrusted code.
- It does not replace OS/container isolation.
- It does not guarantee zero false positives.
- It does not provide cloud sync or remote policy distribution.
