package ratelimit

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// LIMITER WITH RESOLVER INTEGRATION TESTS
// ============================================================================

func TestLimiterWithResolver(t *testing.T) {
	store := newTestMemoryStore()

	// Create resolver with tier configuration
	resolverConfig := NewDefaultResolverConfig()
	resolver, err := NewLimitResolver(resolverConfig)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	// Create limiter with resolver
	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10, // Fallback limit
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Test free tier user (should get 100/hour limit from resolver)
	freeCtx := NewSimpleContext("free_user", ScopeAPI, TierFree, nil)
	result, err := limiter.Allow(ctx, freeCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if !result.Allowed {
		t.Error("free tier user should be allowed")
	}

	if result.Limit != 100 {
		t.Errorf("expected limit 100 from resolver, got %d", result.Limit)
	}

	// Test premium tier user (should get 1000/hour limit from resolver)
	premiumCtx := NewSimpleContext("premium_user", ScopeAPI, TierPremium, nil)
	result, err = limiter.Allow(ctx, premiumCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if !result.Allowed {
		t.Error("premium tier user should be allowed")
	}

	if result.Limit != 1000 {
		t.Errorf("expected limit 1000 from resolver, got %d", result.Limit)
	}
}

func TestLimiterWithResolverEntityOverride(t *testing.T) {
	store := newTestMemoryStore()

	// Create resolver
	resolverConfig := NewDefaultResolverConfig()
	resolver, err := NewLimitResolver(resolverConfig)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	// Add entity override
	specialUserID := "special_user"
	overrideLimit := NewLimitConfig(5000, time.Hour, 500)
	err = resolver.SetEntityOverride(specialUserID, ScopeAPI, overrideLimit)
	if err != nil {
		t.Fatalf("failed to set entity override: %v", err)
	}

	// Create limiter with resolver
	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Special user should get override limit
	specialCtx := NewSimpleContext(specialUserID, ScopeAPI, TierFree, nil)
	result, err := limiter.Allow(ctx, specialCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Limit != 5000 {
		t.Errorf("expected override limit 5000, got %d", result.Limit)
	}

	// Regular free user should get tier limit
	regularCtx := NewSimpleContext("regular_user", ScopeAPI, TierFree, nil)
	result, err = limiter.Allow(ctx, regularCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Limit != 100 {
		t.Errorf("expected tier limit 100, got %d", result.Limit)
	}
}

func TestLimiterWithResolverScopeSpecific(t *testing.T) {
	store := newTestMemoryStore()

	// Create resolver with default config
	resolverConfig := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(resolverConfig)

	// Create limiter
	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	limiter, _ := NewRateLimiter(config)
	defer limiter.Close()

	ctx := context.Background()
	userID := "test_user"

	// Test different scopes get different limits
	tests := []struct {
		scope         string
		tier          string
		expectedLimit int64
	}{
		{ScopeAPI, TierFree, 100},
		{ScopeSearch, TierFree, 20},
		{ScopeUpload, TierFree, 10},
		{ScopeAPI, TierPremium, 1000},
		{ScopeSearch, TierPremium, 500},
	}

	for _, tt := range tests {
		t.Run(tt.scope+"_"+tt.tier, func(t *testing.T) {
			rlCtx := NewSimpleContext(userID+"_"+tt.scope, tt.scope, tt.tier, nil)
			result, err := limiter.Allow(ctx, rlCtx)
			if err != nil {
				t.Fatalf("Allow failed: %v", err)
			}

			if result.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d for %s/%s, got %d",
					tt.expectedLimit, tt.tier, tt.scope, result.Limit)
			}
		})
	}
}

func TestLimiterWithoutResolver(t *testing.T) {
	store := newTestMemoryStore()

	// Create limiter WITHOUT resolver (should use default limits)
	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// All users should get the same default limit regardless of tier
	tests := []struct {
		tier string
	}{
		{TierFree},
		{TierPremium},
		{TierEnterprise},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			rlCtx := NewSimpleContext("user_"+tt.tier, ScopeGlobal, tt.tier, nil)
			result, err := limiter.Allow(ctx, rlCtx)
			if err != nil {
				t.Fatalf("Allow failed: %v", err)
			}

			if result.Limit != 10 {
				t.Errorf("expected default limit 10, got %d", result.Limit)
			}
		})
	}
}

func TestLimiterResolverFallbackOnError(t *testing.T) {
	store := newTestMemoryStore()

	// Create a resolver with minimal config
	// No tier configs set, so resolver will use its global default
	resolverConfig := NewResolverConfig()
	resolver, _ := NewLimitResolver(resolverConfig)

	// Create limiter with fallback defaults
	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  50, // Fallback (won't be used since resolver always succeeds)
		DefaultWindow: time.Minute,
		DefaultBurst:  10,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Resolver falls back to global default (1000) when no config is found
	// This tests that resolver always returns something valid
	rlCtx := NewSimpleContext("test_user", ScopeGlobal, TierFree, nil)
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Limit != DefaultLimit {
		t.Errorf("expected global default limit %d, got %d", DefaultLimit, result.Limit)
	}
}

func TestLimiterStatsWithResolver(t *testing.T) {
	store := newTestMemoryStore()

	resolverConfig := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(resolverConfig)

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	limiter, _ := NewRateLimiter(config)
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)

	// Use some tokens
	limiter.AllowN(ctx, rlCtx, 5)

	// Get stats - should reflect resolved limits
	stats, err := limiter.Stats(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Limit != 1000 {
		t.Errorf("stats should show resolved limit 1000, got %d", stats.Limit)
	}

	if stats.Entity != "user123" {
		t.Errorf("expected entity 'user123', got '%s'", stats.Entity)
	}
}

func TestLimiterResolverPresets(t *testing.T) {

	tests := []struct {
		name           string
		resolverConfig *ResolverConfig
		tier           string
		expectedMin    int64 // Minimum expected limit
		expectedMax    int64 // Maximum expected limit
	}{
		{
			name:           "Default preset - Free tier",
			resolverConfig: NewDefaultResolverConfig(),
			tier:           TierFree,
			expectedMin:    50,
			expectedMax:    200,
		},
		{
			name:           "Strict preset - Free tier",
			resolverConfig: NewStrictResolverConfig(),
			tier:           TierFree,
			expectedMin:    10,
			expectedMax:    100,
		},
		{
			name:           "Generous preset - Free tier",
			resolverConfig: NewGenerousResolverConfig(),
			tier:           TierFree,
			expectedMin:    200,
			expectedMax:    1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, _ := NewLimitResolver(tt.resolverConfig)

			config := &Config{
				Store:         newTestMemoryStore(),
				Algorithm:     NewTokenBucketAlgorithm(),
				DefaultLimit:  10,
				DefaultWindow: time.Second,
				DefaultBurst:  5,
				Resolver:      resolver,
				Logger:        NewNopLogger(),
			}

			limiter, _ := NewRateLimiter(config)
			defer limiter.Close()

			ctx := context.Background()
			rlCtx := NewSimpleContext("test_user", ScopeGlobal, tt.tier, nil)

			result, err := limiter.Allow(ctx, rlCtx)
			if err != nil {
				t.Fatalf("Allow failed: %v", err)
			}

			if result.Limit < tt.expectedMin || result.Limit > tt.expectedMax {
				t.Errorf("expected limit between %d and %d, got %d",
					tt.expectedMin, tt.expectedMax, result.Limit)
			}
		})
	}
}
