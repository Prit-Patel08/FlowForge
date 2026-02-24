package database

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withTempDBPath(t *testing.T) string {
	t.Helper()
	oldPath, hadPath := os.LookupEnv("FLOWFORGE_DB_PATH")
	dbPath := filepath.Join(t.TempDir(), "flowforge-migration-test.db")
	if err := os.Setenv("FLOWFORGE_DB_PATH", dbPath); err != nil {
		t.Fatalf("set FLOWFORGE_DB_PATH: %v", err)
	}
	t.Cleanup(func() {
		CloseDB()
		if hadPath {
			_ = os.Setenv("FLOWFORGE_DB_PATH", oldPath)
		} else {
			_ = os.Unsetenv("FLOWFORGE_DB_PATH")
		}
	})
	return dbPath
}

func TestInitDBBackfillsLegacyTablesIntoUnifiedEvents(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	legacySetup := []string{
		`INSERT INTO incidents (
			command, model_name, exit_reason, max_cpu, pattern, token_savings_estimate, token_count, cost,
			agent_id, agent_version, reason, cpu_score, entropy_score, confidence_score, recovery_status, restart_count
		) VALUES (
			'python3 demo/runaway.py', 'gpt-4', 'LOOP_DETECTED', 95.5, 'repeat loop',
			1.2, 100, 0.05, 'agent-1', '1.0.0', 'legacy incident', 98.0, 10.0, 96.0, 'terminated', 0
		);`,
		`INSERT INTO audit_events (actor, action, reason, source, pid, details)
		VALUES ('flowforge', 'KILL', 'legacy audit', 'monitor', 4242, 'python3 demo/runaway.py');`,
		`INSERT INTO decision_traces (command, pid, cpu_score, entropy_score, confidence_score, decision, reason)
		VALUES ('python3 demo/runaway.py', 4242, 98.0, 10.0, 96.0, 'KILL', 'legacy decision');`,
	}
	for _, stmt := range legacySetup {
		if _, err := GetDB().Exec(stmt); err != nil {
			t.Fatalf("legacy setup exec failed: %v", err)
		}
	}
	if err := migrateLegacyRowsToUnifiedEvents(); err != nil {
		t.Fatalf("migrateLegacyRowsToUnifiedEvents: %v", err)
	}

	rows, err := GetDB().Query("SELECT event_type, COUNT(1) FROM events GROUP BY event_type")
	if err != nil {
		t.Fatalf("query event counts: %v", err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			t.Fatalf("scan event counts: %v", err)
		}
		counts[eventType] = count
	}
	if counts["incident"] != 1 || counts["audit"] != 1 || counts["decision"] != 1 {
		t.Fatalf("unexpected unified counts: %#v", counts)
	}

	incidents, err := GetAllIncidents()
	if err != nil {
		t.Fatalf("GetAllIncidents: %v", err)
	}
	if len(incidents) != 1 {
		t.Fatalf("expected 1 incident from unified events, got %d", len(incidents))
	}
	if incidents[0].ExitReason != "LOOP_DETECTED" {
		t.Fatalf("unexpected exit reason: %q", incidents[0].ExitReason)
	}
	if incidents[0].Command != "python3 demo/runaway.py" {
		t.Fatalf("unexpected command: %q", incidents[0].Command)
	}

	if err := migrateLegacyRowsToUnifiedEvents(); err != nil {
		t.Fatalf("second migrateLegacyRowsToUnifiedEvents: %v", err)
	}
	var total int
	if err := GetDB().QueryRow("SELECT COUNT(1) FROM events").Scan(&total); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 events after second InitDB, got %d", total)
	}
}

