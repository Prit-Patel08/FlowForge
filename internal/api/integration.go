package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flowforge/internal/database"
	"flowforge/internal/state"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var integrationWorkspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type registerWorkspaceRequest struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspacePath string `json:"workspace_path"`
	Profile       string `json:"profile"`
	Client        string `json:"client"`
}

type setProtectionRequest struct {
	Enabled *bool  `json:"enabled"`
	Reason  string `json:"reason"`
}

type workspaceActionRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func HandleIntegrationWorkspaceRegister(w http.ResponseWriter, r *http.Request) {
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
	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}
	idemCtx, handled := beginIdempotentMutation(w, r, "POST /v1/integrations/workspaces/register")
	if handled {
		return
	}

	respond := func(status int, payload map[string]interface{}) {
		persistIdempotentMutation(idemCtx, status, payload)
		writeJSON(w, status, payload)
	}
	respondErr := func(status int, msg string) {
		payload := problemPayload(r, status, msg, nil)
		persistIdempotentMutation(idemCtx, status, payload)
		writeProblem(w, status, payload)
	}

	var req registerWorkspaceRequest
	if err := decodeJSONBody(r, &req); err != nil {
		respondErr(http.StatusBadRequest, err.Error())
		return
	}

	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	if !integrationWorkspaceIDPattern.MatchString(req.WorkspaceID) {
		respondErr(http.StatusBadRequest, "workspace_id must match [A-Za-z0-9._:-] and be <= 128 chars")
		return
	}
	req.WorkspacePath = strings.TrimSpace(req.WorkspacePath)
	if req.WorkspacePath == "" || !filepath.IsAbs(req.WorkspacePath) {
		respondErr(http.StatusBadRequest, "workspace_path must be an absolute path")
		return
	}
	req.Profile = strings.TrimSpace(req.Profile)
	if req.Profile == "" {
		req.Profile = "standard"
	}
	req.Client = strings.TrimSpace(req.Client)
	if req.Client == "" {
		req.Client = "unknown"
	}

	ws, err := database.UpsertIntegrationWorkspace(req.WorkspaceID, req.WorkspacePath, req.Profile, req.Client)
	if err != nil {
		respondErr(http.StatusInternalServerError, fmt.Sprintf("workspace register failed: %v", err))
		return
	}

	reason := fmt.Sprintf("workspace registered via integration API: %s", ws.WorkspaceID)
	_, _ = database.LogAuditEventWithIncidentAndIDAndRequestID(actorFromRequest(r), "WORKSPACE_REGISTER", annotateReasonWithRequestID(reason, r), "integration", state.GetState().PID, ws.WorkspacePath, "", requestIDFromRequest(r))

	respond(http.StatusOK, map[string]interface{}{
		"ok":           true,
		"workspace_id": ws.WorkspaceID,
		"profile":      ws.Profile,
	})
}

func HandleIntegrationWorkspaceScoped(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w, r)
	r = ensureRequestContext(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := ensureAPIDBReady(); err != nil {
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("database init failed: %v", err))
		return
	}

	workspaceID, resource, subresource, err := parseIntegrationWorkspacePath(r.URL.Path)
	if err != nil {
		writeJSONErrorForRequest(w, r, http.StatusNotFound, err.Error())
		return
	}
	if !integrationWorkspaceIDPattern.MatchString(workspaceID) {
		writeJSONErrorForRequest(w, r, http.StatusBadRequest, "invalid workspace_id")
		return
	}

	switch {
	case resource == "status" && subresource == "":
		handleIntegrationWorkspaceStatus(w, r, workspaceID)
	case resource == "protection" && subresource == "":
		handleIntegrationWorkspaceProtection(w, r, workspaceID)
	case resource == "actions" && subresource == "":
		handleIntegrationWorkspaceActions(w, r, workspaceID)
	case resource == "incidents" && subresource == "latest":
		handleIntegrationWorkspaceLatestIncident(w, r, workspaceID)
	default:
		writeJSONErrorForRequest(w, r, http.StatusNotFound, "integration endpoint not found")
	}
}

