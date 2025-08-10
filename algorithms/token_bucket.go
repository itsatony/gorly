// algorithms/token_bucket.go
package algorithms

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// Result represents the result of a rate limit check
type Result struct {
	Allowed    bool          `json:"allowed"`
	Remaining  int64         `json:"remaining"`
	RetryAfter time.Duration `json:"retry_after"`
	ResetTime  time.Time     `json:"reset_time"`
	Limit      int64         `json:"limit"`
	Window     time.Duration `json:"window"`
	Used       int64         `json:"used"`
	Algorithm  string        `json:"algorithm"`
}

// Store interface for rate limiting storage
type Store interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
}

// RateLimitError represents an error in rate limiting operations
type RateLimitError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *RateLimitError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// NewRateLimitError creates a new RateLimitError
func NewRateLimitError(errorType, message string, err error) *RateLimitError {
	return &RateLimitError{
		Type:    errorType,
		Message: message,
		Err:     err,
	}
}

// TokenBucketAlgorithm implements the token bucket rate limiting algorithm
type TokenBucketAlgorithm struct {
	name string
}

// NewTokenBucketAlgorithm creates a new token bucket algorithm
func NewTokenBucketAlgorithm() *TokenBucketAlgorithm {
	return &TokenBucketAlgorithm{
		name: "token_bucket",
	}
}

// Name returns the algorithm name
func (tb *TokenBucketAlgorithm) Name() string {
	return tb.name
}

// TokenBucketState represents the current state of a token bucket
type TokenBucketState struct {
	// Current number of tokens in the bucket
	Tokens float64 `json:"tokens"`

	// Maximum number of tokens (burst capacity)
	Capacity int64 `json:"capacity"`

	// Rate at which tokens are added (tokens per second)
	RefillRate float64 `json:"refill_rate"`

	// Last time the bucket was updated
	LastRefill time.Time `json:"last_refill"`

	// Total requests processed
	TotalRequests int64 `json:"total_requests"`

	// Total requests denied
	DeniedRequests int64 `json:"denied_requests"`

	// Window duration for statistics
	WindowDuration time.Duration `json:"window_duration"`
}

// Allow checks if N requests are allowed and updates the bucket state
func (tb *TokenBucketAlgorithm) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error) {
	if n <= 0 {
		return &Result{
				Allowed:    false,
				Remaining:  0,
				RetryAfter: time.Second,
				Algorithm:  tb.name,
			}, NewRateLimitError(
				"config",
				"request count must be positive",
				nil,
			)
	}

	// Calculate refill rate (tokens per second)
	refillRate := float64(limit) / window.Seconds()

	// Get current bucket state
	state, err := tb.getBucketState(ctx, store, key, limit, refillRate, window)
	if err != nil {
		return nil, err
	}

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(state.LastRefill)
	if elapsed > 0 {
		tokensToAdd := refillRate * elapsed.Seconds()
		state.Tokens = math.Min(state.Tokens+tokensToAdd, float64(state.Capacity))
		state.LastRefill = now
	}

	// Check if we have enough tokens
	allowed := state.Tokens >= float64(n)
	remaining := int64(math.Floor(state.Tokens))

	var retryAfter time.Duration
	var resetTime time.Time

	if allowed {
		// Consume tokens
		state.Tokens -= float64(n)
		state.TotalRequests += n
		remaining = int64(math.Floor(state.Tokens))

		// Calculate when the bucket will be full again
		tokensNeeded := float64(state.Capacity) - state.Tokens
		if tokensNeeded > 0 {
			resetTime = now.Add(time.Duration(tokensNeeded/refillRate) * time.Second)
		} else {
			resetTime = now
		}
	} else {
		// Calculate retry after time
		tokensNeeded := float64(n) - state.Tokens
		retryAfter = time.Duration(tokensNeeded/refillRate) * time.Second
		resetTime = now.Add(retryAfter)
		state.DeniedRequests += n
		remaining = 0
	}

	// Save updated state
	if err := tb.saveBucketState(ctx, store, key, state, window); err != nil {
		return nil, err
	}

	return &Result{
		Allowed:    allowed,
		Remaining:  remaining,
		RetryAfter: retryAfter,
		ResetTime:  resetTime,
		Limit:      limit,
		Window:     window,
		Used:       limit - remaining,
		Algorithm:  tb.name,
	}, nil
}

// Reset resets the token bucket for the given key
func (tb *TokenBucketAlgorithm) Reset(ctx context.Context, store Store, key string) error {
	return store.Delete(ctx, key)
}

// getBucketState retrieves the current bucket state or creates a new one
func (tb *TokenBucketAlgorithm) getBucketState(ctx context.Context, store Store, key string, capacity int64, refillRate float64, window time.Duration) (*TokenBucketState, error) {
	data, err := store.Get(ctx, key)
	if err != nil {
		// If key doesn't exist, create new bucket with full tokens
		return &TokenBucketState{
			Tokens:         float64(capacity),
			Capacity:       capacity,
			RefillRate:     refillRate,
			LastRefill:     time.Now(),
			TotalRequests:  0,
			DeniedRequests: 0,
			WindowDuration: window,
		}, nil
	}

	var state TokenBucketState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, NewRateLimitError(
			"store",
			"failed to unmarshal bucket state",
			err,
		)
	}

	// Update configuration in case it changed
	state.Capacity = capacity
	state.RefillRate = refillRate
	state.WindowDuration = window

	return &state, nil
}

