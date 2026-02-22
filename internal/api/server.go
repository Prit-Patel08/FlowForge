package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flowforge/internal/clouddeps"
	"flowforge/internal/database"
	"flowforge/internal/metrics"
	"flowforge/internal/state"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

var (
	apiMetrics  = metrics.NewStore()
	apiLimiter  = newRateLimiter(120, 10, 10*time.Minute)
	allowedCORS = []string{
		"http://localhost",
		"http://localhost:3000",
		"http://localhost:3001",
	}
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func corsMiddleware(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))

	allowed := make(map[string]struct{}, len(allowedCORS)+1)
	for _, o := range allowedCORS {
		allowed[o] = struct{}{}
	}

	if envOrigin := strings.TrimSpace(os.Getenv("FLOWFORGE_ALLOWED_ORIGIN")); envOrigin != "" && isLocalOrigin(envOrigin) {
		allowed[envOrigin] = struct{}{}
	}

	if origin != "" {
		if _, ok := allowed[origin]; !ok && !isLocalOrigin(origin) {
			origin = ""
		}
	}
	if origin == "" {
		origin = "http://localhost:3000"
	}

	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")
}

func isLocalOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1"
}

func withSecurity(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		corsMiddleware(rec, r)

		if r.Method == "OPTIONS" {
			rec.WriteHeader(http.StatusOK)
			apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
			return
		}

		if !apiLimiter.allow(clientIP(r.RemoteAddr)) {
			http.Error(rec, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
			return
		}

		next(rec, r)
		apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
	}
}

// requireAuth checks the FLOWFORGE_API_KEY env var.
// If no key is set, mutating endpoints are blocked.
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	ip := clientIP(r.RemoteAddr)
	apiKey := os.Getenv("FLOWFORGE_API_KEY")

	if apiKey == "" {
		if r.Method == "POST" {
			http.Error(w, `{"error":"Security Alert: You must set FLOWFORGE_API_KEY environment variable to perform mutations."}`, http.StatusForbidden)
			return false
		}
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			http.Error(w, `{"error":"Too many failed auth attempts. Retry later."}`, http.StatusTooManyRequests)
			return false
		}
		http.Error(w, `{"error":"Authorization required"}`, http.StatusUnauthorized)
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			http.Error(w, `{"error":"Too many failed auth attempts. Retry later."}`, http.StatusTooManyRequests)
			return false
		}
		http.Error(w, `{"error":"Invalid API key"}`, http.StatusForbidden)
		return false
	}

	apiLimiter.clearAuthFailures(ip)
	return true
}

