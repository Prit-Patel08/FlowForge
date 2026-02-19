package cmd

import "testing"

func TestRawDiversityScore(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  float64
	}{
		{name: "empty", lines: nil, want: 1.0},
		{name: "all same", lines: []string{"a", "a", "a"}, want: 1.0 / 3.0},
		{name: "all unique", lines: []string{"a", "b", "c", "d"}, want: 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rawDiversityScore(tt.lines)
			if got != tt.want {
				t.Fatalf("rawDiversityScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectProgressLikeOutput(t *testing.T) {
	progressLines := []string{
		"progress step=1 phase=compute metric=0.901",
		"progress step=2 phase=compute metric=0.903",
		"progress step=3 phase=compute metric=0.915",
		"progress step=4 phase=compute metric=0.922",
		"progress step=5 phase=compute metric=0.930",
	}
	if !detectProgressLikeOutput(progressLines) {
		t.Fatal("expected progress-like output to be detected")
	}

	runawayLines := []string{
		"processing request 4242 failed, retrying endlessly",
		"processing request 4242 failed, retrying endlessly",
		"processing request 4242 failed, retrying endlessly",
		"processing request 4242 failed, retrying endlessly",
		"processing request 4242 failed, retrying endlessly",
	}
	if detectProgressLikeOutput(runawayLines) {
		t.Fatal("expected runaway repetitive output not to be classified as progress")
	}
}
