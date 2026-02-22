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
	deadline := time.Now().Add(2 * time.Second)
	for {
		st := state.GetState()
		if st.Status == "STOPPED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected state STOPPED after process exit, got %q", st.Status)
		}
		time.Sleep(25 * time.Millisecond)
	}
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

	stopDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(stopDeadline) {
		st := state.GetState()
		if st.Status == "STOPPED" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	restartReq2 := httptest.NewRequest("POST", "/process/restart", nil)
	restartReq2.Header.Set("Authorization", "Bearer test-secret-key-12345")
	restartW2 := httptest.NewRecorder()
	api.HandleProcessRestart(restartW2, restartReq2)
	if restartW2.Result().StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected restart status 429 when budget is exhausted, got %d", restartW2.Result().StatusCode)
	}

	var errPayload map[string]interface{}
	if err := json.NewDecoder(restartW2.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode restart budget error payload: %v", err)
	}
	if !strings.Contains(strings.ToLower(stringValue(errPayload["error"])), "restart budget exceeded") {
		t.Fatalf("expected restart budget error message, got %#v", errPayload["error"])
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

	stopDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(stopDeadline) {
		st := state.GetState()
		if st.Status == "STOPPED" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	metricsReq := httptest.NewRequest("GET", "/metrics", nil)
	metricsW := httptest.NewRecorder()
	api.HandleMetrics(metricsW, metricsReq)
	if metricsW.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected metrics status 200, got %d", metricsW.Result().StatusCode)
	}
	body := metricsW.Body.String()

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
