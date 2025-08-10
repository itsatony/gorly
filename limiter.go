// limiter.go
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/itsatony/gorly/algorithms"
	"github.com/itsatony/gorly/stores"
)

// rateLimiter is the main implementation of the RateLimiter interface
type rateLimiter struct {
	config     *Config
	store      Store
	algorithm  Algorithm
	keyBuilder *KeyBuilder
	metrics    *Metrics
	mu         sync.RWMutex
	closed     bool
}

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config *Config) (RateLimiter, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, NewRateLimitError(ErrorTypeConfig, "invalid configuration", err)
	}

	// Create store
	store, err := createStore(config)
	if err != nil {
		return nil, NewRateLimitError(ErrorTypeStore, "failed to create store", err)
	}

	// Create algorithm
	algorithm, err := createAlgorithm(config.Algorithm)
	if err != nil {
		store.Close() // Clean up store on error
		return nil, NewRateLimitError(ErrorTypeAlgorithm, "failed to create algorithm", err)
	}

	// Create key builder
	keyBuilder := NewKeyBuilder(config.KeyPrefix)

	// Create metrics if enabled
	var metrics *Metrics
	if config.EnableMetrics {
		metrics = NewMetrics(config.MetricsPrefix)
	}

	limiter := &rateLimiter{
		config:     config,
		store:      store,
		algorithm:  algorithm,
		keyBuilder: keyBuilder,
		metrics:    metrics,
	}

	return limiter, nil
}

// Allow checks if a request is allowed for the given entity and scope
func (rl *rateLimiter) Allow(ctx context.Context, entity AuthEntity, scope string) (*Result, error) {
	return rl.AllowN(ctx, entity, scope, 1)
}

// AllowN checks if N requests are allowed for the given entity and scope
func (rl *rateLimiter) AllowN(ctx context.Context, entity AuthEntity, scope string, n int64) (*Result, error) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return nil, NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	if !rl.config.Enabled {
		// If rate limiting is disabled, allow all requests
		return &Result{
			Allowed:   true,
			Remaining: 1000000, // Large number to indicate unlimited
			Limit:     1000000,
			Algorithm: rl.algorithm.Name(),
		}, nil
	}

	// Get rate limit configuration for this entity and scope
	rateLimit := rl.config.GetRateLimit(entity, scope)

	// Use full limit as capacity for token bucket
	// Burst size in token bucket is the initial capacity, but the algorithm
	// expects the full limit as the capacity parameter
	capacity := rateLimit.Requests

	// Build key for this entity and scope
	key := rl.keyBuilder.BuildKey(entity, scope)

	// Record metrics start time
	startTime := time.Now()

	// Call the algorithm directly using our interface
	result, err := rl.algorithm.Allow(ctx, rl.store, key, capacity, rateLimit.Window, n)
	if err != nil {
		// Record error metrics
		if rl.metrics != nil {
			rl.metrics.RecordError(entity.Type(), scope, err)
		}
		return nil, err
	}

	// Update result with configuration info
	result.Limit = rateLimit.Requests
	result.Window = rateLimit.Window

	// Record metrics
	if rl.metrics != nil {
		duration := time.Since(startTime)
		rl.metrics.RecordRequest(entity.Type(), entity.Tier(), scope, result.Allowed, duration)

		if !result.Allowed {
			rl.metrics.RecordRateLimit(entity.Type(), entity.Tier(), scope)
		}
	}

	return result, nil
}

// Reset resets the rate limit for the given entity and scope
func (rl *rateLimiter) Reset(ctx context.Context, entity AuthEntity, scope string) error {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	key := rl.keyBuilder.BuildKey(entity, scope)

	// Use our algorithm interface directly
	return rl.algorithm.Reset(ctx, rl.store, key)
}

// Stats returns usage statistics for the given entity
func (rl *rateLimiter) Stats(ctx context.Context, entity AuthEntity) (*Stats, error) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return nil, NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	// This is a simplified implementation
	// In a full implementation, you would gather statistics from multiple scopes
	stats := &Stats{
		Entity:          entity,
		Scopes:          make(map[string]ScopeStats),
		TotalRequests:   0,
		TotalDenied:     0,
		RateLimitHits:   0,
		RateLimitMisses: 0,
	}

	// Get statistics for common scopes
	commonScopes := []string{ScopeGlobal, ScopeMemory, ScopeSearch, ScopeMetadata}

	for _, scope := range commonScopes {
		scopeStats, err := rl.ScopeStats(ctx, entity, scope)
		if err != nil {
			continue // Skip scopes with errors
		}

		if scopeStats != nil {
			stats.Scopes[scope] = *scopeStats
			stats.TotalRequests += scopeStats.RequestCount
			stats.TotalDenied += scopeStats.DeniedCount
		}
	}

	return stats, nil
}

