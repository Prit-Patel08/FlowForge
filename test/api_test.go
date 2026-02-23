package test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"flowforge/internal/api"
	"flowforge/internal/database"
	"flowforge/internal/state"
)

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func boolNonEmptyString(v interface{}) bool {
	s, ok := v.(string)
	return ok && s != ""
}

func floatValue(v interface{}) float64 {
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}

func intValue(v interface{}) int {
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return int(f)
}

func problemDetail(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if detail, ok := payload["detail"].(string); ok && strings.TrimSpace(detail) != "" {
		return detail
	}
	if legacy, ok := payload["error"].(string); ok {
		return legacy
	}
	return ""
}

func metricValue(prometheus, metric string) (float64, bool) {
	prefix := metric + " "
	for _, line := range strings.Split(prometheus, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func lifecycleSnapshotForTest() (map[string]interface{}, int, string) {
	req := httptest.NewRequest("GET", "/worker/lifecycle", nil)
	w := httptest.NewRecorder()
	api.HandleWorkerLifecycle(w, req)
	body := w.Body.String()
	var payload map[string]interface{}
	_ = json.Unmarshal([]byte(body), &payload)
	return payload, w.Result().StatusCode, body
}

func waitForLifecyclePhase(t *testing.T, phase string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		payload, status, _ := lifecycleSnapshotForTest()
		if status == http.StatusOK && stringValue(payload["phase"]) == phase {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	payload, status, body := lifecycleSnapshotForTest()
	t.Fatalf(
		"expected lifecycle phase %q within %s (status=%d observed_phase=%q pid=%v body=%s)",
		phase,
		timeout,
		status,
		stringValue(payload["phase"]),
		payload["pid"],
		body,
	)
}

func snapshotTimelineEvent(raw map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"actor":            stringValue(raw["actor"]),
		"confidence_score": floatValue(raw["confidence_score"]),
		"cpu_score":        floatValue(raw["cpu_score"]),
		"entropy_score":    floatValue(raw["entropy_score"]),
		"has_event_id":     boolNonEmptyString(raw["event_id"]),
		"has_incident_id":  boolNonEmptyString(raw["incident_id"]),
		"has_run_id":       boolNonEmptyString(raw["run_id"]),
		"has_timestamp":    boolNonEmptyString(raw["timestamp"]),
		"pid":              intValue(raw["pid"]),
		"reason":           stringValue(raw["reason"]),
		"summary":          stringValue(raw["summary"]),
		"title":            stringValue(raw["title"]),
		"type":             stringValue(raw["type"]),
	}
}

func snapshotIncidentTimelineEvent(raw map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"actor":            stringValue(raw["actor"]),
		"confidence_score": floatValue(raw["confidence_score"]),
		"cpu_score":        floatValue(raw["cpu_score"]),
		"entropy_score":    floatValue(raw["entropy_score"]),
		"event_type":       stringValue(raw["event_type"]),
		"has_created_at":   boolNonEmptyString(raw["created_at"]),
		"has_event_id":     boolNonEmptyString(raw["event_id"]),
		"has_timestamp":    boolNonEmptyString(raw["timestamp"]),
		"incident_id":      stringValue(raw["incident_id"]),
		"pid":              intValue(raw["pid"]),
		"reason_text":      stringValue(raw["reason_text"]),
		"run_id":           stringValue(raw["run_id"]),
		"title":            stringValue(raw["title"]),
		"type":             stringValue(raw["type"]),
	}
}

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

func setEnvForTest(t *testing.T, key, value string) {
	t.Helper()
	oldValue, hadValue := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv(key, oldValue)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func checkHealthy(t *testing.T, checks map[string]interface{}, key string) bool {
	t.Helper()
	raw, ok := checks[key]
	if !ok {
		t.Fatalf("missing check %q", key)
	}
	checkMap, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("invalid check payload type for %q: %T", key, raw)
	}
	healthy, ok := checkMap["healthy"].(bool)
	if !ok {
		t.Fatalf("missing healthy bool for %q", key)
	}
	return healthy
}

func registerWorkspaceForTest(t *testing.T, workspaceID, workspacePath string) {
	t.Helper()
	reqBody := `{"workspace_id":"` + workspaceID + `","workspace_path":"` + workspacePath + `","profile":"standard","client":"cursor"}`
	req := httptest.NewRequest("POST", "/v1/integrations/workspaces/register", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceRegister(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected workspace register status 200, got %d", w.Result().StatusCode)
	}
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

func TestCORSHeadersAllowLoopbackOrigin(t *testing.T) {
	req := httptest.NewRequest("OPTIONS", "/incidents", nil)
	req.Header.Set("Origin", "http://127.0.0.1:3001")
	w := httptest.NewRecorder()

	api.HandleIncidents(w, req)

	resp := w.Result()
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "http://127.0.0.1:3001" {
		t.Errorf("Expected CORS header %q, got %q", "http://127.0.0.1:3001", origin)
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

func TestKillEndpointAcknowledgesAndTerminatesWorker(t *testing.T) {
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	cmd := exec.Command("/bin/sh", "-c", "trap '' TERM; sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	args := []string{"/bin/sh", "-c", "sleep 30"}
	state.UpdateState(0, "", "RUNNING", "/bin/sh -c sleep 30", args, "", pid)

	req := httptest.NewRequest("POST", "/process/kill", strings.NewReader(`{"reason":"test api kill"}`))
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleProcessKill(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stringValue(body["status"]) != "stop_requested" {
		t.Fatalf("expected status stop_requested, got %#v", body["status"])
	}
	if intValue(body["pid"]) != pid {
		t.Fatalf("expected response pid %d, got %d", pid, intValue(body["pid"]))
	}
	if stringValue(body["lifecycle"]) != "STOPPING" {
		t.Fatalf("expected lifecycle STOPPING, got %#v", body["lifecycle"])
	}

	st := state.GetState()
	if st.Status != "STOPPING" && st.Status != "STOPPED" {
		t.Fatalf("expected state STOPPING/STOPPED, got %q", st.Status)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	select {
	case <-waitCh:
		// Process exited (expected after kill).
	case <-time.After(3 * time.Second):
		t.Fatalf("worker pid %d did not exit after kill request", pid)
	}
	waitForLifecyclePhase(t, "STOPPED", 2*time.Second)
}

func TestRestartEndpointUpdatesRuntimeState(t *testing.T) {
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	restartArgs := []string{"/bin/sh", "-c", "sleep 15"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 15", restartArgs, "", 0)

	req := httptest.NewRequest("POST", "/process/restart", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	w := httptest.NewRecorder()
	api.HandleProcessRestart(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stringValue(body["status"]) != "restart_requested" {
		t.Fatalf("expected status restart_requested, got %#v", body["status"])
	}
	if stringValue(body["lifecycle"]) != "STARTING" {
		t.Fatalf("expected lifecycle STARTING, got %#v", body["lifecycle"])
	}

	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for {
		st := state.GetState()
		if st.Status == "RUNNING" && st.PID > 0 {
			pid = st.PID
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("restart did not reach RUNNING state in time, state=%+v", st)
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	st := state.GetState()
	if st.PID != pid {
		t.Fatalf("expected state pid %d, got %d", pid, st.PID)
	}
	if !reflect.DeepEqual(st.Args, restartArgs) {
		t.Fatalf("expected args %v, got %v", restartArgs, st.Args)
	}
	if st.Command != "/bin/sh -c sleep 15" {
		t.Fatalf("expected command to be preserved, got %q", st.Command)
	}
}

func TestRestartEndpointRejectsWhileProcessRunning(t *testing.T) {
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	restartArgs := []string{"/bin/sh", "-c", "sleep 30"}
	state.UpdateState(0, "", "RUNNING", "/bin/sh -c sleep 30", restartArgs, "", pid)

	req := httptest.NewRequest("POST", "/process/restart", nil)
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	w := httptest.NewRecorder()
	api.HandleProcessRestart(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 while process is running, got %d", resp.StatusCode)
	}

	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("expected original process to remain alive, got kill(0) error: %v", err)
	}
}

func TestRestartEndpointEnforcesRestartBudget(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")
	setEnvForTest(t, "FLOWFORGE_RESTART_BUDGET_MAX", "1")
	setEnvForTest(t, "FLOWFORGE_RESTART_BUDGET_WINDOW_SECONDS", "300")

	restartArgs := []string{"/bin/sh", "-c", "sleep 20"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 20", restartArgs, "", 0)

	restartReq1 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW1 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW1, restartReq1)
	if restartW1.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected first restart status 202, got %d", restartW1.Result().StatusCode)
	}

	var pid int
	restartDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(restartDeadline) {
		st := state.GetState()
		if st.Status == "RUNNING" && st.PID > 0 {
			pid = st.PID
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("expected restarted process pid > 0")
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	killReq := httptest.NewRequest("POST", "/process/kill", nil)
	killReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	killW := httptest.NewRecorder()
	api.HandleProcessKill(killW, killReq)
	if killW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected kill status 202, got %d", killW.Result().StatusCode)
	}

	waitForLifecyclePhase(t, "STOPPED", 4*time.Second)

	restartReq2 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW2 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW2, restartReq2)
	resp2 := restartW2.Result()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected restart status 429 when budget is exhausted, got %d", resp2.StatusCode)
	}
	if retryHeader := resp2.Header.Get("Retry-After"); retryHeader == "" {
		t.Fatalf("expected Retry-After header on restart budget response")
	}

	var errPayload map[string]interface{}
	if err := json.NewDecoder(restartW2.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode restart budget error payload: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(errPayload["error"])), "restart budget exceeded") {
		t.Fatalf("expected restart budget error message, got %#v", errPayload["error"])
	}
	retryAfter := intValue(errPayload["retry_after_seconds"])
	if retryAfter <= 0 {
		t.Fatalf("expected retry_after_seconds > 0, got %d", retryAfter)
	}

	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	api.HandleMetrics(metricsW, metricsReq)
	if metricsW.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metricsW.Result().StatusCode)
	}
	body := metricsW.Body.String()
	blocked, ok := metricValue(body, "flowforge_restart_budget_block_total")
	if !ok || blocked < 1 {
		t.Fatalf("expected flowforge_restart_budget_block_total >= 1, got %v (ok=%v)", blocked, ok)
	}
}

func TestRestartBudgetAllowsRequestsAfterWindow(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")
	setEnvForTest(t, "FLOWFORGE_RESTART_BUDGET_MAX", "1")
	setEnvForTest(t, "FLOWFORGE_RESTART_BUDGET_WINDOW_SECONDS", "1")

	restartArgs := []string{"/bin/sh", "-c", "sleep 20"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 20", restartArgs, "", 0)

	restartReq1 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW1 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW1, restartReq1)
	if restartW1.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected first restart status 202, got %d", restartW1.Result().StatusCode)
	}

	var firstPID int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := state.GetState()
		if st.Status == "RUNNING" && st.PID > 0 {
			firstPID = st.PID
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if firstPID == 0 {
		t.Fatal("expected first restarted process pid > 0")
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-firstPID, syscall.SIGKILL)
		_ = syscall.Kill(firstPID, syscall.SIGKILL)
	})

	killReq := httptest.NewRequest("POST", "/process/kill", nil)
	killReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	killW := httptest.NewRecorder()
	api.HandleProcessKill(killW, killReq)
	if killW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected kill status 202, got %d", killW.Result().StatusCode)
	}
	waitForLifecyclePhase(t, "STOPPED", 4*time.Second)

	restartReq2 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW2 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW2, restartReq2)
	if restartW2.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected second restart status 429 during active budget window, got %d", restartW2.Result().StatusCode)
	}

	time.Sleep(1200 * time.Millisecond)

	restartReq3 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW3 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW3, restartReq3)
	if restartW3.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected third restart status 202 after budget window, got %d", restartW3.Result().StatusCode)
	}

	var secondPID int
	secondDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(secondDeadline) {
		st := state.GetState()
		if st.Status == "RUNNING" && st.PID > 0 && st.PID != firstPID {
			secondPID = st.PID
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if secondPID == 0 {
		t.Fatal("expected second restarted process pid > 0")
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-secondPID, syscall.SIGKILL)
		_ = syscall.Kill(secondPID, syscall.SIGKILL)
	})
}

