package database

import (
	"database/sql"
	"encoding/json"
	"flowforge/internal/encryption"
	"flowforge/internal/redact"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB
var runContext struct {
	mu    sync.RWMutex
	runID string
}

type Incident struct {
	ID                   int     `json:"id"`
	Timestamp            string  `json:"timestamp"`
	Command              string  `json:"command"`
	ModelName            string  `json:"model_name"`
	ExitReason           string  `json:"exit_reason"`
	MaxCPU               float64 `json:"max_cpu"`
	Pattern              string  `json:"pattern"`
	TokenSavingsEstimate float64 `json:"token_savings_estimate"`
	TokenCount           int     `json:"token_count"`
	Cost                 float64 `json:"cost"`
	AgentID              string  `json:"agent_uuid"`
	AgentVersion         string  `json:"agent_version"`
	Reason               string  `json:"reason"`
	CPUScore             float64 `json:"cpu_score"`
	EntropyScore         float64 `json:"entropy_score"`
	ConfidenceScore      float64 `json:"confidence_score"`
	RecoveryStatus       string  `json:"recovery_status"`
	RestartCount         int     `json:"restart_count"`
}

type AuditEvent struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
	PID       int    `json:"pid"`
	Details   string `json:"details"`
	RequestID string `json:"request_id,omitempty"`
}

type DecisionTrace struct {
	ID                    int     `json:"id"`
	Timestamp             string  `json:"timestamp"`
	Command               string  `json:"command"`
	PID                   int     `json:"pid"`
	CPUScore              float64 `json:"cpu_score"`
	EntropyScore          float64 `json:"entropy_score"`
	ConfidenceScore       float64 `json:"confidence_score"`
	Decision              string  `json:"decision"`
	Reason                string  `json:"reason"`
	DecisionEngine        string  `json:"decision_engine,omitempty"`
	DecisionEngineVersion string  `json:"engine_version,omitempty"`
	DecisionContract      string  `json:"decision_contract_version,omitempty"`
	PolicyRolloutMode     string  `json:"rollout_mode,omitempty"`
	ReplayContract        string  `json:"replay_contract_version,omitempty"`
	ReplayDigest          string  `json:"replay_digest,omitempty"`
}

type TimelineEvent struct {
	EventID               string                 `json:"event_id,omitempty"`
	RunID                 string                 `json:"run_id,omitempty"`
	IncidentID            string                 `json:"incident_id,omitempty"`
	RequestID             string                 `json:"request_id,omitempty"`
	Type                  string                 `json:"type"`
	Timestamp             string                 `json:"timestamp"`
	Title                 string                 `json:"title"`
	Summary               string                 `json:"summary"`
	Reason                string                 `json:"reason"`
	Actor                 string                 `json:"actor,omitempty"`
	PID                   int                    `json:"pid"`
	CPUScore              float64                `json:"cpu_score,omitempty"`
	Entropy               float64                `json:"entropy_score,omitempty"`
	Confidence            float64                `json:"confidence_score,omitempty"`
	DecisionEngine        string                 `json:"decision_engine,omitempty"`
	DecisionEngineVersion string                 `json:"engine_version,omitempty"`
	DecisionContract      string                 `json:"decision_contract_version,omitempty"`
	PolicyRolloutMode     string                 `json:"rollout_mode,omitempty"`
	ReplayContract        string                 `json:"replay_contract_version,omitempty"`
	ReplayDigest          string                 `json:"replay_digest,omitempty"`
	Evidence              map[string]interface{} `json:"evidence,omitempty"`
}

type UnifiedEvent struct {
	ID                    int                    `json:"id"`
	EventID               string                 `json:"event_id"`
	RunID                 string                 `json:"run_id"`
	IncidentID            string                 `json:"incident_id,omitempty"`
	RequestID             string                 `json:"request_id,omitempty"`
	EventType             string                 `json:"event_type"`
	Actor                 string                 `json:"actor"`
	ReasonText            string                 `json:"reason_text"`
	CreatedAt             string                 `json:"created_at"`
	Timestamp             string                 `json:"timestamp"`
	Type                  string                 `json:"type"`
	Title                 string                 `json:"title"`
	Summary               string                 `json:"summary"`
	Reason                string                 `json:"reason"`
	PID                   int                    `json:"pid"`
	CPUScore              float64                `json:"cpu_score"`
	Entropy               float64                `json:"entropy_score"`
	Confidence            float64                `json:"confidence_score"`
	DecisionEngine        string                 `json:"decision_engine,omitempty"`
	DecisionEngineVersion string                 `json:"engine_version,omitempty"`
	DecisionContract      string                 `json:"decision_contract_version,omitempty"`
	PolicyRolloutMode     string                 `json:"rollout_mode,omitempty"`
	ReplayContract        string                 `json:"replay_contract_version,omitempty"`
	ReplayDigest          string                 `json:"replay_digest,omitempty"`
	Evidence              map[string]interface{} `json:"evidence,omitempty"`
}

type incidentEventPayload struct {
	ID                   int     `json:"id"`
	Command              string  `json:"command"`
	ModelName            string  `json:"model_name"`
	ExitReason           string  `json:"exit_reason"`
	MaxCPU               float64 `json:"max_cpu"`
	Pattern              string  `json:"pattern"`
	TokenSavingsEstimate float64 `json:"token_savings_estimate"`
	TokenCount           int     `json:"token_count"`
	Cost                 float64 `json:"cost"`
	AgentID              string  `json:"agent_id"`
	AgentVersion         string  `json:"agent_version"`
	Reason               string  `json:"reason"`
	CPUScore             float64 `json:"cpu_score"`
	EntropyScore         float64 `json:"entropy_score"`
	ConfidenceScore      float64 `json:"confidence_score"`
	RecoveryStatus       string  `json:"recovery_status"`
	RestartCount         int     `json:"restart_count"`
}

type auditEventPayload struct {
	ID        int    `json:"id"`
	Source    string `json:"source"`
	Details   string `json:"details"`
	RequestID string `json:"request_id,omitempty"`
}

type decisionEventPayload struct {
	ID                int    `json:"id"`
	Command           string `json:"command"`
	DecisionEngine    string `json:"decision_engine,omitempty"`
	EngineVersion     string `json:"engine_version,omitempty"`
	DecisionContract  string `json:"decision_contract_version,omitempty"`
	PolicyRolloutMode string `json:"rollout_mode,omitempty"`
	ReplayContract    string `json:"replay_contract_version,omitempty"`
	ReplayDigest      string `json:"replay_digest,omitempty"`
}

type DecisionTraceMeta struct {
	DecisionEngine    string `json:"decision_engine,omitempty"`
	EngineVersion     string `json:"engine_version,omitempty"`
	DecisionContract  string `json:"decision_contract_version,omitempty"`
	PolicyRolloutMode string `json:"rollout_mode,omitempty"`
	ReplayContract    string `json:"replay_contract_version,omitempty"`
	ReplayDigest      string `json:"replay_digest,omitempty"`
}

