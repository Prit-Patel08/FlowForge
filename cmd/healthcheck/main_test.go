package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveHealthcheckURLDefaultsToLoopback(t *testing.T) {
	t.Setenv(envHealthcheckURL, "")
	if got := resolveHealthcheckURL(); got != defaultHealthcheckURL {
		t.Fatalf("expected default healthcheck url %q, got %q", defaultHealthcheckURL, got)
	}
}

func TestResolveHealthcheckURLUsesEnvOverride(t *testing.T) {
	want := "http://127.0.0.1:18080/healthz"
	t.Setenv(envHealthcheckURL, want)
	if got := resolveHealthcheckURL(); got != want {
		t.Fatalf("expected env override %q, got %q", want, got)
	}
}

func TestProbeHealthPassesOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	if err := probeHealth(client, srv.URL); err != nil {
		t.Fatalf("expected probe to pass, got error: %v", err)
	}
}

func TestProbeHealthFailsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"degraded"}`))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	if err := probeHealth(client, srv.URL); err == nil {
		t.Fatal("expected probe to fail on non-2xx status")
	}
}

func TestProbeHealthFailsOnConnectionError(t *testing.T) {
	client := &http.Client{Timeout: 200 * time.Millisecond}
	// Use an unroutable localhost port to force a connection error.
	if err := probeHealth(client, "http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("expected probe to fail on connection error")
	}
}