func TestKillAndRestartConflictDuringStop(t *testing.T) {
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	args := []string{"/bin/sh", "-c", "trap '' TERM; sleep 30"}
	state.UpdateState(0, "", "RUNNING", "/bin/sh -c trap '' TERM; sleep 30", args, "", pid)

	killReq := httptest.NewRequest("POST", "/process/kill", nil)
	killReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	killW := httptest.NewRecorder()
	api.HandleProcessKill(killW, killReq)
	if killW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected kill status 202, got %d", killW.Result().StatusCode)
	}

	restartReq := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW := httptest.NewRecorder()
	api.HandleProcessRestart(restartW, restartReq)
	if restartW.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected restart status 409 during STOPPING, got %d", restartW.Result().StatusCode)
	}
	waitForLifecyclePhase(t, "STOPPED", 4*time.Second)
}

func TestRepeatedKillRequestsAreIdempotent(t *testing.T) {
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	args := []string{"/bin/sh", "-c", "sleep 30"}
	state.UpdateState(0, "", "RUNNING", "/bin/sh -c sleep 30", args, "", pid)

	req1 := httptest.NewRequest("POST", "/process/kill", nil)
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	w1 := httptest.NewRecorder()
	api.HandleProcessKill(w1, req1)
	if w1.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected first kill status 202, got %d", w1.Result().StatusCode)
	}

	req2 := httptest.NewRequest("POST", "/process/kill", nil)
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	w2 := httptest.NewRecorder()
	api.HandleProcessKill(w2, req2)
	if w2.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected second kill status 202 idempotent, got %d", w2.Result().StatusCode)
	}
	waitForLifecyclePhase(t, "STOPPED", 4*time.Second)
}

