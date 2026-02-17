package cmd

import (
	"agent-sentry/internal/database"
	"agent-sentry/internal/feedback"
	"agent-sentry/internal/patterns"
	"agent-sentry/internal/state"
	"agent-sentry/internal/sysmon"
	"agent-sentry/internal/tokens"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var maxCpu float64
var modelName string
var noKill bool
var injectFeedback string
var deepWatch bool

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run -- <command> [args...]",
	Short: "Run a command with supervision",
	Long: `Starts a subprocess, captures its stdout/stderr, and monitors its CPU usage.
Example:
  agent-sentry run --model gpt-4 -- python3 script.py
  agent-sentry run --max-cpu 80.0 -- ./my-binary
  agent-sentry run --no-kill -- python3 stuck.py   (watchdog mode)
  agent-sentry run --inject-feedback agent_feedback.txt -- python3 agent.py`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Resolve max-cpu: CLI flag takes priority, then profile/config
		if !cmd.Flags().Changed("max-cpu") {
			configCpu := viper.GetFloat64("max-cpu")
			if configCpu > 0 {
				maxCpu = configCpu
			}
		}
		runProcess(args)
	},
}

func init() {
	rootCmd.AddCommand(runCmd)

	// Local flags
	runCmd.Flags().Float64Var(&maxCpu, "max-cpu", 90.0, "Maximum CPU usage threshold (Default: 90.0)")
	runCmd.Flags().StringVar(&modelName, "model", "gpt-4", "Model name for ROI calculation")
	runCmd.Flags().BoolVar(&noKill, "no-kill", false, "Watchdog mode: log & alert on loops but don't kill the process")
	runCmd.Flags().StringVar(&injectFeedback, "inject-feedback", "", "Path to feedback file to inject into subprocess stdin")
	runCmd.Flags().BoolVar(&deepWatch, "deep", false, "Enable Deep Watch (syscall monitoring)")
}

// LogObserver is a thread-safe ring buffer for log lines.
type LogObserver struct {
	mu          sync.Mutex
	lines       []string
	capacity    int
	buf         bytes.Buffer // Partial line buffer
	totalTokens int64
	modelName   string
}

func NewLogObserver(capacity int, model string) *LogObserver {
	return &LogObserver{
		lines:     make([]string, 0, capacity),
		capacity:  capacity,
		modelName: model,
	}
}

func (l *LogObserver) Write(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	n, _ = l.buf.Write(p)

	// Process lines from buffer
	for {
		i := bytes.IndexByte(l.buf.Bytes(), '\n')
		if i < 0 {
			break
		}

		line := l.buf.String()[:i]
		l.addLine(line)

		// Advance buffer
		l.buf.Next(i + 1)
	}

	return n, nil
}

func (l *LogObserver) addLine(line string) {
	if len(l.lines) >= l.capacity {
		// Shift
		l.lines = l.lines[1:]
	}
	l.lines = append(l.lines, line)

	// Count tokens
	count := tokens.Count(line, l.modelName)
	atomic.AddInt64(&l.totalTokens, int64(count))
}

func (l *LogObserver) TotalTokens() int64 {
	return atomic.LoadInt64(&l.totalTokens)
}

func (l *LogObserver) GetLastLines(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.lines) < n {
		return append([]string(nil), l.lines...)
	}
	return append([]string(nil), l.lines[len(l.lines)-n:]...)
}

func NormalizeLog(line string) string {
	// 1. Hex addresses: 0x...
	reHex := regexp.MustCompile(`0x[0-9a-fA-F]+`)
	line = reHex.ReplaceAllString(line, "<HEX>")

	// 2. ISO 8601 Timestamps, local times:
	reTime := regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`)
	line = reTime.ReplaceAllString(line, "<TIME>")

	// Catch simple times like 12:34:56
	reSimpleTime := regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}\b`)
	line = reSimpleTime.ReplaceAllString(line, "<TIME>")

	// 3. Numbers (integers and floats)
	reNum := regexp.MustCompile(`\b\d+(\.\d+)?\b`)
	line = reNum.ReplaceAllString(line, "<NUM>")

	return line
}

