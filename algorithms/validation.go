// algorithms/validation.go
package algorithms

import (
	"fmt"
	"math"
	"time"
)

// Validation limits (imported from parent package constants)
const (
	// MinLimit is the minimum allowed rate limit
	MinLimit = int64(1)

	// MaxLimit is the maximum allowed rate limit (prevents integer overflow)
	MaxLimit = int64(1000000)

	// MinWindow is the minimum window duration (1 second)
	MinWindow = time.Second

	// MaxWindow is the maximum window duration (24 hours, prevents overflow)
	MaxWindow = 24 * time.Hour

	// MaxRequestCount is the maximum requests that can be requested at once
	// This prevents integer overflow in calculations
	MaxRequestCount = int64(100000)

	// MaxNanoSeconds is the safe maximum for nanosecond calculations
	// Prevents overflow when converting durations to nanoseconds
	MaxNanoSeconds = int64(math.MaxInt64 / 2)

	// P1-1 FIX: MaxKeyLength is the maximum allowed length for storage keys
	// This prevents DOS attacks via oversized keys and ensures Redis compatibility
	// Must match the constant in parent package (ratelimit.MaxKeyLength = 256)
	MaxKeyLength = 256
)

// ValidationError represents an input validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s (value: %v): %s", e.Field, e.Value, e.Message)
}

// ValidateAllowInputs validates all inputs for the Allow method
// This prevents integer overflow, negative values, and other edge cases
func ValidateAllowInputs(limit int64, window time.Duration, n int64) error {
	// Validate limit
	if err := ValidateLimit(limit); err != nil {
		return err
	}

	// Validate window
	if err := ValidateWindow(window); err != nil {
		return err
	}

	// Validate request count
	if err := ValidateRequestCount(n); err != nil {
		return err
	}

	// Validate that request count doesn't exceed limit
	if n > limit {
		return &ValidationError{
			Field:   "n",
			Value:   n,
			Message: fmt.Sprintf("request count (%d) cannot exceed limit (%d)", n, limit),
		}
	}

	// Validate potential overflow in nanosecond calculations
	windowNanos := window.Nanoseconds()
	if windowNanos <= 0 {
		return &ValidationError{
			Field:   "window",
			Value:   window,
			Message: "window duration in nanoseconds must be positive",
		}
	}

	// Check for potential overflow in limit * window calculations
	// This prevents integer overflow in refill rate calculations
	if limit > 0 && windowNanos > MaxNanoSeconds/limit {
		return &ValidationError{
			Field:   "limit/window",
			Value:   fmt.Sprintf("limit=%d, window=%v", limit, window),
			Message: "limit and window combination would cause integer overflow",
		}
	}

	return nil
}

// ValidateLimit validates the rate limit value
func ValidateLimit(limit int64) error {
	if limit < MinLimit {
		return &ValidationError{
			Field:   "limit",
			Value:   limit,
			Message: fmt.Sprintf("limit must be at least %d", MinLimit),
		}
	}

	if limit > MaxLimit {
		return &ValidationError{
			Field:   "limit",
			Value:   limit,
			Message: fmt.Sprintf("limit must not exceed %d (prevents overflow)", MaxLimit),
		}
	}

	return nil
}

// ValidateWindow validates the time window duration
func ValidateWindow(window time.Duration) error {
	if window < MinWindow {
		return &ValidationError{
			Field:   "window",
			Value:   window,
			Message: fmt.Sprintf("window must be at least %v", MinWindow),
		}
	}

	if window > MaxWindow {
		return &ValidationError{
			Field:   "window",
			Value:   window,
			Message: fmt.Sprintf("window must not exceed %v (prevents overflow)", MaxWindow),
		}
	}

	// Additional check for zero or negative (shouldn't happen but be defensive)
	if window <= 0 {
		return &ValidationError{
			Field:   "window",
			Value:   window,
			Message: "window must be positive",
		}
	}

	return nil
}

// ValidateRequestCount validates the number of requests (n parameter)
func ValidateRequestCount(n int64) error {
	if n <= 0 {
		return &ValidationError{
			Field:   "n",
			Value:   n,
			Message: "request count must be positive (at least 1)",
		}
	}

	if n > MaxRequestCount {
		return &ValidationError{
			Field:   "n",
			Value:   n,
			Message: fmt.Sprintf("request count must not exceed %d (prevents overflow)", MaxRequestCount),
		}
	}

	return nil
}

// ValidateBurst validates the burst size
func ValidateBurst(burst int64, limit int64) error {
	if burst < 0 {
		return &ValidationError{
			Field:   "burst",
			Value:   burst,
			Message: "burst must be non-negative",
		}
	}

	// Burst should generally not exceed limit by too much
	// Allow up to 10x the limit for flexibility, but cap at MaxLimit
	maxAllowedBurst := limit * 10
	if maxAllowedBurst > MaxLimit {
		maxAllowedBurst = MaxLimit
	}

	if burst > maxAllowedBurst {
		return &ValidationError{
			Field:   "burst",
			Value:   burst,
			Message: fmt.Sprintf("burst (%d) is too large relative to limit (%d), max allowed: %d", burst, limit, maxAllowedBurst),
		}
	}

	return nil
}

// ValidateKey validates the storage key
// P1-1: Validates key length to prevent memory DOS attacks
func ValidateKey(key string) error {
	if key == "" {
		return &ValidationError{
			Field:   "key",
			Value:   key,
			Message: "key cannot be empty",
		}
	}

	// P1-1 FIX: Check key length to prevent DOS attacks via oversized keys
	// MaxKeyLength = 256 bytes is sufficient for rate limiting keys and ensures
	// compatibility with Redis while preventing memory exhaustion attacks
	if len(key) > MaxKeyLength {
		return &ValidationError{
			Field:   "key",
			Value:   fmt.Sprintf("(length: %d bytes)", len(key)),
			Message: fmt.Sprintf("key length must not exceed %d bytes (prevents memory DOS)", MaxKeyLength),
		}
	}

	return nil
}
