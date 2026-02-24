package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

const DecisionReplayContractVersion = "decision-replay.v1"

const (
	ReplayStatusMatch        = "MATCH"
	ReplayStatusMismatch     = "MISMATCH"
	ReplayStatusMissing      = "MISSING_DIGEST"
	ReplayStatusLegacy       = "LEGACY_FALLBACK"
	ReplayStatusUnreplayable = "NOT_REPLAYABLE"
)

type DecisionReplayInput struct {
	DecisionEngine   string  `json:"decision_engine"`
	EngineVersion    string  `json:"engine_version"`
	DecisionContract string  `json:"decision_contract_version"`
	RolloutMode      string  `json:"rollout_mode"`
	Decision         string  `json:"decision"`
	Reason           string  `json:"reason"`
	CPUScore         float64 `json:"cpu_score"`
	EntropyScore     float64 `json:"entropy_score"`
	ConfidenceScore  float64 `json:"confidence_score"`
}

type DecisionReplayVerification struct {
	ContractVersion    string              `json:"contract_version"`
	Replayable         bool                `json:"replayable"`
	Status             string              `json:"status"`
	StoredDigest       string              `json:"stored_digest,omitempty"`
	ComputedDigest     string              `json:"computed_digest,omitempty"`
	DeterministicMatch bool                `json:"deterministic_match"`
	LegacyFallback     bool                `json:"legacy_fallback"`
	Reason             string              `json:"reason"`
	CanonicalInput     DecisionReplayInput `json:"canonical_input"`
}

func VerifyDecisionReplay(storedDigest string, in DecisionReplayInput) DecisionReplayVerification {
	normalized, legacyFallback := NormalizeDecisionReplayInput(in)
	stored := strings.ToLower(strings.TrimSpace(storedDigest))

	out := DecisionReplayVerification{
		ContractVersion: DecisionReplayContractVersion,
		Replayable:      strings.TrimSpace(normalized.Decision) != "",
		Status:          ReplayStatusUnreplayable,
		StoredDigest:    stored,
		LegacyFallback:  legacyFallback,
		Reason:          "decision value is required for deterministic replay",
		CanonicalInput:  normalized,
	}

	if !out.Replayable {
		return out
	}

	out.ComputedDigest = DecisionReplayDigest(normalized)
	if stored == "" {
		if legacyFallback {
			out.Status = ReplayStatusLegacy
			out.Reason = "legacy decision trace missing replay digest; generated deterministic fallback digest"
		} else {
			out.Status = ReplayStatusMissing
			out.Reason = "decision trace missing replay digest"
		}
		return out
	}

	if subtleDigestMatch(stored, out.ComputedDigest) {
		out.Status = ReplayStatusMatch
		out.DeterministicMatch = true
		out.Reason = "stored replay digest matches deterministic replay computation"
		return out
	}

	out.Status = ReplayStatusMismatch
	out.Reason = "stored replay digest does not match deterministic replay computation"
	return out
}

func DecisionReplayDigest(in DecisionReplayInput) string {
	normalized, _ := NormalizeDecisionReplayInput(in)
	lines := []string{
		"decision_engine=" + normalized.DecisionEngine,
		"engine_version=" + normalized.EngineVersion,
		"decision_contract_version=" + normalized.DecisionContract,
		"rollout_mode=" + normalized.RolloutMode,
		"decision=" + normalized.Decision,
		"reason=" + normalized.Reason,
		"cpu_score=" + formatReplayScore(normalized.CPUScore),
		"entropy_score=" + formatReplayScore(normalized.EntropyScore),
		"confidence_score=" + formatReplayScore(normalized.ConfidenceScore),
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func NormalizeDecisionReplayInput(in DecisionReplayInput) (DecisionReplayInput, bool) {
	normalized := DecisionReplayInput{
		DecisionEngine:   strings.TrimSpace(in.DecisionEngine),
		EngineVersion:    strings.TrimSpace(in.EngineVersion),
		DecisionContract: strings.TrimSpace(in.DecisionContract),
		RolloutMode:      strings.ToLower(strings.TrimSpace(in.RolloutMode)),
		Decision:         strings.ToUpper(strings.TrimSpace(in.Decision)),
		Reason:           strings.TrimSpace(in.Reason),
		CPUScore:         normalizeReplayScore(in.CPUScore),
		EntropyScore:     normalizeReplayScore(in.EntropyScore),
		ConfidenceScore:  normalizeReplayScore(in.ConfidenceScore),
	}

	legacyFallback := false
	if normalized.DecisionEngine == "" {
		normalized.DecisionEngine = "legacy-decider"
		legacyFallback = true
	}
	if normalized.EngineVersion == "" {
		normalized.EngineVersion = "legacy-unknown"
		legacyFallback = true
	}
	if normalized.DecisionContract == "" {
		normalized.DecisionContract = "legacy-decision-trace"
		legacyFallback = true
	}
	if normalized.RolloutMode == "" {
		normalized.RolloutMode = "legacy"
		legacyFallback = true
	}

	return normalized, legacyFallback
}

func subtleDigestMatch(left, right string) bool {
	left = strings.ToLower(strings.TrimSpace(left))
	right = strings.ToLower(strings.TrimSpace(right))
	return left != "" && right != "" && left == right
}

func normalizeReplayScore(v float64) float64 {
	rounded := math.Round(v*1_000_000) / 1_000_000
	if rounded == 0 {
		return 0
	}
	return rounded
}

func formatReplayScore(v float64) string {
	return strconv.FormatFloat(normalizeReplayScore(v), 'f', 6, 64)
}