func TestProcessKillIdempotencyReplayAndConflict(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	state.UpdateState(0, "", "STOPPED", "", nil, "", 0)

	key := "idem-kill-replay-1"
	firstBody := `{"reason":"idempotent kill test"}`

	req1 := httptest.NewRequest("POST", "/process/kill", strings.NewReader(firstBody))
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	req1.Header.Set("X-Request-Id", "req-test-kill-idem")
	w1 := httptest.NewRecorder()
	api.HandleProcessKill(w1, req1)
	status1 := w1.Result().StatusCode
	if status1 >= 500 {
		t.Fatalf("expected non-5xx status for first idempotent request, got %d", status1)
	}

	var payload1 map[string]interface{}
	if err := json.NewDecoder(w1.Body).Decode(&payload1); err != nil {
		t.Fatalf("decode first response: %v", err)
	}

	req2 := httptest.NewRequest("POST", "/process/kill", strings.NewReader(firstBody))
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	req2.Header.Set("X-Request-Id", "req-test-kill-idem")
	w2 := httptest.NewRecorder()
	api.HandleProcessKill(w2, req2)

	if w2.Result().StatusCode != status1 {
		t.Fatalf("expected replay status %d, got %d", status1, w2.Result().StatusCode)
	}
	if replay := w2.Result().Header.Get("X-Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected X-Idempotent-Replay=true, got %q", replay)
	}

	var payload2 map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode replay response: %v", err)
	}
	if !reflect.DeepEqual(payload1, payload2) {
		t.Fatalf("expected replay payload to match first payload, first=%v replay=%v", payload1, payload2)
	}

	req3 := httptest.NewRequest("POST", "/process/kill", strings.NewReader(`{"reason":"different body"}`))
	req3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	req3.Header.Set("X-Request-Id", "req-test-kill-idem")
	w3 := httptest.NewRecorder()
	api.HandleProcessKill(w3, req3)
	if w3.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected idempotency conflict status 409, got %d", w3.Result().StatusCode)
	}

	var conflictPayload map[string]interface{}
	if err := json.NewDecoder(w3.Body).Decode(&conflictPayload); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(conflictPayload["error"])), "idempotency key reused") {
		t.Fatalf("expected idempotency conflict error, got %#v", conflictPayload["error"])
	}

	rec, err := database.GetControlPlaneReplay(key, "POST /process/kill")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay: %v", err)
	}
	if rec.ReplayCount != 1 {
		t.Fatalf("expected replay_count=1, got %d", rec.ReplayCount)
	}

	audits, err := database.GetAuditEvents(10)
	if err != nil {
		t.Fatalf("GetAuditEvents: %v", err)
	}
	foundConflict := false
	for _, ev := range audits {
		if ev.Action != "IDEMPOTENT_CONFLICT" {
			continue
		}
		foundConflict = true
		if !strings.Contains(ev.Reason, "request_id=req-test-kill-idem") {
			t.Fatalf("expected idempotency conflict reason to include request id, got %q", ev.Reason)
		}
		break
	}
	if !foundConflict {
		t.Fatalf("expected IDEMPOTENT_CONFLICT audit event in recent history")
	}
}

func TestWorkerLifecycleEndpointSnapshotContract(t *testing.T) {
	api.ResetWorkerControlForTests()

	cmd := exec.Command("/bin/sh", "-c", "sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start worker process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
	})

	state.UpdateState(0, "", "RUNNING", "/bin/sh -c sleep 30", []string{"/bin/sh", "-c", "sleep 30"}, "", pid)

	req := httptest.NewRequest("GET", "/worker/lifecycle", nil)
	w := httptest.NewRecorder()
	api.HandleWorkerLifecycle(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	requiredKeys := []string{"phase", "operation", "pid", "managed", "last_error", "status", "lifecycle", "command", "timestamp"}
	for _, key := range requiredKeys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing key %q in lifecycle response", key)
		}
	}

	if stringValue(payload["phase"]) != "RUNNING" {
		t.Fatalf("expected phase RUNNING, got %q", stringValue(payload["phase"]))
	}
	if intValue(payload["pid"]) != pid {
		t.Fatalf("expected pid %d, got %d", pid, intValue(payload["pid"]))
	}
	if stringValue(payload["status"]) != "RUNNING" {
		t.Fatalf("expected status RUNNING, got %q", stringValue(payload["status"]))
	}
}

func TestWorkerLifecycleEndpointMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("POST", "/worker/lifecycle", nil)
	w := httptest.NewRecorder()
	api.HandleWorkerLifecycle(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func TestTimelineIncludesLifecycleTransitionEvidence(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	restartArgs := []string{"/bin/sh", "-c", "sleep 5"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 5", restartArgs, "", 0)

	restartReq := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW := httptest.NewRecorder()
	api.HandleProcessRestart(restartW, restartReq)
	if restartW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected restart status 202, got %d", restartW.Result().StatusCode)
	}

	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := state.GetState()
		if st.PID > 0 {
			pid = st.PID
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pid > 0 {
		t.Cleanup(func() {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = syscall.Kill(pid, syscall.SIGKILL)
		})
	}

	var lifecycleEvent map[string]interface{}
	findDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(findDeadline) {
		timelineReq := httptest.NewRequest("GET", "/timeline", nil)
		timelineW := httptest.NewRecorder()
		api.HandleTimeline(timelineW, timelineReq)
		if timelineW.Result().StatusCode != http.StatusOK {
			t.Fatalf("expected timeline status 200, got %d", timelineW.Result().StatusCode)
		}

		var payload []map[string]interface{}
		if err := json.NewDecoder(timelineW.Body).Decode(&payload); err != nil {
			t.Fatalf("decode timeline: %v", err)
		}
		for _, ev := range payload {
			if stringValue(ev["type"]) == "lifecycle" {
				lifecycleEvent = ev
				break
			}
		}
		if lifecycleEvent != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if lifecycleEvent == nil {
		t.Fatal("expected at least one lifecycle event in timeline")
	}
	if !strings.HasPrefix(stringValue(lifecycleEvent["title"]), "LIFECYCLE_") {
		t.Fatalf("expected lifecycle title prefix, got %q", stringValue(lifecycleEvent["title"]))
	}

	evidence, ok := lifecycleEvent["evidence"].(map[string]interface{})
	if !ok || len(evidence) == 0 {
		t.Fatalf("expected lifecycle evidence payload, got %#v", lifecycleEvent["evidence"])
	}
	for _, key := range []string{"phase", "operation", "pid", "managed", "trigger"} {
		if _, ok := evidence[key]; !ok {
			t.Fatalf("expected lifecycle evidence key %q", key)
		}
	}
	if stringValue(evidence["phase"]) == "" {
		t.Fatalf("expected non-empty lifecycle phase evidence, got %#v", evidence["phase"])
	}
}

func TestMetricsIncludeLifecycleLatencySLOSignals(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	restartArgs := []string{"/bin/sh", "-c", "sleep 20"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 20", restartArgs, "", 0)

	restartReq := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW := httptest.NewRecorder()
	api.HandleProcessRestart(restartW, restartReq)
	if restartW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected restart status 202, got %d", restartW.Result().StatusCode)
	}

	lifecycleSnapshot := func() (map[string]interface{}, int, string) {
		req := httptest.NewRequest("GET", "/worker/lifecycle", nil)
		w := httptest.NewRecorder()
		api.HandleWorkerLifecycle(w, req)
		body := w.Body.String()
		var payload map[string]interface{}
		_ = json.Unmarshal([]byte(body), &payload)
		return payload, w.Result().StatusCode, body
	}
	metricsSnapshot := func() (string, int) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		api.HandleMetrics(w, req)
		return w.Body.String(), w.Result().StatusCode
	}

	var pid int
	restartDeadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(restartDeadline) {
		if st := state.GetState(); st.PID > 0 {
			pid = st.PID
			break
		}

		lifecyclePayload, lifecycleStatus, _ := lifecycleSnapshot()
		if lifecycleStatus == http.StatusOK {
			if stringValue(lifecyclePayload["phase"]) == "RUNNING" && intValue(lifecyclePayload["pid"]) > 0 {
				pid = intValue(lifecyclePayload["pid"])
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pid == 0 {
		st := state.GetState()
		lifecyclePayload, lifecycleStatus, lifecycleBody := lifecycleSnapshot()
		t.Fatalf(
			"expected restarted process pid > 0 (state=%+v lifecycle_status=%d lifecycle_phase=%q lifecycle_pid=%v lifecycle_body=%s)",
			st,
			lifecycleStatus,
			stringValue(lifecyclePayload["phase"]),
			lifecyclePayload["pid"],
			lifecycleBody,
		)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	killReq := httptest.NewRequest("POST", "/process/kill", nil)
	killReq.Header.Set("Authorization", "Bearer test-secret-key-12345")
	killW := httptest.NewRecorder()
	api.HandleProcessKill(killW, killReq)
	if killW.Result().StatusCode != http.StatusAccepted {
		t.Fatalf("expected kill status 202, got %d", killW.Result().StatusCode)
	}

	stopDeadline := time.Now().Add(8 * time.Second)
	stopObserved := false
	var latestMetricsBody string
	for time.Now().Before(stopDeadline) {
		lifecyclePayload, lifecycleStatus, _ := lifecycleSnapshot()
		if lifecycleStatus == http.StatusOK && stringValue(lifecyclePayload["phase"]) == "STOPPED" {
			latestMetricsBody, metricsStatus := metricsSnapshot()
			if metricsStatus == http.StatusOK {
				if stopCount, ok := metricValue(latestMetricsBody, "flowforge_stop_latency_count"); ok && stopCount >= 1 {
					stopObserved = true
					break
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !stopObserved {
		lifecyclePayload, lifecycleStatus, lifecycleBody := lifecycleSnapshot()
		t.Fatalf(
			"expected stop lifecycle and metrics to settle (lifecycle_status=%d lifecycle_phase=%q lifecycle_pid=%v lifecycle_body=%s metrics=%s)",
			lifecycleStatus,
			stringValue(lifecyclePayload["phase"]),
			lifecyclePayload["pid"],
			lifecycleBody,
			latestMetricsBody,
		)
	}

	body, metricsStatus := metricsSnapshot()
	if metricsStatus != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metricsStatus)
	}

	restartCount, ok := metricValue(body, "flowforge_restart_latency_count")
	if !ok || restartCount < 1 {
		t.Fatalf("expected flowforge_restart_latency_count >= 1, got %v (ok=%v)", restartCount, ok)
	}
	stopCount, ok := metricValue(body, "flowforge_stop_latency_count")
	if !ok || stopCount < 1 {
		t.Fatalf("expected flowforge_stop_latency_count >= 1, got %v (ok=%v)", stopCount, ok)
	}
	if _, ok := metricValue(body, "flowforge_restart_slo_compliance_ratio"); !ok {
		t.Fatalf("expected flowforge_restart_slo_compliance_ratio metric in output")
	}
	if _, ok := metricValue(body, "flowforge_stop_slo_compliance_ratio"); !ok {
		t.Fatalf("expected flowforge_stop_slo_compliance_ratio metric in output")
	}
	if _, ok := metricValue(body, "flowforge_controlplane_replay_rows"); !ok {
		t.Fatalf("expected flowforge_controlplane_replay_rows metric in output")
	}
	if _, ok := metricValue(body, "flowforge_controlplane_replay_oldest_age_seconds"); !ok {
		t.Fatalf("expected flowforge_controlplane_replay_oldest_age_seconds metric in output")
	}
	if statsErr, ok := metricValue(body, "flowforge_controlplane_replay_stats_error"); !ok || statsErr != 0 {
		t.Fatalf("expected flowforge_controlplane_replay_stats_error=0, got %v (ok=%v)", statsErr, ok)
	}
}

func TestControlPlaneReplayHistoryEndpointContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-replay-history")

	for i := 0; i < 2; i++ {
		if _, err := database.InsertEvent(
			"audit",
			"control-plane",
			"served cached response",
			"run-replay-history",
			"incident-replay-history",
			"IDEMPOTENT_REPLAY",
			"served cached control-plane mutation response",
			0,
			0,
			0,
			0,
		); err != nil {
			t.Fatalf("insert replay event: %v", err)
		}
	}
	if _, err := database.InsertEvent(
		"audit",
		"control-plane",
		"idempotency key reused with different payload",
		"run-replay-history",
		"incident-replay-history",
		"IDEMPOTENT_CONFLICT",
		"idempotency key reused with different request payload",
		0,
		0,
		0,
		0,
	); err != nil {
		t.Fatalf("insert conflict event: %v", err)
	}

	req := httptest.NewRequest("GET", "/v1/ops/controlplane/replay/history?days=7", nil)
	w := httptest.NewRecorder()
	api.HandleControlPlaneReplayHistory(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode replay history payload: %v", err)
	}

	if intValue(payload["days"]) != 7 {
		t.Fatalf("expected days=7, got %#v", payload["days"])
	}
	if _, ok := payload["row_count"]; !ok {
		t.Fatalf("expected row_count in replay history payload")
	}
	if _, ok := payload["oldest_age_seconds"]; !ok {
		t.Fatalf("expected oldest_age_seconds in replay history payload")
	}

	pointsRaw, ok := payload["points"].([]interface{})
	if !ok {
		t.Fatalf("expected points array, got %T", payload["points"])
	}
	if len(pointsRaw) != 7 {
		t.Fatalf("expected 7 history points, got %d", len(pointsRaw))
	}

	today := time.Now().UTC().Format("2006-01-02")
	foundToday := false
	for _, item := range pointsRaw {
		point, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("expected point object, got %T", item)
		}
		day := stringValue(point["day"])
		if day == "" {
			t.Fatalf("expected non-empty point day, got %#v", point["day"])
		}
		if day != today {
			continue
		}
		foundToday = true
		if got := intValue(point["replay_events"]); got != 2 {
			t.Fatalf("expected today replay_events=2, got %d", got)
		}
		if got := intValue(point["conflict_events"]); got != 1 {
			t.Fatalf("expected today conflict_events=1, got %d", got)
		}
	}
	if !foundToday {
		t.Fatalf("expected a history point for today (%s)", today)
	}
}

func TestControlPlaneReplayHistoryEndpointRejectsInvalidDays(t *testing.T) {
	setupTempDBForAPI(t)

	cases := []string{
		"/v1/ops/controlplane/replay/history?days=0",
		"/v1/ops/controlplane/replay/history?days=91",
		"/v1/ops/controlplane/replay/history?days=abc",
	}
	for _, path := range cases {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("X-Request-Id", "req-test-invalid-days")
		w := httptest.NewRecorder()
		api.HandleControlPlaneReplayHistory(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("path %s expected status 400, got %d", path, resp.StatusCode)
		}
		if ctype := resp.Header.Get("Content-Type"); !strings.Contains(ctype, "application/problem+json") {
			t.Fatalf("path %s expected application/problem+json content-type, got %q", path, ctype)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("path %s decode error payload: %v", path, err)
		}
		if title := stringValue(payload["title"]); title != "Bad Request" {
			t.Fatalf("path %s expected title Bad Request, got %q", path, title)
		}
		if got := intValue(payload["status"]); got != http.StatusBadRequest {
			t.Fatalf("path %s expected status field 400, got %d", path, got)
		}
		if typeURI := stringValue(payload["type"]); typeURI != "https://flowforge.dev/problems/bad-request" {
			t.Fatalf("path %s expected bad-request type URI, got %q", path, typeURI)
		}
		if instance := stringValue(payload["instance"]); instance != "/v1/ops/controlplane/replay/history" {
			t.Fatalf("path %s expected instance /v1/ops/controlplane/replay/history, got %q", path, instance)
		}
		if reqID := stringValue(payload["request_id"]); reqID != "req-test-invalid-days" {
			t.Fatalf("path %s expected request_id req-test-invalid-days, got %q", path, reqID)
		}
		if headerReqID := resp.Header.Get("X-Request-Id"); headerReqID != "req-test-invalid-days" {
			t.Fatalf("path %s expected X-Request-Id req-test-invalid-days, got %q", path, headerReqID)
		}
		errMsg := strings.ToLower(problemDetail(payload))
		if !strings.Contains(errMsg, "days must be an integer between 1 and 90") {
			t.Fatalf("path %s expected days validation error, got payload=%#v", path, payload)
		}
	}
}

func TestRequestTraceEndpointContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-request-trace")
	requestID := "req-trace-contract-1"

	if _, err := database.InsertEventWithPayloadAndRequestID(
		"audit",
		"api-key",
		"operator requested restart",
		"run-request-trace",
		"incident-request-trace",
		"RESTART",
		"manual restart request",
		4242,
		0,
		0,
		0,
		requestID,
		map[string]interface{}{"source": "api"},
	); err != nil {
		t.Fatalf("insert request-trace audit event: %v", err)
	}
	if _, err := database.InsertEventWithPayloadAndRequestID(
		"decision",
		"system",
		"restart allowed by policy",
		"run-request-trace",
		"incident-request-trace",
		"RESTART_ALLOWED",
		"policy accepted restart request",
		4242,
		12.5,
		40.1,
		88.2,
		requestID,
		nil,
	); err != nil {
		t.Fatalf("insert request-trace decision event: %v", err)
	}

	handler := api.NewHandler()
	req := httptest.NewRequest("GET", "/v1/ops/requests/"+requestID+"?limit=50", nil)
	req.Header.Set("X-Request-Id", "req-test-request-trace-contract")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if headerReqID := resp.Header.Get("X-Request-Id"); headerReqID != "req-test-request-trace-contract" {
		t.Fatalf("expected X-Request-Id header to echo request id, got %q", headerReqID)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode request trace payload: %v", err)
	}
	if got := stringValue(payload["request_id"]); got != requestID {
		t.Fatalf("expected request_id %q, got %q", requestID, got)
	}
	if got := intValue(payload["count"]); got != 2 {
		t.Fatalf("expected count=2, got %d", got)
	}
	eventsRaw, ok := payload["events"].([]interface{})
	if !ok {
		t.Fatalf("expected events array, got %T", payload["events"])
	}
	if len(eventsRaw) != 2 {
		t.Fatalf("expected events length 2, got %d", len(eventsRaw))
	}
	first, ok := eventsRaw[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first event object, got %T", eventsRaw[0])
	}
	if got := stringValue(first["request_id"]); got != requestID {
		t.Fatalf("expected first event request_id %q, got %q", requestID, got)
	}
}

func TestRequestTraceEndpointRejectsInvalidParams(t *testing.T) {
	setupTempDBForAPI(t)

	handler := api.NewHandler()
	cases := []struct {
		path            string
		detailSubstring string
	}{
		{path: "/v1/ops/requests", detailSubstring: "request_id is required"},
		{path: "/v1/ops/requests/", detailSubstring: "request_id is required"},
		{path: "/v1/ops/requests/req-test-invalid?limit=0", detailSubstring: "limit must be an integer between 1 and 1000"},
		{path: "/v1/ops/requests/req-test-invalid?limit=1001", detailSubstring: "limit must be an integer between 1 and 1000"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.Header.Set("X-Request-Id", "req-test-request-trace-invalid")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("path %s expected status 400, got %d", tc.path, resp.StatusCode)
		}
		if ctype := resp.Header.Get("Content-Type"); !strings.Contains(ctype, "application/problem+json") {
			t.Fatalf("path %s expected application/problem+json content-type, got %q", tc.path, ctype)
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("path %s decode response payload: %v", tc.path, err)
		}
		if reqID := stringValue(payload["request_id"]); reqID != "req-test-request-trace-invalid" {
			t.Fatalf("path %s expected request_id req-test-request-trace-invalid, got %q", tc.path, reqID)
		}
		if detail := strings.ToLower(problemDetail(payload)); !strings.Contains(detail, strings.ToLower(tc.detailSubstring)) {
			t.Fatalf("path %s expected detail to contain %q, got payload=%#v", tc.path, tc.detailSubstring, payload)
		}
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

func TestV1CoreEndpointAliases(t *testing.T) {
	setupTempDBForAPI(t)

	handler := api.NewHandler()
	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "health", path: "/v1/healthz", wantStatus: http.StatusOK},
		{name: "ready", path: "/v1/readyz", wantStatus: http.StatusOK},
		{name: "metrics", path: "/v1/metrics", wantStatus: http.StatusOK},
		{name: "incidents", path: "/v1/incidents", wantStatus: http.StatusOK},
		{name: "timeline", path: "/v1/timeline", wantStatus: http.StatusOK},
		{name: "lifecycle", path: "/v1/worker/lifecycle", wantStatus: http.StatusOK},
		{name: "request-trace", path: "/v1/ops/requests/req-test-alias", wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			resp := w.Result()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("%s expected status %d, got %d", tc.path, tc.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestV1ProcessKillAliasRequiresAuth(t *testing.T) {
	setEnvForTest(t, "FLOWFORGE_API_KEY", "test-secret-key-12345")
	setupTempDBForAPI(t)

	handler := api.NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/v1/process/kill", nil)
	req.Header.Set("X-Request-Id", "req-test-kill-auth")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", resp.StatusCode)
	}
	if ctype := resp.Header.Get("Content-Type"); !strings.Contains(ctype, "application/problem+json") {
		t.Fatalf("expected application/problem+json content-type, got %q", ctype)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	if title := stringValue(payload["title"]); title != "Unauthorized" {
		t.Fatalf("expected title Unauthorized, got %q", title)
	}
	if got := intValue(payload["status"]); got != http.StatusUnauthorized {
		t.Fatalf("expected status field 401, got %d", got)
	}
	if typeURI := stringValue(payload["type"]); typeURI != "https://flowforge.dev/problems/unauthorized" {
		t.Fatalf("expected unauthorized type URI, got %q", typeURI)
	}
	if instance := stringValue(payload["instance"]); instance != "/v1/process/kill" {
		t.Fatalf("expected instance /v1/process/kill, got %q", instance)
	}
	if reqID := stringValue(payload["request_id"]); reqID != "req-test-kill-auth" {
		t.Fatalf("expected request_id req-test-kill-auth, got %q", reqID)
	}
	if headerReqID := resp.Header.Get("X-Request-Id"); headerReqID != "req-test-kill-auth" {
		t.Fatalf("expected X-Request-Id req-test-kill-auth, got %q", headerReqID)
	}
	if detail := strings.ToLower(problemDetail(payload)); !strings.Contains(detail, "authorization required") {
		t.Fatalf("expected auth-required detail, got payload=%#v", payload)
	}
}

func TestReadyEndpointCloudDepsRequiredFailsWhenUnavailable(t *testing.T) {
	setupTempDBForAPI(t)
	setEnvForTest(t, "FLOWFORGE_CLOUD_DEPS_REQUIRED", "1")
	setEnvForTest(t, "FLOWFORGE_CLOUD_POSTGRES_ADDR", "127.0.0.1:1")
	setEnvForTest(t, "FLOWFORGE_CLOUD_REDIS_ADDR", "127.0.0.1:2")
	setEnvForTest(t, "FLOWFORGE_CLOUD_NATS_HEALTH_URL", "http://127.0.0.1:3/healthz")
	setEnvForTest(t, "FLOWFORGE_CLOUD_MINIO_HEALTH_URL", "http://127.0.0.1:4/minio/health/live")
	setEnvForTest(t, "FLOWFORGE_CLOUD_PROBE_TIMEOUT_MS", "100")

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	api.HandleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 when required cloud deps are unavailable, got %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload["status"] != "not-ready" {
		t.Fatalf("expected status not-ready, got %#v", payload["status"])
	}
	if payload["cloud_dependencies_required"] != true {
		t.Fatalf("expected cloud_dependencies_required=true, got %#v", payload["cloud_dependencies_required"])
	}

	checks, ok := payload["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks is not an object: %T", payload["checks"])
	}

	if !checkHealthy(t, checks, "database") {
		t.Fatalf("expected database check healthy=true")
	}
	if checkHealthy(t, checks, "cloud_postgres") {
		t.Fatalf("expected cloud_postgres healthy=false")
	}
	if checkHealthy(t, checks, "cloud_redis") {
		t.Fatalf("expected cloud_redis healthy=false")
	}
	if checkHealthy(t, checks, "cloud_nats") {
		t.Fatalf("expected cloud_nats healthy=false")
	}
	if checkHealthy(t, checks, "cloud_minio") {
		t.Fatalf("expected cloud_minio healthy=false")
	}
}

func TestReadyEndpointCloudDepsRequiredPasses(t *testing.T) {
	setupTempDBForAPI(t)
	setEnvForTest(t, "FLOWFORGE_CLOUD_DEPS_REQUIRED", "1")
	setEnvForTest(t, "FLOWFORGE_CLOUD_PROBE_TIMEOUT_MS", "200")

	pgListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen postgres probe: %v", err)
	}
	defer pgListener.Close()

	redisListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen redis probe: %v", err)
	}
	defer redisListener.Close()

	natsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer natsServer.Close()

	minioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer minioServer.Close()

	setEnvForTest(t, "FLOWFORGE_CLOUD_POSTGRES_ADDR", pgListener.Addr().String())
	setEnvForTest(t, "FLOWFORGE_CLOUD_REDIS_ADDR", redisListener.Addr().String())
	setEnvForTest(t, "FLOWFORGE_CLOUD_NATS_HEALTH_URL", natsServer.URL)
	setEnvForTest(t, "FLOWFORGE_CLOUD_MINIO_HEALTH_URL", minioServer.URL)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	api.HandleReady(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 when all required cloud deps are healthy, got %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload["status"] != "ready" {
		t.Fatalf("expected status ready, got %#v", payload["status"])
	}

	checks, ok := payload["checks"].(map[string]interface{})
	if !ok {
		t.Fatalf("checks is not an object: %T", payload["checks"])
	}

	if !checkHealthy(t, checks, "database") {
		t.Fatalf("expected database check healthy=true")
	}
	if !checkHealthy(t, checks, "cloud_postgres") {
		t.Fatalf("expected cloud_postgres healthy=true")
	}
	if !checkHealthy(t, checks, "cloud_redis") {
		t.Fatalf("expected cloud_redis healthy=true")
	}
	if !checkHealthy(t, checks, "cloud_nats") {
		t.Fatalf("expected cloud_nats healthy=true")
	}
	if !checkHealthy(t, checks, "cloud_minio") {
		t.Fatalf("expected cloud_minio healthy=true")
	}
}

func TestTimelineEndpoint(t *testing.T) {
	setupTempDBForAPI(t)
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

func TestTimelineEndpointSnapshotContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-api-snapshot")
	incidentID := "incident-snapshot-001"

	if _, err := database.InsertEvent(
		"decision",
		"system",
		"CPU exceeded threshold",
		"run-api-snapshot",
		incidentID,
		"KILL",
		"CPU 92 / Entropy 21 / Confidence 93",
		4242,
		92.0,
		21.0,
		93.0,
	); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	req := httptest.NewRequest("GET", "/timeline", nil)
	w := httptest.NewRecorder()
	api.HandleTimeline(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected timeline payload to include at least one event")
	}

	got := snapshotTimelineEvent(payload[0])
	expected := map[string]interface{}{
		"actor":            "system",
		"confidence_score": 93.0,
		"cpu_score":        92.0,
		"entropy_score":    21.0,
		"has_event_id":     true,
		"has_incident_id":  true,
		"has_run_id":       true,
		"has_timestamp":    true,
		"pid":              4242,
		"reason":           "CPU exceeded threshold",
		"summary":          "CPU 92 / Entropy 21 / Confidence 93",
		"title":            "KILL",
		"type":             "decision",
	}

	if !reflect.DeepEqual(got, expected) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		expJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("timeline snapshot mismatch\nexpected:\n%s\ngot:\n%s", expJSON, gotJSON)
	}
}

func TestTimelineIncidentEndpointSnapshotContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-api-snapshot")
	incidentID := "incident-snapshot-002"

	if _, err := database.InsertEvent(
		"audit",
		"api-key",
		"operator requested restart",
		"run-api-snapshot",
		incidentID,
		"RESTART",
		"manual restart by operator",
		5151,
		0,
		0,
		0,
	); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	req := httptest.NewRequest("GET", "/timeline?incident_id="+incidentID, nil)
	w := httptest.NewRecorder()
	api.HandleTimeline(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1 event for incident timeline, got %d", len(payload))
	}

	got := snapshotIncidentTimelineEvent(payload[0])
	expected := map[string]interface{}{
		"actor":            "api-key",
		"confidence_score": 0.0,
		"cpu_score":        0.0,
		"entropy_score":    0.0,
		"event_type":       "audit",
		"has_created_at":   true,
		"has_event_id":     true,
		"has_timestamp":    true,
		"incident_id":      incidentID,
		"pid":              5151,
		"reason_text":      "operator requested restart",
		"run_id":           "run-api-snapshot",
		"title":            "RESTART",
		"type":             "audit",
	}

	if !reflect.DeepEqual(got, expected) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		expJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("incident timeline snapshot mismatch\nexpected:\n%s\ngot:\n%s", expJSON, gotJSON)
	}
}

func TestTimelineDecisionEngineVersionContract(t *testing.T) {
	setupTempDBForAPI(t)
	database.SetRunID("run-timeline-engine-contract")
	incidentID := "incident-engine-contract-1"

	meta := database.DecisionTraceMeta{
		DecisionEngine:    "threshold-decider",
		EngineVersion:     "1.1.0",
		DecisionContract:  "decision-trace.v1",
		PolicyRolloutMode: "canary",
	}
	if err := database.LogDecisionTraceWithIncidentAndMeta(
		"python3 worker.py",
		5150,
		94.0,
		12.0,
		96.0,
		"KILL",
		"timeline contract with engine version",
		incidentID,
		meta,
	); err != nil {
		t.Fatalf("LogDecisionTraceWithIncidentAndMeta: %v", err)
	}

	req := httptest.NewRequest("GET", "/timeline?incident_id="+incidentID, nil)
	w := httptest.NewRecorder()
	api.HandleTimeline(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var payload []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode timeline payload: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected timeline payload to include decision event")
	}
	ev := payload[0]
	if got := stringValue(ev["engine_version"]); got != meta.EngineVersion {
		t.Fatalf("expected engine_version %q, got %q", meta.EngineVersion, got)
	}
	if got := stringValue(ev["decision_engine"]); got != meta.DecisionEngine {
		t.Fatalf("expected decision_engine %q, got %q", meta.DecisionEngine, got)
	}
	if got := stringValue(ev["decision_contract_version"]); got != meta.DecisionContract {
		t.Fatalf("expected decision_contract_version %q, got %q", meta.DecisionContract, got)
	}
	if got := stringValue(ev["rollout_mode"]); got != meta.PolicyRolloutMode {
		t.Fatalf("expected rollout_mode %q, got %q", meta.PolicyRolloutMode, got)
	}
}

func TestRequestTraceEndpointIncludesDecisionEngineVersionContract(t *testing.T) {
	setupTempDBForAPI(t)
	requestID := "req-trace-engine-contract"

	if _, err := database.InsertEventWithPayloadAndRequestID(
		"decision",
		"system",
		"request trace with versioned decision metadata",
		"run-request-trace-engine",
		"incident-request-trace-engine",
		"KILL",
		"policy enforcement executed",
		7070,
		98.0,
		10.0,
		97.5,
		requestID,
		map[string]interface{}{
			"id":                        1,
			"command":                   "python3 worker.py",
			"decision_engine":           "threshold-decider",
			"engine_version":            "1.1.0",
			"decision_contract_version": "decision-trace.v1",
			"rollout_mode":              "enforce",
		},
	); err != nil {
		t.Fatalf("insert versioned decision event: %v", err)
	}

	handler := api.NewHandler()
	req := httptest.NewRequest("GET", "/v1/ops/requests/"+requestID, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode request trace payload: %v", err)
	}
	eventsRaw, ok := body["events"].([]interface{})
	if !ok || len(eventsRaw) == 0 {
		t.Fatalf("expected non-empty events array, got %#v", body["events"])
	}
	ev, ok := eventsRaw[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first event object, got %T", eventsRaw[0])
	}
	if got := stringValue(ev["engine_version"]); got != "1.1.0" {
		t.Fatalf("expected engine_version 1.1.0, got %q", got)
	}
	if got := stringValue(ev["decision_engine"]); got != "threshold-decider" {
		t.Fatalf("expected decision_engine threshold-decider, got %q", got)
	}
}

func TestIntegrationWorkspaceRegisterAndStatus(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-123", "/tmp/ws-123")

	req := httptest.NewRequest("GET", "/v1/integrations/workspaces/ws-123/status", nil)
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stringValue(payload["workspace_id"]) != "ws-123" {
		t.Fatalf("expected workspace_id ws-123, got %#v", payload["workspace_id"])
	}
	if stringValue(payload["profile"]) != "standard" {
		t.Fatalf("expected profile standard, got %#v", payload["profile"])
	}
	enabled, ok := payload["protection_enabled"].(bool)
	if !ok || !enabled {
		t.Fatalf("expected protection_enabled=true, got %#v", payload["protection_enabled"])
	}
}

func TestIntegrationWorkspaceProtectionToggle(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-toggle", "/tmp/ws-toggle")

	req := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-toggle/protection", strings.NewReader(`{"enabled":false,"reason":"disable for maintenance"}`))
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	statusReq := httptest.NewRequest("GET", "/v1/integrations/workspaces/ws-toggle/status", nil)
	statusW := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(statusW, statusReq)
	if statusW.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusW.Result().StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(statusW.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	enabled, ok := payload["protection_enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("expected protection_enabled=false, got %#v", payload["protection_enabled"])
	}
}

func TestIntegrationWorkspaceUnregister(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-delete", "/tmp/ws-delete")

	req := httptest.NewRequest("DELETE", "/v1/integrations/workspaces/ws-delete", strings.NewReader(`{"reason":"remove from integration tests"}`))
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", "req_ws_delete_1")
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if stringValue(payload["workspace_id"]) != "ws-delete" {
		t.Fatalf("expected workspace_id ws-delete, got %#v", payload["workspace_id"])
	}
	if unregistered, _ := payload["unregistered"].(bool); !unregistered {
		t.Fatalf("expected unregistered=true, got %#v", payload["unregistered"])
	}

	statusReq := httptest.NewRequest("GET", "/v1/integrations/workspaces/ws-delete/status", nil)
	statusW := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(statusW, statusReq)
	if statusW.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 after unregister, got %d", statusW.Result().StatusCode)
	}

	var action, reason string
	if err := database.GetDB().QueryRow(`SELECT action, reason FROM audit_events ORDER BY id DESC LIMIT 1`).Scan(&action, &reason); err != nil {
		t.Fatalf("query latest audit event: %v", err)
	}
	if action != "WORKSPACE_UNREGISTER" {
		t.Fatalf("expected audit action WORKSPACE_UNREGISTER, got %q", action)
	}
	if !strings.Contains(reason, "request_id=req_ws_delete_1") {
		t.Fatalf("expected request_id annotation in reason, got %q", reason)
	}
}

func TestIntegrationWorkspaceUnregisterRequiresAuth(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-delete-auth", "/tmp/ws-delete-auth")

	req := httptest.NewRequest("DELETE", "/v1/integrations/workspaces/ws-delete-auth", nil)
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for missing auth, got %d", w.Result().StatusCode)
	}
}

func TestIntegrationUnregisterIdempotencyReplayAndConflict(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-idem-unregister", "/tmp/ws-idem-unregister")

	key := "idem-integration-unregister-1"
	firstBody := `{"reason":"idempotent unregister test"}`

	req1 := httptest.NewRequest("DELETE", "/v1/integrations/workspaces/ws-idem-unregister", strings.NewReader(firstBody))
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	w1 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w1, req1)
	if w1.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first unregister status 200, got %d", w1.Result().StatusCode)
	}

	var payload1 map[string]interface{}
	if err := json.NewDecoder(w1.Body).Decode(&payload1); err != nil {
		t.Fatalf("decode first unregister response: %v", err)
	}

	req2 := httptest.NewRequest("DELETE", "/v1/integrations/workspaces/ws-idem-unregister", strings.NewReader(firstBody))
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	w2 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w2, req2)
	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected replay unregister status 200, got %d", w2.Result().StatusCode)
	}
	if replay := w2.Result().Header.Get("X-Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected X-Idempotent-Replay=true, got %q", replay)
	}

	var payload2 map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode replay unregister response: %v", err)
	}
	if !reflect.DeepEqual(payload1, payload2) {
		t.Fatalf("expected replay unregister payload to match first payload, first=%v replay=%v", payload1, payload2)
	}

	req3 := httptest.NewRequest("DELETE", "/v1/integrations/workspaces/ws-idem-unregister", strings.NewReader(`{"reason":"different unregister payload"}`))
	req3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	w3 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w3, req3)
	if w3.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected unregister idempotency conflict status 409, got %d", w3.Result().StatusCode)
	}

	var conflictPayload map[string]interface{}
	if err := json.NewDecoder(w3.Body).Decode(&conflictPayload); err != nil {
		t.Fatalf("decode unregister conflict response: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(conflictPayload["error"])), "idempotency key reused") {
		t.Fatalf("expected unregister idempotency conflict error, got %#v", conflictPayload["error"])
	}

	rec, err := database.GetControlPlaneReplay(key, "DELETE /v1/integrations/workspaces/ws-idem-unregister")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay(unregister): %v", err)
	}
	if rec.ReplayCount != 1 {
		t.Fatalf("expected unregister replay_count=1, got %d", rec.ReplayCount)
	}
}

