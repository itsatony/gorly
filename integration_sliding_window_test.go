// integration_sliding_window_test.go
package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterWithSlidingWindow(t *testing.T) {
	// Create config with sliding window algorithm
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory" // Use memory store for faster tests

	// Create rate limiter
	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	// Test basic functionality
	entity := NewDefaultAuthEntity("test-user", EntityTypeUser, TierFree)
	ctx := context.Background()

	// First request should be allowed
	result, err := limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected first request to be allowed")
	}

	if result.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm to be sliding_window, got %s", result.Algorithm)
	}

	// Make multiple requests to test window tracking
	for i := 0; i < 10; i++ {
		result, err = limiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("Unexpected error on request %d: %v", i+1, err)
		}

		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}

		if result.Remaining < 0 {
			t.Errorf("Expected remaining to be non-negative, got %d", result.Remaining)
		}

		if result.Used != int64(i+2) { // +2 because we already made the first request
			t.Errorf("Expected %d requests used on request %d, got %d", i+2, i+1, result.Used)
		}
	}

	// Check health
	if err := limiter.Health(ctx); err != nil {
		t.Errorf("Expected health check to pass: %v", err)
	}
}

func TestRateLimiterSlidingWindowRateLimiting(t *testing.T) {
	// Create config with low limits for testing
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory"

	// Set very low limits for testing
	config.TierLimits = map[string]TierConfig{
		TierFree: {
			DefaultLimits: map[string]RateLimit{
				ScopeGlobal: {Requests: 3, Window: time.Minute},
			},
		},
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := NewDefaultAuthEntity("test-user-limited", EntityTypeUser, TierFree)
	ctx := context.Background()

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("Unexpected error on request %d: %v", i+1, err)
		}

		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}

		expectedRemaining := int64(2 - i)
		if result.Remaining != expectedRemaining {
			t.Errorf("Expected %d remaining after request %d, got %d",
				expectedRemaining, i+1, result.Remaining)
		}

		expectedUsed := int64(i + 1)
		if result.Used != expectedUsed {
			t.Errorf("Expected %d requests used after request %d, got %d",
				expectedUsed, i+1, result.Used)
		}
	}

	// 4th request should be denied
	result, err := limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected 4th request to be denied")
	}

	if result.Remaining != 0 {
		t.Errorf("Expected 0 remaining slots, got %d", result.Remaining)
	}

	if result.Used != 3 {
		t.Errorf("Expected 3 requests used, got %d", result.Used)
	}

	if result.RetryAfter <= 0 {
		t.Error("Expected retry after time to be set")
	}
}

func TestRateLimiterSlidingWindowMultipleScopes(t *testing.T) {
	// Create config with sliding window algorithm
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory"

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := NewDefaultAuthEntity("test-user-scopes", EntityTypeUser, TierFree)
	ctx := context.Background()

	// Test different scopes independently
	scopes := []string{ScopeGlobal, ScopeMemory, ScopeSearch}

	for _, scope := range scopes {
		// First request for each scope should be allowed
		result, err := limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error for scope %s: %v", scope, err)
		}

		if !result.Allowed {
			t.Errorf("Expected first request for scope %s to be allowed", scope)
		}

		if result.Algorithm != "sliding_window" {
			t.Errorf("Expected algorithm to be sliding_window for scope %s, got %s", scope, result.Algorithm)
		}

		if result.Used != 1 {
			t.Errorf("Expected 1 request used for scope %s, got %d", scope, result.Used)
		}
	}
}

func TestRateLimiterSlidingWindowReset(t *testing.T) {
	// Create config with sliding window and low limits
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory"

	config.TierLimits = map[string]TierConfig{
		TierFree: {
			DefaultLimits: map[string]RateLimit{
				ScopeGlobal: {Requests: 2, Window: time.Minute},
			},
		},
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := NewDefaultAuthEntity("test-user-reset", EntityTypeUser, TierFree)
	ctx := context.Background()

	// Use up all available slots
	for i := 0; i < 2; i++ {
		result, err := limiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil || !result.Allowed {
			t.Fatalf("Expected request %d to be allowed", i+1)
		}
	}

	// Next request should be denied
	result, err := limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("Expected request to be denied before reset")
	}

	// Reset the rate limit
	if err := limiter.Reset(ctx, entity, ScopeGlobal); err != nil {
		t.Fatalf("Failed to reset rate limit: %v", err)
	}

	// Next request should be allowed again
	result, err = limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error after reset: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected request to be allowed after reset")
	}

	// Should have full capacity available
	if result.Remaining != 1 {
		t.Errorf("Expected 1 slot remaining after reset, got %d", result.Remaining)
	}

	if result.Used != 1 {
		t.Errorf("Expected 1 request used after reset, got %d", result.Used)
	}
}