func InitDB() error {
	dbPath := os.Getenv("FLOWFORGE_DB_PATH")
	if dbPath == "" {
		dbPath = "flowforge.db"
	}
	var err error

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS incidents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		command TEXT,
		model_name TEXT,
		exit_reason TEXT,
		max_cpu REAL,
		pattern TEXT,
		token_savings_estimate REAL,
		token_count INTEGER DEFAULT 0,
		cost REAL DEFAULT 0.0,
		agent_id TEXT DEFAULT '',
		agent_version TEXT DEFAULT '',
		reason TEXT DEFAULT '',
		cpu_score REAL DEFAULT 0.0,
		entropy_score REAL DEFAULT 0.0,
		confidence_score REAL DEFAULT 0.0,
		recovery_status TEXT DEFAULT '',
		restart_count INTEGER DEFAULT 0
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		return err
	}

	// Migrations
	db.Exec("ALTER TABLE incidents ADD COLUMN token_count INTEGER DEFAULT 0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN cost REAL DEFAULT 0.0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN agent_id TEXT DEFAULT '';")
	db.Exec("ALTER TABLE incidents ADD COLUMN agent_version TEXT DEFAULT '';")
	db.Exec("ALTER TABLE incidents ADD COLUMN reason TEXT DEFAULT '';")
	db.Exec("ALTER TABLE incidents ADD COLUMN cpu_score REAL DEFAULT 0.0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN entropy_score REAL DEFAULT 0.0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN confidence_score REAL DEFAULT 0.0;")
	db.Exec("ALTER TABLE incidents ADD COLUMN recovery_status TEXT DEFAULT '';")
	db.Exec("ALTER TABLE incidents ADD COLUMN restart_count INTEGER DEFAULT 0;")

	createAuditTableSQL := `CREATE TABLE IF NOT EXISTS audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		actor TEXT,
		action TEXT,
		reason TEXT,
		source TEXT,
		pid INTEGER,
		details TEXT,
		request_id TEXT DEFAULT ''
	);`
	if _, err := db.Exec(createAuditTableSQL); err != nil {
		return err
	}

	createDecisionTableSQL := `CREATE TABLE IF NOT EXISTS decision_traces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		command TEXT,
		pid INTEGER,
		cpu_score REAL,
		entropy_score REAL,
		confidence_score REAL,
		decision TEXT,
		reason TEXT,
		decision_engine TEXT DEFAULT '',
		engine_version TEXT DEFAULT '',
		decision_contract_version TEXT DEFAULT '',
		rollout_mode TEXT DEFAULT '',
		replay_contract_version TEXT DEFAULT '',
		replay_digest TEXT DEFAULT ''
	);`
	if _, err := db.Exec(createDecisionTableSQL); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "decision_engine", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "engine_version", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "decision_contract_version", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "rollout_mode", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "replay_contract_version", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_traces", "replay_digest", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	db.Exec("UPDATE decision_traces SET decision_engine = COALESCE(decision_engine, '');")
	db.Exec("UPDATE decision_traces SET engine_version = COALESCE(engine_version, '');")
	db.Exec("UPDATE decision_traces SET decision_contract_version = COALESCE(decision_contract_version, '');")
	db.Exec("UPDATE decision_traces SET rollout_mode = COALESCE(rollout_mode, '');")
	db.Exec("UPDATE decision_traces SET replay_contract_version = COALESCE(replay_contract_version, '');")
	db.Exec("UPDATE decision_traces SET replay_digest = COALESCE(replay_digest, '');")

	createEventsTableSQL := `CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		incident_id TEXT,
		request_id TEXT DEFAULT '',
		event_type TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT 'system',
		reason_text TEXT NOT NULL DEFAULT '',
		payload_json TEXT NOT NULL DEFAULT '{}',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		summary TEXT DEFAULT '',
		reason TEXT DEFAULT '',
		pid INTEGER DEFAULT 0,
		cpu_score REAL DEFAULT 0.0,
		entropy_score REAL DEFAULT 0.0,
		confidence_score REAL DEFAULT 0.0
	);`
	if _, err := db.Exec(createEventsTableSQL); err != nil {
		return err
	}

	// Events table migrations for older installs.
	if err := ensureColumnExists("events", "event_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "run_id", "TEXT DEFAULT 'unknown-run'"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "incident_id", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "request_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "event_type", "TEXT DEFAULT 'legacy'"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "actor", "TEXT DEFAULT 'system'"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "reason_text", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	if err := ensureColumnExists("events", "payload_json", "TEXT DEFAULT '{}'"); err != nil {
		return err
	}
	// For ALTER TABLE, avoid CURRENT_TIMESTAMP default to preserve compatibility
	// with older SQLite engines and legacy DB files.
	if err := ensureColumnExists("events", "created_at", "DATETIME"); err != nil {
		return err
	}

	// Backfill required columns where possible.
	db.Exec("UPDATE events SET event_id = COALESCE(event_id, lower(hex(randomblob(16)))) WHERE event_id IS NULL OR event_id = '';")
	db.Exec("UPDATE events SET run_id = COALESCE(NULLIF(run_id, ''), 'unknown-run');")
	db.Exec("UPDATE events SET event_type = COALESCE(NULLIF(event_type, ''), COALESCE(type, 'legacy'));")
	db.Exec("UPDATE events SET reason_text = COALESCE(reason_text, reason, '');")
	db.Exec("UPDATE events SET payload_json = COALESCE(NULLIF(TRIM(payload_json), ''), '{}');")
	db.Exec("UPDATE events SET request_id = COALESCE(request_id, '');")
	db.Exec("UPDATE events SET created_at = COALESCE(created_at, timestamp, CURRENT_TIMESTAMP);")

	if _, err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_events_event_id ON events(event_id);"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_events_incident_created ON events(incident_id, created_at);"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_events_run_created ON events(run_id, created_at);"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_events_type_created ON events(event_type, created_at);"); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_events_request_created ON events(request_id, created_at);"); err != nil {
		return err
	}
	if err := ensureColumnExists("audit_events", "request_id", "TEXT DEFAULT ''"); err != nil {
		return err
	}
	db.Exec("UPDATE audit_events SET request_id = COALESCE(request_id, '');")
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_events_request_time ON audit_events(request_id, timestamp);"); err != nil {
		return err
	}
	if err := migrateLegacyRowsToUnifiedEvents(); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS trg_events_no_update
	BEFORE UPDATE ON events
	BEGIN
		SELECT RAISE(ABORT, 'events table is append-only');
	END;`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS trg_events_no_delete
	BEFORE DELETE ON events
	BEGIN
		SELECT RAISE(ABORT, 'events table is append-only');
	END;`); err != nil {
		return err
	}

	createIntegrationWorkspacesTableSQL := `CREATE TABLE IF NOT EXISTS integration_workspaces (
		workspace_id TEXT PRIMARY KEY,
		workspace_path TEXT NOT NULL,
		profile TEXT NOT NULL DEFAULT 'standard',
		client TEXT NOT NULL DEFAULT 'unknown',
		protection_enabled INTEGER NOT NULL DEFAULT 1,
		active_pid INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_updated DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createIntegrationWorkspacesTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_integration_workspaces_last_updated ON integration_workspaces(last_updated DESC);"); err != nil {
		return err
	}

	createIntegrationActionsTableSQL := `CREATE TABLE IF NOT EXISTS integration_actions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_id TEXT NOT NULL,
		action TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		audit_event_id INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createIntegrationActionsTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_integration_actions_workspace_created ON integration_actions(workspace_id, created_at DESC);"); err != nil {
		return err
	}

	createControlPlaneReplaysTableSQL := `CREATE TABLE IF NOT EXISTS control_plane_replays (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		idempotency_key TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		request_hash TEXT NOT NULL,
		response_status INTEGER NOT NULL,
		response_body TEXT NOT NULL,
		replay_count INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(idempotency_key, endpoint)
	);`
	if _, err := db.Exec(createControlPlaneReplaysTableSQL); err != nil {
		return err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_control_plane_replays_last_seen ON control_plane_replays(last_seen_at DESC);"); err != nil {
		return err
	}

	createSignalBaselineStateTableSQL := `CREATE TABLE IF NOT EXISTS decision_signal_baseline_state (
		bucket_key TEXT PRIMARY KEY,
		latest_trace_id INTEGER NOT NULL DEFAULT 0,
		consecutive_breach_count INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'healthy',
		last_transition_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_checked_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(createSignalBaselineStateTableSQL); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_signal_baseline_state", "latest_trace_id", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_signal_baseline_state", "consecutive_breach_count", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_signal_baseline_state", "status", "TEXT NOT NULL DEFAULT 'healthy'"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_signal_baseline_state", "last_transition_at", "DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP"); err != nil {
		return err
	}
	if err := ensureColumnExists("decision_signal_baseline_state", "last_checked_at", "DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP"); err != nil {
		return err
	}
	db.Exec("UPDATE decision_signal_baseline_state SET latest_trace_id = COALESCE(latest_trace_id, 0);")
	db.Exec("UPDATE decision_signal_baseline_state SET consecutive_breach_count = COALESCE(consecutive_breach_count, 0);")
	db.Exec("UPDATE decision_signal_baseline_state SET status = COALESCE(NULLIF(TRIM(status), ''), 'healthy');")
	db.Exec("UPDATE decision_signal_baseline_state SET last_transition_at = COALESCE(last_transition_at, CURRENT_TIMESTAMP);")
	db.Exec("UPDATE decision_signal_baseline_state SET last_checked_at = COALESCE(last_checked_at, CURRENT_TIMESTAMP);")
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_signal_baseline_state_last_checked ON decision_signal_baseline_state(last_checked_at DESC);"); err != nil {
		return err
	}

	return nil
}

func ensureColumnExists(tableName, columnName, columnDef string) error {
	exists, err := columnExists(tableName, columnName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", tableName, columnName, columnDef)
	if _, err := db.Exec(stmt); err != nil {
		return fmt.Errorf("add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func columnExists(tableName, columnName string) (bool, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", tableName)
	var count int
	if err := db.QueryRow(query, columnName).Scan(&count); err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}
	return count > 0, nil
}

func GetDB() *sql.DB {
	return db
}

func SetRunID(runID string) {
	runContext.mu.Lock()
	defer runContext.mu.Unlock()
	runContext.runID = runID
}

func currentRunID() string {
	runContext.mu.RLock()
	defer runContext.mu.RUnlock()
	if runContext.runID == "" {
		return "unknown-run"
	}
	return runContext.runID
}

func CloseDB() {
	if db != nil {
		db.Close()
	}
}

func LogIncident(command, modelName, exitReason string, maxCpu float64, pattern string, savings float64, tokenCount int, cost float64, agentID, agentVersion string) error {
	return LogIncidentWithDecision(command, modelName, exitReason, maxCpu, pattern, savings, tokenCount, cost, agentID, agentVersion, "", 0, 0, 0, "", 0)
}

func LogIncidentWithDecision(
	command, modelName, exitReason string,
	maxCpu float64,
	pattern string,
	savings float64,
	tokenCount int,
	cost float64,
	agentID, agentVersion string,
	reason string,
	cpuScore, entropyScore, confidenceScore float64,
	recoveryStatus string,
	restartCount int,
) error {
	return LogIncidentWithDecisionForIncident(
		command, modelName, exitReason, maxCpu, pattern, savings, tokenCount, cost, agentID, agentVersion,
		reason, cpuScore, entropyScore, confidenceScore, recoveryStatus, restartCount, "",
	)
}

func LogIncidentWithDecisionForIncident(
	command, modelName, exitReason string,
	maxCpu float64,
	pattern string,
	savings float64,
	tokenCount int,
	cost float64,
	agentID, agentVersion string,
	reason string,
	cpuScore, entropyScore, confidenceScore float64,
	recoveryStatus string,
	restartCount int,
	incidentID string,
) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	sanitizedCommand := sanitizePersistedText(command)
	sanitizedPattern := sanitizePersistedText(pattern)

	// Fail closed: incident persistence must never store raw sensitive command/pattern data.
	encCmd, err := encryption.Encrypt(sanitizedCommand)
	if err != nil {
		return fmt.Errorf("encrypt incident command: %w", err)
	}
	encPat, err := encryption.Encrypt(sanitizedPattern)
	if err != nil {
		return fmt.Errorf("encrypt incident pattern: %w", err)
	}

	stmt, err := db.Prepare("INSERT INTO incidents(command, model_name, exit_reason, max_cpu, pattern, token_savings_estimate, token_count, cost, agent_id, agent_version, reason, cpu_score, entropy_score, confidence_score, recovery_status, restart_count) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	result, err := stmt.Exec(encCmd, modelName, exitReason, maxCpu, encPat, savings, tokenCount, cost, agentID, agentVersion, reason, cpuScore, entropyScore, confidenceScore, recoveryStatus, restartCount)
	if err != nil {
		return err
	}
	insertedID, _ := result.LastInsertId()
	incidentID = normalizeIncidentID(incidentID)
	payload := incidentEventPayload{
		ID:                   int(insertedID),
		Command:              encCmd,
		ModelName:            modelName,
		ExitReason:           exitReason,
		MaxCPU:               maxCpu,
		Pattern:              encPat,
		TokenSavingsEstimate: savings,
		TokenCount:           tokenCount,
		Cost:                 cost,
		AgentID:              agentID,
		AgentVersion:         agentVersion,
		Reason:               reason,
		CPUScore:             cpuScore,
		EntropyScore:         entropyScore,
		ConfidenceScore:      confidenceScore,
		RecoveryStatus:       recoveryStatus,
		RestartCount:         restartCount,
	}
	return logUnifiedEventWithPayload("incident", exitReason, fmt.Sprintf("%s (CPU %.1f%%)", exitReason, maxCpu), reason, "system", incidentID, 0, cpuScore, entropyScore, confidenceScore, payload)
}

func GetIncidentByID(id int) (Incident, error) {
	var i Incident
	if db == nil {
		return i, fmt.Errorf("db missing")
	}

	row := db.QueryRow("SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, ''), COALESCE(reason, ''), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(recovery_status, ''), COALESCE(restart_count, 0) FROM incidents WHERE id = ?", id)
	err := row.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion, &i.Reason, &i.CPUScore, &i.EntropyScore, &i.ConfidenceScore, &i.RecoveryStatus, &i.RestartCount)

	if err == nil {
		i.Command = decryptIfPossible(i.Command)
		i.Pattern = decryptIfPossible(i.Pattern)
	}
	return i, err
}

func GetAllIncidents() ([]Incident, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}

	incidents, err := getIncidentsFromUnifiedEvents()
	if err == nil && len(incidents) > 0 {
		return incidents, nil
	}
	return getAllIncidentsLegacy()
}

func GetIncidentsPage(limit int, cursorID int64) ([]Incident, int64, bool, error) {
	if db == nil {
		return nil, 0, false, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 100
	}
	if cursorID < 0 {
		cursorID = 0
	}

	incidents, nextCursor, hasMore, err := getIncidentsFromUnifiedEventsPage(limit, cursorID)
	if err == nil && len(incidents) > 0 {
		return incidents, nextCursor, hasMore, nil
	}
	if err == nil && cursorID > 0 {
		return []Incident{}, 0, false, nil
	}

	legacy, legacyNextCursor, legacyHasMore, legacyErr := getLegacyIncidentsPage(limit, cursorID)
	if legacyErr != nil {
		if err != nil {
			return nil, 0, false, err
		}
		return nil, 0, false, legacyErr
	}
	return legacy, legacyNextCursor, legacyHasMore, nil
}

func getAllIncidentsLegacy() ([]Incident, error) {
	rows, err := db.Query("SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, ''), COALESCE(reason, ''), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(recovery_status, ''), COALESCE(restart_count, 0) FROM incidents ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Incident
	for rows.Next() {
		var i Incident
		if err := rows.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion, &i.Reason, &i.CPUScore, &i.EntropyScore, &i.ConfidenceScore, &i.RecoveryStatus, &i.RestartCount); err != nil {
			return nil, err
		}
		i.Command = decryptIfPossible(i.Command)
		i.Pattern = decryptIfPossible(i.Pattern)
		list = append(list, i)
	}
	return list, nil
}

func getLegacyIncidentsPage(limit int, cursorID int64) ([]Incident, int64, bool, error) {
	query := "SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, ''), COALESCE(reason, ''), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(recovery_status, ''), COALESCE(restart_count, 0) FROM incidents"
	args := []interface{}{}
	if cursorID > 0 {
		query += " WHERE id < ?"
		args = append(args, cursorID)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	list := make([]Incident, 0, limit+1)
	for rows.Next() {
		var i Incident
		if err := rows.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion, &i.Reason, &i.CPUScore, &i.EntropyScore, &i.ConfidenceScore, &i.RecoveryStatus, &i.RestartCount); err != nil {
			return nil, 0, false, err
		}
		i.Command = decryptIfPossible(i.Command)
		i.Pattern = decryptIfPossible(i.Pattern)
		list = append(list, i)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	var nextCursor int64
	if hasMore && len(list) > 0 {
		nextCursor = int64(list[len(list)-1].ID)
	}
	return list, nextCursor, hasMore, nil
}

func LogAuditEvent(actor, action, reason, source string, pid int, details string) error {
	_, err := LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source, pid, details, "", "")
	return err
}

func LogAuditEventWithRequestID(actor, action, reason, source string, pid int, details, requestID string) error {
	_, err := LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source, pid, details, "", requestID)
	return err
}

func LogAuditEventWithIncident(actor, action, reason, source string, pid int, details, incidentID string) error {
	_, err := LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source, pid, details, incidentID, "")
	return err
}

func LogAuditEventWithIncidentAndRequestID(actor, action, reason, source string, pid int, details, incidentID, requestID string) error {
	_, err := LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source, pid, details, incidentID, requestID)
	return err
}

func LogAuditEventWithIncidentAndID(actor, action, reason, source string, pid int, details, incidentID string) (int, error) {
	return LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source, pid, details, incidentID, "")
}

func LogAuditEventWithIncidentAndIDAndRequestID(actor, action, reason, source string, pid int, details, incidentID, requestID string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db not initialized")
	}
	details = sanitizePersistedText(details)
	stmt, err := db.Prepare("INSERT INTO audit_events(actor, action, reason, source, pid, details, request_id) VALUES(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	requestID = strings.TrimSpace(requestID)
	result, err := stmt.Exec(actor, action, reason, source, pid, details, requestID)
	if err != nil {
		return 0, err
	}
	insertedID, _ := result.LastInsertId()
	payload := auditEventPayload{
		ID:        int(insertedID),
		Source:    source,
		Details:   details,
		RequestID: requestID,
	}
	if err := logUnifiedEventWithPayloadAndRequestID("audit", action, fmt.Sprintf("%s by %s", action, actor), reason, actor, incidentID, pid, 0, 0, 0, requestID, payload); err != nil {
		return 0, err
	}
	return int(insertedID), nil
}

func GetAuditEvents(limit int) ([]AuditEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query("SELECT id, timestamp, COALESCE(actor, ''), COALESCE(action, ''), COALESCE(reason, ''), COALESCE(source, ''), COALESCE(pid, 0), COALESCE(details, ''), COALESCE(request_id, '') FROM audit_events ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.Action, &e.Reason, &e.Source, &e.PID, &e.Details, &e.RequestID); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func LogDecisionTrace(command string, pid int, cpuScore, entropyScore, confidenceScore float64, decision, reason string) error {
	return LogDecisionTraceWithIncidentAndMeta(command, pid, cpuScore, entropyScore, confidenceScore, decision, reason, "", DecisionTraceMeta{})
}

func LogDecisionTraceWithIncident(command string, pid int, cpuScore, entropyScore, confidenceScore float64, decision, reason, incidentID string) error {
	return LogDecisionTraceWithIncidentAndMeta(command, pid, cpuScore, entropyScore, confidenceScore, decision, reason, incidentID, DecisionTraceMeta{})
}

func LogDecisionTraceWithMeta(command string, pid int, cpuScore, entropyScore, confidenceScore float64, decision, reason string, meta DecisionTraceMeta) error {
	return LogDecisionTraceWithIncidentAndMeta(command, pid, cpuScore, entropyScore, confidenceScore, decision, reason, "", meta)
}

func LogDecisionTraceWithIncidentAndMeta(command string, pid int, cpuScore, entropyScore, confidenceScore float64, decision, reason, incidentID string, meta DecisionTraceMeta) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	command = sanitizePersistedText(command)
	stmt, err := db.Prepare("INSERT INTO decision_traces(command, pid, cpu_score, entropy_score, confidence_score, decision, reason, decision_engine, engine_version, decision_contract_version, rollout_mode, replay_contract_version, replay_digest) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	result, err := stmt.Exec(
		command,
		pid,
		cpuScore,
		entropyScore,
		confidenceScore,
		decision,
		reason,
		strings.TrimSpace(meta.DecisionEngine),
		strings.TrimSpace(meta.EngineVersion),
		strings.TrimSpace(meta.DecisionContract),
		strings.TrimSpace(meta.PolicyRolloutMode),
		strings.TrimSpace(meta.ReplayContract),
		strings.TrimSpace(meta.ReplayDigest),
	)
	if err != nil {
		return err
	}
	insertedID, _ := result.LastInsertId()
	summary := fmt.Sprintf("CPU %.1f / Entropy %.1f / Confidence %.1f", cpuScore, entropyScore, confidenceScore)
	payload := decisionEventPayload{
		ID:                int(insertedID),
		Command:           command,
		DecisionEngine:    strings.TrimSpace(meta.DecisionEngine),
		EngineVersion:     strings.TrimSpace(meta.EngineVersion),
		DecisionContract:  strings.TrimSpace(meta.DecisionContract),
		PolicyRolloutMode: strings.TrimSpace(meta.PolicyRolloutMode),
		ReplayContract:    strings.TrimSpace(meta.ReplayContract),
		ReplayDigest:      strings.TrimSpace(meta.ReplayDigest),
	}
	return logUnifiedEventWithPayload("decision", decision, summary, reason, "system", incidentID, pid, cpuScore, entropyScore, confidenceScore, payload)
}

func GetDecisionTraces(limit int) ([]DecisionTrace, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query("SELECT id, timestamp, COALESCE(command, ''), COALESCE(pid, 0), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(decision, ''), COALESCE(reason, ''), COALESCE(decision_engine, ''), COALESCE(engine_version, ''), COALESCE(decision_contract_version, ''), COALESCE(rollout_mode, ''), COALESCE(replay_contract_version, ''), COALESCE(replay_digest, '') FROM decision_traces ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []DecisionTrace
	for rows.Next() {
		var t DecisionTrace
		if err := rows.Scan(
			&t.ID,
			&t.Timestamp,
			&t.Command,
			&t.PID,
			&t.CPUScore,
			&t.EntropyScore,
			&t.ConfidenceScore,
			&t.Decision,
			&t.Reason,
			&t.DecisionEngine,
			&t.DecisionEngineVersion,
			&t.DecisionContract,
			&t.PolicyRolloutMode,
			&t.ReplayContract,
			&t.ReplayDigest,
		); err != nil {
			return nil, err
		}
		traces = append(traces, t)
	}
	return traces, nil
}

func GetDecisionTraceByID(id int) (DecisionTrace, error) {
	if db == nil {
		return DecisionTrace{}, fmt.Errorf("db missing")
	}
	if id <= 0 {
		return DecisionTrace{}, fmt.Errorf("id must be > 0")
	}

	var t DecisionTrace
	err := db.QueryRow(`SELECT
	id,
	timestamp,
	COALESCE(command, ''),
	COALESCE(pid, 0),
	COALESCE(cpu_score, 0.0),
	COALESCE(entropy_score, 0.0),
	COALESCE(confidence_score, 0.0),
	COALESCE(decision, ''),
	COALESCE(reason, ''),
	COALESCE(decision_engine, ''),
	COALESCE(engine_version, ''),
	COALESCE(decision_contract_version, ''),
	COALESCE(rollout_mode, ''),
	COALESCE(replay_contract_version, ''),
	COALESCE(replay_digest, '')
