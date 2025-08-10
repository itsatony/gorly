// internal/core/limiter.go
package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/itsatony/gorly/algorithms"
	"github.com/itsatony/gorly/stores"
)

// storeAdapter adapts concrete store implementations to our Store interface
type storeAdapter struct {
	store interface {
		Get(ctx context.Context, key string) ([]byte, error)
		Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
		IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error)
		Delete(ctx context.Context, key string) error
		Exists(ctx context.Context, key string) (bool, error)
		Health(ctx context.Context) error
		Close() error
	}
}

func (s *storeAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return s.store.Get(ctx, key)
}

func (s *storeAdapter) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return s.store.Set(ctx, key, value, expiration)
}

func (s *storeAdapter) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	return s.store.IncrementBy(ctx, key, amount, expiration)
}

func (s *storeAdapter) Delete(ctx context.Context, key string) error {
	return s.store.Delete(ctx, key)
}

func (s *storeAdapter) Exists(ctx context.Context, key string) (bool, error) {
	return s.store.Exists(ctx, key)
}

func (s *storeAdapter) Health(ctx context.Context) error {
	return s.store.Health(ctx)
}

func (s *storeAdapter) Close() error {
	return s.store.Close()
}

// algorithmStoreAdapter adapts our Store interface to match the algorithms.Store interface
type algorithmStoreAdapter struct {
	store Store
}

func (s *algorithmStoreAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	return s.store.Get(ctx, key)
}

func (s *algorithmStoreAdapter) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return s.store.Set(ctx, key, value, expiration)
}

func (s *algorithmStoreAdapter) Delete(ctx context.Context, key string) error {
	return s.store.Delete(ctx, key)
}

// algorithmAdapter adapts concrete algorithm implementations to our Algorithm interface
type algorithmAdapter struct {
	algorithm interface {
		Name() string
		Allow(ctx context.Context, store algorithms.Store, key string, limit int64, window time.Duration, n int64) (*algorithms.Result, error)
		Reset(ctx context.Context, store algorithms.Store, key string) error
	}
}

func (a *algorithmAdapter) Name() string {
	return a.algorithm.Name()
}

func (a *algorithmAdapter) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*AlgorithmResult, error) {
	// Create an adapter to match the algorithms.Store interface
	algStore := &algorithmStoreAdapter{store}

	result, err := a.algorithm.Allow(ctx, algStore, key, limit, window, n)
	if err != nil {
		return nil, err
	}

	return &AlgorithmResult{
		Allowed:    result.Allowed,
		Remaining:  result.Remaining,
		Limit:      result.Limit,
		Used:       result.Used,
		RetryAfter: result.RetryAfter,
		Window:     result.Window,
		ResetTime:  result.ResetTime,
	}, nil
}

func (a *algorithmAdapter) Reset(ctx context.Context, store Store, key string) error {
	algStore := &algorithmStoreAdapter{store}
	return a.algorithm.Reset(ctx, algStore, key)
}

// Limiter is the internal interface for rate limiting
type Limiter interface {
	Check(ctx context.Context, entity, scope string) (*CoreResult, error)
	Health(ctx context.Context) error
	Close() error
}

// Store represents a storage backend for rate limiting data
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
	IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Health(ctx context.Context) error
	Close() error
}

// Algorithm represents a rate limiting algorithm
type Algorithm interface {
	Name() string
	Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*AlgorithmResult, error)
	Reset(ctx context.Context, store Store, key string) error
}

// AlgorithmResult is the result from an algorithm
type AlgorithmResult struct {
	Allowed    bool
	Remaining  int64
	Limit      int64
	Used       int64
	RetryAfter time.Duration
	Window     time.Duration
	ResetTime  time.Time
}

// limiterImpl implements the Limiter interface
type limiterImpl struct {
	config    *Config
	store     Store
	algorithm Algorithm
}

