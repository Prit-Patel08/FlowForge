package cmd

import (
	"bytes"
	"context"
	"flowforge/internal/api"
	"flowforge/internal/database"
	"flowforge/internal/feedback"
	"flowforge/internal/patterns"
	"flowforge/internal/policy"
	"flowforge/internal/redact"
	"flowforge/internal/state"
	"flowforge/internal/supervisor"
	"flowforge/internal/sysmon"
	"flowforge/internal/tokens"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var maxCpu float64
var modelName string
var noKill bool
var shadowMode bool
var policyRollout string
var policyCanaryPercent int
var injectFeedback string
var deepWatch bool
var firstNumberRegex = regexp.MustCompile(`\d+`)

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run -- <command> [args...]",
	Short: "Run a command with supervision",
	Long: `Starts a subprocess, captures its stdout/stderr, and monitors its CPU usage.
Example:
  flowforge run --model gpt-4 -- python3 script.py
  flowforge run --max-cpu 80.0 -- ./my-binary
  flowforge run --no-kill -- python3 stuck.py   (watchdog mode)
  flowforge run --inject-feedback agent_feedback.txt -- python3 agent.py`,
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
	runCmd.Flags().Float64Var(&maxCpu, "max-cpu", 60.0, "Maximum CPU usage threshold (Default: 60.0)")
	runCmd.Flags().StringVar(&modelName, "model", "gpt-4", "Model name for ROI calculation")
	runCmd.Flags().BoolVar(&noKill, "no-kill", false, "Watchdog mode: log & alert on loops but don't kill the process")
	runCmd.Flags().BoolVar(&shadowMode, "shadow-mode", false, "Policy dry-run mode: evaluate actions but log-only for intervention")
	runCmd.Flags().StringVar(&policyRollout, "policy-rollout", "", "Policy rollout mode: shadow, canary, enforce (default: enforce; shadow-mode remains backward-compatible)")
	runCmd.Flags().IntVar(&policyCanaryPercent, "policy-canary-percent", -1, "Policy canary enforcement percentage (0-100). In canary mode, unsampled runs are log-only")
	runCmd.Flags().StringVar(&injectFeedback, "inject-feedback", "", "Path to feedback file to inject into subprocess stdin")
	runCmd.Flags().BoolVar(&deepWatch, "deep", false, "Enable Deep Watch (syscall monitoring)")
}

// LogObserver is a thread-safe bounded ring buffer for log lines.
type LogObserver struct {
	mu          sync.Mutex
	lines       []string
	capacity    int
	index       int          // Current write index
	isFull      bool         // Whether the buffer has wrapped
	buf         bytes.Buffer // Partial line buffer
	totalTokens int64
	modelName   string
}

