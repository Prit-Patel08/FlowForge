package cmd

import (
	"agent-sentry/internal/database"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var demoMaxCPU float64

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run a 60-second product demo with automatic runaway recovery",
	Long: `Runs a deterministic demonstration using the same supervision pipeline as 'run':
1) launches a runaway process under sentry run,
2) detects runaway behavior,
3) terminates it automatically,
4) restarts a healthy worker,
5) prints an outcome summary.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDemo()
	},
}

func init() {
	rootCmd.AddCommand(demoCmd)
	demoCmd.Flags().Float64Var(&demoMaxCPU, "max-cpu", 30.0, "CPU threshold used to trigger runaway handling")
}

func runDemo() error {
	fmt.Println("[Demo] Starting runaway worker through 'sentry run'...")

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	startTime := time.Now()
	runArgs := []string{
		"run",
		"--max-cpu", fmt.Sprintf("%.1f", demoMaxCPU),
		"--",
		"python3", "demo/runaway.py",
	}

	supervised := exec.Command(exePath, runArgs...)
	supervised.Stdout = os.Stdout
	supervised.Stderr = os.Stderr

	if err := supervised.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Printf("[Demo] Supervision exited with code %d (expected after intervention).\n", exitErr.ExitCode())
		} else {
			return fmt.Errorf("run supervised demo: %w", err)
		}
	}

	if err := database.InitDB(); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer database.CloseDB()

	incident, err := latestDemoIncidentSince(startTime)
	if err != nil {
		return fmt.Errorf("locate demo incident: %w", err)
	}

	detectedAt := incident.TokenSavingsEstimate
	if detectedAt <= 0 {
		detectedAt = time.Since(startTime).Seconds()
	}

	fmt.Println("[Demo] Restarting a healthy worker...")
	recovered, healthyPID := restartHealthyWorker()
	if recovered {
		_ = database.LogAuditEvent("agent-sentry-demo", "AUTO_RESTART", "restarted with healthy worker profile", "demo", healthyPID, "python3 demo/recovered.py")
	}

	fmt.Printf("\nRunaway detected in %.1f seconds\n", detectedAt)
	fmt.Printf("CPU peaked at %.1f%%\n", incident.MaxCPU)
	if recovered {
		fmt.Println("Process recovered")
	} else {
		fmt.Println("Process recovery failed")
	}

	return nil
}

func latestDemoIncidentSince(start time.Time) (database.Incident, error) {
	incidents, err := database.GetAllIncidents()
	if err != nil {
		return database.Incident{}, err
	}
	for _, inc := range incidents {
		ts := parseIncidentTimestamp(inc.Timestamp)
		if !ts.IsZero() && ts.Before(start.Add(-1*time.Second)) {
			continue
		}
		if !strings.Contains(inc.Command, "demo/runaway.py") {
			continue
		}
		switch inc.ExitReason {
		case "LOOP_DETECTED", "WATCHDOG_ALERT", "WATCHDOG_WARN", "WATCHDOG_CRITICAL", "SAFETY_LIMIT_EXCEEDED":
			return inc, nil
		}
	}
	return database.Incident{}, fmt.Errorf("no demo incident found in recent history")
}

func parseIncidentTimestamp(raw string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t
		}
	}
	return time.Time{}
}

func restartHealthyWorker() (bool, int) {
	healthy := exec.Command("python3", "demo/recovered.py")
	healthy.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	healthy.Stdout = os.Stdout
	healthy.Stderr = os.Stderr
	if err := healthy.Start(); err != nil {
		return false, 0
	}

	time.Sleep(3 * time.Second)
	recovered := healthy.Process.Signal(syscall.Signal(0)) == nil
	terminateDemoGroup(healthy.Process.Pid)
	_, _ = healthy.Process.Wait()
	return recovered, healthy.Process.Pid
}

func terminateDemoGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
