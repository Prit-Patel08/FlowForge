package policy

import "testing"

func TestCurrentEngineContractDefaults(t *testing.T) {
	contract := CurrentEngineContract("")
	if contract.EngineName != DecisionEngineName {
		t.Fatalf("expected engine name %q, got %q", DecisionEngineName, contract.EngineName)
	}
	if contract.EngineVersion != DecisionEngineVersion {
		t.Fatalf("expected engine version %q, got %q", DecisionEngineVersion, contract.EngineVersion)
	}
	if contract.ContractVersion != DecisionContractVersion {
		t.Fatalf("expected contract version %q, got %q", DecisionContractVersion, contract.ContractVersion)
	}
	if contract.RolloutMode != string(RolloutEnforce) {
		t.Fatalf("expected rollout mode %q, got %q", RolloutEnforce, contract.RolloutMode)
	}
}

func TestCurrentEngineContractRespectsRolloutMode(t *testing.T) {
	contract := CurrentEngineContract(RolloutCanary)
	if contract.RolloutMode != string(RolloutCanary) {
		t.Fatalf("expected rollout mode %q, got %q", RolloutCanary, contract.RolloutMode)
	}
}

func TestIsValidEngineVersion(t *testing.T) {
	valid := []string{"1.0.0", "v1.2.3", "2.0.1-beta.1", "3.4.5+build.7"}
	for _, version := range valid {
		if !IsValidEngineVersion(version) {
			t.Fatalf("expected version %q to be valid", version)
		}
	}

	invalid := []string{"", "1.0", "one.two.three", "v1", "1.0.0 beta"}
	for _, version := range invalid {
		if IsValidEngineVersion(version) {
			t.Fatalf("expected version %q to be invalid", version)
		}
	}
}
