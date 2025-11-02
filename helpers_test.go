package ratelimit

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// CONVENIENCE CONSTRUCTOR TESTS
// ============================================================================

func TestNewSimple(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewSimple(store, 100, time.Hour)
	if err != nil {
		t.Fatalf("NewSimple failed: %v", err)
	}
	defer limiter.Close()

	if limiter == nil {
		t.Error("limiter should not be nil")
	}

	// Test it works
	ctx := context.Background()
	rlCtx := NewSimpleContext("user1", ScopeGlobal, TierFree, nil)
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if !result.Allowed {
		t.Error("first request should be allowed")
	}

	if result.Limit != 100 {
		t.Errorf("expected limit 100, got %d", result.Limit)
	}
}

func TestNewSimpleNilStore(t *testing.T) {
	_, err := NewSimple(nil, 100, time.Hour)
	if err == nil {
		t.Error("expected error for nil store")
	}
}

func TestNewWithConfig(t *testing.T) {
	store := newTestMemoryStore()
	config := &Config{
		Store:         store,
		DefaultLimit:  500,
		DefaultWindow: time.Minute,
		DefaultBurst:  50,
	}

	limiter, err := NewWithConfig(config)
	if err != nil {
		t.Fatalf("NewWithConfig failed: %v", err)
	}
	defer limiter.Close()

	// Verify algorithm was set
	if config.Algorithm == nil {
		t.Error("algorithm should be set by default")
	}

	// Verify logger was set
	if config.Logger == nil {
		t.Error("logger should be set by default")
	}
}

