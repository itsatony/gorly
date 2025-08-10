// internal/core/config.go
package core

import (
	"errors"
	"net/http"
	"time"
)

// Config holds the configuration for a rate limiter
type Config struct {
	// Store configuration
	Store     string // "memory" or "redis"
	Algorithm string // "token_bucket", "sliding_window", "gcra"

	// Redis configuration
	RedisAddress  string
	RedisPassword string
	RedisDB       int
	RedisPoolSize int

	// Rate limits
	Limits     map[string]string            // scope -> limit (e.g., "global" -> "1000/hour")
	TierLimits map[string]map[string]string // scope -> tier -> limit

	// Extractor functions
	ExtractorFunc func(*http.Request) string // Extract entity from request
	ScopeFunc     func(*http.Request) string // Extract scope from request

	// Event handlers
	ErrorHandler  func(error)                                           // Handle errors
	DeniedHandler func(http.ResponseWriter, *http.Request, *CoreResult) // Handle denied requests

	// Features
	MetricsEnabled bool
}

// CoreResult represents the result of a rate limit check
type CoreResult struct {
	Allowed    bool
	Remaining  int64
	Limit      int64
	Used       int64
	RetryAfter time.Duration
	Window     time.Duration
	ResetTime  time.Time
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Store != "memory" && c.Store != "redis" {
		return errors.New("store must be 'memory' or 'redis'")
	}

	if c.Store == "redis" && c.RedisAddress == "" {
		return errors.New("redis address is required when using redis store")
	}

	if c.Algorithm != "token_bucket" && c.Algorithm != "sliding_window" && c.Algorithm != "gcra" {
		return errors.New("algorithm must be 'token_bucket', 'sliding_window', or 'gcra'")
	}

	if len(c.Limits) == 0 && len(c.TierLimits) == 0 {
		return errors.New("at least one rate limit must be configured")
	}

	if c.ExtractorFunc == nil {
		return errors.New("extractor function is required")
	}

	return nil
}