// NewLimiter creates a new core rate limiter
func NewLimiter(config *Config) (Limiter, error) {
	// Create store
	var store Store

	switch config.Store {
	case "memory":
		memConfig := stores.MemoryConfig{
			CleanupInterval: 10 * time.Minute,
		}
		memStore, err := stores.NewMemoryStore(memConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create memory store: %w", err)
		}
		store = &storeAdapter{memStore}
	case "redis":
		redisConfig := stores.RedisConfig{
			Address:  config.RedisAddress,
			Password: config.RedisPassword,
			Database: config.RedisDB,
			PoolSize: config.RedisPoolSize,
		}
		if redisConfig.PoolSize == 0 {
			redisConfig.PoolSize = 10 // Default pool size
		}
		redisStore, err := stores.NewRedisStore(redisConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create redis store: %w", err)
		}
		store = &storeAdapter{redisStore}
	default:
		return nil, fmt.Errorf("unsupported store: %s", config.Store)
	}

	// Create algorithm
	var algorithm Algorithm
	switch config.Algorithm {
	case "token_bucket":
		algorithm = &algorithmAdapter{algorithms.NewTokenBucketAlgorithm()}
	case "sliding_window":
		algorithm = &algorithmAdapter{algorithms.NewSlidingWindowAlgorithm()}
	case "gcra":
		// TODO: Implement GCRA algorithm
		algorithm = &algorithmAdapter{algorithms.NewSlidingWindowAlgorithm()} // Fallback for now
	default:
		return nil, fmt.Errorf("unsupported algorithm: %s", config.Algorithm)
	}

	return &limiterImpl{
		config:    config,
		store:     store,
		algorithm: algorithm,
	}, nil
}

// Check performs a rate limit check
func (l *limiterImpl) Check(ctx context.Context, entity, scope string) (*CoreResult, error) {
	// Determine the limit for this entity and scope
	limit, window, err := l.getLimit(entity, scope)
	if err != nil {
		return nil, fmt.Errorf("failed to get limit: %w", err)
	}

	// Build the key for this entity and scope
	key := fmt.Sprintf("ratelimit:%s:%s", entity, scope)

	// Check the rate limit using the algorithm
	algResult, err := l.algorithm.Allow(ctx, l.store, key, limit, window, 1)
	if err != nil {
		return nil, fmt.Errorf("rate limit check failed: %w", err)
	}

	// Convert from AlgorithmResult to CoreResult
	return &CoreResult{
		Allowed:    algResult.Allowed,
		Remaining:  algResult.Remaining,
		Limit:      algResult.Limit,
		Used:       algResult.Used,
		RetryAfter: algResult.RetryAfter,
		Window:     algResult.Window,
		ResetTime:  algResult.ResetTime,
	}, nil
}

// getLimit determines the rate limit for an entity and scope
func (l *limiterImpl) getLimit(entity, scope string) (int64, time.Duration, error) {
	// First check for tier-based limits if available
	if tierLimits, ok := l.config.TierLimits[scope]; ok {
		// Extract tier from entity (assumes format "tier:entity" or just "tier")
		tier := "free" // default tier
		if strings.Contains(entity, ":") {
			parts := strings.SplitN(entity, ":", 2)
			if len(parts) == 2 {
				tier = parts[0]
			}
		}

		if limitStr, ok := tierLimits[tier]; ok {
			return parseLimit(limitStr)
		}
	}

	// Fall back to scope-based limits
	if limitStr, ok := l.config.Limits[scope]; ok {
		return parseLimit(limitStr)
	}

	// Fall back to global limit
	if limitStr, ok := l.config.Limits["global"]; ok {
		return parseLimit(limitStr)
	}

	return 0, 0, fmt.Errorf("no limit configured for scope: %s", scope)
}

// parseLimit parses a limit string like "100/hour" into requests and duration
func parseLimit(limitStr string) (int64, time.Duration, error) {
	parts := strings.Split(limitStr, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid limit format: %s (expected 'requests/duration')", limitStr)
	}

	requests, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid request count: %s", parts[0])
	}

	var duration time.Duration
	switch parts[1] {
	case "second", "s":
		duration = time.Second
	case "minute", "min", "m":
		duration = time.Minute
	case "hour", "h":
		duration = time.Hour
	case "day", "d":
		duration = 24 * time.Hour
	default:
		// Try to parse as Go duration string
		duration, err = time.ParseDuration(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid duration: %s", parts[1])
		}
	}

	return requests, duration, nil
}

// Health checks if the limiter is healthy
func (l *limiterImpl) Health(ctx context.Context) error {
	return l.store.Health(ctx)
}

// Close cleans up resources
func (l *limiterImpl) Close() error {
	return l.store.Close()
}
