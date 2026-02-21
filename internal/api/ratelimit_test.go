package api

import (
	"testing"
	"time"
)

func TestRateLimiterPruneRemovesStaleEntries(t *testing.T) {
	now := time.Now()
	limiter := newRateLimiterWithBounds(120, 10, 10*time.Minute, 10, time.Minute, 1)

	limiter.entries["stale"] = &limiterEntry{
		windowStart: now.Add(-2 * time.Hour),
		lastSeen:    now.Add(-2 * time.Hour),
	}
	limiter.entries["blocked-stale"] = &limiterEntry{
		windowStart:  now.Add(-2 * time.Hour),
		lastSeen:     now.Add(-2 * time.Hour),
		blockedUntil: now.Add(5 * time.Minute),
	}
	limiter.entries["fresh"] = &limiterEntry{
		windowStart: now,
		lastSeen:    now,
	}

	limiter.pruneLocked(now)

	if _, ok := limiter.entries["stale"]; ok {
		t.Fatal("expected stale entry to be pruned")
	}
	if _, ok := limiter.entries["blocked-stale"]; !ok {
		t.Fatal("expected blocked stale entry to be retained")
	}
	if _, ok := limiter.entries["fresh"]; !ok {
		t.Fatal("expected fresh entry to be retained")
	}
}

func TestRateLimiterPruneCapsEntriesPrefersNonBlockedEviction(t *testing.T) {
	now := time.Now()
	limiter := newRateLimiterWithBounds(120, 10, 10*time.Minute, 2, 24*time.Hour, 1)

	limiter.entries["blocked"] = &limiterEntry{
		windowStart:  now.Add(-2 * time.Hour),
		lastSeen:     now.Add(-2 * time.Hour),
		blockedUntil: now.Add(3 * time.Minute),
	}
	limiter.entries["u-old"] = &limiterEntry{
		windowStart: now.Add(-90 * time.Minute),
		lastSeen:    now.Add(-90 * time.Minute),
	}
	limiter.entries["u-new"] = &limiterEntry{
		windowStart: now.Add(-30 * time.Minute),
		lastSeen:    now.Add(-30 * time.Minute),
	}

	limiter.pruneLocked(now)

	if len(limiter.entries) != 2 {
		t.Fatalf("expected capped entry count 2, got %d", len(limiter.entries))
	}
	if _, ok := limiter.entries["blocked"]; !ok {
		t.Fatal("expected blocked entry to be retained while trimming")
	}
	if _, ok := limiter.entries["u-old"]; ok {
		t.Fatal("expected oldest unblocked entry to be evicted")
	}
	if _, ok := limiter.entries["u-new"]; !ok {
		t.Fatal("expected newer unblocked entry to be retained")
	}
}

func TestRateLimiterAllowTriggersBoundedPrune(t *testing.T) {
	now := time.Now()
	limiter := newRateLimiterWithBounds(120, 10, 10*time.Minute, 2, 24*time.Hour, 1)

	limiter.entries["old-1"] = &limiterEntry{windowStart: now.Add(-3 * time.Hour), lastSeen: now.Add(-3 * time.Hour)}
	limiter.entries["old-2"] = &limiterEntry{windowStart: now.Add(-2 * time.Hour), lastSeen: now.Add(-2 * time.Hour)}
	limiter.entries["old-3"] = &limiterEntry{windowStart: now.Add(-1 * time.Hour), lastSeen: now.Add(-1 * time.Hour)}

	if allowed := limiter.allow("fresh"); !allowed {
		t.Fatal("expected fresh request to be allowed")
	}
	if len(limiter.entries) > limiter.maxEntries {
		t.Fatalf("expected bounded map size <= %d, got %d", limiter.maxEntries, len(limiter.entries))
	}
	if _, ok := limiter.entries["fresh"]; !ok {
		t.Fatal("expected fresh entry to remain after prune")
	}
}
