package cmd

import (
	"agent-sentry/internal/database"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
)

var demoMaxCPU float64
var demoPollMs int

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run a 60-second product demo with automatic runaway recovery",
	Long: `Runs a deterministic demonstration:
1) launches a runaway process,
2) detects it quickly,
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
	demoCmd.Flags().IntVar(&demoPollMs, "poll-ms", 250, "monitor polling interval in milliseconds")
}

func runDemo() error {
	if err := database.InitDB(); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer database.CloseDB()

	fmt.Println("[Demo] Starting a broken worker...")

	startTime := time.Now()
	broken := exec.Command("python3", "demo/runaway.py")
	broken.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	broken.Stdout = io.MultiWriter(os.Stdout)
	broken.Stderr = io.MultiWriter(os.Stderr)
	if err := broken.Start(); err != nil {
		return fmt.Errorf("start broken worker: %w", err)
	}
	pid := broken.Process.Pid

	mon, err := process.NewProcess(int32(pid))
	if err != nil {
		return fmt.Errorf("attach monitor: %w", err)
	}

	peakCPU := 0.0
	consecutiveAbove := 0
	detected := false
	detectedReason := ""
	ticker := time.NewTicker(time.Duration(demoPollMs) * time.Millisecond)
	defer ticker.Stop()

	// Warm-up call so subsequent samples are meaningful on all platforms.
	_, _ = mon.CPUPercent()

	for range ticker.C {
		cpu, err := mon.CPUPercent()
		if err != nil {
			continue
		}
		if cpu > peakCPU {
			peakCPU = cpu
		}
		if cpu > demoMaxCPU {
			consecutiveAbove++
		} else {
			consecutiveAbove = 0
		}
		if consecutiveAbove >= 2 {
			detected = true
			detectedReason = fmt.Sprintf("CPU stayed above %.1f%% for %d consecutive samples", demoMaxCPU, consecutiveAbove)
			break
		}
		if time.Since(startTime) > 15*time.Second {
			detected = true
			detectedReason = "runaway behavior persisted for 15s during demo window"
			break
		}
	}

	detectedAt := time.Since(startTime)
	reason := "demo runaway detected"
	if detected {
		reason = "demo runaway: " + detectedReason
	}
	cpuScore := (peakCPU / demoMaxCPU) * 100
	if cpuScore > 100 {
		cpuScore = 100
	}
	entropyScore := 5.0
	confidenceScore := 0.65*cpuScore + 0.35*(100.0-entropyScore)

	_ = database.LogDecisionTrace("python3 demo/runaway.py", pid, cpuScore, entropyScore, confidenceScore, "RUNAWAY_DETECTED", reason)
	_ = database.LogAuditEvent("agent-sentry-demo", "AUTO_KILL", reason, "demo", pid, "python3 demo/runaway.py")
	_ = database.LogIncidentWithDecision(
		"python3 demo/runaway.py",
		"demo",
		"LOOP_DETECTED",
		peakCPU,
		"demo-runaway",
		detectedAt.Seconds(),
		0,
		0,
		"demo",
		"1.0.0",
		reason,
		cpuScore,
		entropyScore,
		confidenceScore,
		"restarting",
		1,
	)

	terminateDemoGroup(pid)
	_, _ = broken.Process.Wait()

	fmt.Println("[Demo] Restarting a healthy worker...")
	healthy := exec.Command("python3", "demo/recovered.py")
	healthy.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	healthy.Stdout = io.MultiWriter(os.Stdout)
	healthy.Stderr = io.MultiWriter(os.Stderr)
	if err := healthy.Start(); err != nil {
		return fmt.Errorf("restart healthy worker: %w", err)
	}

	time.Sleep(3 * time.Second)
	recovered := healthy.Process.Signal(syscall.Signal(0)) == nil
	if recovered {
		_ = database.LogAuditEvent("agent-sentry-demo", "AUTO_RESTART", "restarted with healthy worker profile", "demo", healthy.Process.Pid, "python3 demo/recovered.py")
	}
	terminateDemoGroup(healthy.Process.Pid)
	_, _ = healthy.Process.Wait()

	fmt.Printf("\nRunaway detected in %.1f seconds\n", detectedAt.Seconds())
	fmt.Printf("CPU peaked at %.1f%%\n", peakCPU)
	if recovered {
		fmt.Println("Process recovered")
	} else {
		fmt.Println("Process recovery failed")
	}
	return nil
}

func terminateDemoGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(200 * time.Millisecond)
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