FROM decision_traces
WHERE id = ?`, id).Scan(
		&t.ID,
		&t.Timestamp,
		&t.Command,
		&t.PID,
		&t.CPUScore,
		&t.EntropyScore,
		&t.ConfidenceScore,
		&t.Decision,
		&t.Reason,
		&t.DecisionEngine,
		&t.DecisionEngineVersion,
		&t.DecisionContract,
		&t.PolicyRolloutMode,
		&t.ReplayContract,
		&t.ReplayDigest,
	)
	if err != nil {
		return DecisionTrace{}, err
	}
	return t, nil
}

func GetTimeline(limit int) ([]TimelineEvent, error) {
	events, err := GetUnifiedEvents(limit)
	if err == nil && len(events) > 0 {
		out := make([]TimelineEvent, 0, len(events))
		for _, e := range events {
			out = append(out, timelineEventFromUnifiedEvent(e))
		}
		return out, nil
	}
	return getLegacyTimeline(limit)
}

func GetTimelinePage(limit int, cursorID int64) ([]TimelineEvent, int64, bool, error) {
	events, nextCursor, hasMore, err := GetUnifiedEventsPage(limit, cursorID)
	if err == nil && len(events) > 0 {
		out := make([]TimelineEvent, 0, len(events))
		for _, e := range events {
			out = append(out, timelineEventFromUnifiedEvent(e))
		}
		return out, nextCursor, hasMore, nil
	}

	legacy, legacyErr := getLegacyTimeline(limit)
	if legacyErr != nil {
		if err != nil {
			return nil, 0, false, err
		}
		return nil, 0, false, legacyErr
	}
	if cursorID > 0 {
		return []TimelineEvent{}, 0, false, nil
	}
	return legacy, 0, false, nil
}

func timelineEventFromUnifiedEvent(e UnifiedEvent) TimelineEvent {
	return TimelineEvent{
		EventID:               e.EventID,
		RunID:                 e.RunID,
		IncidentID:            e.IncidentID,
		RequestID:             e.RequestID,
		Type:                  e.EventType,
		Timestamp:             e.CreatedAt,
		Title:                 e.Title,
		Summary:               e.Summary,
		Reason:                e.ReasonText,
		Actor:                 e.Actor,
		PID:                   e.PID,
		CPUScore:              e.CPUScore,
		Entropy:               e.Entropy,
		Confidence:            e.Confidence,
		DecisionEngine:        e.DecisionEngine,
		DecisionEngineVersion: e.DecisionEngineVersion,
		DecisionContract:      e.DecisionContract,
		PolicyRolloutMode:     e.PolicyRolloutMode,
		ReplayContract:        e.ReplayContract,
		ReplayDigest:          e.ReplayDigest,
		Evidence:              e.Evidence,
	}
}

func getLegacyTimeline(limit int) ([]TimelineEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	incidents, err := GetAllIncidents()
	if err != nil {
		return nil, err
	}
	audits, err := GetAuditEvents(limit)
	if err != nil {
		return nil, err
	}
	traces, err := GetDecisionTraces(limit)
	if err != nil {
		return nil, err
	}

	type timelineRecord struct {
		at    time.Time
		event TimelineEvent
	}
	records := make([]timelineRecord, 0, len(incidents)+len(audits)+len(traces))

	for _, inc := range incidents {
		records = append(records, timelineRecord{
			at: parseTimestamp(inc.Timestamp),
			event: TimelineEvent{
				Type:       "incident",
				Timestamp:  inc.Timestamp,
				Title:      inc.ExitReason,
				Summary:    fmt.Sprintf("%s (CPU %.1f%%)", inc.ExitReason, inc.MaxCPU),
				Reason:     inc.Reason,
				CPUScore:   inc.CPUScore,
				Entropy:    inc.EntropyScore,
				Confidence: inc.ConfidenceScore,
			},
		})
	}

	for _, a := range audits {
		records = append(records, timelineRecord{
			at: parseTimestamp(a.Timestamp),
			event: TimelineEvent{
				Type:      "audit",
				Timestamp: a.Timestamp,
				Title:     a.Action,
				Summary:   fmt.Sprintf("%s by %s", a.Action, a.Actor),
				Reason:    a.Reason,
				PID:       a.PID,
				RequestID: a.RequestID,
			},
		})
	}

	for _, t := range traces {
		records = append(records, timelineRecord{
			at: parseTimestamp(t.Timestamp),
			event: TimelineEvent{
				Type:                  "decision",
				Timestamp:             t.Timestamp,
				Title:                 t.Decision,
				Summary:               fmt.Sprintf("CPU %.1f / Entropy %.1f / Confidence %.1f", t.CPUScore, t.EntropyScore, t.ConfidenceScore),
				Reason:                t.Reason,
				PID:                   t.PID,
				CPUScore:              t.CPUScore,
				Entropy:               t.EntropyScore,
				Confidence:            t.ConfidenceScore,
				DecisionEngine:        t.DecisionEngine,
				DecisionEngineVersion: t.DecisionEngineVersion,
				DecisionContract:      t.DecisionContract,
				PolicyRolloutMode:     t.PolicyRolloutMode,
				ReplayContract:        t.ReplayContract,
				ReplayDigest:          t.ReplayDigest,
			},
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].at.After(records[j].at)
	})

	if len(records) > limit {
		records = records[:limit]
	}

	out := make([]TimelineEvent, 0, len(records))
	for _, r := range records {
		out = append(out, r.event)
	}
	return out, nil
}

func GetUnifiedEvents(limit int) ([]UnifiedEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
SELECT
	id,
	COALESCE(event_id, ''),
	COALESCE(run_id, ''),
	COALESCE(incident_id, ''),
	COALESCE(request_id, ''),
	COALESCE(event_type, type, ''),
	COALESCE(actor, 'system'),
	COALESCE(reason_text, reason, ''),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(event_type, type, ''),
	COALESCE(title, ''),
	COALESCE(summary, ''),
	COALESCE(reason_text, reason, ''),
	COALESCE(pid, 0),
	COALESCE(cpu_score, 0.0),
	COALESCE(entropy_score, 0.0),
	COALESCE(confidence_score, 0.0),
	COALESCE(payload_json, '{}')
FROM events
ORDER BY created_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []UnifiedEvent
	for rows.Next() {
		var e UnifiedEvent
		var payloadRaw string
		if err := rows.Scan(
			&e.ID,
			&e.EventID,
			&e.RunID,
			&e.IncidentID,
			&e.RequestID,
			&e.EventType,
			&e.Actor,
			&e.ReasonText,
			&e.CreatedAt,
			&e.Timestamp,
			&e.Type,
			&e.Title,
			&e.Summary,
			&e.Reason,
			&e.PID,
			&e.CPUScore,
			&e.Entropy,
			&e.Confidence,
			&payloadRaw,
		); err != nil {
			return nil, err
		}
		e.Evidence = parseEvidencePayload(payloadRaw)
		hydrateDecisionMetadataFromEvidence(&e)
		list = append(list, e)
	}
	return list, nil
}

func GetUnifiedEventsPage(limit int, cursorID int64) ([]UnifiedEvent, int64, bool, error) {
	if db == nil {
		return nil, 0, false, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 50
	}

	query := `
