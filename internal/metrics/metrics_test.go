package metrics

import (
	"strings"
	"testing"
)

func TestPrometheusIncludesLifecycleLatencyMetrics(t *testing.T) {
	store := NewStore()
	store.ObserveStopLatency(0.8, true)
	store.ObserveStopLatency(3.6, false)
	store.ObserveRestartLatency(1.2, true)

	out := store.Prometheus(false)

	required := []string{
		"flowforge_stop_latency_count 2",
		"flowforge_stop_latency_success_total 1",
		"flowforge_stop_latency_within_slo_total 1",
		"flowforge_restart_latency_count 1",
		"flowforge_restart_latency_success_total 1",
		"flowforge_restart_latency_within_slo_total 1",
		"flowforge_stop_slo_compliance_ratio 0.500000",
		"flowforge_restart_slo_compliance_ratio 1.000000",
		"flowforge_stop_slo_target_seconds 3.0",
		"flowforge_restart_slo_target_seconds 5.0",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("expected metric output to contain %q\noutput:\n%s", token, out)
		}
	}
}
