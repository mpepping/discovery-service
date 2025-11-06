package limiter

import (
	"context"
	"sync"
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
	"golang.org/x/time/rate"
)

// IPLimiter provides per-IP rate limiting
type IPLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*limiterEntry
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPLimiter creates a new IP-based rate limiter
func NewIPLimiter() *IPLimiter {
	return &IPLimiter{
		limiters: make(map[string]*limiterEntry),
	}
}

// Allow checks if a request from the given IP should be allowed
func (ipl *IPLimiter) Allow(ip string) bool {
	ipl.mu.RLock()
	entry, exists := ipl.limiters[ip]
	ipl.mu.RUnlock()

	if exists {
		entry.lastSeen = time.Now()
		return entry.limiter.Allow()
	}

	// Create new limiter for this IP
	ipl.mu.Lock()
	defer ipl.mu.Unlock()

	// Double-check after acquiring write lock
	entry, exists = ipl.limiters[ip]
	if exists {
		entry.lastSeen = time.Now()
		return entry.limiter.Allow()
	}

	limiter := rate.NewLimiter(
		rate.Limit(limits.IPRateRequestsPerSecondMax),
		limits.IPRateBurstSizeMax,
	)

	ipl.limiters[ip] = &limiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}

	return limiter.Allow()
}

// GarbageCollect removes limiters that haven't been used recently
func (ipl *IPLimiter) GarbageCollect(maxAge time.Duration) {
	ipl.mu.Lock()
	defer ipl.mu.Unlock()

	now := time.Now()
	toDelete := make([]string, 0)

	for ip, entry := range ipl.limiters {
		if now.Sub(entry.lastSeen) > maxAge {
			toDelete = append(toDelete, ip)
		}
	}

	for _, ip := range toDelete {
		delete(ipl.limiters, ip)
	}
}

// RunGC runs garbage collection periodically
func (ipl *IPLimiter) RunGC(ctx context.Context, interval, maxAge time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ipl.GarbageCollect(maxAge)
		}
	}
}