func TestIntegrationWorkspaceActionsRestartContract(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-actions", "/tmp/ws-actions")
	restartArgs := []string{"/bin/sh", "-c", "sleep 10"}
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 10", restartArgs, "", 0)

	req := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-actions/actions", strings.NewReader(`{"action":"restart","reason":"restart via integration test"}`))
	req.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if stringValue(payload["action"]) != "restart" {
		t.Fatalf("expected action restart, got %#v", payload["action"])
	}
	if intValue(payload["audit_event_id"]) <= 0 {
		t.Fatalf("expected audit_event_id > 0, got %#v", payload["audit_event_id"])
	}

	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := state.GetState()
		if st.Status == "RUNNING" && st.PID > 0 {
			pid = st.PID
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("expected restarted pid > 0")
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
}

func TestIntegrationWorkspaceLatestIncident(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-inc", "/tmp/ws-inc")
	database.SetRunID("run-int-latest")
	if err := database.LogIncidentWithDecisionForIncident(
		"python3 demo/runaway.py",
		"gpt-4",
		"LOOP_DETECTED",
		92.0,
		"repeat loop",
		0.9,
		120,
		0.08,
		"agent-x",
		"1.0.0",
		"loop detected in integration test",
		93.0,
		10.0,
		95.0,
		"terminated",
		0,
		"incident-int-latest-1",
	); err != nil {
		t.Fatalf("log incident: %v", err)
	}

	req := httptest.NewRequest("GET", "/v1/integrations/workspaces/ws-inc/incidents/latest", nil)
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if stringValue(payload["incident_id"]) != "incident-int-latest-1" {
		t.Fatalf("expected incident_id incident-int-latest-1, got %#v", payload["incident_id"])
	}
	if stringValue(payload["exit_reason"]) != "LOOP_DETECTED" {
		t.Fatalf("expected exit_reason LOOP_DETECTED, got %#v", payload["exit_reason"])
	}
	if floatValue(payload["confidence_score"]) <= 0 {
		t.Fatalf("expected confidence_score > 0, got %#v", payload["confidence_score"])
	}
}

func TestIntegrationWorkspaceActionsRequireAuth(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-auth", "/tmp/ws-auth")
	req := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-auth/actions", strings.NewReader(`{"action":"kill","reason":"auth test"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w, req)
	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401 for missing auth, got %d", w.Result().StatusCode)
	}
}

