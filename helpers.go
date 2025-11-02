package ratelimit

import (
	"context"
	"time"
)

// ============================================================================
// CONVENIENCE CONSTRUCTORS
// ============================================================================

// NewSimple creates a rate limiter with the simplest possible configuration
// Requires a store to be provided (use stores.NewMemoryStore(nil) for in-memory storage)
//
// Example:
//
//	store := stores.NewMemoryStore(nil)
//	limiter, err := ratelimit.NewSimple(store, 100, time.Hour)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer limiter.Close()
func NewSimple(store Store, limit int64, window time.Duration) (RateLimiter, error) {
	if store == nil {
		return nil, WrapConfigError(nil, "store is required")
	}

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  limit,
		DefaultWindow: window,
		DefaultBurst:  limit / 10, // 10% of limit as burst
		Logger:        NewNopLogger(),
	}

	return NewRateLimiter(config)
}

// NewWithConfig creates a rate limiter with the given configuration
// Fills in sensible defaults for any missing values
//
// Example:
//
//	store := stores.NewMemoryStore(nil)
//	limiter, err := ratelimit.NewWithConfig(&ratelimit.Config{
//	    Store:         store,
//	    DefaultLimit:  1000,
//	    DefaultWindow: time.Hour,
//	    DefaultBurst:  100,
//	})
func NewWithConfig(config *Config) (RateLimiter, error) {
	if config.Algorithm == nil {
		config.Algorithm = NewTokenBucketAlgorithm()
	}
	if config.Logger == nil {
		config.Logger = NewNopLogger()
	}

	return NewRateLimiter(config)
}

// NewWithTiers creates a rate limiter with multi-tier support
// Uses the provided resolver configuration for tier-based limits
// Requires a store to be provided
//
// Example:
//
//	store := stores.NewMemoryStore(nil)
//	resolverConfig := ratelimit.NewDefaultResolverConfig()
//	limiter, err := ratelimit.NewWithTiers(store, resolverConfig)
func NewWithTiers(store Store, resolverConfig *ResolverConfig) (RateLimiter, error) {
	if store == nil {
		return nil, WrapConfigError(nil, "store is required")
	}

	resolver, err := NewLimitResolver(resolverConfig)
	if err != nil {
		return nil, WrapConfigError(err, "failed to create resolver")
	}

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  DefaultLimit,
		DefaultWindow: DefaultWindow,
		DefaultBurst:  DefaultBurst,
		Resolver:      resolver,
		Logger:        NewNopLogger(),
	}

	return NewRateLimiter(config)
}

// ============================================================================
// QUICK CHECK HELPERS
// ============================================================================

// QuickCheck is a convenience function for simple rate limit checks
// Returns true if the request is allowed, false if rate limited, error if operation failed
//
// BREAKING CHANGE: Now returns (bool, error) instead of just bool
// This allows callers to distinguish between rate limiting and errors
//
// Example:
//
//	allowed, err := ratelimit.QuickCheck(ctx, limiter, "user123", "global", "free")
//	if err != nil {
//	    // Handle error (e.g., store unavailable)
//	    return err
//	}
//	if !allowed {
//	    // Rate limit exceeded
//	    return http.StatusTooManyRequests
//	}
func QuickCheck(ctx context.Context, limiter RateLimiter, identity, scope, tier string) (bool, error) {
	rlCtx := NewSimpleContext(identity, scope, tier, nil)
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		return false, err
	}
	return result.Allowed, nil
}

// QuickCheckN is a convenience function for checking N tokens
// Returns true if the request is allowed, false if rate limited, error if operation failed
//
// BREAKING CHANGE: Now returns (bool, error) instead of just bool
func QuickCheckN(ctx context.Context, limiter RateLimiter, identity, scope, tier string, n int64) (bool, error) {
	rlCtx := NewSimpleContext(identity, scope, tier, nil)
	result, err := limiter.AllowN(ctx, rlCtx, n)
	if err != nil {
		return false, err
	}
	return result.Allowed, nil
}

// QuickStats returns basic usage statistics for an identity
// Returns limit, used, remaining, and error
//
// BREAKING CHANGE: Now returns (int64, int64, int64, error) instead of just (int64, int64, int64)
// Zero values no longer indicate errors - check the error return instead
func QuickStats(ctx context.Context, limiter RateLimiter, identity, scope, tier string) (limit, used, remaining int64, err error) {
	rlCtx := NewSimpleContext(identity, scope, tier, nil)
	result, err := limiter.Stats(ctx, rlCtx)
	if err != nil {
		return 0, 0, 0, err
	}
	return result.Limit, result.Used, result.Remaining, nil
}

