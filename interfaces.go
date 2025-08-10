// interfaces.go
package ratelimit

import (
	"context"
	"time"
)

// Algorithm represents a rate limiting algorithm
type Algorithm interface {
	// Name returns the algorithm name
	Name() string

	// Allow checks if a request is allowed and returns the result
	Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error)

	// Reset resets the rate limit for the given key
	Reset(ctx context.Context, store Store, key string) error
}

// Store represents a storage backend for rate limiting data
type Store interface {
	// Get retrieves a value from the store
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value in the store with an optional expiration
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error

	// Increment atomically increments a counter and returns the new value
	Increment(ctx context.Context, key string, expiration time.Duration) (int64, error)

	// IncrementBy atomically increments a counter by the given amount
	IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error)

	// Delete removes a key from the store
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists in the store
	Exists(ctx context.Context, key string) (bool, error)

	// Health checks the health of the store connection
	Health(ctx context.Context) error

	// Close closes the store connection
	Close() error
}