func TestIntegrationRegisterIdempotencyReplayAndConflict(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	key := "idem-integration-register-1"
	firstBody := `{"workspace_id":"ws-idem-register","workspace_path":"/tmp/ws-idem-register","profile":"standard","client":"cursor"}`

	req1 := httptest.NewRequest("POST", "/v1/integrations/workspaces/register", strings.NewReader(firstBody))
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	w1 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceRegister(w1, req1)
	if w1.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first register status 200, got %d", w1.Result().StatusCode)
	}

	var payload1 map[string]interface{}
	if err := json.NewDecoder(w1.Body).Decode(&payload1); err != nil {
		t.Fatalf("decode first register response: %v", err)
	}

	req2 := httptest.NewRequest("POST", "/v1/integrations/workspaces/register", strings.NewReader(firstBody))
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	w2 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceRegister(w2, req2)

	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected replay register status 200, got %d", w2.Result().StatusCode)
	}
	if replay := w2.Result().Header.Get("X-Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected X-Idempotent-Replay=true, got %q", replay)
	}

	var payload2 map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode replay register response: %v", err)
	}
	if !reflect.DeepEqual(payload1, payload2) {
		t.Fatalf("expected replay register payload to match first payload, first=%v replay=%v", payload1, payload2)
	}

	req3 := httptest.NewRequest("POST", "/v1/integrations/workspaces/register", strings.NewReader(`{"workspace_id":"ws-idem-register-2","workspace_path":"/tmp/ws-idem-register-2","profile":"standard","client":"cursor"}`))
	req3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	w3 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceRegister(w3, req3)
	if w3.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected idempotency conflict status 409, got %d", w3.Result().StatusCode)
	}

	var conflictPayload map[string]interface{}
	if err := json.NewDecoder(w3.Body).Decode(&conflictPayload); err != nil {
		t.Fatalf("decode register conflict response: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(conflictPayload["error"])), "idempotency key reused") {
		t.Fatalf("expected idempotency conflict error, got %#v", conflictPayload["error"])
	}

	rec, err := database.GetControlPlaneReplay(key, "POST /v1/integrations/workspaces/register")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay(register): %v", err)
	}
	if rec.ReplayCount != 1 {
		t.Fatalf("expected register replay_count=1, got %d", rec.ReplayCount)
	}
}

