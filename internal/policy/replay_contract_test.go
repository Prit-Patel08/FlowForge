package policy

import "testing"

func TestDecisionReplayDigestDeterministic(t *testing.T) {
	a := DecisionReplayInput{
		DecisionEngine:   " threshold-decider ",
		EngineVersion:    "1.1.0",
		DecisionContract: "decision-trace.v1",
		RolloutMode:      "ENFORCE",
		Decision:         "kill",
		Reason:           "CPU > 90%",
		CPUScore:         97.12345641,
		EntropyScore:     9.10000000001,
		ConfidenceScore:  96.5000001,
	}
	b := DecisionReplayInput{
		DecisionEngine:   "threshold-decider",
		EngineVersion:    "1.1.0",
		DecisionContract: "decision-trace.v1",
		RolloutMode:      "enforce",
		Decision:         "KILL",
		Reason:           "CPU > 90%",
		CPUScore:         97.12345649,
		EntropyScore:     9.1,
		ConfidenceScore:  96.5,
	}

	da := DecisionReplayDigest(a)
	db := DecisionReplayDigest(b)
	if da == "" || db == "" {
		t.Fatal("expected non-empty deterministic replay digest")
	}
	if da != db {
		t.Fatalf("expected digest stability for equivalent input, got %q != %q", da, db)
	}
}

func TestVerifyDecisionReplayMatch(t *testing.T) {
	in := DecisionReplayInput{
		DecisionEngine:   "threshold-decider",
		EngineVersion:    "1.1.0",
		DecisionContract: "decision-trace.v1",
		RolloutMode:      "canary",
		Decision:         "KILL",
		Reason:           "loop detected",
		CPUScore:         99.4,
		EntropyScore:     10.0,
		ConfidenceScore:  96.5,
	}
	digest := DecisionReplayDigest(in)
	got := VerifyDecisionReplay(digest, in)
	if !got.Replayable {
		t.Fatal("expected replayable=true")
	}
	if !got.DeterministicMatch {
		t.Fatal("expected deterministic_match=true")
	}
	if got.Status != ReplayStatusMatch {
		t.Fatalf("expected status %q, got %q", ReplayStatusMatch, got.Status)
	}
	if got.ComputedDigest != digest {
		t.Fatalf("expected computed digest %q, got %q", digest, got.ComputedDigest)
	}
}

func TestVerifyDecisionReplayMismatch(t *testing.T) {
	in := DecisionReplayInput{
		DecisionEngine:   "threshold-decider",
		EngineVersion:    "1.1.0",
		DecisionContract: "decision-trace.v1",
		RolloutMode:      "enforce",
		Decision:         "RESTART",
		Reason:           "memory limit exceeded",
		CPUScore:         0,
		EntropyScore:     0,
		ConfidenceScore:  88.2,
	}
	got := VerifyDecisionReplay("deadbeef", in)
	if got.Status != ReplayStatusMismatch {
		t.Fatalf("expected status %q, got %q", ReplayStatusMismatch, got.Status)
	}
	if got.DeterministicMatch {
		t.Fatal("expected deterministic_match=false")
	}
}

func TestVerifyDecisionReplayLegacyFallback(t *testing.T) {
	in := DecisionReplayInput{
		Decision:        "KILL",
		Reason:          "legacy row",
		CPUScore:        95,
		EntropyScore:    11,
		ConfidenceScore: 96,
	}
	got := VerifyDecisionReplay("", in)
	if got.Status != ReplayStatusLegacy {
		t.Fatalf("expected status %q, got %q", ReplayStatusLegacy, got.Status)
	}
	if !got.LegacyFallback {
		t.Fatal("expected legacy_fallback=true")
	}
	if got.ComputedDigest == "" {
		t.Fatal("expected computed digest for legacy fallback")
	}
}

func TestVerifyDecisionReplayRejectsMissingDecision(t *testing.T) {
	in := DecisionReplayInput{
		DecisionEngine:   "threshold-decider",
		EngineVersion:    "1.1.0",
		DecisionContract: "decision-trace.v1",
		RolloutMode:      "enforce",
		Reason:           "missing decision",
	}
	got := VerifyDecisionReplay("", in)
	if got.Replayable {
		t.Fatal("expected replayable=false")
	}
	if got.Status != ReplayStatusUnreplayable {
		t.Fatalf("expected status %q, got %q", ReplayStatusUnreplayable, got.Status)
	}
}
