package database

import (
	"database/sql"
	"flowforge/internal/encryption"
	"fmt"
	"os"
	"sort"
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
}

type DecisionTrace struct {
	ID              int     `json:"id"`
	Timestamp       string  `json:"timestamp"`
	Command         string  `json:"command"`
	PID             int     `json:"pid"`
	CPUScore        float64 `json:"cpu_score"`
	EntropyScore    float64 `json:"entropy_score"`
	ConfidenceScore float64 `json:"confidence_score"`
	Decision        string  `json:"decision"`
	Reason          string  `json:"reason"`
}

type TimelineEvent struct {
	Type       string  `json:"type"`
	Timestamp  string  `json:"timestamp"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Reason     string  `json:"reason"`
	PID        int     `json:"pid"`
	CPUScore   float64 `json:"cpu_score,omitempty"`
	Entropy    float64 `json:"entropy_score,omitempty"`
	Confidence float64 `json:"confidence_score,omitempty"`
}

type UnifiedEvent struct {
	ID         int     `json:"id"`
	Timestamp  string  `json:"timestamp"`
	Type       string  `json:"type"`
	Title      string  `json:"title"`
	Summary    string  `json:"summary"`
	Reason     string  `json:"reason"`
	PID        int     `json:"pid"`
	CPUScore   float64 `json:"cpu_score"`
	Entropy    float64 `json:"entropy_score"`
	Confidence float64 `json:"confidence_score"`
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
		details TEXT
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
		reason TEXT
	);`
	if _, err := db.Exec(createDecisionTableSQL); err != nil {
		return err
	}

	createEventsTableSQL := `CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL UNIQUE,
		run_id TEXT NOT NULL,
		incident_id TEXT,
		event_type TEXT NOT NULL,
		actor TEXT NOT NULL DEFAULT 'system',
		reason_text TEXT NOT NULL DEFAULT '',
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
	db.Exec("ALTER TABLE events ADD COLUMN event_id TEXT;")
	db.Exec("ALTER TABLE events ADD COLUMN run_id TEXT DEFAULT 'unknown-run';")
	db.Exec("ALTER TABLE events ADD COLUMN incident_id TEXT;")
	db.Exec("ALTER TABLE events ADD COLUMN event_type TEXT DEFAULT 'legacy';")
	db.Exec("ALTER TABLE events ADD COLUMN actor TEXT DEFAULT 'system';")
	db.Exec("ALTER TABLE events ADD COLUMN reason_text TEXT DEFAULT '';")
	db.Exec("ALTER TABLE events ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;")

	// Backfill required columns where possible.
	db.Exec("UPDATE events SET event_id = COALESCE(event_id, lower(hex(randomblob(16)))) WHERE event_id IS NULL OR event_id = '';")
	db.Exec("UPDATE events SET run_id = COALESCE(NULLIF(run_id, ''), 'unknown-run');")
	db.Exec("UPDATE events SET event_type = COALESCE(NULLIF(event_type, ''), COALESCE(type, 'legacy'));")
	db.Exec("UPDATE events SET reason_text = COALESCE(reason_text, reason, '');")
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

	return nil
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
	if db == nil {
		return fmt.Errorf("db not initialized")
	}

	// Encrypt sensitive fields
	encCmd, _ := encryption.Encrypt(command)
	encPat, _ := encryption.Encrypt(pattern)

	// Fallback to raw if encryption returns empty string
	if encCmd == "" {
		encCmd = command
	}
	if encPat == "" {
		encPat = pattern
	}

	stmt, err := db.Prepare("INSERT INTO incidents(command, model_name, exit_reason, max_cpu, pattern, token_savings_estimate, token_count, cost, agent_id, agent_version, reason, cpu_score, entropy_score, confidence_score, recovery_status, restart_count) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(encCmd, modelName, exitReason, maxCpu, encPat, savings, tokenCount, cost, agentID, agentVersion, reason, cpuScore, entropyScore, confidenceScore, recoveryStatus, restartCount)
	if err != nil {
		return err
	}
	incidentID := uuid.NewString()
	return logUnifiedEventWithMeta("incident", exitReason, fmt.Sprintf("%s (CPU %.1f%%)", exitReason, maxCpu), reason, "system", incidentID, 0, cpuScore, entropyScore, confidenceScore)
}

func GetIncidentByID(id int) (Incident, error) {
	var i Incident
	if db == nil {
		return i, fmt.Errorf("db missing")
	}

	row := db.QueryRow("SELECT id, timestamp, command, COALESCE(model_name, 'unknown'), exit_reason, max_cpu, pattern, token_savings_estimate, COALESCE(token_count, 0), COALESCE(cost, 0.0), COALESCE(agent_id, ''), COALESCE(agent_version, ''), COALESCE(reason, ''), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(recovery_status, ''), COALESCE(restart_count, 0) FROM incidents WHERE id = ?", id)
	err := row.Scan(&i.ID, &i.Timestamp, &i.Command, &i.ModelName, &i.ExitReason, &i.MaxCPU, &i.Pattern, &i.TokenSavingsEstimate, &i.TokenCount, &i.Cost, &i.AgentID, &i.AgentVersion, &i.Reason, &i.CPUScore, &i.EntropyScore, &i.ConfidenceScore, &i.RecoveryStatus, &i.RestartCount)

	if err == nil {
		if dec, e := encryption.Decrypt(i.Command); e == nil {
			i.Command = dec
		}
		if dec, e := encryption.Decrypt(i.Pattern); e == nil {
			i.Pattern = dec
		}
	}
	return i, err
}

func GetAllIncidents() ([]Incident, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}

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
		if dec, e := encryption.Decrypt(i.Command); e == nil {
			i.Command = dec
		}
		if dec, e := encryption.Decrypt(i.Pattern); e == nil {
			i.Pattern = dec
		}
		list = append(list, i)
	}
	return list, nil
}

func LogAuditEvent(actor, action, reason, source string, pid int, details string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	stmt, err := db.Prepare("INSERT INTO audit_events(actor, action, reason, source, pid, details) VALUES(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(actor, action, reason, source, pid, details)
	if err != nil {
		return err
	}
	return logUnifiedEventWithMeta("audit", action, fmt.Sprintf("%s by %s", action, actor), reason, actor, "", pid, 0, 0, 0)
}

func GetAuditEvents(limit int) ([]AuditEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query("SELECT id, timestamp, COALESCE(actor, ''), COALESCE(action, ''), COALESCE(reason, ''), COALESCE(source, ''), COALESCE(pid, 0), COALESCE(details, '') FROM audit_events ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.Action, &e.Reason, &e.Source, &e.PID, &e.Details); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}

func LogDecisionTrace(command string, pid int, cpuScore, entropyScore, confidenceScore float64, decision, reason string) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	stmt, err := db.Prepare("INSERT INTO decision_traces(command, pid, cpu_score, entropy_score, confidence_score, decision, reason) VALUES(?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec(command, pid, cpuScore, entropyScore, confidenceScore, decision, reason)
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("CPU %.1f / Entropy %.1f / Confidence %.1f", cpuScore, entropyScore, confidenceScore)
	return logUnifiedEventWithMeta("decision", decision, summary, reason, "system", "", pid, cpuScore, entropyScore, confidenceScore)
}

func GetDecisionTraces(limit int) ([]DecisionTrace, error) {
	if db == nil {
		return nil, fmt.Errorf("db missing")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query("SELECT id, timestamp, COALESCE(command, ''), COALESCE(pid, 0), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0), COALESCE(decision, ''), COALESCE(reason, '') FROM decision_traces ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var traces []DecisionTrace
	for rows.Next() {
		var t DecisionTrace
		if err := rows.Scan(&t.ID, &t.Timestamp, &t.Command, &t.PID, &t.CPUScore, &t.EntropyScore, &t.ConfidenceScore, &t.Decision, &t.Reason); err != nil {
			return nil, err
		}
		traces = append(traces, t)
	}
	return traces, nil
}

func GetTimeline(limit int) ([]TimelineEvent, error) {
	events, err := GetUnifiedEvents(limit)
	if err == nil && len(events) > 0 {
		out := make([]TimelineEvent, 0, len(events))
		for _, e := range events {
			out = append(out, TimelineEvent{
				Type:       e.Type,
				Timestamp:  e.Timestamp,
				Title:      e.Title,
				Summary:    e.Summary,
				Reason:     e.Reason,
				PID:        e.PID,
				CPUScore:   e.CPUScore,
				Entropy:    e.Entropy,
				Confidence: e.Confidence,
			})
		}
		return out, nil
	}
	return getLegacyTimeline(limit)
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
			},
		})
	}

	for _, t := range traces {
		records = append(records, timelineRecord{
			at: parseTimestamp(t.Timestamp),
			event: TimelineEvent{
				Type:       "decision",
				Timestamp:  t.Timestamp,
				Title:      t.Decision,
				Summary:    fmt.Sprintf("CPU %.1f / Entropy %.1f / Confidence %.1f", t.CPUScore, t.EntropyScore, t.ConfidenceScore),
				Reason:     t.Reason,
				PID:        t.PID,
				CPUScore:   t.CPUScore,
				Entropy:    t.EntropyScore,
				Confidence: t.ConfidenceScore,
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
	rows, err := db.Query("SELECT id, COALESCE(created_at, timestamp, CURRENT_TIMESTAMP), COALESCE(event_type, type, ''), COALESCE(title, ''), COALESCE(summary, ''), COALESCE(reason_text, reason, ''), COALESCE(pid, 0), COALESCE(cpu_score, 0.0), COALESCE(entropy_score, 0.0), COALESCE(confidence_score, 0.0) FROM events ORDER BY created_at DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []UnifiedEvent
	for rows.Next() {
		var e UnifiedEvent
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Type, &e.Title, &e.Summary, &e.Reason, &e.PID, &e.CPUScore, &e.Entropy, &e.Confidence); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, nil
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
	COALESCE(created_at, timestamp, CURRENT_TIMESTAMP) AS ts,
	COALESCE(event_type, type, '') AS event_type,
	COALESCE(title, '') AS title,
	COALESCE(summary, '') AS summary,
	COALESCE(reason_text, reason, '') AS reason_text,
	COALESCE(pid, 0) AS pid,
	COALESCE(cpu_score, 0.0) AS cpu_score,
	COALESCE(entropy_score, 0.0) AS entropy_score,
	COALESCE(confidence_score, 0.0) AS confidence_score
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
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Type, &e.Title, &e.Summary, &e.Reason, &e.PID, &e.CPUScore, &e.Entropy, &e.Confidence); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func LogPolicyDryRun(command string, pid int, reason string, confidenceScore float64) error {
	summary := fmt.Sprintf("Dry-run for %s", command)
	return logUnifiedEventWithMeta("policy_dry_run", "POLICY_DRY_RUN", summary, reason, "system", uuid.NewString(), pid, 0, 0, confidenceScore)
}

func logUnifiedEventWithMeta(eventType, title, summary, reason, actor, incidentID string, pid int, cpuScore, entropyScore, confidenceScore float64) error {
	_, err := InsertEvent(eventType, actor, reason, currentRunID(), incidentID, title, summary, pid, cpuScore, entropyScore, confidenceScore)
	return err
}

func InsertEvent(eventType, actor, reasonText, runID, incidentID, title, summary string, pid int, cpuScore, entropyScore, confidenceScore float64) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db not initialized")
	}
	stmt, err := db.Prepare(`
INSERT INTO events(
	event_id, run_id, incident_id, event_type, actor, reason_text, created_at,
	timestamp, type, title, summary, reason, pid, cpu_score, entropy_score, confidence_score
) VALUES(?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?, ?, ?, ?)
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
	var incidentIDValue interface{}
	if incidentID == "" {
		incidentIDValue = nil
	} else {
		incidentIDValue = incidentID
	}

	_, err = stmt.Exec(
		eventID,
		runID,
		incidentIDValue,
		eventType,
		actor,
		reasonText,
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