func TestIntegrationProtectionIdempotencyReplayAndConflict(t *testing.T) {
	setupTempDBForAPI(t)
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-idem-protection", "/tmp/ws-idem-protection")
	key := "idem-integration-protection-1"
	firstBody := `{"enabled":false,"reason":"idempotent protection test"}`

	req1 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-protection/protection", strings.NewReader(firstBody))
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	w1 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w1, req1)
	if w1.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first protection status 200, got %d", w1.Result().StatusCode)
	}

	var payload1 map[string]interface{}
	if err := json.NewDecoder(w1.Body).Decode(&payload1); err != nil {
		t.Fatalf("decode first protection response: %v", err)
	}

	req2 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-protection/protection", strings.NewReader(firstBody))
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	w2 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w2, req2)
	if w2.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected replay protection status 200, got %d", w2.Result().StatusCode)
	}
	if replay := w2.Result().Header.Get("X-Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected X-Idempotent-Replay=true, got %q", replay)
	}

	var payload2 map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode replay protection response: %v", err)
	}
	if !reflect.DeepEqual(payload1, payload2) {
		t.Fatalf("expected replay protection payload to match first payload, first=%v replay=%v", payload1, payload2)
	}

	req3 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-protection/protection", strings.NewReader(`{"enabled":true,"reason":"flip protection"}`))
	req3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	w3 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w3, req3)
	if w3.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected protection idempotency conflict status 409, got %d", w3.Result().StatusCode)
	}

	var conflictPayload map[string]interface{}
	if err := json.NewDecoder(w3.Body).Decode(&conflictPayload); err != nil {
		t.Fatalf("decode protection conflict response: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(conflictPayload["error"])), "idempotency key reused") {
		t.Fatalf("expected protection idempotency conflict error, got %#v", conflictPayload["error"])
	}

	rec, err := database.GetControlPlaneReplay(key, "POST /v1/integrations/workspaces/ws-idem-protection/protection")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay(protection): %v", err)
	}
	if rec.ReplayCount != 1 {
		t.Fatalf("expected protection replay_count=1, got %d", rec.ReplayCount)
	}
}

