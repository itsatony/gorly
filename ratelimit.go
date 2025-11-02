// Package ratelimit provides production-ready rate limiting for Go applications.
//
// Gorly is designed for simplicity and performance, with a clean API that scales
// from prototype to production without changes. It supports multiple storage backends
// (in-memory for development, Redis for production), flexible identity extraction
// (IP-based, API key-based, user-based), and comprehensive HTTP middleware.
//
// # Quick Start
//
// Get started with a one-liner:
//
//	import "github.com/itsatony/gorly/middleware"
//	http.Handle("/api/", middleware.QuickLimit(100, time.Minute, yourHandler))
//
// Or build a custom limiter in 3 lines:
//
//	store, _ := stores.NewMemoryStore(nil)
//	limiter, _ := ratelimit.NewSimple(store, 100, time.Hour)
//	result, _ := limiter.Allow(ctx, ratelimit.NewIPContext("192.168.1.1"))
//
// # Core Concepts
//
// Identity: Who/what is being rate limited (IP address, user ID, API key, etc.)
//
// Scope: What operation is being rate limited (global, search, upload, etc.)
//
// Tier: The user's subscription level (free, premium, enterprise)
//
// Store: Where rate limit state is persisted (memory or Redis)
//
// # Production Features
//
// - Thread-safe with race detector testing
// - Proper error handling (distinguish errors from rate limits)
// - Redis-backed for distributed systems
// - Configurable algorithms (token bucket, sliding window)
// - Multi-tier support (different limits per subscription level)
// - Scope-based limits (different limits per operation type)
//
// # Architecture
//
// Gorly follows a clean architecture with clear separation:
//
//	RateLimiter (interface) -> Algorithm -> Store
//	     ↓
//	  Identity (who is making the request)
//	     ↓
//	  Result (allowed/denied + metadata)
//
// See the README for detailed examples and recipes.
package ratelimit

import (
	"context"
	"time"
)

// ============================================================================
// CORE RATE LIMITER INTERFACE
// ============================================================================

// RateLimiter is the core interface for rate limiting functionality
// This is the main entry point for all rate limiting operations
type RateLimiter interface {
	// Check performs a rate limit check WITHOUT consuming tokens
	// Useful for preflight checks or monitoring
	Check(ctx context.Context, rlCtx Identity) (*Result, error)

	// Allow performs a rate limit check and CONSUMES one token if allowed
	// This is the main method for enforcing rate limits
	Allow(ctx context.Context, rlCtx Identity) (*Result, error)

	// AllowN performs a rate limit check and consumes N tokens if allowed
	// Useful for batch operations or operations with different costs
	AllowN(ctx context.Context, rlCtx Identity, n int64) (*Result, error)

	// Reset clears the rate limit for the given context
	// Useful for administrative overrides or testing
	Reset(ctx context.Context, rlCtx Identity) error

	// Stats returns usage statistics for the given context
	// Provides visibility into current rate limit state
	Stats(ctx context.Context, rlCtx Identity) (*Result, error)

	// Health checks the health of the rate limiter
	// Verifies store connectivity and system health
	Health(ctx context.Context) error

	// Close cleanly shuts down the rate limiter
	// Releases resources, closes connections, stops goroutines
	Close() error
}

// ============================================================================
// HEALTH CHECK RESULT
// ============================================================================

// HealthStatus represents the health status of the rate limiter
type HealthStatus struct {
	// Overall health
	Healthy bool `json:"healthy"`

	// Component health
	StoreHealthy    bool `json:"store_healthy"`
	StrategyHealthy bool `json:"strategy_healthy"`

	// Details
	Message   string                 `json:"message,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`

	// Performance
	ResponseTime time.Duration `json:"response_time,omitempty"`
}

// NewHealthStatus creates a new health status
func NewHealthStatus(healthy bool, message string) *HealthStatus {
	return &HealthStatus{
		Healthy:   healthy,
		Message:   message,
		Timestamp: time.Now(),
		Metadata:  make(map[string]interface{}),
	}
}
