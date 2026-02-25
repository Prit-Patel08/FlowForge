package database

import (
	"os"
	"strings"
	"testing"

	"flowforge/internal/encryption"
)

const testMasterKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func setMasterKeyForTest(t *testing.T, value string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv("FLOWFORGE_MASTER_KEY")
	if value == "" {
		if err := os.Unsetenv("FLOWFORGE_MASTER_KEY"); err != nil {
			t.Fatalf("unset FLOWFORGE_MASTER_KEY: %v", err)
		}
	} else {
		if err := os.Setenv("FLOWFORGE_MASTER_KEY", value); err != nil {
			t.Fatalf("set FLOWFORGE_MASTER_KEY: %v", err)
		}
	}
	encryption.ResetForTests()
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("FLOWFORGE_MASTER_KEY", oldValue)
		} else {
			_ = os.Unsetenv("FLOWFORGE_MASTER_KEY")
		}
		encryption.ResetForTests()
	})
}

func TestLogIncidentFailsClosedWhenMasterKeyMissing(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	setMasterKeyForTest(t, "")
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	err := LogIncidentWithDecisionForIncident(
		"python3 worker.py --api-key supersecret",
		"gpt-4",
		"LOOP_DETECTED",
		95.1,
		"Authorization: Bearer sk-test",
		1.0,
		10,
		0.01,
		"agent-1",
		"1.0.0",
		"fail-closed validation",
		99.0,
		10.0,
		96.0,
		"terminated",
		0,
		"incident-fail-closed",
	)
	if err == nil {
		t.Fatalf("expected encryption failure when FLOWFORGE_MASTER_KEY is missing")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "encrypt incident command") {
		t.Fatalf("expected encrypt incident command failure, got: %v", err)
	}

	var count int
	if err := GetDB().QueryRow("SELECT COUNT(1) FROM incidents").Scan(&count); err != nil {
		t.Fatalf("count incidents: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no incident rows on encryption failure, got %d", count)
	}
}

func TestIncidentAndDecisionPersistenceSanitizeSecrets(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	setMasterKeyForTest(t, testMasterKeyHex)
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	secretCommand := "python3 worker.py --api-key supersecret --token abc123"
	secretPattern := "Authorization: Bearer sk-super-secret FLOWFORGE_API_KEY=anothersecret"

	if err := LogIncidentWithDecisionForIncident(
		secretCommand,
		"gpt-4",
		"LOOP_DETECTED",
		94.2,
		secretPattern,
		1.0,
		42,
		0.02,
		"agent-sec",
		"1.0.0",
		"sanitization coverage",
		97.0,
		10.0,
		96.5,
		"terminated",
		0,
		"incident-secure-1",
	); err != nil {
		t.Fatalf("LogIncidentWithDecisionForIncident: %v", err)
	}

	incident, err := GetIncidentByID(1)
	if err != nil {
		t.Fatalf("GetIncidentByID: %v", err)
	}
	if strings.Contains(incident.Command, "supersecret") || strings.Contains(incident.Command, "abc123") {
		t.Fatalf("incident command leaked secret: %q", incident.Command)
	}
	if strings.Contains(incident.Pattern, "sk-super-secret") || strings.Contains(incident.Pattern, "anothersecret") {
		t.Fatalf("incident pattern leaked secret: %q", incident.Pattern)
	}
	if !strings.Contains(incident.Command, "<REDACTED>") {
		t.Fatalf("expected redacted marker in command, got %q", incident.Command)
	}

	var storedCiphertext string
	if err := GetDB().QueryRow("SELECT command FROM incidents WHERE id = 1").Scan(&storedCiphertext); err != nil {
		t.Fatalf("select stored incident command: %v", err)
	}
	if strings.Contains(storedCiphertext, "supersecret") || strings.Contains(storedCiphertext, "abc123") {
		t.Fatalf("stored incident command contains plaintext secret: %q", storedCiphertext)
	}

	if err := LogDecisionTraceWithIncident(secretCommand, 4242, 95.0, 10.0, 96.0, "KILL", "sanitized decision trace", "incident-secure-1"); err != nil {
		t.Fatalf("LogDecisionTraceWithIncident: %v", err)
	}
	traces, err := GetDecisionTraces(1)
	if err != nil {
		t.Fatalf("GetDecisionTraces: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 decision trace, got %d", len(traces))
	}
	if strings.Contains(traces[0].Command, "supersecret") || strings.Contains(traces[0].Command, "abc123") {
		t.Fatalf("decision trace command leaked secret: %q", traces[0].Command)
	}
	if !strings.Contains(traces[0].Command, "<REDACTED>") {
		t.Fatalf("expected redacted decision trace command, got %q", traces[0].Command)
	}

	if err := LogAuditEventWithIncident("flowforge", "AUTO_KILL", "sanitized audit details", "monitor", 4242, secretCommand, "incident-secure-1"); err != nil {
		t.Fatalf("LogAuditEventWithIncident: %v", err)
	}
	audits, err := GetAuditEvents(1)
	if err != nil {
		t.Fatalf("GetAuditEvents: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(audits))
	}
	if strings.Contains(audits[0].Details, "supersecret") || strings.Contains(audits[0].Details, "abc123") {
		t.Fatalf("audit details leaked secret: %q", audits[0].Details)
	}

	if err := LogPolicyDryRunWithIncident(secretCommand, 4242, "policy dry-run redact coverage", 88.8, "incident-secure-1"); err != nil {
		t.Fatalf("LogPolicyDryRunWithIncident: %v", err)
	}
	events, err := GetTimeline(20)
	if err != nil {
		t.Fatalf("GetTimeline: %v", err)
	}
	foundDryRun := false
	for _, ev := range events {
		if ev.Type != "policy_dry_run" {
			continue
		}
		foundDryRun = true
		if strings.Contains(ev.Summary, "supersecret") || strings.Contains(ev.Summary, "abc123") {
			t.Fatalf("policy dry-run summary leaked secret: %q", ev.Summary)
		}
		if !strings.Contains(ev.Summary, "<REDACTED>") {
			t.Fatalf("expected policy dry-run summary to include redacted marker, got %q", ev.Summary)
		}
		break
	}
	if !foundDryRun {
		t.Fatalf("expected policy_dry_run event in timeline")
	}
}