func TestRateLimiterSlidingWindowStats(t *testing.T) {
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory"

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := NewDefaultAuthEntity("test-user-stats", EntityTypeUser, TierFree)
	ctx := context.Background()

	// Make some requests
	for i := 0; i < 5; i++ {
		limiter.Allow(ctx, entity, ScopeGlobal)
	}

	// Get stats
	stats, err := limiter.Stats(ctx, entity)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats == nil {
		t.Fatal("Expected stats to be returned")
	}

	if stats.Entity.ID() != entity.ID() {
		t.Errorf("Expected entity ID %s, got %s", entity.ID(), stats.Entity.ID())
	}

	// Should have global scope stats
	if _, exists := stats.Scopes[ScopeGlobal]; !exists {
		t.Error("Expected global scope stats to be present")
	}

	// Get scope-specific stats
	scopeStats, err := limiter.ScopeStats(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Failed to get scope stats: %v", err)
	}

	if scopeStats == nil {
		t.Fatal("Expected scope stats to be returned")
	}

	if scopeStats.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm to be sliding_window, got %s", scopeStats.Algorithm)
	}

	if scopeStats.RequestCount != 5 {
		t.Errorf("Expected 5 total requests, got %d", scopeStats.RequestCount)
	}

	if scopeStats.CurrentUsage != 5 {
		t.Errorf("Expected 5 current usage, got %d", scopeStats.CurrentUsage)
	}
}

func TestRateLimiterSlidingWindowVsTokenBucket(t *testing.T) {
	// Test the difference between sliding window and token bucket algorithms
	// Both should have the same limit and window, but behave differently

	entity := NewDefaultAuthEntity("test-user-comparison", EntityTypeUser, TierFree)
	ctx := context.Background()

	// Create sliding window limiter
	swConfig := DefaultConfig()
	swConfig.Algorithm = "sliding_window"
	swConfig.Store = "memory"
	swConfig.TierLimits = map[string]TierConfig{
		TierFree: {
			DefaultLimits: map[string]RateLimit{
				ScopeGlobal: {Requests: 5, Window: 2 * time.Second},
			},
		},
	}

	swLimiter, err := NewRateLimiter(swConfig)
	if err != nil {
		t.Fatalf("Failed to create sliding window limiter: %v", err)
	}
	defer swLimiter.Close()

	// Create token bucket limiter
	tbConfig := DefaultConfig()
	tbConfig.Algorithm = "token_bucket"
	tbConfig.Store = "memory"
	tbConfig.TierLimits = map[string]TierConfig{
		TierFree: {
			DefaultLimits: map[string]RateLimit{
				ScopeGlobal: {Requests: 5, Window: 2 * time.Second},
			},
		},
	}

	tbLimiter, err := NewRateLimiter(tbConfig)
	if err != nil {
		t.Fatalf("Failed to create token bucket limiter: %v", err)
	}
	defer tbLimiter.Close()

	// Both should allow the first 5 requests
	for i := 0; i < 5; i++ {
		swResult, err := swLimiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("SW: Unexpected error on request %d: %v", i+1, err)
		}
		if !swResult.Allowed {
			t.Errorf("SW: Expected request %d to be allowed", i+1)
		}

		tbResult, err := tbLimiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("TB: Unexpected error on request %d: %v", i+1, err)
		}
		if !tbResult.Allowed {
			t.Errorf("TB: Expected request %d to be allowed", i+1)
		}
	}

	// 6th request should be denied by both
	swResult, err := swLimiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("SW: Unexpected error: %v", err)
	}
	if swResult.Allowed {
		t.Error("SW: Expected 6th request to be denied")
	}

	tbResult, err := tbLimiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("TB: Unexpected error: %v", err)
	}
	if tbResult.Allowed {
		t.Error("TB: Expected 6th request to be denied")
	}

	// Wait for the window to partially slide
	time.Sleep(1100 * time.Millisecond) // Wait 1.1 seconds

	// At this point:
	// - Sliding window: All 5 requests are still in the 2-second window, so no new requests allowed
	// - Token bucket: Some tokens might have been refilled

	swResult, err = swLimiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("SW: Unexpected error after partial wait: %v", err)
	}

	tbResult, err = tbLimiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("TB: Unexpected error after partial wait: %v", err)
	}

	// Sliding window should still deny (all requests still in window)
	if swResult.Allowed {
		t.Error("SW: Expected request to still be denied after partial slide")
	}

	// Token bucket might allow (depending on refill rate)
	t.Logf("SW allowed after 1.1s: %t, TB allowed after 1.1s: %t", swResult.Allowed, tbResult.Allowed)
}

func TestRateLimiterSlidingWindowTimeAccuracy(t *testing.T) {
	// Test that sliding window accurately tracks requests over time
	config := DefaultConfig()
	config.Algorithm = "sliding_window"
	config.Store = "memory"
	config.TierLimits = map[string]TierConfig{
		TierFree: {
			DefaultLimits: map[string]RateLimit{
				ScopeGlobal: {Requests: 3, Window: 1 * time.Second},
			},
		},
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := NewDefaultAuthEntity("test-user-timing", EntityTypeUser, TierFree)
	ctx := context.Background()

	// Make 3 requests quickly
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("Unexpected error on request %d: %v", i+1, err)
		}
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
		time.Sleep(10 * time.Millisecond) // Small delay between requests
	}

	// 4th request should be denied
	result, err := limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("Expected 4th request to be denied")
	}

	// Wait for window to slide past the first request
	time.Sleep(1100 * time.Millisecond)

	// Now should be able to make requests again
	result, err = limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error after window slide: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected request to be allowed after window slides")
	}

	// Should have capacity for more requests since old ones expired
	if result.Remaining < 1 {
		t.Errorf("Expected at least 1 remaining slot, got %d", result.Remaining)
	}
}
