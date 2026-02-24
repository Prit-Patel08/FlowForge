package database

import "testing"

func TestDecisionSignalBaselineStateUpsertAndGet(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	initial := DecisionSignalBaselineState{
		BucketKey:         "threshold-decider@1.1.0|enforce",
		LatestTraceID:     101,
		ConsecutiveBreach: 1,
		Status:            DecisionSignalBaselineStatusPending,
	}
	if err := UpsertDecisionSignalBaselineState(initial); err != nil {
		t.Fatalf("UpsertDecisionSignalBaselineState(initial): %v", err)
	}

	got, err := GetDecisionSignalBaselineState(initial.BucketKey)
	if err != nil {
		t.Fatalf("GetDecisionSignalBaselineState(initial): %v", err)
	}
	if got.LatestTraceID != initial.LatestTraceID {
		t.Fatalf("expected latest_trace_id=%d, got %d", initial.LatestTraceID, got.LatestTraceID)
	}
	if got.ConsecutiveBreach != initial.ConsecutiveBreach {
		t.Fatalf("expected consecutive_breach_count=%d, got %d", initial.ConsecutiveBreach, got.ConsecutiveBreach)
	}
	if got.Status != DecisionSignalBaselineStatusPending {
		t.Fatalf("expected status=%q, got %q", DecisionSignalBaselineStatusPending, got.Status)
	}
	if got.LastCheckedAt == "" {
		t.Fatal("expected non-empty last_checked_at")
	}
	if got.LastTransitionAt == "" {
		t.Fatal("expected non-empty last_transition_at")
	}

	updated := DecisionSignalBaselineState{
		BucketKey:         initial.BucketKey,
		LatestTraceID:     102,
		ConsecutiveBreach: 2,
		Status:            DecisionSignalBaselineStatusAtRisk,
	}
	if err := UpsertDecisionSignalBaselineState(updated); err != nil {
		t.Fatalf("UpsertDecisionSignalBaselineState(updated): %v", err)
	}

	got, err = GetDecisionSignalBaselineState(initial.BucketKey)
	if err != nil {
		t.Fatalf("GetDecisionSignalBaselineState(updated): %v", err)
	}
	if got.LatestTraceID != updated.LatestTraceID {
		t.Fatalf("expected updated latest_trace_id=%d, got %d", updated.LatestTraceID, got.LatestTraceID)
	}
	if got.ConsecutiveBreach != updated.ConsecutiveBreach {
		t.Fatalf("expected updated consecutive_breach_count=%d, got %d", updated.ConsecutiveBreach, got.ConsecutiveBreach)
	}
	if got.Status != DecisionSignalBaselineStatusAtRisk {
		t.Fatalf("expected updated status=%q, got %q", DecisionSignalBaselineStatusAtRisk, got.Status)
	}
}

func TestDecisionSignalBaselineStateUpsertNormalizesInput(t *testing.T) {
	_ = withTempDBPath(t)
	CloseDB()
	if err := InitDB(); err != nil {
		t.Fatalf("InitDB: %v", err)
	}

	state := DecisionSignalBaselineState{
		BucketKey:         "stable-decider@0.2.0|shadow",
		LatestTraceID:     -5,
		ConsecutiveBreach: -3,
		Status:            "unexpected-status",
	}
	if err := UpsertDecisionSignalBaselineState(state); err != nil {
		t.Fatalf("UpsertDecisionSignalBaselineState: %v", err)
	}

	got, err := GetDecisionSignalBaselineState(state.BucketKey)
	if err != nil {
		t.Fatalf("GetDecisionSignalBaselineState: %v", err)
	}
	if got.LatestTraceID != 0 {
		t.Fatalf("expected normalized latest_trace_id=0, got %d", got.LatestTraceID)
	}
	if got.ConsecutiveBreach != 0 {
		t.Fatalf("expected normalized consecutive_breach_count=0, got %d", got.ConsecutiveBreach)
	}
	if got.Status != DecisionSignalBaselineStatusHealthy {
		t.Fatalf("expected normalized status=%q, got %q", DecisionSignalBaselineStatusHealthy, got.Status)
	}
}