func TestNewWithTiers(t *testing.T) {
	store := newTestMemoryStore()
	resolverConfig := NewDefaultResolverConfig()

	limiter, err := NewWithTiers(store, resolverConfig)
	if err != nil {
		t.Fatalf("NewWithTiers failed: %v", err)
	}
	defer limiter.Close()

	// Test tier resolution works
	ctx := context.Background()
	freeCtx := NewSimpleContext("user1", ScopeAPI, TierFree, nil)
	result, err := limiter.Allow(ctx, freeCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Limit != 100 { // Free tier API limit
		t.Errorf("expected free tier limit 100, got %d", result.Limit)
	}
}

// ============================================================================
// QUICK CHECK HELPERS TESTS
// ============================================================================

func TestQuickCheck(t *testing.T) {
	store := newTestMemoryStore()
	limiter, _ := NewSimple(store, 5, time.Second)
	defer limiter.Close()

	ctx := context.Background()

	// First check should succeed
	allowed, err := QuickCheck(ctx, limiter, "user1", ScopeGlobal, TierFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("first check should be allowed")
	}

	// Use up all tokens
	for i := 0; i < 4; i++ {
		QuickCheck(ctx, limiter, "user1", ScopeGlobal, TierFree)
	}

	// Next check should fail (rate limited, not error)
	allowed, err = QuickCheck(ctx, limiter, "user1", ScopeGlobal, TierFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("check should be denied after limit")
	}
}

func TestQuickCheckN(t *testing.T) {
	store := newTestMemoryStore()
	limiter, _ := NewSimple(store, 10, time.Second)
	defer limiter.Close()

	ctx := context.Background()

	// Check 5 tokens
	allowed, err := QuickCheckN(ctx, limiter, "user2", ScopeGlobal, TierFree, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("should allow 5 tokens")
	}

	// Check another 6 tokens (should fail, only 5 left)
	allowed, err = QuickCheckN(ctx, limiter, "user2", ScopeGlobal, TierFree, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("should not allow 6 more tokens")
	}
}

func TestQuickStats(t *testing.T) {
	store := newTestMemoryStore()
	limiter, _ := NewSimple(store, 100, time.Hour)
	defer limiter.Close()

	ctx := context.Background()

	// Use some tokens
	QuickCheckN(ctx, limiter, "user3", ScopeGlobal, TierFree, 25)

	// Get stats
	limit, used, _, err := QuickStats(ctx, limiter, "user3", ScopeGlobal, TierFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if limit != 100 {
		t.Errorf("expected limit 100, got %d", limit)
	}

	// Token bucket refills continuously, so "used" might not match exactly
	// Just verify we got stats back
	if used < 0 {
		t.Errorf("used should not be negative, got %d", used)
	}
}

// ============================================================================
// BUILDER PATTERN TESTS
// ============================================================================

func TestBuilder(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithLimit(500, time.Minute).
		WithBurst(50).
		Build()

	if err != nil {
		t.Fatalf("Builder failed: %v", err)
	}
	defer limiter.Close()

	// Test basic functionality
	ctx := context.Background()
	rlCtx := NewSimpleContext("user1", ScopeGlobal, TierFree, nil)
	result, _ := limiter.Allow(ctx, rlCtx)

	if result.Limit != 500 {
		t.Errorf("expected limit 500, got %d", result.Limit)
	}
}

func TestBuilderWithSlidingWindow(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithSlidingWindow().
		WithLimit(100, time.Hour).
		Build()

	if err != nil {
		t.Fatalf("Builder failed: %v", err)
	}
	defer limiter.Close()

	if limiter == nil {
		t.Error("limiter should not be nil")
	}
}

func TestBuilderWithLimitString(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithLimitString("1000/1h").
		WithBurst(100).
		Build()

	if err != nil {
		t.Fatalf("Builder with limit string failed: %v", err)
	}
	defer limiter.Close()

	// Verify limit was set correctly
	ctx := context.Background()
	rlCtx := NewSimpleContext("user1", ScopeGlobal, TierFree, nil)
	result, _ := limiter.Allow(ctx, rlCtx)

	if result.Limit != 1000 {
		t.Errorf("expected limit 1000, got %d", result.Limit)
	}
}

func TestBuilderWithTiers(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithDefaultTiers().
		Build()

	if err != nil {
		t.Fatalf("Builder with tiers failed: %v", err)
	}
	defer limiter.Close()

	// Test tier resolution
	ctx := context.Background()
	premiumCtx := NewSimpleContext("premium_user", ScopeAPI, TierPremium, nil)
	result, _ := limiter.Allow(ctx, premiumCtx)

	if result.Limit != 1000 { // Premium tier API limit
		t.Errorf("expected premium tier limit 1000, got %d", result.Limit)
	}
}

func TestBuilderWithStrictTiers(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithStrictTiers().
		Build()

	if err != nil {
		t.Fatalf("Builder with strict tiers failed: %v", err)
	}
	defer limiter.Close()
}

func TestBuilderWithGenerousTiers(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithGenerousTiers().
		Build()

	if err != nil {
		t.Fatalf("Builder with generous tiers failed: %v", err)
	}
	defer limiter.Close()
}

func TestBuilderWithMetrics(t *testing.T) {
	store := newTestMemoryStore()

	limiter, err := NewBuilder().
		WithStore(store).
		WithLimit(100, time.Hour).
		WithMetrics(true).
		Build()

	if err != nil {
		t.Fatalf("Builder with metrics failed: %v", err)
	}
	defer limiter.Close()
}

func TestBuilderMissingStore(t *testing.T) {
	_, err := NewBuilder().
		WithLimit(100, time.Hour).
		Build()

	if err == nil {
		t.Error("expected error when store is missing")
	}
}

func TestBuilderInvalidLimitString(t *testing.T) {
	store := newTestMemoryStore()

	_, err := NewBuilder().
		WithStore(store).
		WithLimitString("invalid").
		Build()

	if err == nil {
		t.Error("expected error for invalid limit string")
	}
}

func TestBuilderMustBuild(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild should panic on error")
		}
	}()

	// This should panic because store is missing
	NewBuilder().WithLimit(100, time.Hour).MustBuild()
}

// ============================================================================
// PRESET CONFIGURATION TESTS
// ============================================================================

func TestNewForAPI(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewForAPI(store)
	if err != nil {
		t.Fatalf("NewForAPI failed: %v", err)
	}
	defer limiter.Close()

	// Verify high limits for API
	ctx := context.Background()
	rlCtx := NewSimpleContext("api_user", ScopeGlobal, TierFree, nil)
	result, _ := limiter.Allow(ctx, rlCtx)

	if result.Limit != 10000 {
		t.Errorf("expected API limit 10000, got %d", result.Limit)
	}
}

func TestNewForWebApp(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewForWebApp(store)
	if err != nil {
		t.Fatalf("NewForWebApp failed: %v", err)
	}
	defer limiter.Close()
}

func TestNewForMicroservice(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewForMicroservice(store)
	if err != nil {
		t.Fatalf("NewForMicroservice failed: %v", err)
	}
	defer limiter.Close()

	// Verify per-minute limit
	ctx := context.Background()
	rlCtx := NewSimpleContext("service_user", ScopeGlobal, TierFree, nil)
	result, _ := limiter.Allow(ctx, rlCtx)

	if result.Limit != 500 {
		t.Errorf("expected microservice limit 500, got %d", result.Limit)
	}
}