SELECT
	id,
	COALESCE(event_id, ''),
	COALESCE(run_id, ''),
	COALESCE(incident_id, ''),
	COALESCE(request_id, ''),
	COALESCE(event_type, type, ''),
	COALESCE(actor, 'system'),
	COALESCE(reason_text, reason, ''),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(event_type, type, ''),
	COALESCE(title, ''),
	COALESCE(summary, ''),
	COALESCE(reason_text, reason, ''),
	COALESCE(pid, 0),
	COALESCE(cpu_score, 0.0),
	COALESCE(entropy_score, 0.0),
	COALESCE(confidence_score, 0.0),
	COALESCE(payload_json, '{}')
FROM events`
	args := []interface{}{}
	if cursorID > 0 {
		query += "\nWHERE id < ?"
		args = append(args, cursorID)
	}
	query += "\nORDER BY id DESC\nLIMIT ?"
	args = append(args, limit+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	list := make([]UnifiedEvent, 0, limit+1)
	for rows.Next() {
		var e UnifiedEvent
		var payloadRaw string
		if err := rows.Scan(
			&e.ID,
			&e.EventID,
			&e.RunID,
			&e.IncidentID,
			&e.RequestID,
			&e.EventType,
			&e.Actor,
			&e.ReasonText,
			&e.CreatedAt,
			&e.Timestamp,
			&e.Type,
			&e.Title,
			&e.Summary,
			&e.Reason,
			&e.PID,
			&e.CPUScore,
			&e.Entropy,
			&e.Confidence,
			&payloadRaw,
		); err != nil {
			return nil, 0, false, err
		}
		e.Evidence = parseEvidencePayload(payloadRaw)
		hydrateDecisionMetadataFromEvidence(&e)
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	hasMore := len(list) > limit
	if hasMore {
		list = list[:limit]
	}
	var nextCursor int64
	if hasMore && len(list) > 0 {
		nextCursor = int64(list[len(list)-1].ID)
	}
	return list, nextCursor, hasMore, nil
}

func GetIncidentTimelineByIncidentID(incidentID string, limit int) ([]UnifiedEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if incidentID == "" {
		return nil, fmt.Errorf("incident_id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	const incidentTimelineSQL = `
