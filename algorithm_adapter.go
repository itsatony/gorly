package ratelimit

import (
	"context"
	"time"

	"github.com/itsatony/gorly/algorithms"
)

// ============================================================================
// ALGORITHM ADAPTERS - Bridge between algorithms package and main package
// ============================================================================

// algorithmAdapter wraps an algorithms package algorithm to implement our Algorithm interface
type algorithmAdapter struct {
	name    string
	wrapped interface{} // Can be *algorithms.TokenBucketAlgorithm or *algorithms.SlidingWindowAlgorithm
}

// ============================================================================
// TOKEN BUCKET ADAPTER
// ============================================================================

// NewTokenBucketAlgorithm creates a token bucket algorithm
func NewTokenBucketAlgorithm() Algorithm {
	return &algorithmAdapter{
		name:    StrategyTokenBucket,
		wrapped: algorithms.NewTokenBucketAlgorithm(),
	}
}

// ============================================================================
// SLIDING WINDOW ADAPTER
// ============================================================================

// NewSlidingWindowAlgorithm creates a sliding window algorithm
func NewSlidingWindowAlgorithm() Algorithm {
	return &algorithmAdapter{
		name:    StrategySlidingWindow,
		wrapped: algorithms.NewSlidingWindowAlgorithm(),
	}
}

// ============================================================================
// ADAPTER IMPLEMENTATION
// ============================================================================

// Name returns the algorithm name
func (a *algorithmAdapter) Name() string {
	return a.name
}

// Allow checks if a request is allowed and returns the result
func (a *algorithmAdapter) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error) {
	// Create a store adapter that wraps our Store interface
	storeAdapter := &storeAdapter{store: store}

	var algResult *algorithms.Result
	var err error

	// Call the wrapped algorithm
	switch algo := a.wrapped.(type) {
	case *algorithms.TokenBucketAlgorithm:
		algResult, err = algo.Allow(ctx, storeAdapter, key, limit, window, n)
	case *algorithms.SlidingWindowAlgorithm:
		algResult, err = algo.Allow(ctx, storeAdapter, key, limit, window, n)
	default:
		return nil, WrapStrategyError(nil, "unknown algorithm type")
	}

	if err != nil {
		return nil, WrapStrategyError(err, "algorithm allow failed")
	}

	// Convert algorithms.Result to our Result
	result := convertAlgorithmResult(algResult, window)
	return result, nil
}

// Reset resets the rate limit for the given key
func (a *algorithmAdapter) Reset(ctx context.Context, store Store, key string) error {
	// Create a store adapter
	storeAdapter := &storeAdapter{store: store}

	var err error

	// Call the wrapped algorithm's reset
	switch algo := a.wrapped.(type) {
	case *algorithms.TokenBucketAlgorithm:
		err = algo.Reset(ctx, storeAdapter, key)
	case *algorithms.SlidingWindowAlgorithm:
		err = algo.Reset(ctx, storeAdapter, key)
	default:
		return WrapStrategyError(nil, "unknown algorithm type")
	}

	if err != nil {
		return WrapStrategyError(err, "algorithm reset failed")
	}

	return nil
}

// ============================================================================
// STORE ADAPTER - Adapts our Store interface to algorithms.Store
// ============================================================================

// storeAdapter wraps our Store interface to implement algorithms.Store
type storeAdapter struct {
	store Store
}

// Get retrieves a value from the store
func (sa *storeAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return sa.store.Get(ctx, key)
}

// Set stores a value in the store with an optional expiration
func (sa *storeAdapter) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return sa.store.Set(ctx, key, value, expiration)
}

// Delete removes a key from the store
func (sa *storeAdapter) Delete(ctx context.Context, key string) error {
	return sa.store.Delete(ctx, key)
}

// ExecuteScript executes a Lua script atomically in the store
func (sa *storeAdapter) ExecuteScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return sa.store.ExecuteScript(ctx, script, keys, args...)
}

// ============================================================================
// RESULT CONVERTER
// ============================================================================

// convertAlgorithmResult converts an algorithms.Result to our Result type
func convertAlgorithmResult(algResult *algorithms.Result, window time.Duration) *Result {
	result := &Result{
		Allowed:    algResult.Allowed,
		Limit:      algResult.Limit,
		Remaining:  algResult.Remaining,
		Used:       algResult.Used,
		RetryAfter: algResult.RetryAfter,
		ResetAt:    algResult.ResetTime,
		Window:     window,
		metadata:   make(map[string]interface{}),
	}

	return result
}

// ============================================================================
// FIXED WINDOW ALGORITHM (Placeholder for future implementation)
// ============================================================================

// NewFixedWindowAlgorithm creates a fixed window algorithm
// This is a placeholder - not yet implemented
func NewFixedWindowAlgorithm() Algorithm {
	// For now, use token bucket as a fallback
	// TODO: Implement actual fixed window algorithm
	return NewTokenBucketAlgorithm()
}

// ============================================================================
// LEAKY BUCKET ALGORITHM (Placeholder for future implementation)
// ============================================================================

// NewLeakyBucketAlgorithm creates a leaky bucket algorithm
// This is a placeholder - not yet implemented
func NewLeakyBucketAlgorithm() Algorithm {
	// For now, use token bucket as a fallback
	// TODO: Implement actual leaky bucket algorithm
	return NewTokenBucketAlgorithm()
}
