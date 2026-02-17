package sysmon

import (
	"fmt"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/process"
)

// SysStats holds system monitoring data
type SysStats struct {
	OpenFDs     int
	SocketCount int
}

// Monitor tracks process baselines safely
type Monitor struct {
	mu        sync.Mutex
	baselines map[int]SysStats
}

// NewMonitor creates a thread-safe monitor
func NewMonitor() *Monitor {
	return &Monitor{
		baselines: make(map[int]SysStats),
	}
}

// GetStats returns current file descriptor and socket counts for the PID.
// Uses native gopsutil for high performance and zero-shell security.
func (m *Monitor) GetStats(pid int) (SysStats, error) {
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return SysStats{}, err
	}

	// 1. File Descriptors (Native)
	fds, _ := proc.NumFDs()

	// 2. Sockets (Try Native first)
	socketCount := 0
	conns, err := proc.Connections()
	if err == nil {
		socketCount = len(conns)
	}

	return SysStats{
		OpenFDs:     int(fds),
		SocketCount: socketCount,
	}, nil
}

// IsMonitoring checks if we have a baseline for this PID
func (m *Monitor) IsMonitoring(pid int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.baselines[pid]
	return ok
}

// DetectProbing checks for anomalies against baseline
func (m *Monitor) DetectProbing(pid int, current SysStats) (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	base, ok := m.baselines[pid]
	if !ok {
		// First time seeing this PID, set baseline
		m.baselines[pid] = current
		// Also update baseline if current is "low"? No, just trust first.

		// If startup is busy, we might have high baseline.
		// Allow baseline to settle?
		// For now simple logic: First observation is baseline.
		return false, ""
	}

	// Logic: If sockets double AND > 50
	isProbing := false
	var details strings.Builder

	if current.SocketCount > 50 && current.SocketCount > base.SocketCount*2 {
		isProbing = true
		if base.SocketCount > 0 {
			percentage := (current.SocketCount - base.SocketCount) * 100 / base.SocketCount
			details.WriteString(fmt.Sprintf("Sockets: %d -> %d (+%d%%)", base.SocketCount, current.SocketCount, percentage))
		} else {
			details.WriteString(fmt.Sprintf("Sockets: %d -> %d (New)", base.SocketCount, current.SocketCount))
		}
	}

	if current.OpenFDs > base.OpenFDs*3 && current.OpenFDs > 20 {
		if isProbing {
			details.WriteString(" | ")
		}
		isProbing = true
		details.WriteString(fmt.Sprintf("FDs: %d -> %d", base.OpenFDs, current.OpenFDs))
	}

	// Auto-update baseline if current is LOWER (process became idle), so we catch spikes from idle?
	// This helps with "settling".
	if current.SocketCount < base.SocketCount {
		base.SocketCount = current.SocketCount
		m.baselines[pid] = base
	}

	return isProbing, details.String()
}
