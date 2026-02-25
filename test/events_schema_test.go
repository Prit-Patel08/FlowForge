package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowforge/internal/api"
	"flowforge/internal/database"
)

func setupTempDB(t *testing.T) {
	t.Helper()
	api.ResetWorkerControlForTests()
	oldMasterKey, hadMasterKey := os.LookupEnv("FLOWFORGE_MASTER_KEY")
	if err := os.Setenv("FLOWFORGE_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("set FLOWFORGE_MASTER_KEY: %v", err)
	}
	oldPath, hadPath := os.LookupEnv("FLOWFORGE_DB_PATH")
	dbPath := filepath.Join(t.TempDir(), "flowforge-test.db")
	if err := os.Setenv("FLOWFORGE_DB_PATH", dbPath); err != nil {
		t.Fatalf("set env: %v", err)
	}
	if err := database.InitDB(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
		api.ResetWorkerControlForTests()
		database.CloseDB()
		if hadMasterKey {
			_ = os.Setenv("FLOWFORGE_MASTER_KEY", oldMasterKey)
		} else {
			_ = os.Unsetenv("FLOWFORGE_MASTER_KEY")
		}
		if hadPath {
			_ = os.Setenv("FLOWFORGE_DB_PATH", oldPath)
		} else {
			_ = os.Unsetenv("FLOWFORGE_DB_PATH")
		}
	})
}

func TestIncidentTimelineQueryReturnsChronologicalEvents(t *testing.T) {
	setupTempDB(t)
	database.SetRunID("run-test")
	incidentID := "incident-abc"

	_, err := database.InsertEvent(
		"cpu_spike",
		"system",
		"CPU exceeded 90% for 30s",
		"run-test",
		incidentID,
		"CPU spike",
		"high cpu",
		42,
		97.5,
		0.1,
		93.2,
	)
	if err != nil {
		t.Fatalf("insert event1: %v", err)
	}

	_, err = database.InsertEvent(
		"process_killed",
		"system",
		"Policy action executed",
		"run-test",
		incidentID,
		"Process killed",
		"killed process",
		42,
		98.0,
		0.1,
		95.0,
	)
	if err != nil {
		t.Fatalf("insert event2: %v", err)
	}

	timeline, err := database.GetIncidentTimelineByIncidentID(incidentID, 10)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events, got %d", len(timeline))
	}
	if timeline[0].Type != "cpu_spike" {
		t.Fatalf("expected first event type cpu_spike, got %q", timeline[0].Type)
	}
	if timeline[1].Type != "process_killed" {
		t.Fatalf("expected second event type process_killed, got %q", timeline[1].Type)
	}
}

func TestEventsTableIsAppendOnly(t *testing.T) {
	setupTempDB(t)

	eventID, err := database.InsertEvent(
		"policy_dry_run",
		"system",
		"Dry run event",
		"run-test",
		"incident-append-only",
		"Dry run",
		"log only",
		0,
		0,
		0,
		50.0,
	)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}

	_, err = database.GetDB().Exec("UPDATE events SET title = 'mutated' WHERE event_id = ?", eventID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only update error, got: %v", err)
	}

	_, err = database.GetDB().Exec("DELETE FROM events WHERE event_id = ?", eventID)
	if err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("expected append-only delete error, got: %v", err)
	}
}

func TestCorrelatedIncidentChainFromLoggingHelpers(t *testing.T) {
	setupTempDB(t)
	database.SetRunID("run-correlation")
	incidentID := "incident-chain-1"

	if err := database.LogDecisionTraceWithIncident(
		"python3 worker.py",
		4242,
		98.4,
		10.0,
		96.5,
		"KILL",
		"CPU exceeded 90% for 30s AND log entropy dropped below 0.20",
		incidentID,
	); err != nil {
		t.Fatalf("log decision trace: %v", err)
	}

	if err := database.LogAuditEventWithIncident(
		"flowforge",
		"AUTO_KILL",
		"automatic intervention",
		"monitor",
		4242,
		"python3 worker.py",
		incidentID,
	); err != nil {
		t.Fatalf("log audit event: %v", err)
	}

	if err := database.LogIncidentWithDecisionForIncident(
		"python3 worker.py",
		"gpt-4",
		"LOOP_DETECTED",
		98.4,
		"processing request <NUM> failed, retrying endlessly",
		2.5,
		100,
		0.01,
		"agent-1",
		"1.0.0",
		"automatic intervention",
		98.4,
		10.0,
		96.5,
		"terminated",
		0,
		incidentID,
	); err != nil {
		t.Fatalf("log incident: %v", err)
	}

	timeline, err := database.GetIncidentTimelineByIncidentID(incidentID, 10)
	if err != nil {
		t.Fatalf("get timeline: %v", err)
	}
	if len(timeline) < 3 {
		t.Fatalf("expected at least 3 correlated events, got %d", len(timeline))
	}
	for _, ev := range timeline {
		if ev.IncidentID != incidentID {
			t.Fatalf("unexpected incident_id in timeline: %q", ev.IncidentID)
		}
	}
}