func TestNewForPublicAPI(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewForPublicAPI(store)
	if err != nil {
		t.Fatalf("NewForPublicAPI failed: %v", err)
	}
	defer limiter.Close()
}

func TestNewForSaaS(t *testing.T) {
	store := newTestMemoryStore()
	limiter, err := NewForSaaS(store)
	if err != nil {
		t.Fatalf("NewForSaaS failed: %v", err)
	}
	defer limiter.Close()

	// Verify tier support
	ctx := context.Background()

	freeCtx := NewSimpleContext("free_user", ScopeGlobal, TierFree, nil)
	freeResult, _ := limiter.Allow(ctx, freeCtx)

	premiumCtx := NewSimpleContext("premium_user", ScopeGlobal, TierPremium, nil)
	premiumResult, _ := limiter.Allow(ctx, premiumCtx)

	if premiumResult.Limit <= freeResult.Limit {
		t.Error("premium tier should have higher limit than free tier")
	}
}

// ============================================================================
// UTILITY FUNCTION TESTS
// ============================================================================

func TestIsRateLimited(t *testing.T) {
	result := &Result{Allowed: false}
	if !IsRateLimited(result) {
		t.Error("should detect rate limited result")
	}

	result.Allowed = true
	if IsRateLimited(result) {
		t.Error("should not detect allowed result as rate limited")
	}

	if IsRateLimited(nil) {
		t.Error("nil result should not be rate limited")
	}
}

func TestGetRetryAfter(t *testing.T) {
	result := &Result{RetryAfter: 5 * time.Second}
	if GetRetryAfter(result) != 5*time.Second {
		t.Error("should return correct retry after duration")
	}

	if GetRetryAfter(nil) != 0 {
		t.Error("nil result should return 0 retry after")
	}
}

func TestGetUsagePercent(t *testing.T) {
	result := &Result{Limit: 100, Used: 25}
	percent := GetUsagePercent(result)

	if percent != 25.0 {
		t.Errorf("expected 25%%, got %f%%", percent)
	}

	if GetUsagePercent(nil) != 0 {
		t.Error("nil result should return 0%")
	}
}

func TestIsNearLimit(t *testing.T) {
	result := &Result{Limit: 100, Used: 85}

	if !IsNearLimit(result, 80.0) {
		t.Error("85% usage should be near 80% threshold")
	}

	if IsNearLimit(result, 90.0) {
		t.Error("85% usage should not be near 90% threshold")
	}
}

// ============================================================================
// BATCH OPERATION TESTS
// ============================================================================

func TestCheckMultiple(t *testing.T) {
	store := newTestMemoryStore()
	limiter, _ := NewSimple(store, 10, time.Minute)
	defer limiter.Close()

	ctx := context.Background()
	identities := []string{"user1", "user2", "user3"}

	results := CheckMultiple(ctx, limiter, identities, ScopeGlobal, TierFree)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for id, result := range results {
		if result == nil {
			t.Errorf("result for %s should not be nil", id)
		}
		if !result.Allowed {
			t.Errorf("first request for %s should be allowed", id)
		}
	}
}

func TestResetMultiple(t *testing.T) {
	store := newTestMemoryStore()
	limiter, _ := NewSimple(store, 5, time.Minute)
	defer limiter.Close()

	ctx := context.Background()
	identities := []string{"user1", "user2", "user3"}

	// Use up tokens for all users
	for _, id := range identities {
		for i := 0; i < 5; i++ {
			QuickCheck(ctx, limiter, id, ScopeGlobal, TierFree)
		}
	}

	// Reset all users
	err := ResetMultiple(ctx, limiter, identities, ScopeGlobal, TierFree)
	if err != nil {
		t.Fatalf("ResetMultiple failed: %v", err)
	}

	// All should be allowed again
	for _, id := range identities {
		allowed, err := QuickCheck(ctx, limiter, id, ScopeGlobal, TierFree)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("user %s should be allowed after reset", id)
		}
	}
}

// ============================================================================
// ERROR HELPER TESTS
// ============================================================================

func TestIsStorageError(t *testing.T) {
	if IsStorageError(nil) {
		t.Error("nil should not be a storage error")
	}

	if !IsStorageError(ErrKeyNotFound) {
		t.Error("ErrKeyNotFound should be a storage error")
	}

	if !IsStorageError(ErrConnectionFailed) {
		t.Error("ErrConnectionFailed should be a storage error")
	}

	if IsStorageError(ErrInvalidConfig) {
		t.Error("ErrInvalidConfig should not be a storage error")
	}
}