func StartServer(port string) {
	stop := Start(port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh

	fmt.Println("\n[API] Shutting down gracefully...")
	stop()
	fmt.Println("[API] Server stopped")
}

// Start launches the API server and returns a stop function for graceful shutdown.
func Start(port string) func() {
	apiKey := os.Getenv("FLOWFORGE_API_KEY")
	if apiKey != "" {
		fmt.Println("🔒 API Key authentication ENABLED for /process/* endpoints")
	} else {
		fmt.Println("⚠️  No FLOWFORGE_API_KEY set - mutating endpoints are blocked")
	}

	server := &http.Server{
		Addr:              resolveBindAddr(port),
		Handler:           NewHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		fmt.Printf("API listening on %s\n", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("ListenAndServe warning: %v", err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown failed: %v", err)
		}
	}
}

// NewHandler returns the full API router with legacy and v1-compatible routes.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	registerRoute(mux, "/stream", handleStream)
	registerRoute(mux, "/v1/stream", handleStream)

	registerRoute(mux, "/incidents", HandleIncidents)
	registerRoute(mux, "/v1/incidents", HandleIncidents)

	registerRoute(mux, "/process/kill", HandleProcessKill)
	registerRoute(mux, "/v1/process/kill", HandleProcessKill)

	registerRoute(mux, "/process/restart", HandleProcessRestart)
	registerRoute(mux, "/v1/process/restart", HandleProcessRestart)

	registerRoute(mux, "/healthz", HandleHealth)
	registerRoute(mux, "/v1/healthz", HandleHealth)

	registerRoute(mux, "/readyz", HandleReady)
	registerRoute(mux, "/v1/readyz", HandleReady)

	registerRoute(mux, "/metrics", HandleMetrics)
	registerRoute(mux, "/v1/metrics", HandleMetrics)

	registerRoute(mux, "/worker/lifecycle", HandleWorkerLifecycle)
	registerRoute(mux, "/v1/worker/lifecycle", HandleWorkerLifecycle)

	registerRoute(mux, "/timeline", HandleTimeline)
	registerRoute(mux, "/v1/timeline", HandleTimeline)

	registerRoute(mux, "/v1/ops/controlplane/replay/history", HandleControlPlaneReplayHistory)
	registerRoute(mux, "/v1/integrations/workspaces/register", HandleIntegrationWorkspaceRegister)
	registerRoute(mux, "/v1/integrations/workspaces/", HandleIntegrationWorkspaceScoped)
	return mux
}

func registerRoute(mux *http.ServeMux, path string, handler http.HandlerFunc) {
	mux.HandleFunc(path, withSecurity(handler))
}

func resolveBindAddr(port string) string {
	// Keep local-only binding unless explicitly asked for localhost alias.
	host := os.Getenv("FLOWFORGE_BIND_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if host != "127.0.0.1" && host != "localhost" {
		fmt.Printf("[API] Refusing non-local bind host %q. Falling back to 127.0.0.1.\n", host)
		host = "127.0.0.1"
	}
	return host + ":" + port
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			jsonData, err := state.JSON()
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}
	}
}

// HandleHealth returns process health for container liveness.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleReady checks DB readiness for startup probes.
func HandleReady(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ready := true
	checks := make(map[string]interface{}, 5)

	dbCheck := map[string]interface{}{
		"name":    "database",
		"healthy": true,
		"target":  "sqlite",
	}
	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			dbCheck["healthy"] = false
			dbCheck["error"] = err.Error()
			ready = false
		}
	}
	checks["database"] = dbCheck

	cloudCfg := clouddeps.LoadFromEnv()
	if cloudCfg.Required {
		cloudResults, cloudHealthy := clouddeps.Probe(cloudCfg)
		for _, res := range cloudResults {
			checks[res.Name] = res
		}
		if !cloudHealthy {
			ready = false
		}
	}

	payload := map[string]interface{}{
		"status":                      "ready",
		"cloud_dependencies_required": cloudCfg.Required,
		"checks":                      checks,
	}
	if !ready {
		payload["status"] = "not-ready"
		writeJSON(w, http.StatusServiceUnavailable, payload)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// HandleMetrics emits Prometheus-style metrics.
func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := state.GetState()
	active := st.Status != "STOPPED" && st.PID > 0
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprint(w, apiMetrics.Prometheus(active))
	_, _ = fmt.Fprint(w, controlPlaneReplayPrometheus())
}

// HandleControlPlaneReplayHistory exposes replay/conflict event trend for recent days.
func HandleControlPlaneReplayHistory(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	days := 7
	if rawDays := strings.TrimSpace(r.URL.Query().Get("days")); rawDays != "" {
		parsedDays, err := strconv.Atoi(rawDays)
		if err != nil || parsedDays < 1 || parsedDays > 90 {
			writeJSONError(w, http.StatusBadRequest, "days must be an integer between 1 and 90")
			return
		}
		days = parsedDays
	}

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Database not initialized")
			return
		}
	}

	stats, err := database.GetControlPlaneReplayStats()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load replay stats: %v", err))
		return
	}

	points, err := database.GetControlPlaneReplayDailyTrend(days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to load replay history: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"days":               days,
		"row_count":          stats.RowCount,
		"oldest_age_seconds": stats.OldestAgeSeconds,
		"newest_age_seconds": stats.NewestAgeSeconds,
		"points":             points,
	})
}

