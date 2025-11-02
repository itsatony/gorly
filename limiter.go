package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// CORE RATE LIMITER IMPLEMENTATION
// ============================================================================

// limiter implements the RateLimiter interface
type limiter struct {
	store     Store
	algorithm Algorithm
	config    *Config
	resolver  LimitResolver
	mu        sync.RWMutex
	closed    bool
	logger    Logger
	id        string
}

// ============================================================================
// CONFIGURATION
// ============================================================================

// Config holds rate limiter configuration
type Config struct {
	// Store is the backend storage for rate limit data
	Store Store

	// Algorithm is the rate limiting algorithm to use
	Algorithm Algorithm

	// DefaultLimit is the default rate limit (requests per window)
	DefaultLimit int64

	// DefaultWindow is the default time window
	DefaultWindow time.Duration

	// DefaultBurst is the default burst size for token bucket
	DefaultBurst int64

	// Resolver is the optional limit resolver for dynamic limits
	// If set, it will override DefaultLimit/DefaultWindow/DefaultBurst
	// based on tier and scope resolution
	Resolver LimitResolver

	// Logger for rate limiter operations
	Logger Logger

	// EnableMetrics enables metrics collection
	EnableMetrics bool
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		DefaultLimit:  DefaultLimit,
		DefaultWindow: DefaultWindow,
		DefaultBurst:  DefaultBurst,
		Logger:        NewNopLogger(),
		EnableMetrics: false,
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Store == nil {
		return WrapConfigError(nil, "store is required", "store", nil)
	}

	if c.Algorithm == nil {
		return WrapConfigError(nil, "algorithm is required", "algorithm", nil)
	}

	if c.DefaultLimit < MinLimit {
		return WrapConfigError(nil, "default limit below minimum",
			"default_limit", c.DefaultLimit,
			"min_limit", MinLimit)
	}

	if c.DefaultLimit > MaxLimit {
		return WrapConfigError(nil, "default limit above maximum",
			"default_limit", c.DefaultLimit,
			"max_limit", MaxLimit)
	}

	if c.DefaultWindow < time.Duration(MinWindowSeconds)*time.Second {
		return WrapConfigError(nil, "default window below minimum",
			"default_window", c.DefaultWindow,
			"min_window", MinWindowSeconds)
	}

	if c.DefaultWindow > time.Duration(MaxWindowSeconds)*time.Second {
		return WrapConfigError(nil, "default window above maximum",
			"default_window", c.DefaultWindow,
			"max_window", MaxWindowSeconds)
	}

	if c.DefaultBurst < MinBurst {
		return WrapConfigError(nil, "default burst below minimum",
			"default_burst", c.DefaultBurst,
			"min_burst", MinBurst)
	}

	if c.DefaultBurst > MaxBurst {
		return WrapConfigError(nil, "default burst above maximum",
			"default_burst", c.DefaultBurst,
			"max_burst", MaxBurst)
	}

	if c.Logger == nil {
		c.Logger = NewNopLogger()
	}

	return nil
}

// ============================================================================
// CONSTRUCTOR
// ============================================================================

// NewRateLimiter creates a new rate limiter with the given configuration
func NewRateLimiter(config *Config) (RateLimiter, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	l := &limiter{
		store:     config.Store,
		algorithm: config.Algorithm,
		config:    config,
		resolver:  config.Resolver,
		logger:    config.Logger,
		id:        generateID(IDPrefixRateLimiter),
	}

	l.logger.Info("rate limiter created",
		"id", l.id,
		"algorithm", config.Algorithm.Name(),
		"default_limit", config.DefaultLimit,
		"default_window", config.DefaultWindow,
	)

	return l, nil
}

// ============================================================================
// RATELIMITER INTERFACE IMPLEMENTATION
// ============================================================================

// Check performs a rate limit check without consuming tokens
func (l *limiter) Check(ctx context.Context, rlCtx Identity) (*Result, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	if err := ValidateContext(rlCtx); err != nil {
		return nil, err
	}

	// For check-only, we just get current state without modifying it
	// Create a stats query which doesn't consume tokens
	return l.Stats(ctx, rlCtx)
}

// Allow performs a rate limit check and consumes one token if allowed
func (l *limiter) Allow(ctx context.Context, rlCtx Identity) (*Result, error) {
	return l.AllowN(ctx, rlCtx, 1)
}

