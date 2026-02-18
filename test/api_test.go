package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"agent-sentry/internal/api"
)

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
	os.Setenv("SENTRY_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("SENTRY_API_KEY")

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
	os.Setenv("SENTRY_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("SENTRY_API_KEY")

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

// TestKillEndpointNoKeySetIsBlocked verifies that without SENTRY_API_KEY, mutating endpoints are blocked.
func TestKillEndpointNoKeySetIsBlocked(t *testing.T) {
	os.Unsetenv("SENTRY_API_KEY")

	req := httptest.NewRequest("POST", "/process/kill", nil)
	w := httptest.NewRecorder()

	api.HandleProcessKill(w, req)

	resp := w.Result()

	// Should be 403 Forbidden when no key is set (Mutations blocked for security)
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden when SENTRY_API_KEY is not set, but got %d", resp.StatusCode)
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

func TestKillEndpointBruteForceBlocked(t *testing.T) {
	os.Setenv("SENTRY_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("SENTRY_API_KEY")

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
