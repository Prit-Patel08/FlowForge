package policy

import (
	"testing"
	"time"
)

func TestEvaluateEnforceKillWithReason(t *testing.T) {
	d := NewThresholdDecider()
	p := Policy{
		MaxCPUPercent:    90,
		CPUWindow:        30 * time.Second,
		MinLogEntropy:    0.20,
		MaxLogRepetition: 0.80,
		ShadowMode:       false,
		RestartOnBreach:  false,
	}

	out := d.Evaluate(Telemetry{
		CPUPercent:    95,
		CPUOverFor:    31 * time.Second,
		LogEntropy:    0.10,
		LogRepetition: 0.95,
	}, p)

	if out.Action != ActionKill {
		t.Fatalf("expected ActionKill, got %s", out.Action.String())
	}

	expected := "CPU exceeded 90% for 30s AND log repetition exceeded 0.80 AND log entropy dropped below 0.20"
	if out.Reason != expected {
		t.Fatalf("unexpected reason\nexpected: %q\ngot:      %q", expected, out.Reason)
	}
}

func TestEvaluateShadowModeReturnsLogOnly(t *testing.T) {
	d := NewThresholdDecider()
	p := Policy{
		MaxCPUPercent:   90,
		CPUWindow:       30 * time.Second,
		MinLogEntropy:   0.20,
		ShadowMode:      true,
		RestartOnBreach: false,
	}

	out := d.Evaluate(Telemetry{
		CPUPercent: 95,
		CPUOverFor: 45 * time.Second,
		LogEntropy: 0.15,
	}, p)

	if out.Action != ActionLogOnly {
		t.Fatalf("expected ActionLogOnly, got %s", out.Action.String())
	}
	if out.IntendedAction != ActionKill {
		t.Fatalf("expected intended action KILL, got %s", out.IntendedAction.String())
	}
	expected := "Shadow mode: would KILL. CPU exceeded 90% for 30s AND log entropy dropped below 0.20"
	if out.Reason != expected {
		t.Fatalf("unexpected reason\nexpected: %q\ngot:      %q", expected, out.Reason)
	}
}

func TestEvaluateContinueWhenNoThresholdsBreached(t *testing.T) {
	d := NewThresholdDecider()
	p := Policy{
		MaxCPUPercent:    90,
		CPUWindow:        30 * time.Second,
		MaxMemoryMB:      2048,
		MaxLogRepetition: 0.8,
		MinLogEntropy:    0.2,
	}

	out := d.Evaluate(Telemetry{
		CPUPercent:    50,
		CPUOverFor:    5 * time.Second,
		MemoryMB:      200,
		LogRepetition: 0.2,
		LogEntropy:    0.7,
	}, p)

	if out.Action != ActionContinue {
		t.Fatalf("expected continue, got %s", out.Action.String())
	}
	if out.Reason != "No thresholds breached" {
		t.Fatalf("unexpected reason: %q", out.Reason)
	}
}

func TestEvaluateAlertOnSingleSignal(t *testing.T) {
	d := NewThresholdDecider()
	p := Policy{
		MinLogEntropy: 0.2,
	}

	out := d.Evaluate(Telemetry{LogEntropy: 0.1}, p)
	if out.Action != ActionAlert {
		t.Fatalf("expected alert, got %s", out.Action.String())
	}
	expected := "log entropy dropped below 0.20"
	if out.Reason != expected {
		t.Fatalf("unexpected reason\nexpected: %q\ngot:      %q", expected, out.Reason)
	}
}

func TestEvaluateProgressGuardSuppressesDestructiveAction(t *testing.T) {
	d := NewThresholdDecider()
	p := Policy{
		MaxCPUPercent:    90,
		CPUWindow:        10 * time.Second,
		MinLogEntropy:    0.20,
		MaxLogRepetition: 0.80,
	}

	out := d.Evaluate(Telemetry{
		CPUPercent:    96,
		CPUOverFor:    12 * time.Second,
		LogRepetition: 0.95,
		LogEntropy:    0.10,
		RawDiversity:  0.95,
		ProgressLike:  true,
	}, p)

	if out.Action != ActionAlert {
		t.Fatalf("expected alert due to progress guard, got %s", out.Action.String())
	}
	expected := "CPU exceeded 90% for 10s AND log repetition exceeded 0.80 AND log entropy dropped below 0.20 AND progressing output pattern detected; destructive action suppressed"
	if out.Reason != expected {
		t.Fatalf("unexpected reason\nexpected: %q\ngot:      %q", expected, out.Reason)
	}
}
