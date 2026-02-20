package supervisor

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStopTerminatesProcessTree(t *testing.T) {
	script := strings.Join([]string{
		"import subprocess, time, sys",
		`child = subprocess.Popen(["python3", "-c", "import time; time.sleep(120)"])`,
		"print(child.pid, flush=True)",
		"time.sleep(120)",
	}, "\n")

	cmd := exec.Command("python3", "-c", script)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}

	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}

	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", line, err)
	}

	if err := s.Stop(2 * time.Second); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if !waitForProcessExit(childPID, 2*time.Second) {
		t.Fatalf("child process %d is still running after stop", childPID)
	}
}

func TestStopTerminatesDeepProcessTree(t *testing.T) {
	for attempt := 1; attempt <= 3; attempt++ {
		t.Run(fmt.Sprintf("attempt_%d", attempt), func(t *testing.T) {
			childScript := "import subprocess,time,signal; signal.signal(signal.SIGTERM, signal.SIG_IGN); grand=subprocess.Popen([\"python3\",\"-c\",\"import time,signal; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(120)\"]); print(grand.pid, flush=True); time.sleep(120)"
			parentScript := fmt.Sprintf("import subprocess,time; child=subprocess.Popen([\"python3\",\"-c\",%q], stdout=subprocess.PIPE, text=True); print(child.pid, flush=True); print(child.stdout.readline().strip(), flush=True); time.sleep(120)", childScript)

			cmd := exec.Command("python3", "-c", parentScript)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatalf("stdout pipe: %v", err)
			}

			s := New(cmd)
			if err := s.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			parentPID := s.PID()

			reader := bufio.NewReader(stdout)
			childPID := readPIDLine(t, reader, "child")
			grandchildPID := readPIDLine(t, reader, "grandchild")

			if err := s.Stop(300 * time.Millisecond); err != nil {
				t.Fatalf("stop: %v", err)
			}

			for _, pid := range []int{parentPID, childPID, grandchildPID} {
				if !waitForProcessExit(pid, 3*time.Second) {
					t.Fatalf("process %d is still running after deep-tree stop", pid)
				}
			}
		})
	}
}

func TestStopIsIdempotent(t *testing.T) {
	cmd := exec.Command("python3", "-c", "import time; time.sleep(120)")
	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	if err := s.Stop(500 * time.Millisecond); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if err := s.Stop(500 * time.Millisecond); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func TestTrapSignalsStopsProcess(t *testing.T) {
	cmd := exec.Command("python3", "-c", "import time; time.sleep(120)")
	s := New(cmd)
	if err := s.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := s.PID()

	untrap := s.TrapSignals(500*time.Millisecond, nil, syscall.SIGUSR1)
	defer untrap()

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatalf("send signal: %v", err)
	}

	if !waitForProcessExit(pid, 2*time.Second) {
		t.Fatalf("process %d still running after trapped signal cleanup", pid)
	}
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return !processExists(pid)
}

func readPIDLine(t *testing.T, r *bufio.Reader, label string) int {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read %s pid: %v", label, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse %s pid %q: %v", label, line, err)
	}
	return pid
}