func parseIntegrationWorkspacePath(path string) (workspaceID, resource, subresource string, err error) {
	const prefix = "/v1/integrations/workspaces/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", "", errors.New("integration endpoint not found")
	}
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", errors.New("integration endpoint not found")
	}
	workspaceID = strings.TrimSpace(parts[0])
	resource = strings.TrimSpace(parts[1])
	if len(parts) == 3 {
		subresource = strings.TrimSpace(parts[2])
	}
	if workspaceID == "" || resource == "" {
		return "", "", "", errors.New("integration endpoint not found")
	}
	return workspaceID, resource, subresource, nil
}

func handleIntegrationWorkspaceStatus(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ws, err := database.GetIntegrationWorkspace(workspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONErrorForRequest(w, r, http.StatusNotFound, "workspace not found")
			return
		}
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("workspace lookup failed: %v", err))
		return
	}

	currentState := state.GetState()
	activePID := 0
	if currentState.PID > 0 && currentState.Status != "STOPPED" {
		activePID = currentState.PID
	}
	if err := database.UpdateIntegrationWorkspaceActivePID(workspaceID, activePID); err == nil {
		if refreshed, getErr := database.GetIntegrationWorkspace(workspaceID); getErr == nil {
			ws = refreshed
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"workspace_id":       ws.WorkspaceID,
		"protection_enabled": ws.ProtectionEnabled,
		"profile":            ws.Profile,
		"active_pid":         ws.ActivePID,
		"last_updated":       ws.LastUpdated,
	})
}

func handleIntegrationWorkspaceProtection(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodPost {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !requireAuth(w, r) {
		return
	}
	idemCtx, handled := beginIdempotentMutation(w, r, fmt.Sprintf("POST /v1/integrations/workspaces/%s/protection", workspaceID))
	if handled {
		return
	}

	respond := func(status int, payload map[string]interface{}) {
		persistIdempotentMutation(idemCtx, status, payload)
		writeJSON(w, status, payload)
	}
	respondErr := func(status int, msg string) {
		payload := problemPayload(r, status, msg, nil)
		persistIdempotentMutation(idemCtx, status, payload)
		writeProblem(w, status, payload)
	}

	var req setProtectionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		respondErr(http.StatusBadRequest, err.Error())
		return
	}
	if req.Enabled == nil {
		respondErr(http.StatusBadRequest, "enabled must be explicitly provided")
		return
	}

	ws, err := database.SetIntegrationWorkspaceProtection(workspaceID, *req.Enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondErr(http.StatusNotFound, "workspace not found")
			return
		}
		respondErr(http.StatusInternalServerError, fmt.Sprintf("workspace protection update failed: %v", err))
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = fmt.Sprintf("workspace protection set to %t", ws.ProtectionEnabled)
	}
	reason = annotateReasonWithRequestID(reason, r)
	_, _ = database.LogAuditEventWithIncidentAndIDAndRequestID(actorFromRequest(r), "PROTECTION_UPDATE", reason, "integration", state.GetState().PID, workspaceID, "", requestIDFromRequest(r))

	respond(http.StatusOK, map[string]interface{}{
		"ok":           true,
		"workspace_id": ws.WorkspaceID,
		"enabled":      ws.ProtectionEnabled,
	})
}

