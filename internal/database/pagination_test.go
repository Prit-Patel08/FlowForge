package database

import "testing"

func TestGetIncidentsPage(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	setMasterKeyForTest(t, testMasterKeyHex)
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	commands := []string{
		"python3 worker_a.py",
		"python3 worker_b.py",
		"python3 worker_c.py",
	}
	for idx, command := range commands {
		if err := LogIncidentWithDecisionForIncident(
			command,
			"gpt-4",
			"LOOP_DETECTED",
			95.0,
			"repeat loop",
			1.0,
			50+idx,
			0.01,
			"agent-1",
			"1.0.0",
			"pagination contract",
			95.0,
			10.0,
			96.0,
			"terminated",
			0,
			"",
		); err != nil {
			t.Fatalf("LogIncidentWithDecisionForIncident[%d]: %v", idx, err)
		}
	}

	page1, cursor1, hasMore1, err := GetIncidentsPage(2, 0)
	if err != nil {
		t.Fatalf("GetIncidentsPage page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 incidents on page1, got %d", len(page1))
	}
	if !hasMore1 {
		t.Fatalf("expected page1 hasMore=true")
	}
	if cursor1 <= 0 {
		t.Fatalf("expected positive cursor for page1, got %d", cursor1)
	}

	page2, cursor2, hasMore2, err := GetIncidentsPage(2, cursor1)
	if err != nil {
		t.Fatalf("GetIncidentsPage page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 incident on page2, got %d", len(page2))
	}
	if hasMore2 {
		t.Fatalf("expected page2 hasMore=false")
	}
	if cursor2 != 0 {
		t.Fatalf("expected page2 cursor=0, got %d", cursor2)
	}
	if page1[0].ID == page2[0].ID || page1[1].ID == page2[0].ID {
		t.Fatalf("expected page2 incident to be distinct from page1 incidents")
	}
}

func TestGetTimelinePage(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	setMasterKeyForTest(t, testMasterKeyHex)
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	if _, err := InsertEvent("decision", "system", "timeline page test A", "run-page", "incident-page", "KILL", "summary A", 4040, 90, 10, 95); err != nil {
		t.Fatalf("InsertEvent A: %v", err)
	}
	if _, err := InsertEvent("audit", "api-key", "timeline page test B", "run-page", "incident-page", "RESTART", "summary B", 4040, 0, 0, 0); err != nil {
		t.Fatalf("InsertEvent B: %v", err)
	}
	if _, err := InsertEvent("decision", "system", "timeline page test C", "run-page", "incident-page", "ALERT", "summary C", 4040, 50, 30, 40); err != nil {
		t.Fatalf("InsertEvent C: %v", err)
	}

	page1, cursor1, hasMore1, err := GetTimelinePage(2, 0)
	if err != nil {
		t.Fatalf("GetTimelinePage page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("expected 2 timeline events on page1, got %d", len(page1))
	}
	if !hasMore1 {
		t.Fatalf("expected page1 hasMore=true")
	}
	if cursor1 <= 0 {
		t.Fatalf("expected positive cursor for page1, got %d", cursor1)
	}

	page2, cursor2, hasMore2, err := GetTimelinePage(2, cursor1)
	if err != nil {
		t.Fatalf("GetTimelinePage page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 timeline event on page2, got %d", len(page2))
	}
	if hasMore2 {
		t.Fatalf("expected page2 hasMore=false")
	}
	if cursor2 != 0 {
		t.Fatalf("expected page2 cursor=0, got %d", cursor2)
	}
	if page1[0].EventID == page2[0].EventID || page1[1].EventID == page2[0].EventID {
		t.Fatalf("expected page2 event to be distinct from page1 events")
	}
}