func TestGetAllIncidentsFromUnifiedEventsWhenLegacyRowsDeleted(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	SetRunID("run-unified-incidents")
	if err := LogIncidentWithDecisionForIncident(
		"python3 demo/runaway.py",
		"gpt-4",
		"LOOP_DETECTED",
		94.2,
		"repeat loop",
		1.0,
		10,
		0.02,
		"agent-1",
		"1.0.0",
		"from unified test",
		95.0,
		11.0,
		96.0,
		"terminated",
		0,
		"incident-unified-1",
	); err != nil {
		t.Fatalf("LogIncidentWithDecisionForIncident: %v", err)
	}

	if _, err := GetDB().Exec("DELETE FROM incidents"); err != nil {
		t.Fatalf("delete legacy incidents: %v", err)
	}

	incidents, err := GetAllIncidents()
	if err != nil {
		t.Fatalf("GetAllIncidents: %v", err)
	}
	if len(incidents) != 1 {
		t.Fatalf("expected 1 incident from unified events, got %d", len(incidents))
	}
	if incidents[0].Reason != "from unified test" {
		t.Fatalf("unexpected reason: %q", incidents[0].Reason)
	}
	if incidents[0].Command != "python3 demo/runaway.py" {
		t.Fatalf("unexpected command: %q", incidents[0].Command)
	}
}