SELECT
	id,
	COALESCE(event_id, ''),
	COALESCE(run_id, ''),
	COALESCE(incident_id, ''),
	COALESCE(request_id, ''),
	COALESCE(event_type, type, ''),
	COALESCE(actor, 'system'),
	COALESCE(reason_text, reason, ''),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP) AS created_at,
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP) AS ts,
	COALESCE(event_type, type, '') AS event_type_legacy,
	COALESCE(title, '') AS title,
	COALESCE(summary, '') AS summary,
	COALESCE(reason_text, reason, '') AS reason_text_legacy,
	COALESCE(pid, 0) AS pid,
	COALESCE(cpu_score, 0.0) AS cpu_score,
	COALESCE(entropy_score, 0.0) AS entropy_score,
	COALESCE(confidence_score, 0.0) AS confidence_score,
	COALESCE(payload_json, '{}') AS payload_json
FROM events
WHERE incident_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?;
`
	rows, err := db.Query(incidentTimelineSQL, incidentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UnifiedEvent, 0)
	for rows.Next() {
		var e UnifiedEvent
		var payloadRaw string
		if err := rows.Scan(
			&e.ID,
			&e.EventID,
			&e.RunID,
			&e.IncidentID,
			&e.RequestID,
			&e.EventType,
			&e.Actor,
			&e.ReasonText,
			&e.CreatedAt,
			&e.Timestamp,
			&e.Type,
			&e.Title,
			&e.Summary,
			&e.Reason,
			&e.PID,
			&e.CPUScore,
			&e.Entropy,
			&e.Confidence,
			&payloadRaw,
		); err != nil {
			return nil, err
		}
		e.Evidence = parseEvidencePayload(payloadRaw)
		hydrateDecisionMetadataFromEvidence(&e)
		out = append(out, e)
	}
	return out, nil
}

func GetUnifiedEventsByRequestID(requestID string, limit int) ([]UnifiedEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	const requestTraceSQL = `
SELECT
	id,
	COALESCE(event_id, ''),
	COALESCE(run_id, ''),
	COALESCE(incident_id, ''),
	COALESCE(request_id, ''),
	COALESCE(event_type, type, ''),
	COALESCE(actor, 'system'),
	COALESCE(reason_text, reason, ''),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP),
	COALESCE(event_type, type, ''),
	COALESCE(title, ''),
	COALESCE(summary, ''),
	COALESCE(reason_text, reason, ''),
	COALESCE(pid, 0),
	COALESCE(cpu_score, 0.0),
	COALESCE(entropy_score, 0.0),
	COALESCE(confidence_score, 0.0),
	COALESCE(payload_json, '{}')
