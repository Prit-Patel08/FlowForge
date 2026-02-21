package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu              sync.Mutex
	startedAt       time.Time
	authFailures    uint64
	processKills    uint64
	processRestarts uint64
	httpRequests    map[string]uint64

	stopLatencyCount       uint64
	stopLatencySuccess     uint64
	stopLatencyWithinSLO   uint64
	stopLatencySumSeconds  float64
	stopLatencyLastSeconds float64
	stopLatencyMaxSeconds  float64

	restartLatencyCount       uint64
	restartLatencySuccess     uint64
	restartLatencyWithinSLO   uint64
	restartLatencySumSeconds  float64
	restartLatencyLastSeconds float64
	restartLatencyMaxSeconds  float64
}

const (
	stopSLOSeconds    = 3.0
	restartSLOSeconds = 5.0
)

func NewStore() *Store {
	return &Store{
		startedAt:    time.Now(),
		httpRequests: make(map[string]uint64),
	}
}

func (s *Store) IncRequest(path, method string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s|%s|%d", path, method, status)
	s.httpRequests[key]++
}

func (s *Store) IncAuthFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authFailures++
}

func (s *Store) IncProcessKill() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processKills++
}

func (s *Store) IncProcessRestart() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processRestarts++
}

func (s *Store) ObserveStopLatency(seconds float64, success bool) {
	if seconds < 0 {
		seconds = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLatencyCount++
	if success {
		s.stopLatencySuccess++
	}
	s.stopLatencySumSeconds += seconds
	s.stopLatencyLastSeconds = seconds
	if seconds > s.stopLatencyMaxSeconds {
		s.stopLatencyMaxSeconds = seconds
	}
	if success && seconds <= stopSLOSeconds {
		s.stopLatencyWithinSLO++
	}
}

func (s *Store) ObserveRestartLatency(seconds float64, success bool) {
	if seconds < 0 {
		seconds = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartLatencyCount++
	if success {
		s.restartLatencySuccess++
	}
	s.restartLatencySumSeconds += seconds
	s.restartLatencyLastSeconds = seconds
	if seconds > s.restartLatencyMaxSeconds {
		s.restartLatencyMaxSeconds = seconds
	}
	if success && seconds <= restartSLOSeconds {
		s.restartLatencyWithinSLO++
	}
}

func (s *Store) Prometheus(activeProcess bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var b strings.Builder
	b.WriteString("# HELP flowforge_http_requests_total Total HTTP requests.\n")
	b.WriteString("# TYPE flowforge_http_requests_total counter\n")

	keys := make([]string, 0, len(s.httpRequests))
	for k := range s.httpRequests {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 {
			continue
		}
		fmt.Fprintf(&b, "flowforge_http_requests_total{path=%q,method=%q,status=%q} %d\n",
			parts[0], parts[1], parts[2], s.httpRequests[k])
	}

	b.WriteString("# HELP flowforge_auth_failures_total Failed auth attempts.\n")
	b.WriteString("# TYPE flowforge_auth_failures_total counter\n")
	fmt.Fprintf(&b, "flowforge_auth_failures_total %d\n", s.authFailures)

	b.WriteString("# HELP flowforge_process_kill_total Process kill actions.\n")
	b.WriteString("# TYPE flowforge_process_kill_total counter\n")
	fmt.Fprintf(&b, "flowforge_process_kill_total %d\n", s.processKills)

	b.WriteString("# HELP flowforge_process_restart_total Process restart actions.\n")
	b.WriteString("# TYPE flowforge_process_restart_total counter\n")
	fmt.Fprintf(&b, "flowforge_process_restart_total %d\n", s.processRestarts)

	b.WriteString("# HELP flowforge_uptime_seconds API uptime in seconds.\n")
	b.WriteString("# TYPE flowforge_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_uptime_seconds %.0f\n", time.Since(s.startedAt).Seconds())

	b.WriteString("# HELP flowforge_stop_slo_target_seconds Stop SLO target in seconds.\n")
	b.WriteString("# TYPE flowforge_stop_slo_target_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_stop_slo_target_seconds %.1f\n", stopSLOSeconds)

	b.WriteString("# HELP flowforge_restart_slo_target_seconds Restart SLO target in seconds.\n")
	b.WriteString("# TYPE flowforge_restart_slo_target_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_restart_slo_target_seconds %.1f\n", restartSLOSeconds)

	b.WriteString("# HELP flowforge_stop_latency_count Observed stop latency operations.\n")
	b.WriteString("# TYPE flowforge_stop_latency_count counter\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_count %d\n", s.stopLatencyCount)

	b.WriteString("# HELP flowforge_stop_latency_success_total Successful stop operations.\n")
	b.WriteString("# TYPE flowforge_stop_latency_success_total counter\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_success_total %d\n", s.stopLatencySuccess)

	b.WriteString("# HELP flowforge_stop_latency_within_slo_total Successful stop operations within SLO.\n")
	b.WriteString("# TYPE flowforge_stop_latency_within_slo_total counter\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_within_slo_total %d\n", s.stopLatencyWithinSLO)

	b.WriteString("# HELP flowforge_stop_latency_last_seconds Last observed stop latency in seconds.\n")
	b.WriteString("# TYPE flowforge_stop_latency_last_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_last_seconds %.6f\n", s.stopLatencyLastSeconds)

	b.WriteString("# HELP flowforge_stop_latency_max_seconds Maximum observed stop latency in seconds.\n")
	b.WriteString("# TYPE flowforge_stop_latency_max_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_max_seconds %.6f\n", s.stopLatencyMaxSeconds)

	b.WriteString("# HELP flowforge_stop_latency_sum_seconds Sum of observed stop latencies in seconds.\n")
	b.WriteString("# TYPE flowforge_stop_latency_sum_seconds counter\n")
	fmt.Fprintf(&b, "flowforge_stop_latency_sum_seconds %.6f\n", s.stopLatencySumSeconds)

	stopSLOCompliance := 0.0
	if s.stopLatencyCount > 0 {
		stopSLOCompliance = float64(s.stopLatencyWithinSLO) / float64(s.stopLatencyCount)
	}
	b.WriteString("# HELP flowforge_stop_slo_compliance_ratio Ratio of stop operations that met SLO.\n")
	b.WriteString("# TYPE flowforge_stop_slo_compliance_ratio gauge\n")
	fmt.Fprintf(&b, "flowforge_stop_slo_compliance_ratio %.6f\n", stopSLOCompliance)

	b.WriteString("# HELP flowforge_restart_latency_count Observed restart latency operations.\n")
	b.WriteString("# TYPE flowforge_restart_latency_count counter\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_count %d\n", s.restartLatencyCount)

	b.WriteString("# HELP flowforge_restart_latency_success_total Successful restart operations.\n")
	b.WriteString("# TYPE flowforge_restart_latency_success_total counter\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_success_total %d\n", s.restartLatencySuccess)

	b.WriteString("# HELP flowforge_restart_latency_within_slo_total Successful restart operations within SLO.\n")
	b.WriteString("# TYPE flowforge_restart_latency_within_slo_total counter\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_within_slo_total %d\n", s.restartLatencyWithinSLO)

	b.WriteString("# HELP flowforge_restart_latency_last_seconds Last observed restart latency in seconds.\n")
	b.WriteString("# TYPE flowforge_restart_latency_last_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_last_seconds %.6f\n", s.restartLatencyLastSeconds)

	b.WriteString("# HELP flowforge_restart_latency_max_seconds Maximum observed restart latency in seconds.\n")
	b.WriteString("# TYPE flowforge_restart_latency_max_seconds gauge\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_max_seconds %.6f\n", s.restartLatencyMaxSeconds)

	b.WriteString("# HELP flowforge_restart_latency_sum_seconds Sum of observed restart latencies in seconds.\n")
	b.WriteString("# TYPE flowforge_restart_latency_sum_seconds counter\n")
	fmt.Fprintf(&b, "flowforge_restart_latency_sum_seconds %.6f\n", s.restartLatencySumSeconds)

	restartSLOCompliance := 0.0
	if s.restartLatencyCount > 0 {
		restartSLOCompliance = float64(s.restartLatencyWithinSLO) / float64(s.restartLatencyCount)
	}
	b.WriteString("# HELP flowforge_restart_slo_compliance_ratio Ratio of restart operations that met SLO.\n")
	b.WriteString("# TYPE flowforge_restart_slo_compliance_ratio gauge\n")
	fmt.Fprintf(&b, "flowforge_restart_slo_compliance_ratio %.6f\n", restartSLOCompliance)

	b.WriteString("# HELP flowforge_active_process Whether a supervised process is active.\n")
	b.WriteString("# TYPE flowforge_active_process gauge\n")
	if activeProcess {
		b.WriteString("flowforge_active_process 1\n")
	} else {
		b.WriteString("flowforge_active_process 0\n")
	}

	return b.String()
}