func controlPlaneReplayPrometheus() string {
	var b strings.Builder
	b.WriteString("# HELP flowforge_controlplane_replay_rows Current number of persisted control-plane replay rows.\n")
	b.WriteString("# TYPE flowforge_controlplane_replay_rows gauge\n")
	b.WriteString("# HELP flowforge_controlplane_replay_oldest_age_seconds Age in seconds of the oldest replay row by last_seen_at.\n")
	b.WriteString("# TYPE flowforge_controlplane_replay_oldest_age_seconds gauge\n")
	b.WriteString("# HELP flowforge_controlplane_replay_newest_age_seconds Age in seconds of the newest replay row by last_seen_at.\n")
	b.WriteString("# TYPE flowforge_controlplane_replay_newest_age_seconds gauge\n")
	b.WriteString("# HELP flowforge_controlplane_replay_stats_error Whether replay stats collection failed (1) or succeeded (0).\n")
	b.WriteString("# TYPE flowforge_controlplane_replay_stats_error gauge\n")

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			b.WriteString("flowforge_controlplane_replay_rows 0\n")
			b.WriteString("flowforge_controlplane_replay_oldest_age_seconds 0\n")
			b.WriteString("flowforge_controlplane_replay_newest_age_seconds 0\n")
			b.WriteString("flowforge_controlplane_replay_stats_error 1\n")
			return b.String()
		}
	}

	stats, err := database.GetControlPlaneReplayStats()
	if err != nil {
		b.WriteString("flowforge_controlplane_replay_rows 0\n")
		b.WriteString("flowforge_controlplane_replay_oldest_age_seconds 0\n")
		b.WriteString("flowforge_controlplane_replay_newest_age_seconds 0\n")
		b.WriteString("flowforge_controlplane_replay_stats_error 1\n")
		return b.String()
	}

	fmt.Fprintf(&b, "flowforge_controlplane_replay_rows %d\n", stats.RowCount)
	fmt.Fprintf(&b, "flowforge_controlplane_replay_oldest_age_seconds %d\n", stats.OldestAgeSeconds)
	fmt.Fprintf(&b, "flowforge_controlplane_replay_newest_age_seconds %d\n", stats.NewestAgeSeconds)
	b.WriteString("flowforge_controlplane_replay_stats_error 0\n")
	return b.String()
}

// HandleWorkerLifecycle exposes lifecycle control-plane state for operators/UI.
func HandleWorkerLifecycle(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap := WorkerLifecycleSnapshot()
	st := state.GetState()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"phase":      snap["phase"],
		"operation":  snap["operation"],
		"pid":        snap["pid"],
		"managed":    snap["managed"],
		"last_error": snap["last_err"],
		"status":     st.Status,
		"lifecycle":  st.Lifecycle,
		"command":    st.Command,
		"timestamp":  st.Timestamp,
	})
}

func HandleTimeline(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, "Database not initialized", http.StatusInternalServerError)
			return
		}
	}

	if incidentID := strings.TrimSpace(r.URL.Query().Get("incident_id")); incidentID != "" {
		events, err := database.GetIncidentTimelineByIncidentID(incidentID, 500)
		if err != nil {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(events); err != nil {
			http.Error(w, fmt.Sprintf("Encode error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	events, err := database.GetTimeline(100)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		http.Error(w, fmt.Sprintf("Encode error: %v", err), http.StatusInternalServerError)
	}
}

// HandleIncidents is exported for testing.
func HandleIncidents(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, "Database not initialized", http.StatusInternalServerError)
			return
		}
	}

	incidents, err := database.GetAllIncidents()
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(incidents); err != nil {
		http.Error(w, fmt.Sprintf("Encode error: %v", err), http.StatusInternalServerError)
	}
}

