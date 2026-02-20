package database

import (
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
