package state

import (
	"encoding/json"
	"sync"
	"time"
)

// ProcessState holds the runtime state of the supervised process
type ProcessState struct {
	CPU        float64  `json:"cpu"`
	LastLine   string   `json:"last_line"`
	Status     string   `json:"status"` // RUNNING, STOPPED, LOOP_DETECTED, WATCHDOG_ALERT
	Command    string   `json:"command"`
	Args       []string `json:"args"` // Secure: Exact arguments for restart
	Dir        string   `json:"dir"`  // Working directory
	PID        int      `json:"pid"`
	Reason     string   `json:"reason"`
	CPUScore   float64  `json:"cpu_score"`
	Entropy    float64  `json:"entropy_score"`
	Confidence float64  `json:"confidence_score"`
	Timestamp  int64    `json:"timestamp"`
}

var (
	currentState ProcessState
	mu           sync.RWMutex
)

// UpdateState safely updates the global process state
func UpdateState(cpu float64, lastLine, status, command string, args []string, dir string, pid int) {
	mu.Lock()
	defer mu.Unlock()

	argsCopy := append([]string(nil), args...)

	currentState = ProcessState{
		CPU:       cpu,
		LastLine:  lastLine,
		Status:    status,
		Command:   command,
		Args:      argsCopy,
		Dir:       dir,
		PID:       pid,
		Timestamp: time.Now().UnixMilli(),
	}
}

// UpdateDecision updates decision diagnostics while preserving current process identity.
func UpdateDecision(reason string, cpuScore, entropy, confidence float64) {
	mu.Lock()
	defer mu.Unlock()
	currentState.Reason = reason
	currentState.CPUScore = cpuScore
	currentState.Entropy = entropy
	currentState.Confidence = confidence
	currentState.Timestamp = time.Now().UnixMilli()
}

// GetState safely returns a copy of the current state
func GetState() ProcessState {
	mu.RLock()
	defer mu.RUnlock()
	return currentState
}

// JSON returns the state as a JSON byte slice (for API)
func JSON() ([]byte, error) {
	mu.RLock()
	defer mu.RUnlock()
	return json.Marshal(currentState)
}