func runProcess(args []string) {
	if err := database.InitDB(); err != nil {
		fmt.Printf("Warning: Failed to initialize database: %v\n", err)
	}
	defer database.CloseDB()

	// Pull known bad patterns on startup
	blacklist := patterns.PullPatterns()

	cmdName := args[0]
	cmdArgs := args[1:]
	fullCommand := strings.Join(args, " ")
	startTime := time.Now()

	// Read profile-based config values
	pollInterval := viper.GetInt("poll-interval")
	if pollInterval <= 0 {
		pollInterval = 500
	}
	logWindow := viper.GetInt("log-window")
	if logWindow <= 0 {
		logWindow = 10
	}

	// Generate transient Agent ID for this run
	agentID := uuid.New().String()
	agentVersion := "1.0.0"
	fmt.Printf("[Sentry] 🆔 Agent ID: %s (v%s)\n", agentID, agentVersion)

	fmt.Printf("[Sentry] Config: max-cpu=%.1f%%, poll-interval=%dms, log-window=%d, no-kill=%v\n",
		maxCpu, pollInterval, logWindow, noKill)

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)

	// Initialize LogObserver with profile-based capacity
	observer := NewLogObserver(logWindow*2, modelName)

	// MultiWriter to print to stdout and capture in observer
	stdoutWriter := io.MultiWriter(os.Stdout, observer)
	stderrWriter := io.MultiWriter(os.Stderr, observer)

	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	// Handle --inject-feedback: pipe feedback into subprocess stdin
	if injectFeedback != "" {
		feedbackContent, err := feedback.ReadFeedback(injectFeedback)
		if err != nil {
			fmt.Printf("[Sentry] Warning: Could not read feedback file: %v\n", err)
		} else {
			fmt.Printf("[Sentry] 💉 Injecting feedback from: %s\n", injectFeedback)
			cmd.Stdin = strings.NewReader(feedbackContent)
			// Clean up after injection
			defer feedback.CleanupFeedback(injectFeedback)
		}
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start command: %v\n", err)
		os.Exit(1)
	}

	pid := cmd.Process.Pid
	fmt.Printf("Process started with PID: %d\n", pid)

	var maxObservedCpu float64 = 0.0
	var lastWatchdogAlert time.Time
	var watchdogEscalationLevel int = 0
	var initialFDs int = 0

	// Create Monitor instance
	monitor := sysmon.NewMonitor()

	// CPU Monitoring Goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(pollInterval) * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
					return
				}
				cpuUsage, err := getCPUUsage(pid)
				if err != nil {
					continue
				}

				if cpuUsage > maxObservedCpu {
					maxObservedCpu = cpuUsage
				}

				// Broadcast Live Stats (with PID)
				lastLines := observer.GetLastLines(1)
				lastLine := ""
				if len(lastLines) > 0 {
					lastLine = lastLines[0]
				}

				// Deep Watch (Syscall Monitoring)
				isProbing := false
				sysStatsStr := ""
				if deepWatch {
					stats, err := monitor.GetStats(pid)
					if err == nil {
						// Check for probing (sudden spike)
						// Use monitor's internal logic
						probing, details := monitor.DetectProbing(pid, stats)
						if probing {
							isProbing = true
							sysStatsStr = details
						}

						if initialFDs == 0 {
							initialFDs = stats.OpenFDs
						}

						// Also keep local logic? No, monitor does it.
						// But existing code check:
						/*
							if stats.OpenFDs > initialFDs*2 && stats.OpenFDs > 50 {
								isProbing = true
							}
						*/
						// Let's rely on Monitor's return
						if !isProbing {
							// Display stats if not probing for debug?
							sysStatsStr = fmt.Sprintf("FDs: %d | Sockets: %d", stats.OpenFDs, stats.SocketCount)
						}
					}
				}

				status := "RUNNING"
				if isProbing {
					status = "PROBING_DETECTED"
					fmt.Printf("\n[Sentry] 🚨 PROBING DETECTED: %s\n", sysStatsStr)
				}

				wd, _ := os.Getwd()
				state.UpdateState(
					cpuUsage,
					lastLine,
					status,
					fullCommand,
					args,
					wd,
					pid,
				)

				// Early blacklist check (even before high CPU)
				if len(blacklist) > 0 {
					recentLines := observer.GetLastLines(3)
					for _, line := range recentLines {
						normalized := NormalizeLog(line)
						if patterns.IsBlacklisted(normalized, blacklist) {
							fmt.Printf("\n⚡ EARLY WARNING: Output matches a known bad pattern from blacklist!\n")
							fmt.Println("Pattern:", normalized)
							break
						}
					}
				}

				// Loop Detection Logic
				if cpuUsage > maxCpu {
					// Check for Semantic Stagnation (Fuzzy)
					lastLines := observer.GetLastLines(logWindow)
					if len(lastLines) == logWindow {
						firstNormalized := NormalizeLog(lastLines[0])
						isStagnant := true
						lev := metrics.NewLevenshtein()

						for _, line := range lastLines[1:] {
							currentNormalized := NormalizeLog(line)
							similarity := strutil.Similarity(firstNormalized, currentNormalized, lev)

							// DEBUG
							fmt.Printf("DEBUG: Sim=%.2f\nBase: %s\nCurr: %s\n", similarity, firstNormalized, currentNormalized)

							if similarity < 0.9 { // < 90% similarity means different
								isStagnant = false
								break
							}
						}

						if isStagnant {
							// Sync pattern to blacklist
							patterns.SyncPatterns(firstNormalized)

							// Generate feedback file
							feedback.GenerateFeedback(feedback.FeedbackData{
								Command:    fullCommand,
								Pattern:    firstNormalized,
								ExitReason: "LOOP_DETECTED",
								MaxCPU:     cpuUsage,
								ModelName:  modelName,
								Savings:    0, // Will be calculated by DB
							})

							if noKill {
								// WATCHDOG MODE: Escalation Logic
								cooldown := 30 * time.Second
								if watchdogEscalationLevel == 1 {
									cooldown = 15 * time.Second
								} else if watchdogEscalationLevel >= 2 {
									cooldown = 5 * time.Second
								}

								if time.Since(lastWatchdogAlert) > cooldown {
									lastWatchdogAlert = time.Now()
									watchdogEscalationLevel++

									alertType := "WATCHDOG_ALERT"
									if watchdogEscalationLevel == 2 {
										alertType = "WATCHDOG_WARN"
									} else if watchdogEscalationLevel > 2 {
										alertType = "WATCHDOG_CRITICAL"
									}

									fmt.Printf("\n🔍 WATCHDOG [%s]: Loop detected. Escalation Level %d.\n", alertType, watchdogEscalationLevel)
									fmt.Println("Pattern (Normalized):", firstNormalized)

									finalTokens := int(observer.TotalTokens())
									finalCost := tokens.EstimateCost(finalTokens, modelName)

									// Log to DB
									database.LogIncident(fullCommand, modelName, alertType, cpuUsage, firstNormalized, time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

									// Broadcast
									wd, _ := os.Getwd()
									state.UpdateState(
										cpuUsage,
										fmt.Sprintf("WATCHDOG: Loop detected (%s)", alertType),
										alertType,
										fullCommand,
										args,
										wd,
										pid,
									)
								}
							} else {
								// NORMAL MODE: Kill the process
								fmt.Printf("\n🚨 LOOP DETECTED: Semantic Stagnation (CPU: %.2f%% > %.2f%%)\n", cpuUsage, maxCpu)
								fmt.Println("Pattern (Normalized):", firstNormalized)

								finalTokens := int(observer.TotalTokens())
								finalCost := tokens.EstimateCost(finalTokens, modelName)

								// Log to DB
								database.LogIncident(fullCommand, modelName, "LOOP_DETECTED", cpuUsage, firstNormalized, time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

								// Broadcast Stop
								wd, _ := os.Getwd()
								state.UpdateState(
									cpuUsage,
									"LOOP DETECTED - Terminating...",
									"LOOP_DETECTED",
									fullCommand,
									args,
									wd,
									pid,
								)

								// Kill the process group
								if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
									cmd.Process.Kill()
								}

								fmt.Println("Synthetic Error: Loop detected. Terminating process.")
								os.Exit(1)
								return
							}
						}
					}
					fmt.Printf("[Sentry] WARNING: High CPU (%.2f%%) detected. %s\n", cpuUsage, sysStatsStr)
				}

				// --- SAFETY CHOKE POINT ---
				// 1. Memory Limit
				maxMemMB := viper.GetFloat64("max-memory-mb")
				if maxMemMB > 0 {
					memMB, err := getMemoryRSS(pid)
					if err == nil {
						if memMB > maxMemMB {
							fmt.Printf("\n[Sentry] 🛑 SAFETY CHOKE: Memory usage (%.2f MB) exceeded limit (%.2f MB). TERMINATING.\n", memMB, maxMemMB)
							time.Sleep(100 * time.Millisecond) // Ensure log flushes

							finalTokens := int(observer.TotalTokens())
							finalCost := tokens.EstimateCost(finalTokens, modelName)
							database.LogIncident(fullCommand, modelName, "SAFETY_LIMIT_EXCEEDED", cpuUsage, fmt.Sprintf("Memory Limit: %.2fMB", memMB), time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

							if err := cmd.Process.Kill(); err != nil {
								// force kill
								syscall.Kill(-pid, syscall.SIGKILL)
							}
							return
						}
					}
				}

				// 2. Token Rate Limit (Choke)
				maxTokensRate := viper.GetFloat64("max-tokens-per-min")
				if maxTokensRate > 0 {
					currentTokens := observer.TotalTokens()
					elapsedMin := time.Since(startTime).Minutes()
					if elapsedMin > 0.1 { // Warmup 6s
						rate := float64(currentTokens) / elapsedMin
						if rate > maxTokensRate {
							fmt.Printf("\n[Sentry] 🛑 SAFETY CHOKE: Token generation rate (%.0f/min) exceeded limit (%.0f/min). TERMINATING.\n", rate, maxTokensRate)

							finalTokens := int(currentTokens)
							finalCost := tokens.EstimateCost(finalTokens, modelName)
							database.LogIncident(fullCommand, modelName, "SAFETY_LIMIT_EXCEEDED", cpuUsage, fmt.Sprintf("Token Rate: %.0f/min", rate), time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

							if err := cmd.Process.Kill(); err != nil {
								syscall.Kill(-pid, syscall.SIGKILL)
							}
							return
						}
					}
				}
			}
		}
	}()

	// Signal Handling Goroutine
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var userTerminated atomic.Bool

	go func() {
		sig := <-sigChan
		fmt.Printf("\n[Sentry] Received signal: %v. Forwarding to subprocess...\n", sig)

		userTerminated.Store(true)
		finalTokens := int(observer.TotalTokens())
		finalCost := tokens.EstimateCost(finalTokens, modelName)
		database.LogIncident(fullCommand, modelName, "USER_TERMINATED", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

		// Write STOPPED state
		wd, _ := os.Getwd()
		state.UpdateState(
			0,
			"",
			"STOPPED",
			fullCommand,
			args,
			wd,
			pid,
		)

		if err := syscall.Kill(-pid, sig.(syscall.Signal)); err != nil {
			cmd.Process.Signal(sig)
		}
	}()

	err := cmd.Wait()
	cancel()

	// Write STOPPED state on exit
	wd, _ := os.Getwd()
	state.UpdateState(
		0,
		"",
		"STOPPED",
		fullCommand,
		args,
		wd,
		pid,
	)

	if err != nil {
		fmt.Printf("DEBUG: Wait returned error: %v | UserTerminated: %v\n", err, userTerminated.Load())
		if exitErr, ok := err.(*exec.ExitError); ok {
			isSignal := strings.Contains(exitErr.String(), "signal: killed") || strings.Contains(exitErr.String(), "signal: interrupt")

			if !userTerminated.Load() && !isSignal {
				finalTokens := int(observer.TotalTokens())
				finalCost := tokens.EstimateCost(finalTokens, modelName)
				database.LogIncident(fullCommand, modelName, "COMMAND_FAILURE", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)
			}
			os.Exit(exitErr.ExitCode())
		} else {
			fmt.Printf("Command finished with error: %v\n", err)
			if !userTerminated.Load() {
				finalTokens := int(observer.TotalTokens())
				finalCost := tokens.EstimateCost(finalTokens, modelName)
				database.LogIncident(fullCommand, modelName, "COMMAND_FAILURE", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)
			}
			os.Exit(1)
		}
	} else {
		fmt.Printf("DEBUG: Wait success | UserTerminated: %v\n", userTerminated.Load())
		// Success
		finalTokens := int(observer.TotalTokens())
		finalCost := tokens.EstimateCost(finalTokens, modelName)
		database.LogIncident(fullCommand, modelName, "SUCCESS", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)
	}
}

func getCPUUsage(pid int) (float64, error) {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu").Output()
	if err != nil {
		return 0, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output")
	}

	usageStr := strings.TrimSpace(lines[1])
	return strconv.ParseFloat(usageStr, 64)
}

func getMemoryRSS(pid int) (float64, error) {
	// Returns in MB
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "rss").Output()
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output")
	}
	// RSS is in KB
	rssKB_str := strings.TrimSpace(lines[1])
	rssKB, err := strconv.ParseFloat(rssKB_str, 64)
	if err != nil {
		return 0, err
	}
	return rssKB / 1024.0, nil
}