// ScopeStats returns statistics for a specific entity and scope
func (rl *rateLimiter) ScopeStats(ctx context.Context, entity AuthEntity, scope string) (*ScopeStats, error) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return nil, NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	rateLimit := rl.config.GetRateLimit(entity, scope)
	key := rl.keyBuilder.BuildKey(entity, scope)

	// Get algorithm-specific information if supported
	if tbWrapper, ok := rl.algorithm.(*tokenBucketWrapper); ok {
		// Use full limit as capacity (same as in Allow method)
		capacity := rateLimit.Requests

		info, err := tbWrapper.GetBucketInfo(ctx, rl.store, key, capacity, rateLimit.Window)
		if err != nil {
			return nil, err
		}

		return &ScopeStats{
			Scope:        scope,
			RequestCount: info["total_requests"].(int64),
			DeniedCount:  info["denied_requests"].(int64),
			CurrentUsage: rateLimit.Requests - int64(info["current_tokens"].(float64)),
			Limit:        rateLimit.Requests,
			Window:       rateLimit.Window,
			Algorithm:    rl.algorithm.Name(),
		}, nil
	}

	// Handle sliding window algorithm
	if swWrapper, ok := rl.algorithm.(*slidingWindowWrapper); ok {
		info, err := swWrapper.GetWindowInfo(ctx, rl.store, key, rateLimit.Requests, rateLimit.Window)
		if err != nil {
			return nil, err
		}

		return &ScopeStats{
			Scope:        scope,
			RequestCount: info["total_requests"].(int64),
			DeniedCount:  info["denied_requests"].(int64),
			CurrentUsage: int64(info["current_requests"].(int)),
			Limit:        rateLimit.Requests,
			Window:       rateLimit.Window,
			Algorithm:    rl.algorithm.Name(),
		}, nil
	}

	// Fallback for other algorithms
	return &ScopeStats{
		Scope:     scope,
		Limit:     rateLimit.Requests,
		Window:    rateLimit.Window,
		Algorithm: rl.algorithm.Name(),
	}, nil
}

// Health checks the health of the rate limiter
func (rl *rateLimiter) Health(ctx context.Context) error {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if rl.closed {
		return NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	return rl.store.Health(ctx)
}

// Close cleans up resources used by the rate limiter
func (rl *rateLimiter) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.closed {
		return nil
	}

	rl.closed = true

	var err error
	if rl.store != nil {
		err = rl.store.Close()
	}

	return err
}

// createStore creates a store based on the configuration
func createStore(config *Config) (Store, error) {
	switch config.Store {
	case "redis":
		// Convert to stores.RedisConfig
		redisConfig := stores.RedisConfig{
			Address:     config.Redis.Address,
			Password:    config.Redis.Password,
			Database:    config.Redis.Database,
			PoolSize:    config.Redis.PoolSize,
			MinIdleConn: config.Redis.MinIdleConn,
			MaxRetries:  config.Redis.MaxRetries,
			Timeout:     config.Redis.Timeout,
			TLS:         config.Redis.TLS,
		}
		return stores.NewRedisStore(redisConfig)
	case "memory":
		// Convert to stores.MemoryConfig with defaults
		memoryConfig := stores.MemoryConfig{
			MaxKeys:         1000000,         // 1M keys default
			CleanupInterval: 5 * time.Minute, // Cleanup every 5 minutes
			DefaultTTL:      time.Hour,       // 1 hour default TTL
		}
		return stores.NewMemoryStore(memoryConfig)
	default:
		return nil, fmt.Errorf("unknown store type: %s", config.Store)
	}
}

// createAlgorithm creates an algorithm based on the configuration
func createAlgorithm(algorithmName string) (Algorithm, error) {
	switch algorithmName {
	case "token_bucket":
		// Create a wrapper for the token bucket algorithm
		return &tokenBucketWrapper{
			algorithm: algorithms.NewTokenBucketAlgorithm(),
		}, nil
	case "sliding_window":
		// Create a wrapper for the sliding window algorithm
		return &slidingWindowWrapper{
			algorithm: algorithms.NewSlidingWindowAlgorithm(),
		}, nil
	case "gcra":
		// TODO: Implement GCRA algorithm
		return nil, fmt.Errorf("GCRA algorithm not implemented yet")
	default:
		return nil, fmt.Errorf("unknown algorithm: %s", algorithmName)
	}
}

// GetConfig returns the current configuration (read-only copy)
func (rl *rateLimiter) GetConfig() *Config {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *rl.config
	return &configCopy
}

