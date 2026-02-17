package sysmon

import (
	"fmt"
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
// Uses gopsutil; no external shelling (no lsof fallback).
func (m *Monitor) GetStats(pid int) (SysStats, error) {
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return SysStats{}, err
	}

	// 1. File Descriptors
	fds, err := proc.NumFDs()
	if err != nil {
		// If unable to get FD count, set 0 but return no fatal error;
		// caller can decide. Avoid shell fallbacks.
		fds = 0
	}

	// 2. Socket/connection count
	socketCount := 0
	conns, err := proc.Connections()
	if err == nil {
		socketCount = len(conns)
	} else {
		// If gopsutil cannot enumerate connections on this platform,
		// return socketCount = 0 rather than shelling out.
		socketCount = 0
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
		return false, ""
	}

	isProbing := false
	var details string

	if current.SocketCount > 50 && current.SocketCount > base.SocketCount*2 {
		isProbing = true
		if base.SocketCount > 0 {
			percentage := (current.SocketCount - base.SocketCount) * 100 / base.SocketCount
			details = fmt.Sprintf("Sockets: %d -> %d (+%d%%)", base.SocketCount, current.SocketCount, percentage)
		} else {
			details = fmt.Sprintf("Sockets: %d -> %d (New)", base.SocketCount, current.SocketCount)
		}
	}

	if current.OpenFDs > base.OpenFDs*3 && current.OpenFDs > 20 {
		if isProbing {
			details = details + " | "
		}
		isProbing = true
		details = details + fmt.Sprintf("FDs: %d -> %d", base.OpenFDs, current.OpenFDs)
	}

	// Auto-update baseline if current is LOWER
	if current.SocketCount < base.SocketCount {
		base.SocketCount = current.SocketCount
		m.baselines[pid] = base
	}

	return isProbing, details
}
