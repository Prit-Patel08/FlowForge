package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type IntegrationWorkspace struct {
	WorkspaceID       string `json:"workspace_id"`
	WorkspacePath     string `json:"workspace_path"`
	Profile           string `json:"profile"`
	Client            string `json:"client"`
	ProtectionEnabled bool   `json:"protection_enabled"`
	ActivePID         int    `json:"active_pid"`
	CreatedAt         string `json:"created_at"`
	LastUpdated       string `json:"last_updated"`
}

type IntegrationLatestIncident struct {
	IncidentID      string  `json:"incident_id"`
	ExitReason      string  `json:"exit_reason"`
	ReasonText      string  `json:"reason_text"`
	ConfidenceScore float64 `json:"confidence_score"`
	CreatedAt       string  `json:"created_at"`
}

func UpsertIntegrationWorkspace(workspaceID, workspacePath, profile, client string) (IntegrationWorkspace, error) {
	if db == nil {
		return IntegrationWorkspace{}, fmt.Errorf("db not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	workspacePath = strings.TrimSpace(workspacePath)
	profile = strings.TrimSpace(profile)
	client = strings.TrimSpace(client)

	if workspaceID == "" {
		return IntegrationWorkspace{}, fmt.Errorf("workspace_id is required")
	}
	if workspacePath == "" {
		return IntegrationWorkspace{}, fmt.Errorf("workspace_path is required")
	}
	if profile == "" {
		profile = "standard"
	}
	if client == "" {
		client = "unknown"
	}

	_, err := db.Exec(`
INSERT INTO integration_workspaces(
	workspace_id, workspace_path, profile, client, protection_enabled, active_pid, created_at, last_updated
) VALUES(?, ?, ?, ?, 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(workspace_id) DO UPDATE SET
	workspace_path = excluded.workspace_path,
	profile = excluded.profile,
	client = excluded.client,
	last_updated = CURRENT_TIMESTAMP
`, workspaceID, workspacePath, profile, client)
	if err != nil {
		return IntegrationWorkspace{}, err
	}

	return GetIntegrationWorkspace(workspaceID)
}

func GetIntegrationWorkspace(workspaceID string) (IntegrationWorkspace, error) {
	if db == nil {
		return IntegrationWorkspace{}, fmt.Errorf("db not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return IntegrationWorkspace{}, fmt.Errorf("workspace_id is required")
	}

	var out IntegrationWorkspace
	var protectionInt int
	err := db.QueryRow(`
SELECT
	workspace_id,
	workspace_path,
	COALESCE(profile, 'standard'),
	COALESCE(client, 'unknown'),
	COALESCE(protection_enabled, 1),
	COALESCE(active_pid, 0),
	COALESCE(created_at, CURRENT_TIMESTAMP),
	COALESCE(last_updated, CURRENT_TIMESTAMP)
FROM integration_workspaces
WHERE workspace_id = ?
`, workspaceID).Scan(
		&out.WorkspaceID,
		&out.WorkspacePath,
		&out.Profile,
		&out.Client,
		&protectionInt,
		&out.ActivePID,
		&out.CreatedAt,
		&out.LastUpdated,
	)
	if err != nil {
		return IntegrationWorkspace{}, err
	}
	out.ProtectionEnabled = protectionInt == 1
	return out, nil
}

func SetIntegrationWorkspaceProtection(workspaceID string, enabled bool) (IntegrationWorkspace, error) {
	if db == nil {
		return IntegrationWorkspace{}, fmt.Errorf("db not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return IntegrationWorkspace{}, fmt.Errorf("workspace_id is required")
	}

	flag := 0
	if enabled {
		flag = 1
	}
	res, err := db.Exec(`
UPDATE integration_workspaces
SET protection_enabled = ?, last_updated = CURRENT_TIMESTAMP
WHERE workspace_id = ?
`, flag, workspaceID)
	if err != nil {
		return IntegrationWorkspace{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return IntegrationWorkspace{}, sql.ErrNoRows
	}

	return GetIntegrationWorkspace(workspaceID)
}

func UpdateIntegrationWorkspaceActivePID(workspaceID string, pid int) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	if pid < 0 {
		pid = 0
	}
	res, err := db.Exec(`
UPDATE integration_workspaces
SET active_pid = ?, last_updated = CURRENT_TIMESTAMP
WHERE workspace_id = ?
`, pid, workspaceID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func InsertIntegrationAction(workspaceID, action, reason string, auditEventID int, status string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	action = strings.TrimSpace(action)
	reason = strings.TrimSpace(reason)
	status = strings.TrimSpace(status)
	if workspaceID == "" {
		return 0, fmt.Errorf("workspace_id is required")
	}
	if action == "" {
		return 0, fmt.Errorf("action is required")
	}

	result, err := db.Exec(`
INSERT INTO integration_actions(workspace_id, action, reason, audit_event_id, status, created_at)
VALUES(?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, workspaceID, action, reason, auditEventID, status)
	if err != nil {
		return 0, err
	}
	insertedID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(insertedID), nil
}

func GetLatestIntegrationIncident(_ string) (IntegrationLatestIncident, error) {
	if db == nil {
		return IntegrationLatestIncident{}, fmt.Errorf("db not initialized")
	}

	var (
		incidentID      string
		reasonText      string
		confidenceScore float64
		createdAt       string
		payloadRaw      string
	)
	err := db.QueryRow(`
SELECT
	COALESCE(incident_id, ''),
	COALESCE(reason_text, ''),
	COALESCE(confidence_score, 0.0),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(payload_json, '{}')
FROM events
WHERE event_type = 'incident'
ORDER BY created_at DESC, id DESC
LIMIT 1
`).Scan(&incidentID, &reasonText, &confidenceScore, &createdAt, &payloadRaw)
	if err != nil {
		return IntegrationLatestIncident{}, err
	}

	var payload incidentEventPayload
	if strings.TrimSpace(payloadRaw) != "" && strings.TrimSpace(payloadRaw) != "{}" {
		_ = json.Unmarshal([]byte(payloadRaw), &payload)
	}

	if incidentID == "" && payload.ID > 0 {
		incidentID = fmt.Sprintf("incident-%d", payload.ID)
	}

	exitReason := strings.TrimSpace(payload.ExitReason)
	if exitReason == "" {
		exitReason = "UNKNOWN"
	}
	reasonText = strings.TrimSpace(reasonText)
	if reasonText == "" {
		reasonText = strings.TrimSpace(payload.Reason)
	}

	return IntegrationLatestIncident{
		IncidentID:      incidentID,
		ExitReason:      exitReason,
		ReasonText:      reasonText,
		ConfidenceScore: confidenceScore,
		CreatedAt:       createdAt,
	}, nil
}