func handleIntegrationWorkspaceActions(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodPost {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !requireAuth(w, r) {
		return
	}
	idemCtx, handled := beginIdempotentMutation(w, r, fmt.Sprintf("POST /v1/integrations/workspaces/%s/actions", workspaceID))
	if handled {
		return
	}

	respond := func(status int, payload map[string]interface{}) {
		persistIdempotentMutation(idemCtx, status, payload)
		writeJSON(w, status, payload)
	}
	respondErr := func(status int, msg string) {
		payload := problemPayload(r, status, msg, nil)
		persistIdempotentMutation(idemCtx, status, payload)
		writeProblem(w, status, payload)
	}

	ws, err := database.GetIntegrationWorkspace(workspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondErr(http.StatusNotFound, "workspace not found")
			return
		}
		respondErr(http.StatusInternalServerError, fmt.Sprintf("workspace lookup failed: %v", err))
		return
	}

	var req workspaceActionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		respondErr(http.StatusBadRequest, err.Error())
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "restart" && action != "kill" {
		respondErr(http.StatusBadRequest, "action must be one of: restart, kill")
		return
	}

	if !ws.ProtectionEnabled {
		respondErr(http.StatusConflict, "workspace protection is disabled")
		return
	}

	workerControl.registerSpecFromStateIfMissing()
	var (
		decision lifecycleAction
		reqErr   error
	)
	switch action {
	case "restart":
		decision, reqErr = requestLifecycleRestart()
	case "kill":
		decision, reqErr = requestLifecycleKill()
	}
	if reqErr != nil {
		statusCode := lifecycleHTTPCode(reqErr, http.StatusInternalServerError)
		msg := lifecycleErrorMessage(reqErr, "failed to request action")
		if retryAfter := lifecycleRetryAfter(reqErr); retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			payload := problemPayload(r, statusCode, msg, map[string]interface{}{"retry_after_seconds": retryAfter})
			persistIdempotentMutation(idemCtx, statusCode, payload)
			writeProblem(w, statusCode, payload)
			return
		}
		respondErr(statusCode, msg)
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = fmt.Sprintf("integration workspace %s requested %s", workspaceID, action)
	}
	reason = annotateReasonWithRequestID(reason, r)
	actionLabel := strings.ToUpper("INTEGRATION_" + action)
	auditEventID, err := database.LogAuditEventWithIncidentAndIDAndRequestID(actorFromRequest(r), actionLabel, reason, "integration", decision.PID, workspaceID, "", requestIDFromRequest(r))
	if err != nil {
		respondErr(http.StatusInternalServerError, fmt.Sprintf("audit event write failed: %v", err))
		return
	}
	if _, err := database.InsertIntegrationAction(workspaceID, action, reason, auditEventID, decision.Status); err != nil {
		respondErr(http.StatusInternalServerError, fmt.Sprintf("integration action write failed: %v", err))
		return
	}
	if decision.AcceptedNew {
		if action == "kill" {
			apiMetrics.IncProcessKill()
		}
		if action == "restart" {
			apiMetrics.IncProcessRestart()
		}
	}

	respond(http.StatusOK, map[string]interface{}{
		"ok":             true,
		"action":         action,
		"audit_event_id": auditEventID,
		"status":         decision.Status,
		"lifecycle":      decision.Lifecycle,
		"pid":            decision.PID,
	})
}

func handleIntegrationWorkspaceLatestIncident(w http.ResponseWriter, r *http.Request, workspaceID string) {
	if r.Method != http.MethodGet {
		writeJSONErrorForRequest(w, r, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	if _, err := database.GetIntegrationWorkspace(workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONErrorForRequest(w, r, http.StatusNotFound, "workspace not found")
			return
		}
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("workspace lookup failed: %v", err))
		return
	}

	latest, err := database.GetLatestIntegrationIncident(workspaceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONErrorForRequest(w, r, http.StatusNotFound, "no incidents found")
			return
		}
		writeJSONErrorForRequest(w, r, http.StatusInternalServerError, fmt.Sprintf("incident lookup failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"incident_id":      latest.IncidentID,
		"exit_reason":      latest.ExitReason,
		"reason_text":      latest.ReasonText,
		"confidence_score": latest.ConfidenceScore,
		"created_at":       latest.CreatedAt,
	})
}

func ensureAPIDBReady() error {
	if database.GetDB() != nil {
		return nil
	}
	return database.InitDB()
}

func decodeJSONBody(r *http.Request, out interface{}) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 32*1024))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if len(body) == 0 {
		return errors.New("request body is required")
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	var trailing interface{}
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: multiple JSON values are not allowed")
	}
	return nil
}