FROM events
WHERE request_id = ?
ORDER BY created_at ASC, id ASC
LIMIT ?;
`
	rows, err := db.Query(requestTraceSQL, requestID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]UnifiedEvent, 0)
	for rows.Next() {
		var e UnifiedEvent
		var payloadRaw string
		if err := rows.Scan(
			&e.ID,
			&e.EventID,
			&e.RunID,
			&e.IncidentID,
			&e.RequestID,
			&e.EventType,
			&e.Actor,
			&e.ReasonText,
			&e.CreatedAt,
			&e.Timestamp,
			&e.Type,
			&e.Title,
			&e.Summary,
			&e.Reason,
			&e.PID,
			&e.CPUScore,
			&e.Entropy,
			&e.Confidence,
			&payloadRaw,
		); err != nil {
			return nil, err
		}
		e.Evidence = parseEvidencePayload(payloadRaw)
		hydrateDecisionMetadataFromEvidence(&e)
		out = append(out, e)
	}
	return out, nil
}

func LogPolicyDryRun(command string, pid int, reason string, confidenceScore float64) error {
	return LogPolicyDryRunWithIncident(command, pid, reason, confidenceScore, "")
}

func LogPolicyDryRunWithIncident(command string, pid int, reason string, confidenceScore float64, incidentID string) error {
	command = sanitizePersistedText(command)
	summary := fmt.Sprintf("Dry-run for %s", command)
	return logUnifiedEventWithMeta("policy_dry_run", "POLICY_DRY_RUN", summary, reason, "system", incidentID, pid, 0, 0, confidenceScore)
}

func logUnifiedEventWithMeta(eventType, title, summary, reason, actor, incidentID string, pid int, cpuScore, entropyScore, confidenceScore float64) error {
	return logUnifiedEventWithPayload(eventType, title, summary, reason, actor, incidentID, pid, cpuScore, entropyScore, confidenceScore, nil)
}

func logUnifiedEventWithPayload(eventType, title, summary, reason, actor, incidentID string, pid int, cpuScore, entropyScore, confidenceScore float64, payload any) error {
	return logUnifiedEventWithPayloadAndRequestID(eventType, title, summary, reason, actor, incidentID, pid, cpuScore, entropyScore, confidenceScore, "", payload)
}

func logUnifiedEventWithPayloadAndRequestID(eventType, title, summary, reason, actor, incidentID string, pid int, cpuScore, entropyScore, confidenceScore float64, requestID string, payload any) error {
	_, err := InsertEventWithPayloadAndRequestID(eventType, actor, reason, currentRunID(), incidentID, title, summary, pid, cpuScore, entropyScore, confidenceScore, requestID, payload)
	return err
}

func InsertEvent(eventType, actor, reasonText, runID, incidentID, title, summary string, pid int, cpuScore, entropyScore, confidenceScore float64) (string, error) {
	return InsertEventWithPayloadAndRequestID(eventType, actor, reasonText, runID, incidentID, title, summary, pid, cpuScore, entropyScore, confidenceScore, "", nil)
}

func InsertEventWithPayload(eventType, actor, reasonText, runID, incidentID, title, summary string, pid int, cpuScore, entropyScore, confidenceScore float64, payload any) (string, error) {
	return InsertEventWithPayloadAndRequestID(eventType, actor, reasonText, runID, incidentID, title, summary, pid, cpuScore, entropyScore, confidenceScore, "", payload)
}

func InsertEventWithPayloadAndRequestID(eventType, actor, reasonText, runID, incidentID, title, summary string, pid int, cpuScore, entropyScore, confidenceScore float64, requestID string, payload any) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db not initialized")
	}
	payloadJSON, err := marshalPayload(payload)
	if err != nil {
		return "", err
	}
	stmt, err := db.Prepare(`
INSERT INTO events(
	event_id, run_id, incident_id, request_id, event_type, actor, reason_text, created_at,
	payload_json, timestamp, type, title, summary, reason, pid, cpu_score, entropy_score, confidence_score
) VALUES(?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return "", err
	}
	defer stmt.Close()

	if eventType == "" {
		eventType = "unknown"
	}
	if actor == "" {
		actor = "system"
	}
	eventID := uuid.NewString()
	if runID == "" {
		runID = "unknown-run"
	}
	incidentID = strings.TrimSpace(incidentID)
	var incidentIDValue interface{}
	if incidentID == "" {
		incidentIDValue = nil
	} else {
		incidentIDValue = incidentID
	}
	requestID = strings.TrimSpace(requestID)
	var requestIDValue interface{}
	if requestID == "" {
		requestIDValue = nil
	} else {
		requestIDValue = requestID
	}

	_, err = stmt.Exec(
		eventID,
		runID,
		incidentIDValue,
		requestIDValue,
		eventType,
		actor,
		reasonText,
		payloadJSON,
		eventType,
		title,
		summary,
		reasonText,
		pid,
		cpuScore,
		entropyScore,
		confidenceScore,
	)
	if err != nil {
		return "", err
	}
	return eventID, nil
}

