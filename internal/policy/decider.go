package policy

import (
	"fmt"
	"strings"
	"time"
)

type Action int

const (
	ActionContinue Action = iota
	ActionAlert
	ActionKill
	ActionRestart
	ActionLogOnly
)

func (a Action) String() string {
	switch a {
	case ActionContinue:
		return "CONTINUE"
	case ActionAlert:
		return "ALERT"
	case ActionKill:
		return "KILL"
	case ActionRestart:
		return "RESTART"
	case ActionLogOnly:
		return "LOG_ONLY"
	default:
		return "UNKNOWN"
	}
}

type Telemetry struct {
	CPUPercent    float64
	CPUOverFor    time.Duration
	MemoryMB      float64
	LogRepetition float64 // 0..1 where 1 means highly repetitive
	LogEntropy    float64 // 0..1 where 0 means repetitive
	RawDiversity  float64 // 0..1 where 1 means highly diverse raw lines
	ProgressLike  bool    // true when output suggests forward progress, not stagnation
}

type Policy struct {
	MaxCPUPercent    float64
	CPUWindow        time.Duration
	MaxMemoryMB      float64
	MaxLogRepetition float64
	MinLogEntropy    float64
	RestartOnBreach  bool
	ShadowMode       bool

	// Metadata fields used by callers when recording shadow-mode evidence.
	DryRunEventType   string
	DryRunActor       string
	DryRunEventPrefix string
}

type Decision struct {
	Action         Action
	IntendedAction Action
	Reason         string
}

type Decider interface {
	Evaluate(t Telemetry, p Policy) Decision
}

type ThresholdDecider struct{}

func NewThresholdDecider() Decider {
	return ThresholdDecider{}
}

func (ThresholdDecider) Evaluate(t Telemetry, p Policy) Decision {
	cpuBreach := p.MaxCPUPercent > 0 && t.CPUPercent > p.MaxCPUPercent
	if cpuBreach && p.CPUWindow > 0 && t.CPUOverFor < p.CPUWindow {
		cpuBreach = false
	}
	memBreach := p.MaxMemoryMB > 0 && t.MemoryMB > p.MaxMemoryMB
	repetitionBreach := p.MaxLogRepetition > 0 && t.LogRepetition > p.MaxLogRepetition
	entropyBreach := p.MinLogEntropy > 0 && t.LogEntropy < p.MinLogEntropy

	reasons := make([]string, 0, 4)
	if cpuBreach {
		if p.CPUWindow > 0 {
			reasons = append(reasons, fmt.Sprintf("CPU exceeded %.0f%% for %ds", p.MaxCPUPercent, int(p.CPUWindow.Seconds())))
		} else {
			reasons = append(reasons, fmt.Sprintf("CPU exceeded %.0f%%", p.MaxCPUPercent))
		}
	}
	if memBreach {
		reasons = append(reasons, fmt.Sprintf("memory exceeded %.0fMB", p.MaxMemoryMB))
	}
	if repetitionBreach {
		reasons = append(reasons, fmt.Sprintf("log repetition exceeded %.2f", p.MaxLogRepetition))
	}
	if entropyBreach {
		reasons = append(reasons, fmt.Sprintf("log entropy dropped below %.2f", p.MinLogEntropy))
	}

	if len(reasons) == 0 {
		return Decision{
			Action:         ActionContinue,
			IntendedAction: ActionContinue,
			Reason:         "No thresholds breached",
		}
	}

	potentialRuntimeRisk := cpuBreach && (repetitionBreach || entropyBreach)
	progressGuard := potentialRuntimeRisk && t.ProgressLike && t.RawDiversity >= 0.85
	highRisk := memBreach || (potentialRuntimeRisk && !progressGuard)

	action := ActionAlert
	if highRisk {
		if p.RestartOnBreach {
			action = ActionRestart
		} else {
			action = ActionKill
		}
	}

	if progressGuard {
		reasons = append(reasons, "progressing output pattern detected; destructive action suppressed")
	}

	reason := strings.Join(reasons, " AND ")
	if p.ShadowMode && (action == ActionKill || action == ActionRestart) {
		return Decision{
			Action:         ActionLogOnly,
			IntendedAction: action,
			Reason:         fmt.Sprintf("Shadow mode: would %s. %s", action.String(), reason),
		}
	}

	return Decision{
		Action:         action,
		IntendedAction: action,
		Reason:         reason,
	}
}
