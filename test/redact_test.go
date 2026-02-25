package test

import (
	"flowforge/internal/redact"
	"strings"
	"testing"
)

func TestRedactLine(t *testing.T) {
	input := "Authorization: Bearer secret-token FLOWFORGE_API_KEY=supersecret"
	out := redact.Line(input)
	if strings.Contains(out, "secret-token") || strings.Contains(out, "supersecret") {
		t.Fatalf("expected secrets to be redacted, got %q", out)
	}
}

func TestRedactLineCLISecretFlags(t *testing.T) {
	input := "python3 worker.py --api-key supersecret --token abc123 --password 'letmein'"
	out := redact.Line(input)
	if strings.Contains(out, "supersecret") || strings.Contains(out, "abc123") || strings.Contains(out, "letmein") {
		t.Fatalf("expected CLI flag secrets to be redacted, got %q", out)
	}
	if !strings.Contains(out, "--api-key <REDACTED>") || !strings.Contains(out, "--token <REDACTED>") || !strings.Contains(out, "--password <REDACTED>") {
		t.Fatalf("expected CLI flag redaction markers, got %q", out)
	}
}
