package policy

import (
	"regexp"
	"strings"
)

const (
	DecisionEngineName      = "threshold-decider"
	DecisionEngineVersion   = "1.1.0"
	DecisionContractVersion = "decision-trace.v1"
)

var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

type EngineContract struct {
	EngineName      string `json:"decision_engine"`
	EngineVersion   string `json:"engine_version"`
	ContractVersion string `json:"decision_contract_version"`
	RolloutMode     string `json:"rollout_mode"`
}

func CurrentEngineContract(rolloutMode RolloutMode) EngineContract {
	mode := normalizeRolloutMode(rolloutMode, false)
	return EngineContract{
		EngineName:      DecisionEngineName,
		EngineVersion:   DecisionEngineVersion,
		ContractVersion: DecisionContractVersion,
		RolloutMode:     string(mode),
	}
}

func IsValidEngineVersion(version string) bool {
	version = strings.TrimSpace(version)
	return semverRe.MatchString(version)
}
