package test

import (
	"agent-sentry/cmd"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type detectionFixture struct {
	name  string
	lines []string
}

func loadFixture(tb testing.TB, filename string) detectionFixture {
	tb.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatalf("failed to resolve runtime caller")
	}
	path := filepath.Join(filepath.Dir(current), "fixtures", filename)
	body, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read fixture %s: %v", filename, err)
	}

	raw := strings.Split(strings.TrimSpace(string(body)), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}

	return detectionFixture{name: filename, lines: lines}
}

func entropyScore(lines []string) float64 {
	if len(lines) == 0 {
		return 100.0
	}
	uniq := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		uniq[cmd.NormalizeLog(line)] = struct{}{}
	}
	return (float64(len(uniq)) / float64(len(lines))) * 100.0
}

func classifyRunaway(cpuUsage, threshold float64, lines []string) bool {
	if cpuUsage <= threshold {
		return false
	}
	return entropyScore(lines) <= 35.0
}

func TestDetectionFixtureBaseline(t *testing.T) {
	runaway := loadFixture(t, "runaway.txt")
	healthy := loadFixture(t, "healthy.txt")

	if !classifyRunaway(96.0, 80.0, runaway.lines) {
		t.Fatalf("expected runaway fixture to be classified as runaway")
	}
	if classifyRunaway(96.0, 80.0, healthy.lines) {
		t.Fatalf("expected healthy fixture not to be classified as runaway")
	}

	runawayEntropy := entropyScore(runaway.lines)
	healthyEntropy := entropyScore(healthy.lines)
	if runawayEntropy >= healthyEntropy {
		t.Fatalf("expected runaway entropy (%0.2f) to be lower than healthy entropy (%0.2f)", runawayEntropy, healthyEntropy)
	}
}

func BenchmarkDetectionRunawayFixture(b *testing.B) {
	fixture := loadFixture(b, "runaway.txt")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = classifyRunaway(96.0, 80.0, fixture.lines)
	}
}

func BenchmarkDetectionHealthyFixture(b *testing.B) {
	fixture := loadFixture(b, "healthy.txt")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = classifyRunaway(96.0, 80.0, fixture.lines)
	}
}
