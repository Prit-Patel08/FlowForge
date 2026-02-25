package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowforge/internal/database"
)

func setupEvidenceTestDB(t *testing.T) string {
	t.Helper()
	oldMasterKey, hadMasterKey := os.LookupEnv("FLOWFORGE_MASTER_KEY")
	if err := os.Setenv("FLOWFORGE_MASTER_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatalf("set FLOWFORGE_MASTER_KEY: %v", err)
	}
	oldPath, hadPath := os.LookupEnv("FLOWFORGE_DB_PATH")
	dbPath := filepath.Join(t.TempDir(), "flowforge-evidence-test.db")
	if err := os.Setenv("FLOWFORGE_DB_PATH", dbPath); err != nil {
		t.Fatalf("set FLOWFORGE_DB_PATH: %v", err)
	}
	database.CloseDB()
	if err := database.InitDB(); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() {
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
	return dbPath
}

func seedEvidenceData(t *testing.T) {
	t.Helper()
	database.SetRunID("run-evidence-test")
	if err := database.LogIncidentWithDecisionForIncident(
		"python3 demo/runaway.py",
		"gpt-4",
		"LOOP_DETECTED",
		91.2,
		"repeat loop",
		1.1,
		42,
		0.03,
		"agent-e2e",
		"1.0.0",
		"evidence test incident",
		92.0,
		12.0,
		95.0,
		"terminated",
		0,
		"incident-evidence-1",
	); err != nil {
		t.Fatalf("log incident: %v", err)
	}
	if err := database.LogAuditEventWithIncident("api-key", "INTEGRATION_RESTART", "evidence action", "integration", 1234, "workspace-1", "incident-evidence-1"); err != nil {
		t.Fatalf("log audit: %v", err)
	}
	if err := database.LogDecisionTraceWithIncident("python3 demo/runaway.py", 1234, 92.0, 12.0, 95.0, "KILL", "threshold breached", "incident-evidence-1"); err != nil {
		t.Fatalf("log decision: %v", err)
	}
}

func TestExportAndVerifyBundle(t *testing.T) {
	setupEvidenceTestDB(t)
	seedEvidenceData(t)

	outDir := filepath.Join(t.TempDir(), "bundle")
	key := []byte("0123456789abcdef0123456789abcdef")
	result, err := Export(ExportOptions{
		OutDir:        outDir,
		IncidentID:    "incident-evidence-1",
		TimelineLimit: 100,
		AuditLimit:    100,
		DecisionLimit: 100,
		ChainLimit:    100,
	}, key)
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	if result.BundleDir != outDir {
		t.Fatalf("expected outDir %q, got %q", outDir, result.BundleDir)
	}
	if len(result.Manifest.Files) < 6 {
		t.Fatalf("expected at least 6 files in manifest, got %d", len(result.Manifest.Files))
	}

	verify, err := Verify(outDir, key)
	if err != nil {
		t.Fatalf("verify bundle: %v", err)
	}
	if !verify.ManifestOK || !verify.SignatureOK {
		t.Fatalf("expected manifest/signature OK, got %+v", verify)
	}
}

func TestVerifyDetectsTampering(t *testing.T) {
	setupEvidenceTestDB(t)
	seedEvidenceData(t)

	outDir := filepath.Join(t.TempDir(), "bundle")
	key := []byte("0123456789abcdef0123456789abcdef")
	if _, err := Export(ExportOptions{
		OutDir:        outDir,
		TimelineLimit: 100,
		AuditLimit:    100,
		DecisionLimit: 100,
		ChainLimit:    100,
	}, key); err != nil {
		t.Fatalf("export bundle: %v", err)
	}

	incPath := filepath.Join(outDir, "incidents.json")
	b, err := os.ReadFile(incPath)
	if err != nil {
		t.Fatalf("read incidents.json: %v", err)
	}
	if err := os.WriteFile(incPath, append(b, []byte("\n{\"tampered\":true}\n")...), 0o644); err != nil {
		t.Fatalf("write tampered incidents.json: %v", err)
	}

	_, err = Verify(outDir, key)
	if err == nil {
		t.Fatal("expected verify to fail for tampered bundle")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "mismatch") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}
