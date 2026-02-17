package state

import (
	"encoding/json"
	"sync"
	"time"
)

// ProcessState holds the runtime state of the supervised process
type ProcessState struct {
	CPU       float64  `json:"cpu"`
	LastLine  string   `json:"last_line"`
	Status    string   `json:"status"` // RUNNING, STOPPED, LOOP_DETECTED, WATCHDOG_ALERT
	Command   string   `json:"command"`
	Args      []string `json:"args"` // Secure: Exact arguments for restart
	Dir       string   `json:"dir"`  // Working directory
	PID       int      `json:"pid"`
	Timestamp int64    `json:"timestamp"`
}

var (
	currentState ProcessState
	mu           sync.RWMutex
)

// UpdateState safely updates the global process state
func UpdateState(cpu float64, lastLine, status, command string, args []string, dir string, pid int) {
	mu.Lock()
	defer mu.Unlock()

	currentState = ProcessState{
		CPU:       cpu,
		LastLine:  lastLine,
		Status:    status,
		Command:   command,
		Args:      args,
		Dir:       dir,
		PID:       pid,
		Timestamp: time.Now().UnixMilli(),
	}
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
