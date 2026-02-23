package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultHealthcheckURL = "http://127.0.0.1:8080/healthz"
	envHealthcheckURL     = "FLOWFORGE_HEALTHCHECK_URL"
)

func resolveHealthcheckURL() string {
	if raw := strings.TrimSpace(os.Getenv(envHealthcheckURL)); raw != "" {
		return raw
	}
	return defaultHealthcheckURL
}

func probeHealth(client *http.Client, healthURL string) error {
	resp, err := client.Get(healthURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected health status %d", resp.StatusCode)
	}
	return nil
}

func main() {
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := resolveHealthcheckURL()
	err := probeHealth(client, healthURL)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			fmt.Printf("Healthcheck timed out: %s\n", healthURL)
		} else {
			fmt.Printf("Healthcheck failed (%s): %v\n", healthURL, err)
		}
		os.Exit(1)
	}
	os.Exit(0)
}
