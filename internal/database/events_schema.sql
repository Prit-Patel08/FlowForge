-- Append-only unified events schema for FlowForge (SQLite).
-- This file is intentionally raw SQL (no ORM assumptions).

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id TEXT NOT NULL UNIQUE,
  run_id TEXT NOT NULL,
  incident_id TEXT,
  event_type TEXT NOT NULL,
  actor TEXT NOT NULL,
  reason_text TEXT NOT NULL,
  payload_json TEXT NOT NULL DEFAULT '{}',
  confidence_score REAL NOT NULL DEFAULT 0.0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

  -- Backward-compatible timeline fields used by current dashboard/API.
  title TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  pid INTEGER NOT NULL DEFAULT 0,
  cpu_score REAL NOT NULL DEFAULT 0.0,
  entropy_score REAL NOT NULL DEFAULT 0.0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_events_event_id
  ON events(event_id);

CREATE INDEX IF NOT EXISTS idx_events_incident_created
  ON events(incident_id, created_at);

CREATE INDEX IF NOT EXISTS idx_events_run_created
  ON events(run_id, created_at);

CREATE TRIGGER IF NOT EXISTS trg_events_no_update
BEFORE UPDATE ON events
BEGIN
  SELECT RAISE(ABORT, 'events table is append-only');
END;

CREATE TRIGGER IF NOT EXISTS trg_events_no_delete
BEFORE DELETE ON events
BEGIN
  SELECT RAISE(ABORT, 'events table is append-only');
END;

-- Canonical insert statement.
INSERT INTO events (
  event_id,
  run_id,
  incident_id,
  event_type,
  actor,
  reason_text,
  payload_json,
  confidence_score,
  created_at,
  title,
  summary,
  pid,
  cpu_score,
  entropy_score
) VALUES (
  ?1, -- event_id (TEXT UUID)
  ?2, -- run_id
  ?3, -- incident_id nullable
  ?4, -- event_type
  ?5, -- actor
  ?6, -- reason_text
  ?7, -- payload_json
  ?8, -- confidence_score
  CURRENT_TIMESTAMP,
  ?9, -- title
  ?10, -- summary
  ?11, -- pid
  ?12, -- cpu_score
  ?13  -- entropy_score
);

-- Canonical "Incident Timeline" query.
SELECT
  event_id,
  run_id,
  incident_id,
  event_type,
  actor,
  reason_text,
  payload_json,
  confidence_score,
  created_at,
  title,
  summary,
  pid,
  cpu_score,
  entropy_score
FROM events
WHERE incident_id = ?1
ORDER BY created_at ASC, id ASC;
