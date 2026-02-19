package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"flowforge/internal/api"
	"flowforge/internal/database"
)

func setupTempDBForAPI(t *testing.T) {
	t.Helper()
	oldPath, hadPath := os.LookupEnv("FLOWFORGE_DB_PATH")
	dbPath := filepath.Join(t.TempDir(), "flowforge-api-test.db")

	if err := os.Setenv("FLOWFORGE_DB_PATH", dbPath); err != nil {
		t.Fatalf("set db path: %v", err)
	}

	database.CloseDB()
	if err := database.InitDB(); err != nil {
		t.Fatalf("init db: %v", err)
	}

	t.Cleanup(func() {
		database.CloseDB()
		if hadPath {
			_ = os.Setenv("FLOWFORGE_DB_PATH", oldPath)
		} else {
			_ = os.Unsetenv("FLOWFORGE_DB_PATH")
		}
	})
}

// TestCORSHeaders ensures that the /incidents endpoint returns proper CORS headers.
func TestCORSHeaders(t *testing.T) {
	req := httptest.NewRequest("OPTIONS", "/incidents", nil)
	w := httptest.NewRecorder()

	api.HandleIncidents(w, req)

	resp := w.Result()

	// Strict CORS: Expect specific origin, not *
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Errorf("Expected CORS header 'http://localhost:3000', got %q", origin)
	}

	methods := resp.Header.Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("Expected Access-Control-Allow-Methods header to be set")
	}
}

// TestIncidentsEndpointHealth verifies that /incidents returns 200 + valid JSON.
func TestIncidentsEndpointHealth(t *testing.T) {
	// Initialize the database first
	req := httptest.NewRequest("GET", "/incidents", nil)
	w := httptest.NewRecorder()

	api.HandleIncidents(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got: %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type: application/json, got: %q", contentType)
	}

	// Check that the response is valid JSON
	var result interface{}
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&result); err != nil {
		t.Errorf("Response is not valid JSON: %v", err)
	}
}

// TestKillEndpointRequiresAuth verifies that /process/kill rejects unauthorized requests.
func TestKillEndpointRequiresAuth(t *testing.T) {
	// Set the API key
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	req := httptest.NewRequest("POST", "/process/kill", nil)
	w := httptest.NewRecorder()

	api.HandleProcessKill(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized, got: %d", resp.StatusCode)
	}
}

// TestKillEndpointAuthPasses verifies that /process/kill accepts authorized requests.
func TestKillEndpointAuthPasses(t *testing.T) {
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	req := httptest.NewRequest("POST", "/process/kill", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	w := httptest.NewRecorder()

	api.HandleProcessKill(w, req)

	resp := w.Result()

	// Should not be 401 (might be 400 "no active process" which is fine)
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("Expected request to pass auth, but got 401 Unauthorized")
	}
}

// TestKillEndpointNoKeySetIsBlocked verifies that without FLOWFORGE_API_KEY, mutating endpoints are blocked.
func TestKillEndpointNoKeySetIsBlocked(t *testing.T) {
	os.Unsetenv("FLOWFORGE_API_KEY")

	req := httptest.NewRequest("POST", "/process/kill", nil)
	w := httptest.NewRecorder()

	api.HandleProcessKill(w, req)

	resp := w.Result()

	// Should be 403 Forbidden when no key is set (Mutations blocked for security)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when FLOWFORGE_API_KEY is not set, but got %d", resp.StatusCode)
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()

	api.HandleHealth(w, req)
	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTimelineEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()

	api.HandleTimeline(w, req)
	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestTimelineEndpointIncidentFilterAndContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-api-contract")
	incidentID := "incident-contract-001"

	if _, err := database.InsertEvent(
		"decision",
		"system",
		"CPU threshold breach",
		"run-api-contract",
		incidentID,
		"KILL",
		"CPU 100 / Entropy 12 / Confidence 95",
		4040,
		100.0,
		12.0,
		95.0,
	); err != nil {
		t.Fatalf("insert decision event: %v", err)
	}
	if _, err := database.InsertEvent(
		"audit",
		"api-key",
		"operator restart",
		"run-api-contract",
		incidentID,
		"RESTART",
		"manual restart by operator",
		4040,
		0,
		0,
		0,
	); err != nil {
		t.Fatalf("insert audit event: %v", err)
	}
	if _, err := database.InsertEvent(
		"decision",
		"system",
		"different incident",
		"run-api-contract",
		"incident-other-002",
		"ALERT",
		"not part of contract chain",
		9090,
		70.0,
		45.0,
		50.0,
	); err != nil {
		t.Fatalf("insert unrelated event: %v", err)
	}

	req := httptest.NewRequest("GET", "/timeline?incident_id="+incidentID, nil)
	w := httptest.NewRecorder()
	api.HandleTimeline(w, req)
	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	var payload []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 filtered events, got %d", len(payload))
	}

	for _, ev := range payload {
		if ev["incident_id"] != incidentID {
			t.Fatalf("unexpected incident_id %v", ev["incident_id"])
		}
		for _, key := range []string{"event_id", "run_id", "event_type", "actor", "reason_text", "created_at"} {
			raw, ok := ev[key]
			if !ok {
				t.Fatalf("missing key %q in response object", key)
			}
			if s, ok := raw.(string); !ok || s == "" {
				t.Fatalf("expected non-empty string for key %q, got %#v", key, raw)
			}
		}
	}
}

func TestKillEndpointBruteForceBlocked(t *testing.T) {
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	for i := 0; i < 11; i++ {
		req := httptest.NewRequest("POST", "/process/kill", nil)
		req.RemoteAddr = "198.51.100.77:1234"
		req.Header.Set("Authorization", "Bearer wrong-key")
		w := httptest.NewRecorder()
		api.HandleProcessKill(w, req)
	}

	req := httptest.NewRequest("POST", "/process/kill", nil)
	req.RemoteAddr = "198.51.100.77:1234"
	req.Header.Set("Authorization", "Bearer wrong-key")
	w := httptest.NewRecorder()
	api.HandleProcessKill(w, req)

	if w.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated auth failures, got %d", w.Result().StatusCode)
	}
}
