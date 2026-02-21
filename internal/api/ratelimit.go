package api

import (
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type limiterEntry struct {
	windowStart  time.Time
	requestCount int
	authFailures int
	blockedUntil time.Time
	lastSeen     time.Time
}

type rateLimiter struct {
	mu            sync.Mutex
	requestLimit  int
	authFailLimit int
	blockDuration time.Duration
	maxEntries    int
	staleTTL      time.Duration
	pruneEvery    uint64
	opCount       uint64
	entries       map[string]*limiterEntry
}

func newRateLimiter(requestLimit, authFailLimit int, blockDuration time.Duration) *rateLimiter {
	return newRateLimiterWithBounds(requestLimit, authFailLimit, blockDuration, 10_000, 0, 256)
}

func newRateLimiterWithBounds(requestLimit, authFailLimit int, blockDuration time.Duration, maxEntries int, staleTTL time.Duration, pruneEvery uint64) *rateLimiter {
	if requestLimit <= 0 {
		requestLimit = 120
	}
	if authFailLimit <= 0 {
		authFailLimit = 10
	}
	if blockDuration <= 0 {
		blockDuration = 10 * time.Minute
	}
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	if staleTTL <= 0 {
		staleTTL = 30 * time.Minute
		if d := blockDuration * 3; d > staleTTL {
			staleTTL = d
		}
	}
	if pruneEvery == 0 {
		pruneEvery = 256
	}
	return &rateLimiter{
		requestLimit:  requestLimit,
		authFailLimit: authFailLimit,
		blockDuration: blockDuration,
		maxEntries:    maxEntries,
		staleTTL:      staleTTL,
		pruneEvery:    pruneEvery,
		entries:       make(map[string]*limiterEntry),
	}
}

func (r *rateLimiter) allow(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	e := r.getEntry(ip, now)
	if r.shouldPruneLocked() {
		r.pruneLocked(now)
	}
	if now.Before(e.blockedUntil) {
		return false
	}
	if now.Sub(e.windowStart) >= time.Minute {
		e.windowStart = now
		e.requestCount = 0
		e.authFailures = 0
	}
	e.requestCount++
	return e.requestCount <= r.requestLimit
}

func (r *rateLimiter) addAuthFailure(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	e := r.getEntry(ip, now)
	if r.shouldPruneLocked() {
		r.pruneLocked(now)
	}
	if now.Sub(e.windowStart) >= time.Minute {
		e.windowStart = now
		e.requestCount = 0
		e.authFailures = 0
	}
	e.authFailures++
	if e.authFailures >= r.authFailLimit {
		e.blockedUntil = now.Add(r.blockDuration)
		return true
	}
	return false
}

func (r *rateLimiter) clearAuthFailures(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	e := r.getEntry(ip, now)
	e.authFailures = 0
	if r.shouldPruneLocked() {
		r.pruneLocked(now)
	}
}

func (r *rateLimiter) getEntry(ip string, now time.Time) *limiterEntry {
	e, ok := r.entries[ip]
	if !ok {
		e = &limiterEntry{
			windowStart: now,
			lastSeen:    now,
		}
		r.entries[ip] = e
		return e
	}
	e.lastSeen = now
	return e
}

func (r *rateLimiter) shouldPruneLocked() bool {
	r.opCount++
	if len(r.entries) > r.maxEntries {
		return true
	}
	return r.opCount%r.pruneEvery == 0
}

func (r *rateLimiter) pruneLocked(now time.Time) {
	if len(r.entries) == 0 {
		return
	}

	cutoff := now.Add(-r.staleTTL)
	for ip, entry := range r.entries {
		if entry.lastSeen.Before(cutoff) && !now.Before(entry.blockedUntil) {
			delete(r.entries, ip)
		}
	}
	if len(r.entries) <= r.maxEntries {
		return
	}

	type evictCandidate struct {
		ip       string
		lastSeen time.Time
		blocked  bool
	}

	candidates := make([]evictCandidate, 0, len(r.entries))
	for ip, entry := range r.entries {
		candidates = append(candidates, evictCandidate{
			ip:       ip,
			lastSeen: entry.lastSeen,
			blocked:  now.Before(entry.blockedUntil),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].blocked != candidates[j].blocked {
			// Prefer evicting non-blocked entries first.
			return !candidates[i].blocked && candidates[j].blocked
		}
		if candidates[i].lastSeen.Equal(candidates[j].lastSeen) {
			return candidates[i].ip < candidates[j].ip
		}
		return candidates[i].lastSeen.Before(candidates[j].lastSeen)
	})

	over := len(r.entries) - r.maxEntries
	for i := 0; i < over && i < len(candidates); i++ {
		delete(r.entries, candidates[i].ip)
	}
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	if strings.Contains(remoteAddr, ":") && strings.Count(remoteAddr, ":") == 1 {
		if _, pErr := strconv.Atoi(strings.Split(remoteAddr, ":")[1]); pErr == nil {
			return strings.Split(remoteAddr, ":")[0]
		}
	}
	return remoteAddr
}
