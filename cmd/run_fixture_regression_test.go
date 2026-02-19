package cmd

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"flowforge/internal/policy"
)

func fixtureScriptPath(tb testing.TB, filename string) string {
	tb.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("failed to resolve runtime caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
	return filepath.Join(root, "test", "fixtures", "scripts", filename)
}

func runFixtureAndCollectLines(tb testing.TB, file string, args []string, expectZero bool) []string {
	tb.Helper()

	cmdArgs := append([]string{fixtureScriptPath(tb, file)}, args...)
	cmd := exec.Command("python3", cmdArgs...)

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if expectZero && err != nil {
		tb.Fatalf("expected zero exit for %s, got err=%v output=%s", file, err, output.String())
	}
	if !expectZero && err == nil {
		tb.Fatalf("expected non-zero exit for %s, got nil error", file)
	}

	lines := make([]string, 0)
	scanner := bufio.NewScanner(strings.NewReader(output.String()))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		tb.Fatalf("scan output: %v", err)
	}
	return lines
}

func tailLines(lines []string, n int) []string {
	if n <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func TestHealthySpikeFixtureRemainsNonDestructive(t *testing.T) {
	lines := runFixtureAndCollectLines(t, "healthy_spike.py", []string{"--timeout", "20", "--spike-seconds", "2"}, true)
	window := tailLines(lines, 10)
	if len(window) < 5 {
		t.Fatalf("expected at least 5 lines from healthy fixture, got %d", len(window))
	}

	rawDiversity := rawDiversityScore(window)
	progressLike := detectProgressLikeOutput(window)
	if !progressLike {
		t.Fatalf("expected healthy spike fixture to look like progress; window=%v", window)
	}
	if rawDiversity < 0.85 {
		t.Fatalf("expected high raw diversity for healthy spike, got %.2f", rawDiversity)
	}

	d := policy.NewThresholdDecider()
	decision := d.Evaluate(policy.Telemetry{
		CPUPercent:    96,
		CPUOverFor:    15 * time.Second,
		LogRepetition: 0.95,
		LogEntropy:    0.10,
		RawDiversity:  rawDiversity,
		ProgressLike:  progressLike,
	}, policy.Policy{
		MaxCPUPercent:    90,
		CPUWindow:        10 * time.Second,
		MinLogEntropy:    0.20,
		MaxLogRepetition: 0.80,
	})

	if decision.Action != policy.ActionAlert {
		t.Fatalf("expected ALERT (non-destructive) for healthy spike, got %s", decision.Action.String())
	}
	if !strings.Contains(decision.Reason, "progressing output pattern detected") {
		t.Fatalf("expected progress-guard reason, got: %q", decision.Reason)
	}
}

func TestInfiniteLooperFixtureStillDestructive(t *testing.T) {
	lines := runFixtureAndCollectLines(t, "infinite_looper.py", []string{"--timeout", "2"}, false)
	window := tailLines(lines, 10)
	if len(window) < 5 {
		t.Fatalf("expected at least 5 lines from infinite fixture, got %d", len(window))
	}

	rawDiversity := rawDiversityScore(window)
	progressLike := detectProgressLikeOutput(window)
	if progressLike {
		t.Fatalf("expected infinite looper not to look like progress; window=%v", window)
	}
	if rawDiversity > 0.60 {
		t.Fatalf("expected low raw diversity for infinite looper, got %.2f", rawDiversity)
	}

	d := policy.NewThresholdDecider()
	decision := d.Evaluate(policy.Telemetry{
		CPUPercent:    96,
		CPUOverFor:    15 * time.Second,
		LogRepetition: 0.95,
		LogEntropy:    0.10,
		RawDiversity:  rawDiversity,
		ProgressLike:  progressLike,
	}, policy.Policy{
		MaxCPUPercent:    90,
		CPUWindow:        10 * time.Second,
		MinLogEntropy:    0.20,
		MaxLogRepetition: 0.80,
	})

	if decision.Action != policy.ActionKill {
		t.Fatalf("expected KILL for infinite looper, got %s", decision.Action.String())
	}
}
