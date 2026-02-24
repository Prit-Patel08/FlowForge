package database

import (
	"fmt"
	"strings"
)

const (
	DecisionSignalBaselineStatusHealthy             = "healthy"
	DecisionSignalBaselineStatusPending             = "pending"
	DecisionSignalBaselineStatusAtRisk              = "at_risk"
	DecisionSignalBaselineStatusInsufficientHistory = "insufficient_history"
)

type DecisionSignalBaselineState struct {
	BucketKey         string `json:"bucket_key"`
	LatestTraceID     int    `json:"latest_trace_id"`
	ConsecutiveBreach int    `json:"consecutive_breach_count"`
	Status            string `json:"status"`
	LastTransitionAt  string `json:"last_transition_at"`
	LastCheckedAt     string `json:"last_checked_at"`
}

func normalizeDecisionSignalBaselineStatus(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case DecisionSignalBaselineStatusPending:
		return DecisionSignalBaselineStatusPending
	case DecisionSignalBaselineStatusAtRisk:
		return DecisionSignalBaselineStatusAtRisk
	case DecisionSignalBaselineStatusInsufficientHistory:
		return DecisionSignalBaselineStatusInsufficientHistory
	default:
		return DecisionSignalBaselineStatusHealthy
	}
}

func GetDecisionSignalBaselineState(bucketKey string) (DecisionSignalBaselineState, error) {
	if db == nil {
		return DecisionSignalBaselineState{}, fmt.Errorf("db not initialized")
	}
	bucketKey = strings.TrimSpace(bucketKey)
	if bucketKey == "" {
		return DecisionSignalBaselineState{}, fmt.Errorf("bucket_key is required")
	}

	var out DecisionSignalBaselineState
	err := db.QueryRow(`
SELECT
	bucket_key,
	COALESCE(latest_trace_id, 0),
	COALESCE(consecutive_breach_count, 0),
	COALESCE(status, 'healthy'),
	COALESCE(last_transition_at, CURRENT_TIMESTAMP),
	COALESCE(last_checked_at, CURRENT_TIMESTAMP)
FROM decision_signal_baseline_state
WHERE bucket_key = ?
`, bucketKey).Scan(
		&out.BucketKey,
		&out.LatestTraceID,
		&out.ConsecutiveBreach,
		&out.Status,
		&out.LastTransitionAt,
		&out.LastCheckedAt,
	)
	if err != nil {
		return DecisionSignalBaselineState{}, err
	}
	out.Status = normalizeDecisionSignalBaselineStatus(out.Status)
	if out.ConsecutiveBreach < 0 {
		out.ConsecutiveBreach = 0
	}
	if out.LatestTraceID < 0 {
		out.LatestTraceID = 0
	}
	return out, nil
}

func UpsertDecisionSignalBaselineState(state DecisionSignalBaselineState) error {
	if db == nil {
		return fmt.Errorf("db not initialized")
	}
	state.BucketKey = strings.TrimSpace(state.BucketKey)
	if state.BucketKey == "" {
		return fmt.Errorf("bucket_key is required")
	}
	if state.LatestTraceID < 0 {
		state.LatestTraceID = 0
	}
	if state.ConsecutiveBreach < 0 {
		state.ConsecutiveBreach = 0
	}
	state.Status = normalizeDecisionSignalBaselineStatus(state.Status)

	_, err := db.Exec(`
INSERT INTO decision_signal_baseline_state(
	bucket_key,
	latest_trace_id,
	consecutive_breach_count,
	status,
	last_transition_at,
	last_checked_at
) VALUES(?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(bucket_key) DO UPDATE SET
	latest_trace_id = excluded.latest_trace_id,
	consecutive_breach_count = excluded.consecutive_breach_count,
	status = excluded.status,
	last_transition_at = CASE
		WHEN COALESCE(decision_signal_baseline_state.status, 'healthy') <> COALESCE(excluded.status, 'healthy')
			THEN CURRENT_TIMESTAMP
		ELSE COALESCE(decision_signal_baseline_state.last_transition_at, CURRENT_TIMESTAMP)
	END,
	last_checked_at = CURRENT_TIMESTAMP
`, state.BucketKey, state.LatestTraceID, state.ConsecutiveBreach, state.Status)
	return err
}
