package api

import (
	"agent-sentry/internal/database"
	"agent-sentry/internal/state"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func corsMiddleware(w http.ResponseWriter) {
	// Strict CORS: Allow Dashboard (localhost:3000) or local tools
	// In production, this should be configurable via env, but for now we lock it down.
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

// requireAuth checks the SENTRY_API_KEY env var.
// POLICY:
//   - GET requests: If NO key set, allowed (Dev mode). If key set, require it.
//   - POST requests: ALWAYS require key? Or allow if NO key set (Dev mode)?
//     Security Audit said: "All mutating endpoints require auth by default".
//     But if user just downloaded it and ran it... they didn't set a key.
//     Proposed Middle Ground: POST requires key IF key is set.
//     Wait, audit said: "Dev mode is still safe".
//     If we create a random key on startup and print it? That's the safest.
//     For now, we will enforce: POST requests MUST have Auth if Sentry is running in "Secure Mode" (Key set).
//     Refined Policy:
//   - If SENTRY_API_KEY is set: All requests must match.
//   - If NOT set: POST requests are BLOCKED for security (Force user to set key for control).
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	apiKey := os.Getenv("SENTRY_API_KEY")

	if apiKey == "" {
		if r.Method == "POST" {
			// Block dangerous actions if no key is configured
			http.Error(w, `{"error":"Security Alert: You must set SENTRY_API_KEY environment variable to perform mutations."}`, http.StatusForbidden)
			return false
		}
		return true // Allow Read-Only access in Dev Mode
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error":"Authorization required"}`, http.StatusUnauthorized)
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token != apiKey {
		http.Error(w, `{"error":"Invalid API key"}`, http.StatusForbidden)
		return false
	}

	return true
}

func StartServer(port string) {
	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/incidents", HandleIncidents)
	http.HandleFunc("/process/kill", HandleProcessKill)
	http.HandleFunc("/process/restart", HandleProcessRestart)
	fmt.Printf("Starting Dashboard API on port %s...\n", port)

	apiKey := os.Getenv("SENTRY_API_KEY")
	if apiKey != "" {
		fmt.Println("🔒 API Key authentication ENABLED for /process/* endpoints")
	} else {
		fmt.Println("⚠️  No SENTRY_API_KEY set — /process/* endpoints are OPEN")
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			jsonData, err := state.JSON()
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", jsonData)
				flusher.Flush()
			}
		}
	}
}

// HandleIncidents is exported for testing.
func HandleIncidents(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	db := database.GetDB()
	if db == nil {
		if err := database.InitDB(); err != nil {
			http.Error(w, "Database not initialized", http.StatusInternalServerError)
			return
		}
		db = database.GetDB()
	}

	incidents, err := database.GetAllIncidents()
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(incidents); err != nil {
		http.Error(w, fmt.Sprintf("Encode error: %v", err), http.StatusInternalServerError)
	}
}

// HandleProcessKill is exported for testing.
func HandleProcessKill(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if !requireAuth(w, r) {
		return
	}

	// Read from in-memory state
	stats := state.GetState()

	if stats.Status == "STOPPED" || stats.PID == 0 {
		http.Error(w, `{"error":"No active process to kill"}`, http.StatusBadRequest)
		return
	}

	// Kill the process group (negative PID targets the group)
	if err := syscall.Kill(-stats.PID, syscall.SIGKILL); err != nil {
		// Try killing just the process
		if err2 := syscall.Kill(stats.PID, syscall.SIGKILL); err2 != nil {
			http.Error(w, fmt.Sprintf(`{"error":"Failed to kill process: %v"}`, err2), http.StatusInternalServerError)
			return
		}
	}

	// Update State to STOPPED
	state.UpdateState(0, "", "STOPPED", stats.Command, stats.Args, stats.Dir, stats.PID)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"killed","pid":%d}`, stats.PID)
}

// HandleProcessRestart is exported for testing.
func HandleProcessRestart(w http.ResponseWriter, r *http.Request) {
	corsMiddleware(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if !requireAuth(w, r) {
		return
	}

	// Read state
	stats := state.GetState()

	if stats.Command == "" {
		http.Error(w, `{"error":"No command available to restart"}`, http.StatusBadRequest)
		return
	}

	// Kill existing process if running
	if stats.Status != "STOPPED" && stats.PID > 0 {
		syscall.Kill(-stats.PID, syscall.SIGKILL)
		time.Sleep(500 * time.Millisecond) // Brief wait for cleanup
	}

	// Restart in background
	go func() {
		var cmd *exec.Cmd
		if len(stats.Args) > 0 {
			// Secure path: execute directly without shell
			cmd = exec.Command(stats.Args[0], stats.Args[1:]...)
			if stats.Dir != "" {
				cmd.Dir = stats.Dir
			}
			fmt.Printf("[API] 🛡️ Secure Restart: %v\n", stats.Args)
		} else {
			// Legacy fallback (should handle gracefully during upgrade)
			cmd = exec.Command("sh", "-c", stats.Command)
			fmt.Printf("[API] ⚠️ Legacy Restart (Shell): %s\n", stats.Command)
		}

		if err := cmd.Start(); err != nil {
			fmt.Printf("[API] Failed to restart command: %v\n", err)
			return
		}
		fmt.Printf("[API] Restarted process with PID: %d\n", cmd.Process.Pid)
	}()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"restarting","command":"%s"}`, stats.Command)
}