func NewLogObserver(capacity int, model string) *LogObserver {
	if capacity <= 0 {
		capacity = 10 // Safety floor
	}
	return &LogObserver{
		lines:     make([]string, capacity),
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
	// Prevent token/key leakage to state/dashboard surfaces.
	line = redact.Line(line)

	l.lines[l.index] = line
	l.index++
	if l.index >= l.capacity {
		l.index = 0
		l.isFull = true
	}

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

	if n > l.capacity {
		n = l.capacity
	}

	total := l.index
	if l.isFull {
		total = l.capacity
	}

	if n > total {
		n = total
	}

	result := make([]string, 0, n)
	// Calculate starting point (n lines back from current index)
	start := (l.index - n + l.capacity) % l.capacity

	for i := 0; i < n; i++ {
		idx := (start + i) % l.capacity
		result = append(result, l.lines[idx])
	}

	return result
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

func calculateDecisionScores(cpuUsage, threshold float64, lines []string) (cpuScore, entropyScore, confidence float64) {
	if threshold <= 0 {
		threshold = 100
	}
	cpuScore = (cpuUsage / threshold) * 100.0
	if cpuScore > 100 {
		cpuScore = 100
	}
	if cpuScore < 0 {
		cpuScore = 0
	}

	if len(lines) == 0 {
		entropyScore = 100
	} else {
		uniq := make(map[string]struct{}, len(lines))
		for _, line := range lines {
			uniq[NormalizeLog(line)] = struct{}{}
		}
		entropyScore = (float64(len(uniq)) / float64(len(lines))) * 100.0
	}

	// Confidence increases with CPU pressure and repetitive output (low entropy).
	confidence = 0.65*cpuScore + 0.35*(100.0-entropyScore)
	if confidence > 100 {
		confidence = 100
	}
	if confidence < 0 {
		confidence = 0
	}

	return cpuScore, entropyScore, confidence
}

func calculateRepetitionScore(lines []string) (firstNormalized string, repetitionScore float64) {
	if len(lines) == 0 {
		return "", 0
	}
	if len(lines) == 1 {
		return NormalizeLog(lines[0]), 0
	}

	firstNormalized = NormalizeLog(lines[0])
	stagnantMatches := 0
	lev := metrics.NewLevenshtein()
	for _, line := range lines[1:] {
		currentNormalized := NormalizeLog(line)
		if strutil.Similarity(firstNormalized, currentNormalized, lev) >= 0.9 {
			stagnantMatches++
		}
	}

	repetitionScore = float64(stagnantMatches) / float64(len(lines)-1)
	return firstNormalized, repetitionScore
}

func rawDiversityScore(lines []string) float64 {
	if len(lines) == 0 {
		return 1.0
	}
	uniq := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		uniq[line] = struct{}{}
	}
	return float64(len(uniq)) / float64(len(lines))
}

func detectProgressLikeOutput(lines []string) bool {
	if len(lines) < 4 {
		return false
	}

	progressHints := 0
	numericLines := 0
	increaseCount := 0
	comparisons := 0

	var prevNumber int
	hasPrevNumber := false

	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "progress") ||
			strings.Contains(lower, "step=") ||
			strings.Contains(lower, "tick=") ||
			strings.Contains(lower, "phase=") ||
			strings.Contains(lower, "heartbeat") {
			progressHints++
		}

		match := firstNumberRegex.FindString(lower)
		if match == "" {
			continue
		}

		numericLines++
		value, err := strconv.Atoi(match)
		if err != nil {
			continue
		}

		if hasPrevNumber {
			comparisons++
			if value > prevNumber {
				increaseCount++
			}
		}

		prevNumber = value
		hasPrevNumber = true
	}

	if comparisons == 0 {
		return false
	}

	progressHintRatio := float64(progressHints) / float64(len(lines))
	numericCoverage := float64(numericLines) / float64(len(lines))
	increaseRatio := float64(increaseCount) / float64(comparisons)

	return progressHintRatio >= 0.40 && numericCoverage >= 0.70 && increaseRatio >= 0.70
}

func resolvePolicyRolloutConfig() (policy.RolloutMode, int) {
	mode := strings.ToLower(strings.TrimSpace(policyRollout))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(viper.GetString("policy-rollout")))
	}

	switch mode {
	case string(policy.RolloutShadow), string(policy.RolloutCanary), string(policy.RolloutEnforce):
		// valid
	case "":
		if shadowMode {
			mode = string(policy.RolloutShadow)
		} else {
			mode = string(policy.RolloutEnforce)
		}
	default:
		mode = string(policy.RolloutEnforce)
	}

	// Backward compatibility: old --shadow-mode still forces shadow unless rollout is explicitly non-enforce.
	if shadowMode && mode == string(policy.RolloutEnforce) {
		mode = string(policy.RolloutShadow)
	}

	canaryPercent := policyCanaryPercent
	if canaryPercent < 0 {
		if viper.IsSet("policy-canary-percent") {
			canaryPercent = viper.GetInt("policy-canary-percent")
		} else if mode == string(policy.RolloutCanary) {
			canaryPercent = 10
		} else {
			canaryPercent = 0
		}
	}
	if canaryPercent < 0 {
		canaryPercent = 0
	}
	if canaryPercent > 100 {
		canaryPercent = 100
	}

	return policy.RolloutMode(mode), canaryPercent
}