// AllowN performs a rate limit check and consumes N tokens if allowed
func (l *limiter) AllowN(ctx context.Context, rlCtx Identity, n int64) (*Result, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	if err := ValidateContext(rlCtx); err != nil {
		return nil, err
	}

	if n < 0 {
		return nil, WrapConfigError(nil, "token count cannot be negative", "n", n)
	}

	if n == 0 {
		// No tokens requested, just return current state
		return l.Check(ctx, rlCtx)
	}

	key := l.buildKey(rlCtx)

	// Resolve limits dynamically if resolver is configured
	var limit int64
	var window time.Duration

	if l.resolver != nil {
		limitConfig, err := l.resolver.ResolveLimit(rlCtx)
		if err != nil {
			// Fall back to default limits if resolution fails
			l.logger.Warn("failed to resolve limit, using defaults",
				"id", l.id,
				"identity", rlCtx.Identity(),
				"scope", rlCtx.Scope(),
				"tier", rlCtx.Tier(),
				"error", err)
			limit = l.config.DefaultLimit
			window = l.config.DefaultWindow
		} else {
			limit = limitConfig.Limit
			window = limitConfig.Window
		}
	} else {
		// Use configured defaults
		limit = l.config.DefaultLimit
		window = l.config.DefaultWindow
	}

	// Call the algorithm
	result, err := l.algorithm.Allow(ctx, l.store, key, limit, window, n)
	if err != nil {
		return nil, WrapStrategyError(err, "rate limit check failed",
			"key", key,
			"identity", rlCtx.Identity(),
			"scope", rlCtx.Scope(),
			"n", n)
	}

	// Add context information to result
	result.WithContext(rlCtx.Scope(), rlCtx.Identity(), rlCtx.Tier())
	result.WithStrategy(l.algorithm.Name())
	result.Window = window

	// Log denial
	if !result.Allowed {
		l.logger.Debug("rate limit exceeded",
			"id", l.id,
			"identity", rlCtx.Identity(),
			"scope", rlCtx.Scope(),
			"tier", rlCtx.Tier(),
			"limit", limit,
			"window", window,
			"requested", n,
			"retry_after", result.RetryAfter,
		)
	}

	return result, nil
}

// Reset clears the rate limit for the given context
func (l *limiter) Reset(ctx context.Context, rlCtx Identity) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	if err := ValidateContext(rlCtx); err != nil {
		return err
	}

	key := l.buildKey(rlCtx)

	if err := l.algorithm.Reset(ctx, l.store, key); err != nil {
		return WrapStrategyError(err, "reset failed",
			"key", key,
			"identity", rlCtx.Identity(),
			"scope", rlCtx.Scope())
	}

	l.logger.Info("rate limit reset",
		"id", l.id,
		"identity", rlCtx.Identity(),
		"scope", rlCtx.Scope(),
		"tier", rlCtx.Tier(),
	)

	return nil
}

// Stats returns usage statistics for the given context
func (l *limiter) Stats(ctx context.Context, rlCtx Identity) (*Result, error) {
	if err := l.checkClosed(); err != nil {
		return nil, err
	}

	if err := ValidateContext(rlCtx); err != nil {
		return nil, err
	}

	// Get current state from store without modifying it
	key := l.buildKey(rlCtx)

	// Resolve limits dynamically if resolver is configured
	var limit int64
	var window time.Duration

	if l.resolver != nil {
		limitConfig, err := l.resolver.ResolveLimit(rlCtx)
		if err != nil {
			limit = l.config.DefaultLimit
			window = l.config.DefaultWindow
		} else {
			limit = limitConfig.Limit
			window = limitConfig.Window
		}
	} else {
		limit = l.config.DefaultLimit
		window = l.config.DefaultWindow
	}

	// Get current state by checking with 0 tokens (no modification)
	result, err := l.algorithm.Allow(ctx, l.store, key, limit, window, 0)
	if err != nil {
		// If we can't get stats, return empty result
		result = NewEmptyResult(limit, window)
	}

	// Add context information
	result.WithContext(rlCtx.Scope(), rlCtx.Identity(), rlCtx.Tier())
	result.WithStrategy(l.algorithm.Name())
	result.Window = window

	return result, nil
}

// Health checks the health of the rate limiter
func (l *limiter) Health(ctx context.Context) error {
	if err := l.checkClosed(); err != nil {
		return err
	}

	// Check store health
	if err := l.store.Health(ctx); err != nil {
		return WrapStorageError(err, "store health check failed")
	}

	return nil
}

// Close cleanly shuts down the rate limiter
func (l *limiter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true

	// Close the store
	if err := l.store.Close(); err != nil {
		l.logger.Warn("error closing store",
			"id", l.id,
			"error", err)
		return err
	}

	l.logger.Info("rate limiter closed", "id", l.id)
	return nil
}

// ============================================================================
// INTERNAL HELPERS
// ============================================================================

// buildKey constructs the storage key for a rate limit context
func (l *limiter) buildKey(rlCtx Identity) string {
	return rlCtx.Key()
}

// checkClosed checks if the limiter is closed
func (l *limiter) checkClosed() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return ErrClosed
	}
	return nil
}

// generateID generates a unique ID with the given prefix
func generateID(prefix string) string {
	// Simple implementation - in production might use UUID or similar
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}