// ============================================================================
// BUILDER PATTERN
// ============================================================================

// Builder provides a fluent API for constructing rate limiters
type Builder struct {
	config *Config
	err    error
}

// NewBuilder creates a new rate limiter builder
func NewBuilder() *Builder {
	return &Builder{
		config: &Config{
			DefaultLimit:  DefaultLimit,
			DefaultWindow: DefaultWindow,
			DefaultBurst:  DefaultBurst,
			Logger:        NewNopLogger(),
		},
	}
}

// WithStore sets a custom store
func (b *Builder) WithStore(store Store) *Builder {
	if b.err != nil {
		return b
	}
	b.config.Store = store
	return b
}

// WithTokenBucket sets the token bucket algorithm
func (b *Builder) WithTokenBucket() *Builder {
	if b.err != nil {
		return b
	}
	b.config.Algorithm = NewTokenBucketAlgorithm()
	return b
}

// WithSlidingWindow sets the sliding window algorithm
func (b *Builder) WithSlidingWindow() *Builder {
	if b.err != nil {
		return b
	}
	b.config.Algorithm = NewSlidingWindowAlgorithm()
	return b
}

// WithAlgorithm sets a custom algorithm
func (b *Builder) WithAlgorithm(algorithm Algorithm) *Builder {
	if b.err != nil {
		return b
	}
	b.config.Algorithm = algorithm
	return b
}

// WithLimit sets the default rate limit
func (b *Builder) WithLimit(limit int64, window time.Duration) *Builder {
	if b.err != nil {
		return b
	}
	b.config.DefaultLimit = limit
	b.config.DefaultWindow = window
	return b
}

// WithLimitString sets the limit from a rate string like "1000/1h"
func (b *Builder) WithLimitString(rateStr string) *Builder {
	if b.err != nil {
		return b
	}
	limit, window, err := ParseRateString(rateStr)
	if err != nil {
		b.err = err
		return b
	}
	b.config.DefaultLimit = limit
	b.config.DefaultWindow = window
	return b
}

// WithBurst sets the burst size
func (b *Builder) WithBurst(burst int64) *Builder {
	if b.err != nil {
		return b
	}
	b.config.DefaultBurst = burst
	return b
}

// WithResolver sets a limit resolver for tier-based limits
func (b *Builder) WithResolver(resolver LimitResolver) *Builder {
	if b.err != nil {
		return b
	}
	b.config.Resolver = resolver
	return b
}

// WithTiers sets up tier-based limiting with the given configuration
func (b *Builder) WithTiers(resolverConfig *ResolverConfig) *Builder {
	if b.err != nil {
		return b
	}
	resolver, err := NewLimitResolver(resolverConfig)
	if err != nil {
		b.err = WrapConfigError(err, "failed to create resolver")
		return b
	}
	b.config.Resolver = resolver
	return b
}

// WithDefaultTiers sets up tier-based limiting with default configuration
func (b *Builder) WithDefaultTiers() *Builder {
	return b.WithTiers(NewDefaultResolverConfig())
}

// WithStrictTiers sets up tier-based limiting with strict limits
func (b *Builder) WithStrictTiers() *Builder {
	return b.WithTiers(NewStrictResolverConfig())
}

// WithGenerousTiers sets up tier-based limiting with generous limits
func (b *Builder) WithGenerousTiers() *Builder {
	return b.WithTiers(NewGenerousResolverConfig())
}

// WithLogger sets a custom logger
func (b *Builder) WithLogger(logger Logger) *Builder {
	if b.err != nil {
		return b
	}
	b.config.Logger = logger
	return b
}

// WithMetrics enables metrics collection
func (b *Builder) WithMetrics(enable bool) *Builder {
	if b.err != nil {
		return b
	}
	b.config.EnableMetrics = enable
	return b
}

// Build creates the rate limiter
func (b *Builder) Build() (RateLimiter, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Set defaults if not configured
	if b.config.Store == nil {
		return nil, WrapConfigError(nil, "store is required - use WithStore() to set it")
	}
	if b.config.Algorithm == nil {
		b.config.Algorithm = NewTokenBucketAlgorithm()
	}

	return NewRateLimiter(b.config)
}