// HandleProcessKill is exported for testing.
func HandleProcessKill(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !requireAuth(w, r) {
		return
	}
	idemCtx, handled := beginIdempotentMutation(w, r, "POST /process/kill")
	if handled {
		return
	}
	reason := mutationReason(r)
	if reason == "" {
		reason = "manual API kill request"
	}

	workerControl.registerSpecFromStateIfMissing()
	decision, err := requestLifecycleKill()
	if err != nil {
		statusCode := lifecycleHTTPCode(err, http.StatusInternalServerError)
		msg := lifecycleErrorMessage(err, "failed to request kill")
		payload := map[string]interface{}{"error": msg}
		persistIdempotentMutation(idemCtx, statusCode, payload)
		writeJSON(w, statusCode, payload)
		return
	}

	stats := state.GetState()
	if decision.AcceptedNew {
		apiMetrics.IncProcessKill()
		incidentID := uuid.NewString()
		_ = database.LogAuditEventWithIncident(actorFromRequest(r), "KILL", reason, "api", decision.PID, stats.Command, incidentID)
	}
	payload := map[string]interface{}{
		"status":    decision.Status,
		"pid":       decision.PID,
		"lifecycle": decision.Lifecycle,
	}
	persistIdempotentMutation(idemCtx, http.StatusAccepted, payload)
	writeJSON(w, http.StatusAccepted, payload)
}

// HandleProcessRestart is exported for testing.
func HandleProcessRestart(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !requireAuth(w, r) {
		return
	}
	idemCtx, handled := beginIdempotentMutation(w, r, "POST /process/restart")
	if handled {
		return
	}
	reason := mutationReason(r)
	if reason == "" {
		reason = "manual API restart request"
	}

	workerControl.registerSpecFromStateIfMissing()
	decision, err := requestLifecycleRestart()
	if err != nil {
		statusCode := lifecycleHTTPCode(err, http.StatusInternalServerError)
		msg := lifecycleErrorMessage(err, "failed to request restart")
		if statusCode == http.StatusTooManyRequests {
			stats := state.GetState()
			incidentID := uuid.NewString()
			_ = database.LogAuditEventWithIncident(actorFromRequest(r), "RESTART_BLOCKED", msg, "api", stats.PID, stats.Command, incidentID)
		}
		if retryAfter := lifecycleRetryAfter(err); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			payload := map[string]interface{}{
				"error":               msg,
				"retry_after_seconds": retryAfter,
			}
			persistIdempotentMutation(idemCtx, statusCode, payload)
			writeJSON(w, statusCode, payload)
			return
		}
		payload := map[string]interface{}{"error": msg}
		persistIdempotentMutation(idemCtx, statusCode, payload)
		writeJSON(w, statusCode, payload)
		return
	}

	stats := state.GetState()
	if decision.AcceptedNew {
		apiMetrics.IncProcessRestart()
		incidentID := uuid.NewString()
		_ = database.LogAuditEventWithIncident(actorFromRequest(r), "RESTART", reason, "api", decision.PID, stats.Command, incidentID)
	}
	payload := map[string]interface{}{
		"status":    decision.Status,
		"pid":       decision.PID,
		"lifecycle": decision.Lifecycle,
		"command":   stats.Command,
	}
	persistIdempotentMutation(idemCtx, http.StatusAccepted, payload)
	writeJSON(w, http.StatusAccepted, payload)
}

func killProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	groupErr := syscall.Kill(-pid, syscall.SIGKILL)
	if groupErr == nil {
		return nil
	}

	pidErr := syscall.Kill(pid, syscall.SIGKILL)
	if pidErr == nil || errors.Is(pidErr, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf("group kill failed: %v; pid kill failed: %w", groupErr, pidErr)
}

func actorFromRequest(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		// Never persist any token material in audit logs.
		return "api-key"
	}
	return "anonymous"
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[API] encode response failed: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, statusCode int, msg string) {
	writeJSON(w, statusCode, map[string]string{"error": msg})
}

func mutationReason(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2048))
	if err != nil || len(body) == 0 {
		return ""
	}
	// Restore the body for handlers that might read again in the future.
	r.Body = io.NopCloser(bytes.NewReader(body))
	type reqBody struct {
		Reason string `json:"reason"`
	}
	var payload reqBody
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Reason)
}
