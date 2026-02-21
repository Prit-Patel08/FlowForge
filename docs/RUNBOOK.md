# FlowForge Operator Runbook

## 1. Start Service

```bash
./scripts/install.sh --open-browser
```

Build/setup without starting services:

```bash
./scripts/install.sh --no-services --no-demo
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

Expected response shape:
`{"status":"restart_requested","lifecycle":"STARTING","command":"<command>","pid":0}`

Guardrail:
- Restart only proceeds when no active worker PID is alive.
- If a worker is still running, API returns `409` and you should run `/process/kill` first.

```bash
curl -X POST \
  -H "Authorization: Bearer $FLOWFORGE_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"reason":"operator emergency stop"}' \
  http://127.0.0.1:8080/process/kill
```

Expected response shape:
`{"status":"stop_requested","lifecycle":"STOPPING","pid":12345}`

## 6. Recovery Checks

Run reliability drills:

```bash
./scripts/recovery_drill.sh
```

The drill must confirm:
- parent SIGTERM leaves no child orphan
- API kill removes active process

Teardown guarantee and limit:
- FlowForge sends `SIGTERM` to the entire supervised process group first.
- After grace timeout, FlowForge escalates to `SIGKILL` for the same process group.
- This guarantees cleanup for descendants that remain inside the supervised process group.
- Detached descendants that explicitly break away (`setsid`/new PGID) are outside this guarantee and must be handled with host/container controls.

## 7. Detection Tuning

Run pilot and threshold tuning:

```bash
./scripts/week1_pilot.sh
./scripts/tune_detection.sh
```

Choose a threshold where:
- runaway profiles are terminated
- bursty/healthy profiles are not terminated

## 8. Policy Rollout States

Use policy rollout to introduce destructive actions safely:

- `shadow`: evaluate policy but always log-only for kill/restart.
- `canary`: enforce kill/restart for sampled runs only (`policy-canary-percent`), log-only otherwise.
- `enforce`: full policy enforcement.

Examples:

```bash
./flowforge run --shadow-mode -- python3 your_worker.py
./flowforge run --policy-rollout canary --policy-canary-percent 10 -- python3 your_worker.py
./flowforge run --policy-rollout enforce -- python3 your_worker.py
```

## 9. Weekly SLO Review

Run weekly SLO dashboard operations:

```bash
./scripts/slo_weekly_review.sh --days 7
```

Review:
- `pilot_artifacts/slo-weekly-<timestamp>/slo_weekly_report.md`
- `pilot_artifacts/slo-weekly-<timestamp>/summary.tsv`

Decision rule:
- `GREEN`: continue roadmap execution.
- `YELLOW`: reliability-first week, defer non-critical expansion.
- `RED`: feature freeze and reliability sprint.

Detailed policy and ritual ownership:
- `docs/SLO_OPERATIONS.md`

## 10. What FlowForge Does Not Do

- It does not sandbox untrusted code.
- It does not replace OS/container isolation.
- It does not guarantee zero false positives.
- It does not provide cloud sync or remote policy distribution.
