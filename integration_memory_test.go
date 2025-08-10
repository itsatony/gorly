// integration_memory_test.go
package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiterWithMemoryStore(t *testing.T) {
	// Create config with memory store
	config := DefaultConfig()
	config.Store = "memory"

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

	if result.Algorithm != "token_bucket" {
		t.Errorf("Expected algorithm to be token_bucket, got %s", result.Algorithm)
	}

	// Make multiple requests to test token consumption
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
	}

	// Check health
	if err := limiter.Health(ctx); err != nil {
		t.Errorf("Expected health check to pass: %v", err)
	}
}

func TestRateLimiterMemoryStoreRateLimiting(t *testing.T) {
	// Create config with low limits for testing
	config := DefaultConfig()
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
		t.Errorf("Expected 0 remaining tokens, got %d", result.Remaining)
	}

	if result.RetryAfter == 0 {
		t.Error("Expected retry after time to be set")
	}
}

func TestRateLimiterMemoryStoreMultipleScopes(t *testing.T) {
	// Create config with memory store
	config := DefaultConfig()
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
	}
}

func TestRateLimiterMemoryStoreReset(t *testing.T) {
	// Create config with memory store and low limits
	config := DefaultConfig()
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

	// Use up all tokens
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

	// Should have full tokens available
	if result.Remaining != 1 {
		t.Errorf("Expected 1 token remaining after reset, got %d", result.Remaining)
	}
}

func TestRateLimiterMemoryStoreStats(t *testing.T) {
	config := DefaultConfig()
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

	if scopeStats.Algorithm != "token_bucket" {
		t.Errorf("Expected algorithm to be token_bucket, got %s", scopeStats.Algorithm)
	}
}