// saveBucketState saves the bucket state to the store
func (tb *TokenBucketAlgorithm) saveBucketState(ctx context.Context, store Store, key string, state *TokenBucketState, window time.Duration) error {
	data, err := json.Marshal(state)
	if err != nil {
		return NewRateLimitError(
			"algorithm",
			"failed to marshal bucket state",
			err,
		)
	}

	// Set expiration to 2x the window to account for burst scenarios
	expiration := window * 2
	if expiration < time.Minute {
		expiration = time.Minute
	}

	return store.Set(ctx, key, data, expiration)
}

// GetBucketInfo returns detailed information about a token bucket
func (tb *TokenBucketAlgorithm) GetBucketInfo(ctx context.Context, store Store, key string, limit int64, window time.Duration) (map[string]interface{}, error) {
	refillRate := float64(limit) / window.Seconds()

	state, err := tb.getBucketState(ctx, store, key, limit, refillRate, window)
	if err != nil {
		return nil, err
	}

	// Refill tokens to get current state
	now := time.Now()
	elapsed := now.Sub(state.LastRefill)
	if elapsed > 0 {
		tokensToAdd := refillRate * elapsed.Seconds()
		state.Tokens = math.Min(state.Tokens+tokensToAdd, float64(state.Capacity))
	}

	// Calculate additional metrics
	utilizationRate := float64(state.TotalRequests) / float64(limit) * 100
	if state.TotalRequests == 0 {
		utilizationRate = 0
	}

	denialRate := float64(state.DeniedRequests) / float64(state.TotalRequests+state.DeniedRequests) * 100
	if state.TotalRequests+state.DeniedRequests == 0 {
		denialRate = 0
	}

	timeUntilFull := time.Duration(0)
	if state.Tokens < float64(state.Capacity) {
		tokensNeeded := float64(state.Capacity) - state.Tokens
		timeUntilFull = time.Duration(tokensNeeded/refillRate) * time.Second
	}

	return map[string]interface{}{
		"algorithm":        tb.name,
		"current_tokens":   state.Tokens,
		"capacity":         state.Capacity,
		"refill_rate":      refillRate,
		"window":           window,
		"total_requests":   state.TotalRequests,
		"denied_requests":  state.DeniedRequests,
		"utilization_rate": utilizationRate,
		"denial_rate":      denialRate,
		"last_refill":      state.LastRefill,
		"time_until_full":  timeUntilFull,
		"burst_available":  int64(math.Floor(state.Tokens)),
	}, nil
}

// TokenBucketMetrics provides metrics for monitoring
type TokenBucketMetrics struct {
	BucketKey       string        `json:"bucket_key"`
	CurrentTokens   float64       `json:"current_tokens"`
	Capacity        int64         `json:"capacity"`
	RefillRate      float64       `json:"refill_rate"`
	TotalRequests   int64         `json:"total_requests"`
	DeniedRequests  int64         `json:"denied_requests"`
	UtilizationRate float64       `json:"utilization_rate"`
	DenialRate      float64       `json:"denial_rate"`
	TimeUntilFull   time.Duration `json:"time_until_full"`
}

// GetMetrics returns metrics for the token bucket
func (tb *TokenBucketAlgorithm) GetMetrics(ctx context.Context, store Store, key string, limit int64, window time.Duration) (*TokenBucketMetrics, error) {
	info, err := tb.GetBucketInfo(ctx, store, key, limit, window)
	if err != nil {
		return nil, err
	}

	return &TokenBucketMetrics{
		BucketKey:       key,
		CurrentTokens:   info["current_tokens"].(float64),
		Capacity:        info["capacity"].(int64),
		RefillRate:      info["refill_rate"].(float64),
		TotalRequests:   info["total_requests"].(int64),
		DeniedRequests:  info["denied_requests"].(int64),
		UtilizationRate: info["utilization_rate"].(float64),
		DenialRate:      info["denial_rate"].(float64),
		TimeUntilFull:   info["time_until_full"].(time.Duration),
	}, nil
}

// ValidateConfig validates token bucket specific configuration
func (tb *TokenBucketAlgorithm) ValidateConfig(limit int64, window time.Duration, burstSize int64) error {
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}

	if window <= 0 {
		return fmt.Errorf("window must be positive")
	}

	if burstSize <= 0 {
		return fmt.Errorf("burst size must be positive")
	}

	if burstSize > limit {
		return fmt.Errorf("burst size cannot exceed limit")
	}

	// Check for reasonable refill rate
	refillRate := float64(limit) / window.Seconds()
	if refillRate > 1000 { // More than 1000 tokens per second
		return fmt.Errorf("refill rate too high: %f tokens/second", refillRate)
	}

	return nil
}