func TestInitDBMigratesLegacyEventsWithoutCreatedAt(t *testing.T) {
	dbPath := withTempDBPath(t)
	CloseDB()

	legacyDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	const legacyEventsSchema = `CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT DEFAULT '',
		reason TEXT DEFAULT '',
		pid INTEGER DEFAULT 0,
		cpu_score REAL DEFAULT 0.0,
		entropy_score REAL DEFAULT 0.0,
		confidence_score REAL DEFAULT 0.0,
		event_id TEXT,
		run_id TEXT DEFAULT 'unknown-run',
		incident_id TEXT,
		event_type TEXT DEFAULT 'legacy',
		actor TEXT DEFAULT 'system',
		reason_text TEXT DEFAULT ''
	);`
	if _, err := legacyDB.Exec(legacyEventsSchema); err != nil {
		t.Fatalf("create legacy events schema: %v", err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO events(type, title, summary) VALUES ('legacy', 'legacy-row', 'legacy')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	if err := InitDB(); err != nil {
		t.Fatalf("InitDB should migrate legacy events schema without created_at: %v", err)
	}

	hasCreatedAt, err := columnExists("events", "created_at")
	if err != nil {
		t.Fatalf("columnExists(events, created_at): %v", err)
	}
	if !hasCreatedAt {
		t.Fatalf("expected created_at column to be added")
	}

	indexes := map[string]bool{}
	rows, err := GetDB().Query("PRAGMA index_list('events')")
	if err != nil {
		t.Fatalf("PRAGMA index_list(events): %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  string
			partial int
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index list: %v", err)
		}
		indexes[name] = true
	}
	for _, idx := range []string{"idx_events_event_id", "idx_events_incident_created", "idx_events_run_created", "idx_events_type_created"} {
		if !indexes[idx] {
			t.Fatalf("expected index %s to exist after migration", idx)
		}
	}
}

func TestDecisionTraceMetadataPersistsToDecisionTableAndUnifiedEvents(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	SetRunID("run-decision-engine-meta")

	incidentID := "incident-engine-meta-1"
	meta := DecisionTraceMeta{
		DecisionEngine:    "threshold-decider",
		EngineVersion:     "1.1.0",
		DecisionContract:  "decision-trace.v1",
		PolicyRolloutMode: "canary",
		ReplayContract:    "decision-replay.v1",
		ReplayDigest:      "abc123replaydigest",
	}
	if err := LogDecisionTraceWithIncidentAndMeta(
		"python3 worker.py",
		4242,
		92.1,
		12.0,
		95.4,
		"KILL",
		"contract metadata persist test",
		incidentID,
		meta,
	); err != nil {
		t.Fatalf("LogDecisionTraceWithIncidentAndMeta: %v", err)
	}

	traces, err := GetDecisionTraces(10)
	if err != nil {
		t.Fatalf("GetDecisionTraces: %v", err)
	}
	if len(traces) == 0 {
		t.Fatal("expected at least one decision trace")
	}
	got := traces[0]
	if got.DecisionEngine != meta.DecisionEngine {
		t.Fatalf("expected decision_engine %q, got %q", meta.DecisionEngine, got.DecisionEngine)
	}
	if got.DecisionEngineVersion != meta.EngineVersion {
		t.Fatalf("expected engine_version %q, got %q", meta.EngineVersion, got.DecisionEngineVersion)
	}
	if got.DecisionContract != meta.DecisionContract {
		t.Fatalf("expected decision_contract_version %q, got %q", meta.DecisionContract, got.DecisionContract)
	}
	if got.PolicyRolloutMode != meta.PolicyRolloutMode {
		t.Fatalf("expected rollout_mode %q, got %q", meta.PolicyRolloutMode, got.PolicyRolloutMode)
	}
	if got.ReplayContract != meta.ReplayContract {
		t.Fatalf("expected replay_contract_version %q, got %q", meta.ReplayContract, got.ReplayContract)
	}
	if got.ReplayDigest != meta.ReplayDigest {
		t.Fatalf("expected replay_digest %q, got %q", meta.ReplayDigest, got.ReplayDigest)
	}

	events, err := GetIncidentTimelineByIncidentID(incidentID, 10)
	if err != nil {
		t.Fatalf("GetIncidentTimelineByIncidentID: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected incident timeline to include decision event")
	}
	ev := events[0]
	if ev.DecisionEngineVersion != meta.EngineVersion {
		t.Fatalf("expected event engine_version %q, got %q", meta.EngineVersion, ev.DecisionEngineVersion)
	}
	if ev.DecisionEngine != meta.DecisionEngine {
		t.Fatalf("expected event decision_engine %q, got %q", meta.DecisionEngine, ev.DecisionEngine)
	}
	if ev.DecisionContract != meta.DecisionContract {
		t.Fatalf("expected event decision_contract_version %q, got %q", meta.DecisionContract, ev.DecisionContract)
	}
	if ev.PolicyRolloutMode != meta.PolicyRolloutMode {
		t.Fatalf("expected event rollout_mode %q, got %q", meta.PolicyRolloutMode, ev.PolicyRolloutMode)
	}
	if ev.ReplayContract != meta.ReplayContract {
		t.Fatalf("expected event replay_contract_version %q, got %q", meta.ReplayContract, ev.ReplayContract)
	}
	if ev.ReplayDigest != meta.ReplayDigest {
		t.Fatalf("expected event replay_digest %q, got %q", meta.ReplayDigest, ev.ReplayDigest)
	}
}

func TestInitDBBackfillsLegacyDecisionTraceEngineMetadataForReplayCompatibility(t *testing.T) {
	dbPath := withTempDBPath(t)
	CloseDB()

	legacyDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	const legacyDecisionSchema = `CREATE TABLE IF NOT EXISTS decision_traces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		command TEXT,
		pid INTEGER,
		cpu_score REAL,
		entropy_score REAL,
		confidence_score REAL,
		decision TEXT,
		reason TEXT
	);`
	if _, err := legacyDB.Exec(legacyDecisionSchema); err != nil {
		t.Fatalf("create legacy decision_traces schema: %v", err)
	}
	if _, err := legacyDB.Exec(`INSERT INTO decision_traces(command, pid, cpu_score, entropy_score, confidence_score, decision, reason)
	VALUES ('python3 legacy.py', 1234, 97.0, 11.0, 96.0, 'KILL', 'legacy replay record')`); err != nil {
		t.Fatalf("insert legacy decision trace: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	var payloadRaw string
	if err := GetDB().QueryRow(`SELECT COALESCE(payload_json, '{}') FROM events WHERE event_type='decision' ORDER BY id DESC LIMIT 1`).Scan(&payloadRaw); err != nil {
		t.Fatalf("query decision payload: %v", err)
	}

	var payload decisionEventPayload
	if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
		t.Fatalf("decode decision payload: %v", err)
	}
	if payload.EngineVersion != "legacy-unknown" {
		t.Fatalf("expected legacy engine version fallback, got %q", payload.EngineVersion)
	}
	if payload.DecisionEngine != "legacy-decider" {
		t.Fatalf("expected legacy decision engine fallback, got %q", payload.DecisionEngine)
	}
	if payload.DecisionContract != "legacy-decision-trace" {
		t.Fatalf("expected legacy contract fallback, got %q", payload.DecisionContract)
	}
	if payload.ReplayContract != "legacy-decision-replay" {
		t.Fatalf("expected legacy replay contract fallback, got %q", payload.ReplayContract)
	}
}
