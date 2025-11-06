package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/mpepping/discovery-service/pkg/limits"
)

func TestNewIPLimiter(t *testing.T) {
	limiter := NewIPLimiter()

	if limiter == nil {
		t.Fatal("NewIPLimiter returned nil")
	}

	if limiter.limiters == nil {
		t.Error("limiters map not initialized")
	}
}

func TestAllowFirstRequest(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"
	allowed := limiter.Allow(ip)

	if !allowed {
		t.Error("first request should be allowed")
	}

	// Verify limiter was created
	limiter.mu.RLock()
	_, exists := limiter.limiters[ip]
	limiter.mu.RUnlock()

	if !exists {
		t.Error("limiter not created for IP")
	}
}

func TestAllowMultipleIPs(t *testing.T) {
	limiter := NewIPLimiter()

	ips := []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"}

	for _, ip := range ips {
		allowed := limiter.Allow(ip)
		if !allowed {
			t.Errorf("first request from %s should be allowed", ip)
		}
	}

	// Verify all limiters were created
	limiter.mu.RLock()
	count := len(limiter.limiters)
	limiter.mu.RUnlock()

	if count != len(ips) {
		t.Errorf("expected %d limiters, got %d", len(ips), count)
	}
}

func TestAllowRateLimit(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"

	// Burst should allow IPRateBurstSizeMax requests immediately
	allowedCount := 0
	for i := 0; i < limits.IPRateBurstSizeMax+10; i++ {
		if limiter.Allow(ip) {
			allowedCount++
		}
	}

	// Should have allowed burst size
	if allowedCount != limits.IPRateBurstSizeMax {
		t.Errorf("expected %d allowed requests, got %d", limits.IPRateBurstSizeMax, allowedCount)
	}
}

func TestAllowRateLimitWithDelay(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"

	// Use up the burst
	for i := 0; i < limits.IPRateBurstSizeMax; i++ {
		limiter.Allow(ip)
	}

	// Next request should be denied
	if limiter.Allow(ip) {
		t.Error("request should be denied after burst exhausted")
	}

	// Wait for token to refill (1/15 second for one token)
	time.Sleep(time.Second / limits.IPRateRequestsPerSecondMax)

	// Now should be allowed again
	if !limiter.Allow(ip) {
		t.Error("request should be allowed after token refill")
	}
}

func TestAllowConcurrent(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"
	done := make(chan bool, 10)

	// Concurrently try to allow requests
	for i := 0; i < 10; i++ {
		go func() {
			limiter.Allow(ip)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have created exactly one limiter
	limiter.mu.RLock()
	count := len(limiter.limiters)
	limiter.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 limiter, got %d", count)
	}
}

func TestAllowUpdatesLastSeen(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"

	// First request
	limiter.Allow(ip)

	// Get initial lastSeen
	limiter.mu.RLock()
	firstSeen := limiter.limiters[ip].lastSeen
	limiter.mu.RUnlock()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Second request
	limiter.Allow(ip)

	// Get updated lastSeen
	limiter.mu.RLock()
	secondSeen := limiter.limiters[ip].lastSeen
	limiter.mu.RUnlock()

	// Should be updated
	if !secondSeen.After(firstSeen) {
		t.Error("lastSeen was not updated")
	}
}

func TestGarbageCollect(t *testing.T) {
	limiter := NewIPLimiter()

	oldIP := "192.168.1.1"
	newIP := "192.168.1.2"

	// Create old entry
	limiter.Allow(oldIP)
	limiter.mu.Lock()
	limiter.limiters[oldIP].lastSeen = time.Now().Add(-2 * time.Hour)
	limiter.mu.Unlock()

	// Create new entry
	limiter.Allow(newIP)

	// Run GC with 1 hour max age
	limiter.GarbageCollect(1 * time.Hour)

	// Old entry should be removed
	limiter.mu.RLock()
	_, oldExists := limiter.limiters[oldIP]
	_, newExists := limiter.limiters[newIP]
	limiter.mu.RUnlock()

	if oldExists {
		t.Error("old limiter was not garbage collected")
	}

	if !newExists {
		t.Error("new limiter was incorrectly garbage collected")
	}
}

func TestGarbageCollectEmpty(t *testing.T) {
	limiter := NewIPLimiter()

	// GC on empty limiter should not panic
	limiter.GarbageCollect(1 * time.Hour)
}

func TestGarbageCollectNothingToCollect(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"
	limiter.Allow(ip)

	// GC with short max age should not remove recent entries
	limiter.GarbageCollect(1 * time.Second)

	limiter.mu.RLock()
	_, exists := limiter.limiters[ip]
	limiter.mu.RUnlock()

	if !exists {
		t.Error("recent limiter was incorrectly removed")
	}
}

func TestRunGC(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"
	limiter.Allow(ip)

	// Set lastSeen to old value
	limiter.mu.Lock()
	limiter.limiters[ip].lastSeen = time.Now().Add(-2 * time.Hour)
	limiter.mu.Unlock()

	// Start GC with short interval and short max age
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go limiter.RunGC(ctx, 30*time.Millisecond, 1*time.Hour)

	// Wait for GC to run
	time.Sleep(50 * time.Millisecond)

	// Entry should be removed
	limiter.mu.RLock()
	_, exists := limiter.limiters[ip]
	limiter.mu.RUnlock()

	if exists {
		t.Error("old limiter was not garbage collected by RunGC")
	}

	// Cancel and verify it stops
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestRunGCStopsOnContextCancel(t *testing.T) {
	limiter := NewIPLimiter()

	ctx, cancel := context.WithCancel(context.Background())

	// Start GC
	done := make(chan bool)
	go func() {
		limiter.RunGC(ctx, 10*time.Millisecond, 1*time.Hour)
		done <- true
	}()

	// Cancel immediately
	cancel()

	// Should exit quickly
	select {
	case <-done:
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Error("RunGC did not stop after context cancel")
	}
}

func TestIndependentIPLimits(t *testing.T) {
	limiter := NewIPLimiter()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Exhaust ip1's burst
	for i := 0; i < limits.IPRateBurstSizeMax; i++ {
		limiter.Allow(ip1)
	}

	// ip1 should be limited
	if limiter.Allow(ip1) {
		t.Error("ip1 should be rate limited")
	}

	// ip2 should still be allowed
	if !limiter.Allow(ip2) {
		t.Error("ip2 should not be affected by ip1's rate limit")
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	limiter := NewIPLimiter()

	ip := "192.168.1.1"

	// Use burst
	for i := 0; i < limits.IPRateBurstSizeMax; i++ {
		limiter.Allow(ip)
	}

	// Should be denied
	if limiter.Allow(ip) {
		t.Error("should be denied after burst")
	}

	// Wait for multiple tokens to refill
	// At 15 req/s, 5 tokens take ~333ms
	time.Sleep(400 * time.Millisecond)

	// Should allow a few requests
	allowedCount := 0
	for i := 0; i < 10; i++ {
		if limiter.Allow(ip) {
			allowedCount++
		}
	}

	// Should have refilled at least 4-5 tokens (accounting for timing variance)
	if allowedCount < 4 {
		t.Errorf("expected at least 4 refilled tokens, got %d", allowedCount)
	}
}