// UpdateConfig updates the rate limiter configuration
// Note: This only updates certain safe-to-change settings
func (rl *rateLimiter) UpdateConfig(newConfig *Config) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.closed {
		return NewRateLimitError(ErrorTypeConfig, "rate limiter is closed", nil)
	}

	// Validate new configuration
	if err := newConfig.Validate(); err != nil {
		return NewRateLimitError(ErrorTypeConfig, "invalid new configuration", err)
	}

	// Only allow updating certain fields to avoid breaking existing connections
	rl.config.Enabled = newConfig.Enabled
	rl.config.DefaultLimits = newConfig.DefaultLimits
	rl.config.ScopeLimits = newConfig.ScopeLimits
	rl.config.TierLimits = newConfig.TierLimits
	rl.config.EntityOverrides = newConfig.EntityOverrides
	rl.config.MaxConcurrentRequests = newConfig.MaxConcurrentRequests
	rl.config.OperationTimeout = newConfig.OperationTimeout

	return nil
}

// GetMetrics returns the metrics collector (if enabled)
func (rl *rateLimiter) GetMetrics() *Metrics {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return rl.metrics
}

// storeAdapter adapts our Store interface to the algorithms.Store interface
type storeAdapter struct {
	store Store
}

func (sa *storeAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return sa.store.Get(ctx, key)
}

func (sa *storeAdapter) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return sa.store.Set(ctx, key, value, expiration)
}

func (sa *storeAdapter) Delete(ctx context.Context, key string) error {
	return sa.store.Delete(ctx, key)
}

// tokenBucketWrapper wraps the algorithms.TokenBucketAlgorithm to match our Algorithm interface
type tokenBucketWrapper struct {
	algorithm *algorithms.TokenBucketAlgorithm
}

func (tbw *tokenBucketWrapper) Name() string {
	return tbw.algorithm.Name()
}

func (tbw *tokenBucketWrapper) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error) {
	// Convert store to algorithm store interface
	storeAdapter := &storeAdapter{store: store}

	// Call the underlying algorithm
	algorithmResult, err := tbw.algorithm.Allow(ctx, storeAdapter, key, limit, window, n)
	if err != nil {
		return nil, err
	}

	// Convert result from algorithm result to our result type
	return &Result{
		Allowed:    algorithmResult.Allowed,
		Remaining:  algorithmResult.Remaining,
		RetryAfter: algorithmResult.RetryAfter,
		ResetTime:  algorithmResult.ResetTime,
		Limit:      algorithmResult.Limit,
		Window:     algorithmResult.Window,
		Used:       algorithmResult.Used,
		Algorithm:  algorithmResult.Algorithm,
	}, nil
}

func (tbw *tokenBucketWrapper) Reset(ctx context.Context, store Store, key string) error {
	storeAdapter := &storeAdapter{store: store}
	return tbw.algorithm.Reset(ctx, storeAdapter, key)
}

func (tbw *tokenBucketWrapper) GetBucketInfo(ctx context.Context, store Store, key string, capacity int64, window time.Duration) (map[string]interface{}, error) {
	storeAdapter := &storeAdapter{store: store}
	return tbw.algorithm.GetBucketInfo(ctx, storeAdapter, key, capacity, window)
}

// slidingWindowWrapper wraps the algorithms.SlidingWindowAlgorithm to match our Algorithm interface
type slidingWindowWrapper struct {
	algorithm *algorithms.SlidingWindowAlgorithm
}

func (sww *slidingWindowWrapper) Name() string {
	return sww.algorithm.Name()
}

func (sww *slidingWindowWrapper) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error) {
	// Convert store to algorithm store interface
	storeAdapter := &storeAdapter{store: store}

	// Call the underlying algorithm
	algorithmResult, err := sww.algorithm.Allow(ctx, storeAdapter, key, limit, window, n)
	if err != nil {
		return nil, err
	}

	// Convert result from algorithm result to our result type
	return &Result{
		Allowed:    algorithmResult.Allowed,
		Remaining:  algorithmResult.Remaining,
		RetryAfter: algorithmResult.RetryAfter,
		ResetTime:  algorithmResult.ResetTime,
		Limit:      algorithmResult.Limit,
		Window:     algorithmResult.Window,
		Used:       algorithmResult.Used,
		Algorithm:  algorithmResult.Algorithm,
	}, nil
}

func (sww *slidingWindowWrapper) Reset(ctx context.Context, store Store, key string) error {
	storeAdapter := &storeAdapter{store: store}
	return sww.algorithm.Reset(ctx, storeAdapter, key)
}

func (sww *slidingWindowWrapper) GetWindowInfo(ctx context.Context, store Store, key string, limit int64, window time.Duration) (map[string]interface{}, error) {
	storeAdapter := &storeAdapter{store: store}
	return sww.algorithm.GetWindowInfo(ctx, storeAdapter, key, limit, window)
}