func marshalPayload(payload any) (string, error) {
	if payload == nil {
		return "{}", nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "{}", nil
	}
	return string(b), nil
}

func parseEvidencePayload(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func evidenceStringValue(evidence map[string]interface{}, key string) string {
	if evidence == nil {
		return ""
	}
	raw, ok := evidence[key]
	if !ok {
		return ""
	}
	v, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(v)
}

func hydrateDecisionMetadataFromEvidence(event *UnifiedEvent) {
	if event == nil || event.Evidence == nil {
		return
	}
	if event.DecisionEngine == "" {
		event.DecisionEngine = evidenceStringValue(event.Evidence, "decision_engine")
	}
	if event.DecisionEngineVersion == "" {
		event.DecisionEngineVersion = evidenceStringValue(event.Evidence, "engine_version")
	}
	if event.DecisionContract == "" {
		event.DecisionContract = evidenceStringValue(event.Evidence, "decision_contract_version")
	}
	if event.PolicyRolloutMode == "" {
		event.PolicyRolloutMode = evidenceStringValue(event.Evidence, "rollout_mode")
	}
	if event.ReplayContract == "" {
		event.ReplayContract = evidenceStringValue(event.Evidence, "replay_contract_version")
	}
	if event.ReplayDigest == "" {
		event.ReplayDigest = evidenceStringValue(event.Evidence, "replay_digest")
	}
}

func decryptIfPossible(value string) string {
	if value == "" {
		return value
	}
	if dec, err := encryption.Decrypt(value); err == nil {
		return dec
	}
	return value
}

func sanitizePersistedText(value string) string {
	return redact.Line(value)
}

func getIncidentsFromUnifiedEvents() ([]Incident, error) {
	rows, err := db.Query(`
SELECT
	COALESCE(payload_json, '{}'),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP)
FROM events
WHERE event_type = 'incident'
ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	incidents := make([]Incident, 0)
	for rows.Next() {
		var payloadRaw, ts string
		if err := rows.Scan(&payloadRaw, &ts); err != nil {
			return nil, err
		}
		payloadRaw = strings.TrimSpace(payloadRaw)
		if payloadRaw == "" || payloadRaw == "{}" {
			continue
		}

		var payload incidentEventPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.ExitReason) == "" {
			continue
		}

		incidents = append(incidents, Incident{
			ID:                   payload.ID,
			Timestamp:            ts,
			Command:              decryptIfPossible(payload.Command),
			ModelName:            payload.ModelName,
			ExitReason:           payload.ExitReason,
			MaxCPU:               payload.MaxCPU,
			Pattern:              decryptIfPossible(payload.Pattern),
			TokenSavingsEstimate: payload.TokenSavingsEstimate,
			TokenCount:           payload.TokenCount,
			Cost:                 payload.Cost,
			AgentID:              payload.AgentID,
			AgentVersion:         payload.AgentVersion,
			Reason:               payload.Reason,
			CPUScore:             payload.CPUScore,
			EntropyScore:         payload.EntropyScore,
			ConfidenceScore:      payload.ConfidenceScore,
			RecoveryStatus:       payload.RecoveryStatus,
			RestartCount:         payload.RestartCount,
		})
	}
	return incidents, nil
}

func getIncidentsFromUnifiedEventsPage(limit int, cursorID int64) ([]Incident, int64, bool, error) {
	query := `
SELECT
	id,
	COALESCE(payload_json, '{}'),
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP)
FROM events
WHERE event_type = 'incident'`
	args := []interface{}{}
	if cursorID > 0 {
		query += "\n  AND id < ?"
		args = append(args, cursorID)
	}
	query += "\nORDER BY id DESC\nLIMIT ?"
	args = append(args, limit+1)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	incidents := make([]Incident, 0, limit+1)
	cursorCandidates := make([]int64, 0, limit+1)
	for rows.Next() {
		var rowID int64
		var payloadRaw, ts string
		if err := rows.Scan(&rowID, &payloadRaw, &ts); err != nil {
			return nil, 0, false, err
		}
		payloadRaw = strings.TrimSpace(payloadRaw)
		if payloadRaw == "" || payloadRaw == "{}" {
			continue
		}

		var payload incidentEventPayload
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			continue
		}
		if strings.TrimSpace(payload.ExitReason) == "" {
			continue
		}

		incidentID := payload.ID
		if incidentID <= 0 {
			incidentID = int(rowID)
		}
		incidents = append(incidents, Incident{
			ID:                   incidentID,
			Timestamp:            ts,
			Command:              decryptIfPossible(payload.Command),
			ModelName:            payload.ModelName,
			ExitReason:           payload.ExitReason,
			MaxCPU:               payload.MaxCPU,
			Pattern:              decryptIfPossible(payload.Pattern),
			TokenSavingsEstimate: payload.TokenSavingsEstimate,
			TokenCount:           payload.TokenCount,
			Cost:                 payload.Cost,
			AgentID:              payload.AgentID,
			AgentVersion:         payload.AgentVersion,
			Reason:               payload.Reason,
			CPUScore:             payload.CPUScore,
			EntropyScore:         payload.EntropyScore,
			ConfidenceScore:      payload.ConfidenceScore,
			RecoveryStatus:       payload.RecoveryStatus,
			RestartCount:         payload.RestartCount,
		})
		cursorCandidates = append(cursorCandidates, rowID)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	hasMore := len(incidents) > limit
	if hasMore {
		incidents = incidents[:limit]
		cursorCandidates = cursorCandidates[:limit]
	}
	var nextCursor int64
	if hasMore && len(cursorCandidates) > 0 {
		nextCursor = cursorCandidates[len(cursorCandidates)-1]
	}
	return incidents, nextCursor, hasMore, nil
}

func migrateLegacyRowsToUnifiedEvents() error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	if err := backfillLegacyIncidents(); err != nil {
		return err
	}
	if err := backfillLegacyAudits(); err != nil {
		return err
	}
	if err := backfillLegacyDecisions(); err != nil {
		return err
	}
	return nil
}

func countRows(table string) (int, error) {
	query := fmt.Sprintf("SELECT COUNT(1) FROM %s", table)
	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func countEventTypeRows(eventType string) (int, error) {
	var count int
	if err := db.QueryRow("SELECT COUNT(1) FROM events WHERE event_type = ?", eventType).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func backfillLegacyIncidents() error {
	legacyCount, err := countRows("incidents")
	if err != nil || legacyCount == 0 {
		return err
	}
	unifiedCount, err := countEventTypeRows("incident")
	if err != nil || unifiedCount > 0 {
		return err
	}

	rows, err := db.Query(`SELECT id, timestamp, command, COALESCE(model_name, ''), COALESCE(exit_reason, ''), COALESCE(max_cpu, 0.0), COALESCE(pattern, ''), COALESCE(token_savings_estimate, 0.0), COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, ''), COALESCE(reason, ''), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(recovery_status, ''), COALESCE(restart_count, 0) FROM incidents ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	incidents := make([]Incident, 0, legacyCount)
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(
			&inc.ID,
			&inc.Timestamp,
			&inc.Command,
			&inc.ModelName,
			&inc.ExitReason,
			&inc.MaxCPU,
			&inc.Pattern,
			&inc.TokenSavingsEstimate,
			&inc.TokenCount,
			&inc.Cost,
			&inc.AgentID,
			&inc.AgentVersion,
			&inc.Reason,
			&inc.CPUScore,
			&inc.EntropyScore,
			&inc.ConfidenceScore,
			&inc.RecoveryStatus,
			&inc.RestartCount,
		); err != nil {
			return err
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, inc := range incidents {
		payload := incidentEventPayload{
			ID:                   inc.ID,
			Command:              inc.Command,
			ModelName:            inc.ModelName,
			ExitReason:           inc.ExitReason,
			MaxCPU:               inc.MaxCPU,
			Pattern:              inc.Pattern,
			TokenSavingsEstimate: inc.TokenSavingsEstimate,
			TokenCount:           inc.TokenCount,
			Cost:                 inc.Cost,
			AgentID:              inc.AgentID,
			AgentVersion:         inc.AgentVersion,
			Reason:               inc.Reason,
			CPUScore:             inc.CPUScore,
			EntropyScore:         inc.EntropyScore,
			ConfidenceScore:      inc.ConfidenceScore,
			RecoveryStatus:       inc.RecoveryStatus,
			RestartCount:         inc.RestartCount,
		}
		payloadJSON, err := marshalPayload(payload)
		if err != nil {
			return err
		}
		eventID := fmt.Sprintf("legacy-incident-%d", inc.ID)
		incidentID := eventID
		if err := insertLegacyUnifiedEvent(
			eventID,
			"unknown-run",
			incidentID,
			"",
			"incident",
			"system",
			inc.Reason,
			inc.Timestamp,
			inc.ExitReason,
			fmt.Sprintf("%s (CPU %.1f%%)", inc.ExitReason, inc.MaxCPU),
			inc.Reason,
			0,
			inc.CPUScore,
			inc.EntropyScore,
			inc.ConfidenceScore,
			payloadJSON,
		); err != nil {
			return err
		}
	}
	return nil
}

func backfillLegacyAudits() error {
	legacyCount, err := countRows("audit_events")
	if err != nil || legacyCount == 0 {
		return err
	}
	unifiedCount, err := countEventTypeRows("audit")
	if err != nil || unifiedCount > 0 {
		return err
	}

	rows, err := db.Query(`SELECT id, timestamp, COALESCE(actor, ''), COALESCE(action, ''), COALESCE(reason, ''), COALESCE(source, ''), COALESCE(pid, 0), COALESCE(details, ''), COALESCE(request_id, '') FROM audit_events ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	audits := make([]AuditEvent, 0, legacyCount)
	for rows.Next() {
		var a AuditEvent
		if err := rows.Scan(&a.ID, &a.Timestamp, &a.Actor, &a.Action, &a.Reason, &a.Source, &a.PID, &a.Details, &a.RequestID); err != nil {
			return err
		}
		audits = append(audits, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, a := range audits {
		payloadJSON, err := marshalPayload(auditEventPayload{
			ID:        a.ID,
			Source:    a.Source,
			Details:   a.Details,
			RequestID: a.RequestID,
		})
		if err != nil {
			return err
		}
		eventID := fmt.Sprintf("legacy-audit-%d", a.ID)
		if err := insertLegacyUnifiedEvent(
			eventID,
			"unknown-run",
			"",
			a.RequestID,
			"audit",
			a.Actor,
			a.Reason,
			a.Timestamp,
			a.Action,
			fmt.Sprintf("%s by %s", a.Action, a.Actor),
			a.Reason,
			a.PID,
			0,
			0,
			0,
			payloadJSON,
		); err != nil {
			return err
		}
	}
	return nil
}

func backfillLegacyDecisions() error {
	legacyCount, err := countRows("decision_traces")
	if err != nil || legacyCount == 0 {
		return err
	}
	unifiedCount, err := countEventTypeRows("decision")
	if err != nil || unifiedCount > 0 {
		return err
	}

	rows, err := db.Query(`SELECT id, timestamp, COALESCE(command, ''), COALESCE(pid, 0), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(decision, ''), COALESCE(reason, ''), COALESCE(decision_engine, ''), COALESCE(engine_version, ''), COALESCE(decision_contract_version, ''), COALESCE(rollout_mode, ''), COALESCE(replay_contract_version, ''), COALESCE(replay_digest, '') FROM decision_traces ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	decisions := make([]DecisionTrace, 0, legacyCount)
	for rows.Next() {
		var d DecisionTrace
		if err := rows.Scan(
			&d.ID,
			&d.Timestamp,
			&d.Command,
			&d.PID,
			&d.CPUScore,
			&d.EntropyScore,
			&d.ConfidenceScore,
			&d.Decision,
			&d.Reason,
			&d.DecisionEngine,
			&d.DecisionEngineVersion,
			&d.DecisionContract,
			&d.PolicyRolloutMode,
			&d.ReplayContract,
			&d.ReplayDigest,
		); err != nil {
			return err
		}
		decisions = append(decisions, d)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, d := range decisions {
		payloadJSON, err := marshalPayload(decisionEventPayload{
			ID:                d.ID,
			Command:           d.Command,
			DecisionEngine:    withDefault(strings.TrimSpace(d.DecisionEngine), "legacy-decider"),
			EngineVersion:     withDefault(strings.TrimSpace(d.DecisionEngineVersion), "legacy-unknown"),
			DecisionContract:  withDefault(strings.TrimSpace(d.DecisionContract), "legacy-decision-trace"),
			PolicyRolloutMode: strings.TrimSpace(d.PolicyRolloutMode),
			ReplayContract:    withDefault(strings.TrimSpace(d.ReplayContract), "legacy-decision-replay"),
			ReplayDigest:      strings.TrimSpace(d.ReplayDigest),
		})
		if err != nil {
			return err
		}
		eventID := fmt.Sprintf("legacy-decision-%d", d.ID)
		if err := insertLegacyUnifiedEvent(
			eventID,
			"unknown-run",
			"",
			"",
			"decision",
			"system",
			d.Reason,
			d.Timestamp,
			d.Decision,
			fmt.Sprintf("CPU %.1f / Entropy %.1f / Confidence %.1f", d.CPUScore, d.EntropyScore, d.ConfidenceScore),
			d.Reason,
			d.PID,
			d.CPUScore,
			d.EntropyScore,
			d.ConfidenceScore,
			payloadJSON,
		); err != nil {
			return err
		}
	}
	return nil
}

func insertLegacyUnifiedEvent(eventID, runID, incidentID, requestID, eventType, actor, reasonText, createdAt, title, summary, reason string, pid int, cpuScore, entropyScore, confidenceScore float64, payloadJSON string) error {
	incidentID = strings.TrimSpace(incidentID)
	var incidentIDValue interface{}
	if incidentID == "" {
		incidentIDValue = nil
	} else {
		incidentIDValue = incidentID
	}
	requestID = strings.TrimSpace(requestID)
	var requestIDValue interface{}
	if requestID == "" {
		requestIDValue = nil
	} else {
		requestIDValue = requestID
	}
	_, err := db.Exec(`
INSERT OR IGNORE INTO events(
	event_id, run_id, incident_id, request_id, event_type, actor, reason_text, created_at,
	payload_json, timestamp, type, title, summary, reason, pid, cpu_score, entropy_score, confidence_score
) VALUES(?, ?, ?, ?, ?, ?, ?, COALESCE(NULLIF(?, ''), CURRENT_TIMESTAMP), ?, COALESCE(NULLIF(?, ''), CURRENT_TIMESTAMP), ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID,
		runID,
		incidentIDValue,
		requestIDValue,
		eventType,
		actor,
		reasonText,
		createdAt,
		payloadJSON,
		createdAt,
		eventType,
		title,
		summary,
		reason,
		pid,
		cpuScore,
		entropyScore,
		confidenceScore,
	)
	return err
}

func normalizeIncidentID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return uuid.NewString()
	}
	return id
}

func withDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func parseTimestamp(raw string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func PruneIncidents(days int) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("db missing")
	}

	result, err := db.Exec("DELETE FROM incidents WHERE timestamp < datetime('now', ?)", fmt.Sprintf("-%d days", days))
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()

	// Optimize DB to reclaim space
	_, err = db.Exec("VACUUM")
	if err != nil {
		return rowsAffected, fmt.Errorf("vacuum failed: %v", err)
	}

	return rowsAffected, nil
}