// MustBuild creates the rate limiter or panics on error
// Use only when you're certain the configuration is valid
func (b *Builder) MustBuild() RateLimiter {
	limiter, err := b.Build()
	if err != nil {
		panic(err)
	}
	return limiter
}

// ============================================================================
// PRESET CONFIGURATIONS
// ============================================================================

// NewForAPI creates a rate limiter optimized for API gateway use
// - Higher limits for throughput
// - Token bucket for burst handling
// Requires a store to be provided
func NewForAPI(store Store) (RateLimiter, error) {
	return NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithLimit(10000, time.Hour).
		WithBurst(1000).
		Build()
}

// NewForWebApp creates a rate limiter optimized for web applications
// - Moderate limits
// - Sliding window for fairness
// Requires a store to be provided
func NewForWebApp(store Store) (RateLimiter, error) {
	return NewBuilder().
		WithStore(store).
		WithSlidingWindow().
		WithLimit(1000, time.Hour).
		WithBurst(100).
		Build()
}

// NewForMicroservice creates a rate limiter optimized for microservices
// - Lower limits to protect services
// - Token bucket for burst handling
// Requires a store to be provided
func NewForMicroservice(store Store) (RateLimiter, error) {
	return NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithLimit(500, time.Minute).
		WithBurst(50).
		Build()
}

// NewForPublicAPI creates a rate limiter for public-facing APIs
// - Strict limits to prevent abuse
// - Sliding window for fairness
// - Multi-tier support
// Requires a store to be provided
func NewForPublicAPI(store Store) (RateLimiter, error) {
	return NewBuilder().
		WithStore(store).
		WithSlidingWindow().
		WithStrictTiers().
		Build()
}

// NewForSaaS creates a rate limiter for SaaS applications
// - Tier-based limits (free, premium, enterprise)
// - Token bucket for user experience
// Requires a store to be provided
func NewForSaaS(store Store) (RateLimiter, error) {
	return NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithDefaultTiers().
		Build()
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// IsRateLimited checks if a result indicates rate limiting
func IsRateLimited(result *Result) bool {
	return result != nil && !result.Allowed
}

// GetRetryAfter returns the retry-after duration from a result
func GetRetryAfter(result *Result) time.Duration {
	if result == nil {
		return 0
	}
	return result.RetryAfter
}

// GetUsagePercent returns the usage as a percentage (0-100)
func GetUsagePercent(result *Result) float64 {
	if result == nil || result.Limit == 0 {
		return 0
	}
	return (float64(result.Used) / float64(result.Limit)) * 100
}

// IsNearLimit checks if usage is near the limit (>= threshold %)
func IsNearLimit(result *Result, thresholdPercent float64) bool {
	return GetUsagePercent(result) >= thresholdPercent
}

// ============================================================================
// CONTEXT HELPERS
// ============================================================================

// NewScopedContext creates a context for a user with a specific scope
// This is a helper that wraps NewSimpleContext with clearer naming
func NewScopedContext(identity, scope, tier string, metadata map[string]interface{}) Identity {
	return NewSimpleContext(identity, scope, tier, metadata)
}

// ============================================================================
// BATCH OPERATIONS
// ============================================================================

// CheckMultiple checks rate limits for multiple identities at once
// Returns a map of identity -> result
func CheckMultiple(ctx context.Context, limiter RateLimiter, identities []string, scope, tier string) map[string]*Result {
	results := make(map[string]*Result, len(identities))

	for _, identity := range identities {
		rlCtx := NewSimpleContext(identity, scope, tier, nil)
		result, err := limiter.Allow(ctx, rlCtx)
		if err != nil {
			// Create a denied result on error
			result = &Result{
				Allowed: false,
				Entity:  identity,
			}
		}
		results[identity] = result
	}

	return results
}

// ResetMultiple resets rate limits for multiple identities
func ResetMultiple(ctx context.Context, limiter RateLimiter, identities []string, scope, tier string) error {
	for _, identity := range identities {
		rlCtx := NewSimpleContext(identity, scope, tier, nil)
		if err := limiter.Reset(ctx, rlCtx); err != nil {
			return WrapConfigError(err, "failed to reset identity", "identity", identity)
		}
	}
	return nil
}

// ============================================================================
// ERROR HELPERS
// ============================================================================

// IsStorageError checks if an error is a storage-related error
func IsStorageError(err error) bool {
	if err == nil {
		return false
	}
	return err == ErrKeyNotFound || err == ErrConnectionFailed
}
