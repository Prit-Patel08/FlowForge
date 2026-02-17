package main

import (
	"log"

	"agent-sentry/cmd"
)

func main() {
	// keep main tiny; cmd.Execute implements CLI and server bootstrap
	if err := cmd.Execute(); err != nil {
		log.Fatalf("sentry: %v", err)
	}
}
