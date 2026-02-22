# Integration API Contract (Cursor + Antigravity MVP)

Status: Implemented (v1 local integration surface)  
Owner: API + Integration  
Target: P1 (Day 31-90)

## Contract Principles

1. Local-only transport (`127.0.0.1`), never public by default.
2. Versioned path prefix (`/v1`) for compatibility.
3. Deterministic JSON response shape.
4. Explicit reason strings for any destructive action.

## Auth

All write endpoints require:

- `Authorization: Bearer <FLOWFORGE_API_KEY>`

Read-only status endpoints may be open locally in MVP, but write endpoints must remain protected.

Optional for safe retries:

- `Idempotency-Key: <client-generated-key>`

When provided on write endpoints (`register`, `unregister`, `protection`, `actions`):
1. same key + same payload replays the original response with `X-Idempotent-Replay: true`
2. same key + different payload returns `409` with an idempotency conflict error

## Base URL

`http://127.0.0.1:8080` (existing API in MVP)  
Future daemon-specific port can be introduced after daemon command is stable.

## Endpoints

## 1) Register Workspace

`POST /v1/integrations/workspaces/register`

Request:

```json
{
  "workspace_id": "ws-123",
  "workspace_path": "/absolute/path/to/workspace",
  "profile": "standard",
  "client": "cursor"
}
```

Response:

```json
{
  "ok": true,
  "workspace_id": "ws-123",
  "profile": "standard"
}
```

## 2) Unregister Workspace

`DELETE /v1/integrations/workspaces/{workspace_id}`

Request (optional):

```json
{
  "reason": "workspace removed from IDE integration"
}
```

Response:

```json
{
  "ok": true,
  "workspace_id": "ws-123",
  "unregistered": true
}
```

## 3) Workspace Status

`GET /v1/integrations/workspaces/{workspace_id}/status`

Response:

```json
{
  "workspace_id": "ws-123",
  "protection_enabled": true,
  "profile": "standard",
  "active_pid": 12345,
  "last_updated": "2026-02-19T12:00:00Z"
}
```

## 4) Enable/Disable Protection

`POST /v1/integrations/workspaces/{workspace_id}/protection`

Request:

```json
{
  "enabled": true,
  "reason": "enable protection for agent tasks"
}
```

Response:

```json
{
  "ok": true,
  "workspace_id": "ws-123",
  "enabled": true
}
```

## 5) Latest Incident

`GET /v1/integrations/workspaces/{workspace_id}/incidents/latest`

Response:

```json
{
  "incident_id": "inc-789",
  "exit_reason": "LOOP_DETECTED",
  "reason_text": "CPU exceeded threshold and repetition score remained high",
  "confidence_score": 96.4,
  "created_at": "2026-02-19T12:01:10Z"
}
```

## 6) Manual Action

`POST /v1/integrations/workspaces/{workspace_id}/actions`

Request:

```json
{
  "action": "restart",
  "reason": "operator requested restart from IDE panel"
}
```

Response:

```json
{
  "ok": true,
  "action": "restart",
  "audit_event_id": 42
}
```

## Error Contract

Errors use RFC 7807 Problem Details (`Content-Type: application/problem+json`).

```json
{
  "type": "https://flowforge.dev/problems/idempotency-conflict",
  "title": "Conflict",
  "status": 409,
  "detail": "human-readable error message",
  "instance": "/v1/integrations/workspaces/ws-123/actions",
  "request_id": "req_1234abcd"
}
```

Compatibility note:
- `error` field is still present with the same value as `detail` for legacy clients.
- Response also echoes `X-Request-Id`; clients should pass their own value for cross-system correlation.

Common codes:
- `bad_request`
- `unauthorized`
- `not_found`
- `conflict`
- `internal_error`

## Compatibility Rules

1. Additive fields are allowed in responses.
2. Existing response fields must not be removed in `v1`.
3. Breaking changes require `/v2` path and migration note in release docs.

## Current Implementation Notes

1. All endpoints in this contract are now available under `/v1/integrations/workspaces/...`.
2. Write endpoints (`register`, `unregister`, `protection`, `actions`) enforce API key auth via `Authorization: Bearer <FLOWFORGE_API_KEY>`.
3. Workspace registration validates:
- `workspace_id` pattern `[A-Za-z0-9._:-]` (max 128 chars)
- absolute `workspace_path`
4. `actions` supports `kill` and `restart` and persists integration action records with linked audit events.
5. `incidents/latest` currently returns the latest supervisor incident in local runtime context.
6. `DELETE /v1/integrations/workspaces/{workspace_id}` unregisters a workspace and emits `WORKSPACE_UNREGISTER` audit evidence.
7. Idempotent mutation replay state is persisted in `control_plane_replays` for cross-request replay safety.