func TestIntegrationActionsIdempotencyReplayAndConflict(t *testing.T) {
	setupTempDBForAPI(t)
	api.ResetWorkerControlForTests()
	os.Setenv("FLOWFORGE_API_KEY", "test-secret-key-12345")
	defer os.Unsetenv("FLOWFORGE_API_KEY")

	registerWorkspaceForTest(t, "ws-idem-actions", "/tmp/ws-idem-actions")
	state.UpdateState(0, "", "STOPPED", "/bin/sh -c sleep 5", []string{"/bin/sh", "-c", "sleep 5"}, "", 0)

	key := "idem-integration-actions-1"
	firstBody := `{"action":"kill","reason":"idempotent action test"}`

	req1 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-actions/actions", strings.NewReader(firstBody))
	req1.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", key)
	w1 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w1, req1)
	status1 := w1.Result().StatusCode
	if status1 >= 500 {
		t.Fatalf("expected non-5xx first actions status, got %d", status1)
	}

	var payload1 map[string]interface{}
	if err := json.NewDecoder(w1.Body).Decode(&payload1); err != nil {
		t.Fatalf("decode first actions response: %v", err)
	}

	req2 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-actions/actions", strings.NewReader(firstBody))
	req2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", key)
	w2 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w2, req2)
	if w2.Result().StatusCode != status1 {
		t.Fatalf("expected replay actions status %d, got %d", status1, w2.Result().StatusCode)
	}
	if replay := w2.Result().Header.Get("X-Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected X-Idempotent-Replay=true, got %q", replay)
	}

	var payload2 map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&payload2); err != nil {
		t.Fatalf("decode replay actions response: %v", err)
	}
	if !reflect.DeepEqual(payload1, payload2) {
		t.Fatalf("expected replay actions payload to match first payload, first=%v replay=%v", payload1, payload2)
	}

	req3 := httptest.NewRequest("POST", "/v1/integrations/workspaces/ws-idem-actions/actions", strings.NewReader(`{"action":"restart","reason":"different action payload"}`))
	req3.Header.Set("Authorization", "Bearer test-secret-key-12345")
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", key)
	w3 := httptest.NewRecorder()
	api.HandleIntegrationWorkspaceScoped(w3, req3)
	if w3.Result().StatusCode != http.StatusConflict {
		t.Fatalf("expected actions idempotency conflict status 409, got %d", w3.Result().StatusCode)
	}

	var conflictPayload map[string]interface{}
	if err := json.NewDecoder(w3.Body).Decode(&conflictPayload); err != nil {
		t.Fatalf("decode actions conflict response: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(conflictPayload["error"])), "idempotency key reused") {
		t.Fatalf("expected actions idempotency conflict error, got %#v", conflictPayload["error"])
	}

	rec, err := database.GetControlPlaneReplay(key, "POST /v1/integrations/workspaces/ws-idem-actions/actions")
	if err != nil {
		t.Fatalf("GetControlPlaneReplay(actions): %v", err)
	}
	if rec.ReplayCount != 1 {
		t.Fatalf("expected actions replay_count=1, got %d", rec.ReplayCount)
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
