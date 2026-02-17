package test

import (
	"agent-sentry/internal/sysmon"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestBehavior_DeepWatchVerification simulates a malicious agent opening many file descriptors
// and verifies that the system detects it.
func TestBehavior_DeepWatchVerification(t *testing.T) {
	// 1. Create a "malicious" script
	script := `
import socket
import time
import os
import sys

# Signal readiness
print("READY")
sys.stdout.flush()
time.sleep(2) # Wait for baseline capture

sockets = []
try:
    for i in range(100):
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sockets.append(s)
    print("OPENED")
    sys.stdout.flush()
    time.sleep(10) # Keep them open
except Exception as e:
    print(e)
`
	// Write script to temp file
	tmpfile, err := os.CreateTemp("", "malicious_*.py")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// 2. Start the malicious process
	cmd := exec.Command("python3", tmpfile.Name())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid
	t.Logf("Malicious process started with PID: %d", pid)

	// Wait for READY
	buf := make([]byte, 1024)
	n, _ := stdout.Read(buf)
	if string(buf[:n]) == "" {
		// Wait a bit more
		time.Sleep(1 * time.Second)
	}
	t.Logf("Process signaled READY. Capturing baseline...")

	// Now monitor it using sysmon logic
	monitor := sysmon.NewMonitor()

	// Establishing baseline (initial state)
	if !monitor.IsMonitoring(pid) {
		stats, err := monitor.GetStats(pid)
		if err != nil {
			t.Logf("GetStats error (might be early): %v", err)
		} else {
			monitor.DetectProbing(pid, stats) // call once to init
		}
	}

	t.Logf("Waiting for sockets to open...")

	// Poll for detection
	detected := false
	for i := 0; i < 15; i++ {
		time.Sleep(1 * time.Second)
		stats, err := monitor.GetStats(pid)
		if err != nil {
			continue
		}

		isProbing, details := monitor.DetectProbing(pid, stats)
		if isProbing {
			t.Logf("Creating sockets detected: %s", details)
			detected = true
			break
		}
	}

	if !detected {
		t.Errorf("❌ Deep Watch FAILED to detect socket spike within 10 seconds.")
	} else {
		t.Logf("✅ Verification Passed: High-frequency probing was caught.")
	}
}
