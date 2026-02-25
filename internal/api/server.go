package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"flowforge/internal/clouddeps"
	"flowforge/internal/database"
	"flowforge/internal/metrics"
	"flowforge/internal/policy"
	"flowforge/internal/state"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
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

type requestContextKey string

const requestIDContextKey requestContextKey = "flowforge_request_id"

const (
	requestIDHeader                               = "X-Request-Id"
	maxRequestIDLength                            = 128
	problemTypeBaseURI                            = "https://flowforge.dev/problems/"
	defaultCursorPageLimit                        = 100
	maxCursorPageLimit                            = 500
	defaultDecisionReplayHealthLimit              = 500
	maxDecisionReplayHealthLimit                  = 5000
	defaultDecisionSignalBaselineLimit            = 500
	maxDecisionSignalBaselineLimit                = 5000
	decisionSignalBaselineContractVersion         = "decision-signal-baseline.v2"
	defaultDecisionSignalCPUDeltaThreshold        = 25.0
	defaultDecisionSignalEntropyDeltaThreshold    = 20.0
	defaultDecisionSignalConfidenceDeltaThreshold = 20.0
	defaultDecisionSignalBaselineMinSamples       = 3
	defaultDecisionSignalBaselineRequiredStreak   = 2
	maxDecisionSignalBaselineMinSamples           = 100
	maxDecisionSignalBaselineRequiredStreak       = 10
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

type paginatedIncidentsResponse struct {
	Items      []database.Incident `json:"items"`
	NextCursor string              `json:"next_cursor,omitempty"`
	HasMore    bool                `json:"has_more"`
	Limit      int                 `json:"limit"`
}

type paginatedTimelineResponse struct {
	Items      []database.TimelineEvent `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
	HasMore    bool                     `json:"has_more"`
	Limit      int                      `json:"limit"`
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
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
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
		r = withRequestID(r)
		if rid := requestIDFromRequest(r); rid != "" {
			rec.Header().Set(requestIDHeader, rid)
		}

		if r.Method == "OPTIONS" {
			rec.WriteHeader(http.StatusOK)
			apiMetrics.IncRequest(r.URL.Path, r.Method, rec.status)
			return
		}

		if !apiLimiter.allow(clientIP(r.RemoteAddr)) {
			writeJSONErrorForRequest(rec, r, http.StatusTooManyRequests, "rate limit exceeded")
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
		if isUnsafeMethod(r.Method) {
			writeJSONErrorForRequest(w, r, http.StatusForbidden, "Security Alert: You must set FLOWFORGE_API_KEY environment variable to perform mutations.")
			return false
		}
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			writeJSONErrorForRequest(w, r, http.StatusTooManyRequests, "Too many failed auth attempts. Retry later.")
			return false
		}
		writeJSONErrorForRequest(w, r, http.StatusUnauthorized, "Authorization required")
		return false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
		apiMetrics.IncAuthFailure()
		if apiLimiter.addAuthFailure(ip) {
			writeJSONErrorForRequest(w, r, http.StatusTooManyRequests, "Too many failed auth attempts. Retry later.")
			return false
		}
		writeJSONErrorForRequest(w, r, http.StatusForbidden, "Invalid API key")
		return false
	}

	apiLimiter.clearAuthFailures(ip)
	return true
}

func isUnsafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func withRequestID(r *http.Request) *http.Request {
	if r == nil {
		return r
	}
	if existing := requestIDFromRequest(r); existing != "" {
		ctx := context.WithValue(r.Context(), requestIDContextKey, existing)
		return r.WithContext(ctx)
	}
	rid := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if !isValidRequestID(rid) {
		rid = ""
	}
	if rid == "" {
		rid = "req_" + uuid.NewString()
	}
	ctx := context.WithValue(r.Context(), requestIDContextKey, rid)
	return r.WithContext(ctx)
}

func ensureRequestContext(w http.ResponseWriter, r *http.Request) *http.Request {
	r = withRequestID(r)
	if w != nil {
		if rid := requestIDFromRequest(r); rid != "" {
			w.Header().Set(requestIDHeader, rid)
		}
	}
	return r
}

func isValidRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, ch := range id {
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
}

func requestIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if rid, ok := r.Context().Value(requestIDContextKey).(string); ok && isValidRequestID(strings.TrimSpace(rid)) {
		return strings.TrimSpace(rid)
	}
	rid := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if isValidRequestID(rid) {
		return rid
	}
	return ""
}

func annotateReasonWithRequestID(reason string, r *http.Request) string {
	rid := requestIDFromRequest(r)
	if rid == "" {
		return reason
	}
	trimmed := strings.TrimSpace(reason)
	if strings.Contains(trimmed, "request_id=") {
		return trimmed
	}
	if trimmed == "" {
		return fmt.Sprintf("request_id=%s", rid)
	}
	return fmt.Sprintf("%s [request_id=%s]", trimmed, rid)
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
	registerRoute(mux, "/v1/ops/requests", HandleRequestTrace)
	registerRoute(mux, "/v1/ops/requests/", HandleRequestTrace)
	registerRoute(mux, "/v1/ops/decisions/replay/health", HandleDecisionReplayHealth)
	registerRoute(mux, "/v1/ops/decisions/signals/baseline", HandleDecisionSignalBaseline)
	registerRoute(mux, "/v1/ops/decisions/replay", HandleDecisionReplay)
	registerRoute(mux, "/v1/ops/decisions/replay/", HandleDecisionReplay)
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
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, "Streaming unsupported")
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
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleReady checks DB readiness for startup probes.
func HandleReady(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
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
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	st := state.GetState()
	active := st.Status != "STOPPED" && st.PID > 0
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprint(w, apiMetrics.Prometheus(active))
	_, _ = fmt.Fprint(w, controlPlaneReplayPrometheus())
	_, _ = fmt.Fprint(w, decisionReplayPrometheus())
	_, _ = fmt.Fprint(w, decisionSignalBaselinePrometheus())
}

// HandleControlPlaneReplayHistory exposes replay/conflict event trend for recent days.
func HandleControlPlaneReplayHistory(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	days := 7
	if rawDays := strings.TrimSpace(r.URL.Query().Get("days")); rawDays != "" {
		parsedDays, err := strconv.Atoi(rawDays)
		if err != nil || parsedDays < 1 || parsedDays > 90 {
			writeJSONErrorForRequest(w, r, http.StatusBadRequest, "days must be an integer between 1 and 90")
			return
		}
		days = parsedDays
	}

	if database.GetDB() == nil {
		if err := database.InitDB(); err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, "Database not initialized")
			return
		}
	}

	stats, err := database.GetControlPlaneReplayStats()
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to load replay stats: %v", err))
		return
	}

	points, err := database.GetControlPlaneReplayDailyTrend(days)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to load replay history: %v", err))
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

// HandleRequestTrace returns all unified events correlated to a single request ID.
func HandleRequestTrace(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	requestID, err := parseRequestTraceID(r.URL.Path)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}

	limit := 200
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 || parsedLimit > 1000 {
			writeJSONErrorForRequest(w, r, http.StatusBadRequest, "limit must be an integer between 1 and 1000")
			return
		}
		limit = parsedLimit
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	events, err := database.GetUnifiedEventsByRequestID(requestID, limit)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to load request trace: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"request_id": requestID,
		"count":      len(events),
		"events":     events,
	})
}

// HandleDecisionReplay verifies deterministic replay digest integrity for a recorded decision trace.
func HandleDecisionReplay(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	traceID, err := parseDecisionReplayTraceID(r.URL.Path)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	trace, err := database.GetDecisionTraceByID(traceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONErrorForRequest(w, r, http.StatusNotFound, "decision trace not found")
			return
		}
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to load decision trace: %v", err))
		return
	}

	verification := policy.VerifyDecisionReplay(trace.ReplayDigest, policy.DecisionReplayInput{
		DecisionEngine:   trace.DecisionEngine,
		EngineVersion:    trace.DecisionEngineVersion,
		DecisionContract: trace.DecisionContract,
		RolloutMode:      trace.PolicyRolloutMode,
		Decision:         trace.Decision,
		Reason:           trace.Reason,
		CPUScore:         trace.CPUScore,
		EntropyScore:     trace.EntropyScore,
		ConfidenceScore:  trace.ConfidenceScore,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"trace_id":                      trace.ID,
		"timestamp":                     trace.Timestamp,
		"command":                       trace.Command,
		"pid":                           trace.PID,
		"decision":                      trace.Decision,
		"reason":                        trace.Reason,
		"cpu_score":                     trace.CPUScore,
		"entropy_score":                 trace.EntropyScore,
		"confidence_score":              trace.ConfidenceScore,
		"decision_engine":               trace.DecisionEngine,
		"engine_version":                trace.DecisionEngineVersion,
		"decision_contract_version":     trace.DecisionContract,
		"rollout_mode":                  trace.PolicyRolloutMode,
		"trace_replay_contract_version": strings.TrimSpace(trace.ReplayContract),
		"trace_replay_digest":           strings.TrimSpace(trace.ReplayDigest),
		"replay_contract_version":       verification.ContractVersion,
		"replay_status":                 verification.Status,
		"replayable":                    verification.Replayable,
		"deterministic_match":           verification.DeterministicMatch,
		"legacy_fallback":               verification.LegacyFallback,
		"replay_reason":                 verification.Reason,
		"stored_replay_digest":          verification.StoredDigest,
		"computed_replay_digest":        verification.ComputedDigest,
		"canonical_input":               verification.CanonicalInput,
	})
}

type decisionReplayHealthSummary struct {
	ContractVersion       string  `json:"contract_version"`
	Limit                 int     `json:"limit"`
	Scanned               int     `json:"scanned"`
	Healthy               bool    `json:"healthy"`
	MatchCount            int     `json:"match_count"`
	MismatchCount         int     `json:"mismatch_count"`
	MissingDigestCount    int     `json:"missing_digest_count"`
	LegacyFallbackCount   int     `json:"legacy_fallback_count"`
	UnreplayableCount     int     `json:"unreplayable_count"`
	MismatchRatio         float64 `json:"mismatch_ratio"`
	CheckedAt             string  `json:"checked_at"`
	MismatchTraceIDs      []int   `json:"mismatch_trace_ids,omitempty"`
	MissingDigestTraceIDs []int   `json:"missing_digest_trace_ids,omitempty"`
}

type decisionSignalBaselineFilter struct {
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	RolloutMode   string `json:"rollout_mode"`
}

func (f decisionSignalBaselineFilter) matches(trace database.DecisionTrace) bool {
	engine := strings.ToLower(strings.TrimSpace(trace.DecisionEngine))
	version := strings.ToLower(strings.TrimSpace(trace.DecisionEngineVersion))
	rollout := strings.ToLower(strings.TrimSpace(trace.PolicyRolloutMode))
	if f.Engine != "" && engine != f.Engine {
		return false
	}
	if f.EngineVersion != "" && version != f.EngineVersion {
		return false
	}
	if f.RolloutMode != "" && rollout != f.RolloutMode {
		return false
	}
	return true
}

type decisionSignalBaselineThresholds struct {
	CPUDelta        float64 `json:"cpu_delta"`
	EntropyDelta    float64 `json:"entropy_delta"`
	ConfidenceDelta float64 `json:"confidence_delta"`
}

type decisionSignalBaselineGuardrails struct {
	MinBaselineSamples int `json:"min_baseline_samples"`
	RequiredStreak     int `json:"required_consecutive_breaches"`
}

type decisionSignalBaselineBucket struct {
	BucketKey              string  `json:"bucket_key"`
	DecisionEngine         string  `json:"decision_engine"`
	EngineVersion          string  `json:"engine_version"`
	RolloutMode            string  `json:"rollout_mode"`
	SampleCount            int     `json:"sample_count"`
	BaselineSampleCount    int     `json:"baseline_sample_count"`
	LatestTraceID          int     `json:"latest_trace_id"`
	LatestTimestamp        string  `json:"latest_timestamp"`
	LatestCPUScore         float64 `json:"latest_cpu_score"`
	LatestEntropyScore     float64 `json:"latest_entropy_score"`
	LatestConfidenceScore  float64 `json:"latest_confidence_score"`
	BaselineCPUMean        float64 `json:"baseline_cpu_mean"`
	BaselineEntropyMean    float64 `json:"baseline_entropy_mean"`
	BaselineConfidenceMean float64 `json:"baseline_confidence_mean"`
	CPUDelta               float64 `json:"cpu_delta"`
	EntropyDelta           float64 `json:"entropy_delta"`
	ConfidenceDelta        float64 `json:"confidence_delta"`
	CPUDrift               bool    `json:"cpu_drift"`
	EntropyDrift           bool    `json:"entropy_drift"`
	ConfidenceDrift        bool    `json:"confidence_drift"`
	BreachSignalCount      int     `json:"breach_signal_count"`
	ConsecutiveBreachCount int     `json:"consecutive_breach_count"`
	PendingEscalation      bool    `json:"pending_escalation"`
	InsufficientHistory    bool    `json:"insufficient_history"`
	Status                 string  `json:"status"`
	StateTransition        string  `json:"state_transition,omitempty"`
	Healthy                bool    `json:"healthy"`
}

type decisionSignalBaselineSummary struct {
	ContractVersion        string                           `json:"contract_version"`
	Limit                  int                              `json:"limit"`
	Scanned                int                              `json:"scanned"`
	BucketCount            int                              `json:"bucket_count"`
	AtRiskBucketCount      int                              `json:"at_risk_bucket_count"`
	PendingBucketCount     int                              `json:"pending_bucket_count"`
	InsufficientCount      int                              `json:"insufficient_history_bucket_count"`
	TransitionCount        int                              `json:"transition_count"`
	MaxCPUDeltaAbs         float64                          `json:"max_cpu_delta_abs"`
	MaxEntropyDeltaAbs     float64                          `json:"max_entropy_delta_abs"`
	MaxConfidenceDeltaAbs  float64                          `json:"max_confidence_delta_abs"`
	Healthy                bool                             `json:"healthy"`
	CheckedAt              string                           `json:"checked_at"`
	Filter                 decisionSignalBaselineFilter     `json:"filter"`
	Thresholds             decisionSignalBaselineThresholds `json:"thresholds"`
	Guardrails             decisionSignalBaselineGuardrails `json:"guardrails"`
	Buckets                []decisionSignalBaselineBucket   `json:"buckets"`
	AtRiskBucketKeys       []string                         `json:"at_risk_bucket_keys,omitempty"`
	PendingBucketKeys      []string                         `json:"pending_bucket_keys,omitempty"`
	InsufficientBucketKeys []string                         `json:"insufficient_history_bucket_keys,omitempty"`
}

type decisionSignalBaselineBuildOptions struct {
	PersistState         bool
	EmitAuditTransitions bool
	RequestID            string
}

const (
	signalBaselineStatusHealthy             = "healthy"
	signalBaselineStatusPending             = "pending"
	signalBaselineStatusAtRisk              = "at_risk"
	signalBaselineStatusInsufficientHistory = "insufficient_history"
)

// HandleDecisionReplayHealth returns replay integrity summary over recent decision traces.
func HandleDecisionReplayHealth(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limit, err := parseDecisionReplayHealthLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	summary, err := buildDecisionReplayHealthSummary(limit)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to compute decision replay health: %v", err))
		return
	}

	if parseBoolQueryValue(r.URL.Query().Get("strict")) && !summary.Healthy {
		payload := problemPayload(
			r,
			http.StatusConflict,
			"decision replay strict health check failed",
			map[string]interface{}{"replay_health": summary},
		)
		writeProblem(w, http.StatusConflict, payload)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// HandleDecisionSignalBaseline returns grouped signal baselines over recent decision traces.
func HandleDecisionSignalBaseline(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	limit, err := parseDecisionSignalBaselineLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}
	filter := decisionSignalBaselineFilter{
		Engine:        strings.ToLower(strings.TrimSpace(r.URL.Query().Get("engine"))),
		EngineVersion: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("engine_version"))),
		RolloutMode:   strings.ToLower(strings.TrimSpace(r.URL.Query().Get("rollout_mode"))),
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	thresholds := decisionSignalBaselineThresholdsFromEnv()
	guardrails := decisionSignalBaselineGuardrailsFromEnv()
	summary, err := buildDecisionSignalBaselineSummary(
		limit,
		filter,
		thresholds,
		guardrails,
		decisionSignalBaselineBuildOptions{
			PersistState:         true,
			EmitAuditTransitions: true,
			RequestID:            requestIDFromRequest(r),
		},
	)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to compute decision signal baseline: %v", err))
		return
	}

	if parseBoolQueryValue(r.URL.Query().Get("strict")) && !summary.Healthy {
		payload := problemPayload(
			r,
			http.StatusConflict,
			"decision signal baseline strict health check failed",
			map[string]interface{}{"signal_baseline": summary},
		)
		writeProblem(w, http.StatusConflict, payload)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

func parseRequestTraceID(path string) (string, error) {
	const basePath = "/v1/ops/requests"
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == basePath || trimmedPath == basePath+"/" {
		return "", fmt.Errorf("request_id is required in path /v1/ops/requests/{request_id}")
	}
	if !strings.HasPrefix(trimmedPath, basePath+"/") {
		return "", fmt.Errorf("request trace endpoint not found")
	}
	rawID := strings.TrimPrefix(trimmedPath, basePath+"/")
	if strings.Contains(rawID, "/") {
		return "", fmt.Errorf("request_id must be a single path segment")
	}
	decodedID, err := url.PathUnescape(strings.TrimSpace(rawID))
	if err != nil {
		return "", fmt.Errorf("request_id is invalid: %v", err)
	}
	decodedID = strings.TrimSpace(decodedID)
	if !isValidRequestID(decodedID) {
		return "", fmt.Errorf("request_id must contain only visible ASCII and be <= %d chars", maxRequestIDLength)
	}
	return decodedID, nil
}

func parseDecisionReplayTraceID(path string) (int, error) {
	const basePath = "/v1/ops/decisions/replay"
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == basePath || trimmedPath == basePath+"/" {
		return 0, fmt.Errorf("trace_id is required in path /v1/ops/decisions/replay/{trace_id}")
	}
	if !strings.HasPrefix(trimmedPath, basePath+"/") {
		return 0, fmt.Errorf("decision replay endpoint not found")
	}
	rawID := strings.TrimPrefix(trimmedPath, basePath+"/")
	if strings.Contains(rawID, "/") {
		return 0, fmt.Errorf("trace_id must be a single path segment")
	}
	decodedID, err := url.PathUnescape(strings.TrimSpace(rawID))
	if err != nil {
		return 0, fmt.Errorf("trace_id is invalid: %v", err)
	}
	parsedID, err := strconv.Atoi(decodedID)
	if err != nil || parsedID <= 0 {
		return 0, fmt.Errorf("trace_id must be a positive integer")
	}
	return parsedID, nil
}

func parseDecisionReplayHealthLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultDecisionReplayHealthLimit, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > maxDecisionReplayHealthLimit {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", maxDecisionReplayHealthLimit)
	}
	return parsed, nil
}

func parseDecisionSignalBaselineLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultDecisionSignalBaselineLimit, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 || parsed > maxDecisionSignalBaselineLimit {
		return 0, fmt.Errorf("limit must be an integer between 1 and %d", maxDecisionSignalBaselineLimit)
	}
	return parsed, nil
}

func parseCursorPageQuery(rawLimit, rawCursor string, defaultLimit, maxLimit int) (int, int64, error) {
	limit := defaultLimit
	rawLimit = strings.TrimSpace(rawLimit)
	if rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > maxLimit {
			return 0, 0, fmt.Errorf("limit must be an integer between 1 and %d", maxLimit)
		}
		limit = parsed
	}

	cursor := int64(0)
	rawCursor = strings.TrimSpace(rawCursor)
	if rawCursor != "" {
		parsed, err := strconv.ParseInt(rawCursor, 10, 64)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("cursor must be a positive integer")
		}
		cursor = parsed
	}
	return limit, cursor, nil
}

func parseBoolQueryValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildDecisionReplayHealthSummary(limit int) (decisionReplayHealthSummary, error) {
	if limit <= 0 {
		limit = defaultDecisionReplayHealthLimit
	}
	if limit > maxDecisionReplayHealthLimit {
		limit = maxDecisionReplayHealthLimit
	}

	traces, err := database.GetDecisionTraces(limit)
	if err != nil {
		return decisionReplayHealthSummary{}, err
	}

	summary := decisionReplayHealthSummary{
		ContractVersion: policy.DecisionReplayContractVersion,
		Limit:           limit,
		Scanned:         len(traces),
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	for _, trace := range traces {
		verification := policy.VerifyDecisionReplay(trace.ReplayDigest, policy.DecisionReplayInput{
			DecisionEngine:   trace.DecisionEngine,
			EngineVersion:    trace.DecisionEngineVersion,
			DecisionContract: trace.DecisionContract,
			RolloutMode:      trace.PolicyRolloutMode,
			Decision:         trace.Decision,
			Reason:           trace.Reason,
			CPUScore:         trace.CPUScore,
			EntropyScore:     trace.EntropyScore,
			ConfidenceScore:  trace.ConfidenceScore,
		})

		switch verification.Status {
		case policy.ReplayStatusMatch:
			summary.MatchCount++
		case policy.ReplayStatusMismatch:
			summary.MismatchCount++
			if len(summary.MismatchTraceIDs) < 20 {
				summary.MismatchTraceIDs = append(summary.MismatchTraceIDs, trace.ID)
			}
		case policy.ReplayStatusMissing:
			summary.MissingDigestCount++
			if len(summary.MissingDigestTraceIDs) < 20 {
				summary.MissingDigestTraceIDs = append(summary.MissingDigestTraceIDs, trace.ID)
			}
		case policy.ReplayStatusLegacy:
			summary.LegacyFallbackCount++
		default:
			summary.UnreplayableCount++
		}
	}

	if summary.Scanned > 0 {
		summary.MismatchRatio = float64(summary.MismatchCount) / float64(summary.Scanned)
	}
	summary.Healthy = summary.MismatchCount == 0 && summary.MissingDigestCount == 0 && summary.UnreplayableCount == 0

	return summary, nil
}

func decisionSignalBaselineThresholdsFromEnv() decisionSignalBaselineThresholds {
	return decisionSignalBaselineThresholds{
		CPUDelta:        positiveFloatFromEnv("FLOWFORGE_DECISION_SIGNAL_CPU_DELTA_THRESHOLD", defaultDecisionSignalCPUDeltaThreshold),
		EntropyDelta:    positiveFloatFromEnv("FLOWFORGE_DECISION_SIGNAL_ENTROPY_DELTA_THRESHOLD", defaultDecisionSignalEntropyDeltaThreshold),
		ConfidenceDelta: positiveFloatFromEnv("FLOWFORGE_DECISION_SIGNAL_CONFIDENCE_DELTA_THRESHOLD", defaultDecisionSignalConfidenceDeltaThreshold),
	}
}

func decisionSignalBaselineGuardrailsFromEnv() decisionSignalBaselineGuardrails {
	return decisionSignalBaselineGuardrails{
		MinBaselineSamples: positiveIntFromEnv(
			"FLOWFORGE_DECISION_SIGNAL_BASELINE_MIN_SAMPLES",
			defaultDecisionSignalBaselineMinSamples,
			1,
			maxDecisionSignalBaselineMinSamples,
		),
		RequiredStreak: positiveIntFromEnv(
			"FLOWFORGE_DECISION_SIGNAL_BASELINE_REQUIRED_CONSECUTIVE",
			defaultDecisionSignalBaselineRequiredStreak,
			1,
			maxDecisionSignalBaselineRequiredStreak,
		),
	}
}

func positiveFloatFromEnv(name string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func positiveIntFromEnv(name string, fallback, minValue, maxValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func normalizeSignalBucketDimension(v string, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func decisionSignalBucketKey(trace database.DecisionTrace) string {
	engine := normalizeSignalBucketDimension(trace.DecisionEngine, "unknown-engine")
	version := normalizeSignalBucketDimension(trace.DecisionEngineVersion, "unknown-version")
	rollout := normalizeSignalBucketDimension(trace.PolicyRolloutMode, "unknown-rollout")
	return fmt.Sprintf("%s@%s|%s", engine, version, rollout)
}

func meanSignalScores(traces []database.DecisionTrace) (cpu, entropy, confidence float64) {
	if len(traces) == 0 {
		return 0, 0, 0
	}
	for _, trace := range traces {
		cpu += trace.CPUScore
		entropy += trace.EntropyScore
		confidence += trace.ConfidenceScore
	}
	n := float64(len(traces))
	return cpu / n, entropy / n, confidence / n
}

func buildDecisionSignalBaselineSummary(
	limit int,
	filter decisionSignalBaselineFilter,
	thresholds decisionSignalBaselineThresholds,
	guardrails decisionSignalBaselineGuardrails,
	options decisionSignalBaselineBuildOptions,
) (decisionSignalBaselineSummary, error) {
	if limit <= 0 {
		limit = defaultDecisionSignalBaselineLimit
	}
	if limit > maxDecisionSignalBaselineLimit {
		limit = maxDecisionSignalBaselineLimit
	}
	if guardrails.MinBaselineSamples <= 0 {
		guardrails.MinBaselineSamples = defaultDecisionSignalBaselineMinSamples
	}
	if guardrails.MinBaselineSamples > maxDecisionSignalBaselineMinSamples {
		guardrails.MinBaselineSamples = maxDecisionSignalBaselineMinSamples
	}
	if guardrails.RequiredStreak <= 0 {
		guardrails.RequiredStreak = defaultDecisionSignalBaselineRequiredStreak
	}
	if guardrails.RequiredStreak > maxDecisionSignalBaselineRequiredStreak {
		guardrails.RequiredStreak = maxDecisionSignalBaselineRequiredStreak
	}
	options.RequestID = strings.TrimSpace(options.RequestID)

	traces, err := database.GetDecisionTraces(limit)
	if err != nil {
		return decisionSignalBaselineSummary{}, err
	}

	filtered := make([]database.DecisionTrace, 0, len(traces))
	for _, trace := range traces {
		if filter.matches(trace) {
			filtered = append(filtered, trace)
		}
	}

	summary := decisionSignalBaselineSummary{
		ContractVersion: decisionSignalBaselineContractVersion,
		Limit:           limit,
		Scanned:         len(filtered),
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
		Filter:          filter,
		Thresholds:      thresholds,
		Guardrails:      guardrails,
	}
	if len(filtered) == 0 {
		summary.Healthy = true
		return summary, nil
	}

	bucketMap := make(map[string][]database.DecisionTrace)
	for _, trace := range filtered {
		key := decisionSignalBucketKey(trace)
		bucketMap[key] = append(bucketMap[key], trace)
	}

	buckets := make([]decisionSignalBaselineBucket, 0, len(bucketMap))
	for key, bucketTraces := range bucketMap {
		if len(bucketTraces) == 0 {
			continue
		}
		latest := bucketTraces[0]
		baselineTraces := bucketTraces
		if len(bucketTraces) > 1 {
			baselineTraces = bucketTraces[1:]
		}
		baselineCPUMean, baselineEntropyMean, baselineConfidenceMean := meanSignalScores(baselineTraces)
		cpuDelta := latest.CPUScore - baselineCPUMean
		entropyDelta := latest.EntropyScore - baselineEntropyMean
		confidenceDelta := latest.ConfidenceScore - baselineConfidenceMean
		cpuDrift := math.Abs(cpuDelta) >= thresholds.CPUDelta
		entropyDrift := math.Abs(entropyDelta) >= thresholds.EntropyDelta
		confidenceDrift := math.Abs(confidenceDelta) >= thresholds.ConfidenceDelta
		breachSignalCount := 0
		if cpuDrift {
			breachSignalCount++
		}
		if entropyDrift {
			breachSignalCount++
		}
		if confidenceDrift {
			breachSignalCount++
		}
		insufficientHistory := len(baselineTraces) < guardrails.MinBaselineSamples
		engine := normalizeSignalBucketDimension(latest.DecisionEngine, "unknown-engine")
		version := normalizeSignalBucketDimension(latest.DecisionEngineVersion, "unknown-version")
		rollout := normalizeSignalBucketDimension(latest.PolicyRolloutMode, "unknown-rollout")

		previous := database.DecisionSignalBaselineState{
			BucketKey: key,
			Status:    signalBaselineStatusHealthy,
		}
		hasPrevious := false
		loadedState, err := database.GetDecisionSignalBaselineState(key)
		if err == nil {
			hasPrevious = true
			previous = loadedState
		} else if !errors.Is(err, sql.ErrNoRows) {
			return decisionSignalBaselineSummary{}, err
		}
		previous.Status = normalizeSignalBaselineStatus(previous.Status)
		if previous.ConsecutiveBreach < 0 {
			previous.ConsecutiveBreach = 0
		}
		if previous.LatestTraceID < 0 {
			previous.LatestTraceID = 0
		}
		latestIsNew := !hasPrevious || latest.ID != previous.LatestTraceID
		consecutiveBreachCount := previous.ConsecutiveBreach
		status := signalBaselineStatusHealthy
		pendingEscalation := false

		switch {
		case insufficientHistory:
			consecutiveBreachCount = 0
			status = signalBaselineStatusInsufficientHistory
		case breachSignalCount == 0:
			consecutiveBreachCount = 0
			status = signalBaselineStatusHealthy
		default:
			if latestIsNew {
				if previous.Status == signalBaselineStatusPending || previous.Status == signalBaselineStatusAtRisk {
					consecutiveBreachCount++
				} else {
					consecutiveBreachCount = 1
				}
			}
			if consecutiveBreachCount <= 0 {
				consecutiveBreachCount = 1
			}
			if consecutiveBreachCount >= guardrails.RequiredStreak {
				status = signalBaselineStatusAtRisk
			} else {
				status = signalBaselineStatusPending
				pendingEscalation = true
			}
		}

		stateTransition := ""
		if hasPrevious && previous.Status != status {
			stateTransition = fmt.Sprintf("%s->%s", previous.Status, status)
			summary.TransitionCount++
		}

		healthy := status != signalBaselineStatusAtRisk
		switch status {
		case signalBaselineStatusAtRisk:
			summary.AtRiskBucketKeys = append(summary.AtRiskBucketKeys, key)
		case signalBaselineStatusPending:
			summary.PendingBucketKeys = append(summary.PendingBucketKeys, key)
		case signalBaselineStatusInsufficientHistory:
			summary.InsufficientBucketKeys = append(summary.InsufficientBucketKeys, key)
		}
		if abs := math.Abs(cpuDelta); abs > summary.MaxCPUDeltaAbs {
			summary.MaxCPUDeltaAbs = abs
		}
		if abs := math.Abs(entropyDelta); abs > summary.MaxEntropyDeltaAbs {
			summary.MaxEntropyDeltaAbs = abs
		}
		if abs := math.Abs(confidenceDelta); abs > summary.MaxConfidenceDeltaAbs {
			summary.MaxConfidenceDeltaAbs = abs
		}

		buckets = append(buckets, decisionSignalBaselineBucket{
			BucketKey:              key,
			DecisionEngine:         engine,
			EngineVersion:          version,
			RolloutMode:            rollout,
			SampleCount:            len(bucketTraces),
			BaselineSampleCount:    len(baselineTraces),
			LatestTraceID:          latest.ID,
			LatestTimestamp:        latest.Timestamp,
			LatestCPUScore:         latest.CPUScore,
			LatestEntropyScore:     latest.EntropyScore,
			LatestConfidenceScore:  latest.ConfidenceScore,
			BaselineCPUMean:        baselineCPUMean,
			BaselineEntropyMean:    baselineEntropyMean,
			BaselineConfidenceMean: baselineConfidenceMean,
			CPUDelta:               cpuDelta,
			EntropyDelta:           entropyDelta,
			ConfidenceDelta:        confidenceDelta,
			CPUDrift:               cpuDrift,
			EntropyDrift:           entropyDrift,
			ConfidenceDrift:        confidenceDrift,
			BreachSignalCount:      breachSignalCount,
			ConsecutiveBreachCount: consecutiveBreachCount,
			PendingEscalation:      pendingEscalation,
			InsufficientHistory:    insufficientHistory,
			Status:                 status,
			StateTransition:        stateTransition,
			Healthy:                healthy,
		})

		if options.PersistState {
			shouldPersist := !hasPrevious ||
				previous.LatestTraceID != latest.ID ||
				previous.ConsecutiveBreach != consecutiveBreachCount ||
				previous.Status != status
			if shouldPersist {
				if err := database.UpsertDecisionSignalBaselineState(database.DecisionSignalBaselineState{
					BucketKey:         key,
					LatestTraceID:     latest.ID,
					ConsecutiveBreach: consecutiveBreachCount,
					Status:            status,
				}); err != nil {
					return decisionSignalBaselineSummary{}, err
				}
			}
			if options.EmitAuditTransitions && hasPrevious && previous.Status != status {
				if err := emitSignalBaselineTransitionEvent(options.RequestID, key, previous.Status, status, guardrails, thresholds, latest, breachSignalCount, consecutiveBreachCount, cpuDelta, entropyDelta, confidenceDelta); err != nil {
					return decisionSignalBaselineSummary{}, err
				}
			}
		}
	}

	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].SampleCount == buckets[j].SampleCount {
			return buckets[i].BucketKey < buckets[j].BucketKey
		}
		return buckets[i].SampleCount > buckets[j].SampleCount
	})
	sort.Strings(summary.AtRiskBucketKeys)
	sort.Strings(summary.PendingBucketKeys)
	sort.Strings(summary.InsufficientBucketKeys)

	summary.Buckets = buckets
	summary.BucketCount = len(buckets)
	summary.AtRiskBucketCount = len(summary.AtRiskBucketKeys)
	summary.PendingBucketCount = len(summary.PendingBucketKeys)
	summary.InsufficientCount = len(summary.InsufficientBucketKeys)
	summary.Healthy = summary.AtRiskBucketCount == 0
	return summary, nil
}

func normalizeSignalBaselineStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case signalBaselineStatusPending:
		return signalBaselineStatusPending
	case signalBaselineStatusAtRisk:
		return signalBaselineStatusAtRisk
	case signalBaselineStatusInsufficientHistory:
		return signalBaselineStatusInsufficientHistory
	default:
		return signalBaselineStatusHealthy
	}
}

func emitSignalBaselineTransitionEvent(
	requestID string,
	bucketKey string,
	previousStatus string,
	currentStatus string,
	guardrails decisionSignalBaselineGuardrails,
	thresholds decisionSignalBaselineThresholds,
	latest database.DecisionTrace,
	breachSignalCount int,
	consecutiveBreachCount int,
	cpuDelta float64,
	entropyDelta float64,
	confidenceDelta float64,
) error {
	previousStatus = normalizeSignalBaselineStatus(previousStatus)
	currentStatus = normalizeSignalBaselineStatus(currentStatus)
	if !(previousStatus == signalBaselineStatusAtRisk || currentStatus == signalBaselineStatusAtRisk) {
		return nil
	}
	title := "SIGNAL_BASELINE_RECOVERED"
	summary := fmt.Sprintf("signal baseline recovered for %s", bucketKey)
	if currentStatus == signalBaselineStatusAtRisk {
		title = "SIGNAL_BASELINE_AT_RISK"
		summary = fmt.Sprintf("signal baseline drift breached guardrail for %s", bucketKey)
	}
	reason := fmt.Sprintf(
		"signal baseline transition %s -> %s (bucket=%s, breaches=%d, streak=%d/%d)",
		previousStatus,
		currentStatus,
		bucketKey,
		breachSignalCount,
		consecutiveBreachCount,
		guardrails.RequiredStreak,
	)
	payload := map[string]interface{}{
		"bucket_key":                    bucketKey,
		"previous_status":               previousStatus,
		"status":                        currentStatus,
		"latest_trace_id":               latest.ID,
		"latest_timestamp":              latest.Timestamp,
		"decision_engine":               normalizeSignalBucketDimension(latest.DecisionEngine, "unknown-engine"),
		"engine_version":                normalizeSignalBucketDimension(latest.DecisionEngineVersion, "unknown-version"),
		"rollout_mode":                  normalizeSignalBucketDimension(latest.PolicyRolloutMode, "unknown-rollout"),
		"breach_signal_count":           breachSignalCount,
		"consecutive_breach_count":      consecutiveBreachCount,
		"required_consecutive_breaches": guardrails.RequiredStreak,
		"min_baseline_samples":          guardrails.MinBaselineSamples,
		"cpu_delta":                     cpuDelta,
		"entropy_delta":                 entropyDelta,
		"confidence_delta":              confidenceDelta,
		"cpu_delta_threshold":           thresholds.CPUDelta,
		"entropy_delta_threshold":       thresholds.EntropyDelta,
		"confidence_delta_threshold":    thresholds.ConfidenceDelta,
	}
	_, err := database.InsertEventWithPayloadAndRequestID(
		"audit",
		"decision-intelligence",
		reason,
		"ops-signal-baseline",
		"",
		title,
		summary,
		latest.PID,
		latest.CPUScore,
		latest.EntropyScore,
		latest.ConfidenceScore,
		requestID,
		payload,
	)
	return err
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

func decisionReplayHealthSampleLimitFromEnv() int {
	limit := defaultDecisionReplayHealthLimit
	raw := strings.TrimSpace(os.Getenv("FLOWFORGE_DECISION_REPLAY_HEALTH_LIMIT"))
	if raw == "" {
		return limit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return limit
	}
	if parsed < 1 {
		return 1
	}
	if parsed > maxDecisionReplayHealthLimit {
		return maxDecisionReplayHealthLimit
	}
	return parsed
}

func decisionReplayPrometheus() string {
	var b strings.Builder
	b.WriteString("# HELP flowforge_decision_replay_checked_rows Number of decision traces scanned for replay integrity checks.\n")
	b.WriteString("# TYPE flowforge_decision_replay_checked_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_match_rows Decision traces where deterministic replay digest matched.\n")
	b.WriteString("# TYPE flowforge_decision_replay_match_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_mismatch_rows Decision traces where deterministic replay digest mismatched.\n")
	b.WriteString("# TYPE flowforge_decision_replay_mismatch_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_missing_digest_rows Decision traces missing replay digest under non-legacy contract.\n")
	b.WriteString("# TYPE flowforge_decision_replay_missing_digest_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_legacy_fallback_rows Decision traces replayed using legacy metadata fallback.\n")
	b.WriteString("# TYPE flowforge_decision_replay_legacy_fallback_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_unreplayable_rows Decision traces not replayable due to incomplete deterministic input.\n")
	b.WriteString("# TYPE flowforge_decision_replay_unreplayable_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_mismatch_ratio Mismatch ratio across sampled decision traces.\n")
	b.WriteString("# TYPE flowforge_decision_replay_mismatch_ratio gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_healthiness Replay healthiness flag (1 healthy, 0 at risk).\n")
	b.WriteString("# TYPE flowforge_decision_replay_healthiness gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_health_sample_limit Sample size used for replay health scan.\n")
	b.WriteString("# TYPE flowforge_decision_replay_health_sample_limit gauge\n")
	b.WriteString("# HELP flowforge_decision_replay_stats_error Whether decision replay health collection failed (1) or succeeded (0).\n")
	b.WriteString("# TYPE flowforge_decision_replay_stats_error gauge\n")

	if err := ensureAPIDBReady(); err != nil {
		b.WriteString("flowforge_decision_replay_checked_rows 0\n")
		b.WriteString("flowforge_decision_replay_match_rows 0\n")
		b.WriteString("flowforge_decision_replay_mismatch_rows 0\n")
		b.WriteString("flowforge_decision_replay_missing_digest_rows 0\n")
		b.WriteString("flowforge_decision_replay_legacy_fallback_rows 0\n")
		b.WriteString("flowforge_decision_replay_unreplayable_rows 0\n")
		b.WriteString("flowforge_decision_replay_mismatch_ratio 0\n")
		b.WriteString("flowforge_decision_replay_healthiness 0\n")
		fmt.Fprintf(&b, "flowforge_decision_replay_health_sample_limit %d\n", decisionReplayHealthSampleLimitFromEnv())
		b.WriteString("flowforge_decision_replay_stats_error 1\n")
		return b.String()
	}

	limit := decisionReplayHealthSampleLimitFromEnv()
	summary, err := buildDecisionReplayHealthSummary(limit)
	if err != nil {
		b.WriteString("flowforge_decision_replay_checked_rows 0\n")
		b.WriteString("flowforge_decision_replay_match_rows 0\n")
		b.WriteString("flowforge_decision_replay_mismatch_rows 0\n")
		b.WriteString("flowforge_decision_replay_missing_digest_rows 0\n")
		b.WriteString("flowforge_decision_replay_legacy_fallback_rows 0\n")
		b.WriteString("flowforge_decision_replay_unreplayable_rows 0\n")
		b.WriteString("flowforge_decision_replay_mismatch_ratio 0\n")
		b.WriteString("flowforge_decision_replay_healthiness 0\n")
		fmt.Fprintf(&b, "flowforge_decision_replay_health_sample_limit %d\n", limit)
		b.WriteString("flowforge_decision_replay_stats_error 1\n")
		return b.String()
	}

	fmt.Fprintf(&b, "flowforge_decision_replay_checked_rows %d\n", summary.Scanned)
	fmt.Fprintf(&b, "flowforge_decision_replay_match_rows %d\n", summary.MatchCount)
	fmt.Fprintf(&b, "flowforge_decision_replay_mismatch_rows %d\n", summary.MismatchCount)
	fmt.Fprintf(&b, "flowforge_decision_replay_missing_digest_rows %d\n", summary.MissingDigestCount)
	fmt.Fprintf(&b, "flowforge_decision_replay_legacy_fallback_rows %d\n", summary.LegacyFallbackCount)
	fmt.Fprintf(&b, "flowforge_decision_replay_unreplayable_rows %d\n", summary.UnreplayableCount)
	fmt.Fprintf(&b, "flowforge_decision_replay_mismatch_ratio %.6f\n", summary.MismatchRatio)
	if summary.Healthy {
		b.WriteString("flowforge_decision_replay_healthiness 1\n")
	} else {
		b.WriteString("flowforge_decision_replay_healthiness 0\n")
	}
	fmt.Fprintf(&b, "flowforge_decision_replay_health_sample_limit %d\n", summary.Limit)
	b.WriteString("flowforge_decision_replay_stats_error 0\n")
	return b.String()
}

func decisionSignalBaselineSampleLimitFromEnv() int {
	limit := defaultDecisionSignalBaselineLimit
	raw := strings.TrimSpace(os.Getenv("FLOWFORGE_DECISION_SIGNAL_BASELINE_LIMIT"))
	if raw == "" {
		return limit
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return limit
	}
	if parsed < 1 {
		return 1
	}
	if parsed > maxDecisionSignalBaselineLimit {
		return maxDecisionSignalBaselineLimit
	}
	return parsed
}

func decisionSignalBaselinePrometheus() string {
	var b strings.Builder
	b.WriteString("# HELP flowforge_decision_signal_baseline_checked_rows Number of decision traces scanned for signal baseline checks.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_checked_rows gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_bucket_count Number of grouped signal baseline buckets.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_bucket_count gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_at_risk_buckets Number of signal baseline buckets currently marked at risk.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_at_risk_buckets gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_pending_buckets Number of baseline buckets that breached once but have not reached escalation streak.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_pending_buckets gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_insufficient_history_buckets Number of baseline buckets skipped due to insufficient baseline sample history.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_insufficient_history_buckets gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_transition_count Number of bucket status transitions detected in this baseline evaluation.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_transition_count gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_max_cpu_delta_abs Maximum absolute CPU-score delta from baseline.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_max_cpu_delta_abs gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_max_entropy_delta_abs Maximum absolute entropy-score delta from baseline.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_max_entropy_delta_abs gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_max_confidence_delta_abs Maximum absolute confidence-score delta from baseline.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_max_confidence_delta_abs gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_healthiness Signal baseline healthiness flag (1 healthy, 0 at risk).\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_healthiness gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_sample_limit Sample size used for signal baseline scan.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_sample_limit gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_required_streak Required consecutive breaches before a bucket is marked at risk.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_required_streak gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_min_baseline_samples Minimum baseline samples required before drift escalation logic applies.\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_min_baseline_samples gauge\n")
	b.WriteString("# HELP flowforge_decision_signal_baseline_stats_error Whether signal baseline collection failed (1) or succeeded (0).\n")
	b.WriteString("# TYPE flowforge_decision_signal_baseline_stats_error gauge\n")

	guardrails := decisionSignalBaselineGuardrailsFromEnv()
	limit := decisionSignalBaselineSampleLimitFromEnv()

	if err := ensureAPIDBReady(); err != nil {
		b.WriteString("flowforge_decision_signal_baseline_checked_rows 0\n")
		b.WriteString("flowforge_decision_signal_baseline_bucket_count 0\n")
		b.WriteString("flowforge_decision_signal_baseline_at_risk_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_pending_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_insufficient_history_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_transition_count 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_cpu_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_entropy_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_confidence_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_healthiness 0\n")
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_sample_limit %d\n", limit)
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_required_streak %d\n", guardrails.RequiredStreak)
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_min_baseline_samples %d\n", guardrails.MinBaselineSamples)
		b.WriteString("flowforge_decision_signal_baseline_stats_error 1\n")
		return b.String()
	}

	thresholds := decisionSignalBaselineThresholdsFromEnv()
	summary, err := buildDecisionSignalBaselineSummary(
		limit,
		decisionSignalBaselineFilter{},
		thresholds,
		guardrails,
		decisionSignalBaselineBuildOptions{
			PersistState:         true,
			EmitAuditTransitions: false,
		},
	)
	if err != nil {
		b.WriteString("flowforge_decision_signal_baseline_checked_rows 0\n")
		b.WriteString("flowforge_decision_signal_baseline_bucket_count 0\n")
		b.WriteString("flowforge_decision_signal_baseline_at_risk_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_pending_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_insufficient_history_buckets 0\n")
		b.WriteString("flowforge_decision_signal_baseline_transition_count 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_cpu_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_entropy_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_max_confidence_delta_abs 0\n")
		b.WriteString("flowforge_decision_signal_baseline_healthiness 0\n")
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_sample_limit %d\n", limit)
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_required_streak %d\n", guardrails.RequiredStreak)
		fmt.Fprintf(&b, "flowforge_decision_signal_baseline_min_baseline_samples %d\n", guardrails.MinBaselineSamples)
		b.WriteString("flowforge_decision_signal_baseline_stats_error 1\n")
		return b.String()
	}

	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_checked_rows %d\n", summary.Scanned)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_bucket_count %d\n", summary.BucketCount)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_at_risk_buckets %d\n", summary.AtRiskBucketCount)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_pending_buckets %d\n", summary.PendingBucketCount)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_insufficient_history_buckets %d\n", summary.InsufficientCount)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_transition_count %d\n", summary.TransitionCount)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_max_cpu_delta_abs %.6f\n", summary.MaxCPUDeltaAbs)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_max_entropy_delta_abs %.6f\n", summary.MaxEntropyDeltaAbs)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_max_confidence_delta_abs %.6f\n", summary.MaxConfidenceDeltaAbs)
	if summary.Healthy {
		b.WriteString("flowforge_decision_signal_baseline_healthiness 1\n")
	} else {
		b.WriteString("flowforge_decision_signal_baseline_healthiness 0\n")
	}
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_sample_limit %d\n", summary.Limit)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_required_streak %d\n", summary.Guardrails.RequiredStreak)
	fmt.Fprintf(&b, "flowforge_decision_signal_baseline_min_baseline_samples %d\n", summary.Guardrails.MinBaselineSamples)
	b.WriteString("flowforge_decision_signal_baseline_stats_error 0\n")
	return b.String()
}

// HandleWorkerLifecycle exposes lifecycle control-plane state for operators/UI.
func HandleWorkerLifecycle(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
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
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	if incidentID := strings.TrimSpace(r.URL.Query().Get("incident_id")); incidentID != "" {
		events, err := database.GetIncidentTimelineByIncidentID(incidentID, 500)
		if err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(events); err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Encode error: %v", err))
		}
		return
	}

	if strings.HasPrefix(r.URL.Path, "/v1/") {
		limit, cursor, err := parseCursorPageQuery(
			r.URL.Query().Get("limit"),
			r.URL.Query().Get("cursor"),
			defaultCursorPageLimit,
			maxCursorPageLimit,
		)
		if err != nil {
			writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
			return
		}

		events, nextCursor, hasMore, err := database.GetTimelinePage(limit, cursor)
		if err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
			return
		}

		payload := paginatedTimelineResponse{
			Items:   events,
			HasMore: hasMore,
			Limit:   limit,
		}
		if hasMore && nextCursor > 0 {
			payload.NextCursor = strconv.FormatInt(nextCursor, 10)
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	events, err := database.GetTimeline(100)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Encode error: %v", err))
	}
}

// HandleIncidents is exported for testing.
func HandleIncidents(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	if strings.HasPrefix(r.URL.Path, "/v1/") {
		limit, cursor, err := parseCursorPageQuery(
			r.URL.Query().Get("limit"),
			r.URL.Query().Get("cursor"),
			defaultCursorPageLimit,
			maxCursorPageLimit,
		)
		if err != nil {
			writeJSONErrorForRequest(w, r, http.StatusBadRequest, err.Error())
			return
		}

		incidents, nextCursor, hasMore, err := database.GetIncidentsPage(limit, cursor)
		if err != nil {
			writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
			return
		}

		payload := paginatedIncidentsResponse{
			Items:   incidents,
			HasMore: hasMore,
			Limit:   limit,
		}
		if hasMore && nextCursor > 0 {
			payload.NextCursor = strconv.FormatInt(nextCursor, 10)
		}
		writeJSON(w, http.StatusOK, payload)
		return
	}

	incidents, err := database.GetAllIncidents()
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, incidents)
}

// HandleProcessKill is exported for testing.
func HandleProcessKill(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
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
		payload := problemPayload(r, statusCode, msg, nil)
		persistIdempotentMutation(idemCtx, statusCode, payload)
		writeProblem(w, statusCode, payload)
		return
	}

	stats := state.GetState()
	if decision.AcceptedNew {
		apiMetrics.IncProcessKill()
		incidentID := uuid.NewString()
		_ = database.LogAuditEventWithIncidentAndRequestID(actorFromRequest(r), "KILL", annotateReasonWithRequestID(reason, r), "api", decision.PID, stats.Command, incidentID, requestIDFromRequest(r))
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
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
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
			_ = database.LogAuditEventWithIncidentAndRequestID(actorFromRequest(r), "RESTART_BLOCKED", annotateReasonWithRequestID(msg, r), "api", stats.PID, stats.Command, incidentID, requestIDFromRequest(r))
		}
		if retryAfter := lifecycleRetryAfter(err); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			payload := problemPayload(r, statusCode, msg, map[string]interface{}{"retry_after_seconds": retryAfter})
			persistIdempotentMutation(idemCtx, statusCode, payload)
			writeProblem(w, statusCode, payload)
			return
		}
		payload := problemPayload(r, statusCode, msg, nil)
		persistIdempotentMutation(idemCtx, statusCode, payload)
		writeProblem(w, statusCode, payload)
		return
	}

	stats := state.GetState()
	if decision.AcceptedNew {
		apiMetrics.IncProcessRestart()
		incidentID := uuid.NewString()
		_ = database.LogAuditEventWithIncidentAndRequestID(actorFromRequest(r), "RESTART", annotateReasonWithRequestID(reason, r), "api", decision.PID, stats.Command, incidentID, requestIDFromRequest(r))
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

func problemTypeURI(statusCode int, detail string) string {
	detailLower := strings.ToLower(strings.TrimSpace(detail))
	switch statusCode {
	case http.StatusBadRequest:
		return problemTypeBaseURI + "bad-request"
	case http.StatusUnauthorized:
		return problemTypeBaseURI + "unauthorized"
	case http.StatusForbidden:
		return problemTypeBaseURI + "forbidden"
	case http.StatusNotFound:
		return problemTypeBaseURI + "not-found"
	case http.StatusMethodNotAllowed:
		return problemTypeBaseURI + "method-not-allowed"
	case http.StatusConflict:
		if strings.Contains(detailLower, "idempotency") {
			return problemTypeBaseURI + "idempotency-conflict"
		}
		return problemTypeBaseURI + "conflict"
	case http.StatusTooManyRequests:
		if strings.Contains(detailLower, "restart budget") {
			return problemTypeBaseURI + "restart-budget-exceeded"
		}
		if strings.Contains(detailLower, "auth attempt") {
			return problemTypeBaseURI + "auth-rate-limited"
		}
		return problemTypeBaseURI + "rate-limited"
	case http.StatusServiceUnavailable:
		return problemTypeBaseURI + "not-ready"
	case http.StatusInternalServerError:
		return problemTypeBaseURI + "internal"
	default:
		return problemTypeBaseURI + "http-" + strconv.Itoa(statusCode)
	}
}

func problemPayload(r *http.Request, statusCode int, detail string, extra map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"type":   problemTypeURI(statusCode, detail),
		"title":  http.StatusText(statusCode),
		"status": statusCode,
	}
	if payload["title"] == "" {
		payload["title"] = "Error"
	}
	if detail != "" {
		payload["detail"] = detail
		// Compatibility field for existing clients and scripts.
		payload["error"] = detail
	}
	if r != nil && r.URL != nil {
		if instance := strings.TrimSpace(r.URL.Path); instance != "" {
			payload["instance"] = instance
		}
	}
	if rid := requestIDFromRequest(r); rid != "" {
		payload["request_id"] = rid
	}
	for k, v := range extra {
		payload[k] = v
	}
	return payload
}

func writeProblem(w http.ResponseWriter, statusCode int, payload map[string]interface{}) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[API] encode problem response failed: %v", err)
	}
}

func writeJSONErrorForRequest(w http.ResponseWriter, r *http.Request, statusCode int, msg string) {
	if r != nil {
		r = withRequestID(r)
		if rid := requestIDFromRequest(r); rid != "" {
			w.Header().Set(requestIDHeader, rid)
		}
	}
	writeProblem(w, statusCode, problemPayload(r, statusCode, msg, nil))
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