func runProcess(args []string) {
	if err := database.InitDB(); err != nil {
		fmt.Printf("Warning: Failed to initialize database: %v\n", err)
	}
	defer database.CloseDB()

	// Start API server in background
	fmt.Println("[FlowForge] Starting API server on port 8080...")
	stopAPI := api.Start("8080")
	defer stopAPI()

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
	rolloutMode, canaryPercent := resolvePolicyRolloutConfig()

	// Generate transient Agent ID for this run
	agentID := uuid.New().String()
	agentVersion := "1.0.0"
	fmt.Printf("[FlowForge] 🆔 Agent ID: %s (v%s)\n", agentID, agentVersion)

	fmt.Printf("[FlowForge] Config: max-cpu=%.1f%%, poll-interval=%dms, log-window=%d, no-kill=%v, policy-rollout=%s, policy-canary=%d%%\n",
		maxCpu, pollInterval, logWindow, noKill, rolloutMode, canaryPercent)

	// Create a context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.Command(cmdName, cmdArgs...)

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
			fmt.Printf("[FlowForge] Warning: Could not read feedback file: %v\n", err)
		} else {
			fmt.Printf("[FlowForge] 💉 Injecting feedback from: %s\n", injectFeedback)
			cmd.Stdin = strings.NewReader(feedbackContent)
			// Clean up after injection
			defer feedback.CleanupFeedback(injectFeedback)
		}
	}

	procSupervisor := supervisor.New(cmd)
	if err := procSupervisor.Start(); err != nil {
		fmt.Printf("Failed to start command: %v\n", err)
		os.Exit(1)
	}

	pid := procSupervisor.PID()
	fmt.Printf("Process started with PID: %d\n", pid)
	database.SetRunID(agentID)
	startWD, _ := os.Getwd()
	api.RegisterExternalWorker(fullCommand, args, startWD, procSupervisor)
	api.SetWorkerSpec(fullCommand, args, startWD)
	state.UpdateState(0, "", "RUNNING", fullCommand, args, startWD, pid)
	state.UpdateLifecycle("RUNNING", "RUNNING", pid)

	var maxObservedCpu float64 = 0.0
	var lastWatchdogAlert time.Time
	var lastDecisionTrace time.Time
	var watchdogEscalationLevel int = 0
	var initialFDs int = 0
	var flowforgeTerminated atomic.Bool
	var highCPUStart time.Time

	cpuWindow := time.Duration(viper.GetInt("cpu-window-seconds")) * time.Second
	if cpuWindow <= 0 {
		cpuWindow = time.Duration(pollInterval*logWindow) * time.Millisecond
	}
	policyDecider := policy.NewThresholdDecider()
	policyConfig := policy.Policy{
		MaxCPUPercent:     maxCpu,
		CPUWindow:         cpuWindow,
		MinLogEntropy:     0.20,
		MaxLogRepetition:  0.80,
		MaxMemoryMB:       viper.GetFloat64("max-memory-mb"),
		RestartOnBreach:   false,
		ShadowMode:        shadowMode,
		RolloutMode:       rolloutMode,
		CanaryPercent:     canaryPercent,
		DryRunEventType:   "policy_dry_run",
		DryRunActor:       "system",
		DryRunEventPrefix: "Policy dry-run",
	}
	engineContract := policy.CurrentEngineContract(rolloutMode)
	decisionTraceMeta := database.DecisionTraceMeta{
		DecisionEngine:    engineContract.EngineName,
		EngineVersion:     engineContract.EngineVersion,
		DecisionContract:  engineContract.ContractVersion,
		PolicyRolloutMode: engineContract.RolloutMode,
	}
	buildDecisionMeta := func(decisionValue, reasonText string, cpuScore, entropyScore, confidenceScore float64) database.DecisionTraceMeta {
		meta := decisionTraceMeta
		meta.ReplayContract = policy.DecisionReplayContractVersion
		meta.ReplayDigest = policy.DecisionReplayDigest(policy.DecisionReplayInput{
			DecisionEngine:   meta.DecisionEngine,
			EngineVersion:    meta.EngineVersion,
			DecisionContract: meta.DecisionContract,
			RolloutMode:      meta.PolicyRolloutMode,
			Decision:         decisionValue,
			Reason:           reasonText,
			CPUScore:         cpuScore,
			EntropyScore:     entropyScore,
			ConfidenceScore:  confidenceScore,
		})
		return meta
	}
	fmt.Printf("[FlowForge] Decision engine=%s version=%s contract=%s rollout=%s\n",
		engineContract.EngineName,
		engineContract.EngineVersion,
		engineContract.ContractVersion,
		engineContract.RolloutMode,
	)

	// Create Monitor instance
	monitor := sysmon.NewMonitor()

	// CPU Monitoring Goroutine
	go func() {
		ticker := time.NewTicker(time.Duration(pollInterval) * time.Millisecond)
		defer ticker.Stop()

		p, err := process.NewProcess(int32(pid))
		if err != nil {
			fmt.Printf("[FlowForge] Error attaching monitor to PID %d: %v\n", pid, err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if procSupervisor.Exited() {
					return
				}
				cpuUsage, err := p.CPUPercent()
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
					fmt.Printf("\n[FlowForge] 🚨 PROBING DETECTED: %s\n", sysStatsStr)
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

				if cpuUsage > maxCpu {
					if highCPUStart.IsZero() {
						highCPUStart = time.Now()
					}
				} else {
					highCPUStart = time.Time{}
				}

				windowLines := observer.GetLastLines(logWindow)
				if len(windowLines) == logWindow {
					firstNormalized, repetitionScore := calculateRepetitionScore(windowLines)

					cpuScore, entropyScore, confidenceScore := calculateDecisionScores(cpuUsage, maxCpu, windowLines)
					rawDiversity := rawDiversityScore(windowLines)
					progressLike := detectProgressLikeOutput(windowLines)
					cpuOverFor := time.Duration(0)
					if !highCPUStart.IsZero() {
						cpuOverFor = time.Since(highCPUStart)
					}
					memMB := 0.0
					if memInfo, err := p.MemoryInfo(); err == nil {
						memMB = float64(memInfo.RSS) / 1024.0 / 1024.0
					}

					decision := policyDecider.Evaluate(policy.Telemetry{
						CPUPercent:    cpuUsage,
						CPUOverFor:    cpuOverFor,
						MemoryMB:      memMB,
						LogRepetition: repetitionScore,
						LogEntropy:    entropyScore / 100.0,
						RawDiversity:  rawDiversity,
						ProgressLike:  progressLike,
						RolloutKey:    agentID,
					}, policyConfig)
					reason := decision.Reason

					if time.Since(lastDecisionTrace) > 5*time.Second || decision.Action != policy.ActionContinue {
						meta := buildDecisionMeta(decision.Action.String(), reason, cpuScore, entropyScore, confidenceScore)
						_ = database.LogDecisionTraceWithMeta(fullCommand, pid, cpuScore, entropyScore, confidenceScore, decision.Action.String(), reason, meta)
						lastDecisionTrace = time.Now()
					}
					state.UpdateDecision(reason, cpuScore, entropyScore, confidenceScore)

					switch decision.Action {
					case policy.ActionContinue:
						// no-op
					case policy.ActionAlert:
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
							incidentID := uuid.NewString()

							alertType := "WATCHDOG_ALERT"
							if watchdogEscalationLevel == 2 {
								alertType = "WATCHDOG_WARN"
							} else if watchdogEscalationLevel > 2 {
								alertType = "WATCHDOG_CRITICAL"
							}

							if repetitionScore >= policyConfig.MaxLogRepetition {
								patterns.SyncPatterns(firstNormalized)
							}

							fmt.Printf("\n🔍 WATCHDOG [%s]: Policy alert. Escalation Level %d.\n", alertType, watchdogEscalationLevel)
							fmt.Println("Pattern (Normalized):", firstNormalized)
							fmt.Printf("[FlowForge] Decision: CPU=%.1f Entropy=%.1f Confidence=%.1f\n", cpuScore, entropyScore, confidenceScore)

							finalTokens := int(observer.TotalTokens())
							finalCost := tokens.EstimateCost(finalTokens, modelName)

							meta := buildDecisionMeta(decision.Action.String(), reason, cpuScore, entropyScore, confidenceScore)
							_ = database.LogDecisionTraceWithIncidentAndMeta(fullCommand, pid, cpuScore, entropyScore, confidenceScore, decision.Action.String(), reason, incidentID, meta)
							_ = database.LogIncidentWithDecisionForIncident(
								fullCommand,
								modelName,
								alertType,
								cpuUsage,
								firstNormalized,
								time.Since(startTime).Seconds(),
								finalTokens,
								finalCost,
								agentID,
								agentVersion,
								reason,
								cpuScore,
								entropyScore,
								confidenceScore,
								"watchdog",
								0,
								incidentID,
							)
							_ = database.LogAuditEventWithIncident("flowforge", "WATCHDOG_ALERT", reason, "monitor", pid, fullCommand, incidentID)

							wd, _ := os.Getwd()
							state.UpdateState(
								cpuUsage,
								fmt.Sprintf("WATCHDOG: Policy alert (%s)", alertType),
								alertType,
								fullCommand,
								args,
								wd,
								pid,
							)
						}
					case policy.ActionLogOnly:
						fmt.Printf("\n[FlowForge] 🧪 %s\n", reason)
						incidentID := uuid.NewString()
						meta := buildDecisionMeta(decision.Action.String(), reason, cpuScore, entropyScore, confidenceScore)
						_ = database.LogDecisionTraceWithIncidentAndMeta(fullCommand, pid, cpuScore, entropyScore, confidenceScore, decision.Action.String(), reason, incidentID, meta)
						_ = database.LogPolicyDryRunWithIncident(fullCommand, pid, reason, confidenceScore, incidentID)
					case policy.ActionKill, policy.ActionRestart:
						if noKill {
							// Legacy watchdog mode always suppresses destructive actions.
							fmt.Printf("\n[FlowForge] WATCHDOG MODE: %s\n", reason)
							incidentID := uuid.NewString()
							blockedReason := "watchdog mode blocked destructive action: " + reason
							meta := buildDecisionMeta("ACTION_BLOCKED", blockedReason, cpuScore, entropyScore, confidenceScore)
							_ = database.LogDecisionTraceWithIncidentAndMeta(fullCommand, pid, cpuScore, entropyScore, confidenceScore, "ACTION_BLOCKED", blockedReason, incidentID, meta)
							_ = database.LogPolicyDryRunWithIncident(fullCommand, pid, blockedReason, confidenceScore, incidentID)
							continue
						}
						incidentID := uuid.NewString()

						patterns.SyncPatterns(firstNormalized)
						feedback.GenerateFeedback(feedback.FeedbackData{
							Command:    fullCommand,
							Pattern:    firstNormalized,
							ExitReason: "LOOP_DETECTED",
							MaxCPU:     cpuUsage,
							ModelName:  modelName,
							Savings:    0,
						})

						actionName := "AUTO_KILL"
						exitReason := "LOOP_DETECTED"
						if decision.Action == policy.ActionRestart {
							actionName = "AUTO_RESTART"
							exitReason = "RESTART_TRIGGERED"
						}

						fmt.Printf("\n🚨 %s: %s\n", actionName, reason)
						finalTokens := int(observer.TotalTokens())
						finalCost := tokens.EstimateCost(finalTokens, modelName)

						meta := buildDecisionMeta(decision.Action.String(), reason, cpuScore, entropyScore, confidenceScore)
						_ = database.LogDecisionTraceWithIncidentAndMeta(fullCommand, pid, cpuScore, entropyScore, confidenceScore, decision.Action.String(), reason, incidentID, meta)
						_ = database.LogIncidentWithDecisionForIncident(
							fullCommand,
							modelName,
							exitReason,
							cpuUsage,
							firstNormalized,
							time.Since(startTime).Seconds(),
							finalTokens,
							finalCost,
							agentID,
							agentVersion,
							reason,
							cpuScore,
							entropyScore,
							confidenceScore,
							"terminated",
							0,
							incidentID,
						)
						_ = database.LogAuditEventWithIncident("flowforge", actionName, reason, "monitor", pid, fullCommand, incidentID)

						wd, _ := os.Getwd()
						state.UpdateState(
							cpuUsage,
							"POLICY ACTION - Terminating process group...",
							exitReason,
							fullCommand,
							args,
							wd,
							pid,
						)

						flowforgeTerminated.Store(true)
						_ = procSupervisor.Stop(2 * time.Second)
						cancel()
						fmt.Println("[FlowForge] Process group terminated after policy decision.")
						return
					}
				}

				if cpuUsage > maxCpu {
					fmt.Printf("[FlowForge] WARNING: High CPU (%.2f%%) detected. %s\n", cpuUsage, sysStatsStr)
				}

				// --- SAFETY CHOKE POINT ---
				// 1. Memory Limit
				maxMemMB := viper.GetFloat64("max-memory-mb")
				if maxMemMB > 0 {
					memInfo, err := p.MemoryInfo()
					if err == nil {
						memMB := float64(memInfo.RSS) / 1024.0 / 1024.0
						if memMB > maxMemMB {
							fmt.Printf("\n[FlowForge] 🛑 SAFETY CHOKE: Memory usage (%.2f MB) exceeded limit (%.2f MB). TERMINATING.\n", memMB, maxMemMB)
							// ... (rest of logic same)
							finalTokens := int(observer.TotalTokens())
							finalCost := tokens.EstimateCost(finalTokens, modelName)
							database.LogIncident(fullCommand, modelName, "SAFETY_LIMIT_EXCEEDED", cpuUsage, fmt.Sprintf("Memory Limit: %.2fMB", memMB), time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

							flowforgeTerminated.Store(true)
							_ = procSupervisor.Stop(2 * time.Second)
							cancel()
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
							fmt.Printf("\n[FlowForge] 🛑 SAFETY CHOKE: Token generation rate (%.0f/min) exceeded limit (%.0f/min). TERMINATING.\n", rate, maxTokensRate)

							finalTokens := int(currentTokens)
							finalCost := tokens.EstimateCost(finalTokens, modelName)
							database.LogIncident(fullCommand, modelName, "SAFETY_LIMIT_EXCEEDED", cpuUsage, fmt.Sprintf("Token Rate: %.0f/min", rate), time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)

							flowforgeTerminated.Store(true)
							_ = procSupervisor.Stop(2 * time.Second)
							cancel()
							return
						}
					}
				}
			}
		}
	}()

	var userTerminated atomic.Bool
	untrap := procSupervisor.TrapSignals(3*time.Second, func(sig os.Signal) {
		fmt.Printf("\n[FlowForge] Received signal: %v. Cleaning up process group...\n", sig)

		userTerminated.Store(true)
		incidentID := uuid.NewString()
		finalTokens := int(observer.TotalTokens())
		finalCost := tokens.EstimateCost(finalTokens, modelName)
		_ = database.LogIncidentWithDecisionForIncident(fullCommand, modelName, "USER_TERMINATED", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion, "received OS signal", 0, 0, 0, "terminated", 0, incidentID)
		_ = database.LogAuditEventWithIncident("operator", "TERMINATE", "received OS signal", "cli", pid, fullCommand, incidentID)

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
		cancel()
	}, os.Interrupt, syscall.SIGTERM)
	defer untrap()

	err := procSupervisor.Wait()
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
		if exitErr, ok := err.(*exec.ExitError); ok {
			isSignal := strings.Contains(exitErr.String(), "signal: killed") || strings.Contains(exitErr.String(), "signal: interrupt")

			if !userTerminated.Load() && !isSignal {
				finalTokens := int(observer.TotalTokens())
				finalCost := tokens.EstimateCost(finalTokens, modelName)
				_ = database.LogIncident(fullCommand, modelName, "COMMAND_FAILURE", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)
			}
			if flowforgeTerminated.Load() {
				os.Exit(1)
			}
			code := exitErr.ExitCode()
			if code < 0 {
				code = 1
			}
			os.Exit(code)
		} else {
			fmt.Printf("Command finished with error: %v\n", err)
			if !userTerminated.Load() {
				finalTokens := int(observer.TotalTokens())
				finalCost := tokens.EstimateCost(finalTokens, modelName)
				_ = database.LogIncident(fullCommand, modelName, "COMMAND_FAILURE", maxObservedCpu, "N/A", time.Since(startTime).Seconds(), finalTokens, finalCost, agentID, agentVersion)
			}
			os.Exit(1)
		}
	}

	if flowforgeTerminated.Load() {
		os.Exit(1)
	}
}
