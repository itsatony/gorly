// Package ratelimit provides comprehensive typed error handling
package ratelimit

import (
	"errors"
	"fmt"
	"time"
)

// ErrorCode represents specific error types
type ErrorCode string

const (
	// Configuration errors
	ErrCodeInvalidLimit     ErrorCode = "INVALID_LIMIT"
	ErrCodeInvalidAlgorithm ErrorCode = "INVALID_ALGORITHM"
	ErrCodeInvalidConfig    ErrorCode = "INVALID_CONFIG"
	ErrCodeMissingConfig    ErrorCode = "MISSING_CONFIG"

	// Connection errors
	ErrCodeRedisConnection  ErrorCode = "REDIS_CONNECTION"
	ErrCodeRedisTimeout     ErrorCode = "REDIS_TIMEOUT"
	ErrCodeRedisAuth        ErrorCode = "REDIS_AUTH"
	ErrCodeStoreUnavailable ErrorCode = "STORE_UNAVAILABLE"

	// Rate limiting errors
	ErrCodeRateLimitExceeded ErrorCode = "RATE_LIMIT_EXCEEDED"
	ErrCodeQuotaExceeded     ErrorCode = "QUOTA_EXCEEDED"
	ErrCodeWindowExpired     ErrorCode = "WINDOW_EXPIRED"
	ErrCodeInvalidEntity     ErrorCode = "INVALID_ENTITY"
	ErrCodeInvalidScope      ErrorCode = "INVALID_SCOPE"

	// System errors
	ErrCodeInternalError  ErrorCode = "INTERNAL_ERROR"
	ErrCodeTimeout        ErrorCode = "TIMEOUT"
	ErrCodeUnavailable    ErrorCode = "UNAVAILABLE"
	ErrCodeNotInitialized ErrorCode = "NOT_INITIALIZED"

	// Middleware errors
	ErrCodeFrameworkNotSupported ErrorCode = "FRAMEWORK_NOT_SUPPORTED"
	ErrCodeMiddlewareError       ErrorCode = "MIDDLEWARE_ERROR"
)

// AdvancedRateLimitError represents a comprehensive rate limiting error
type AdvancedRateLimitError struct {
	Code      ErrorCode              `json:"code"`
	Message   string                 `json:"message"`
	Details   string                 `json:"details,omitempty"`
	Cause     error                  `json:"cause,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Context   map[string]interface{} `json:"context,omitempty"`

	// Rate limiting specific fields
	Entity     string        `json:"entity,omitempty"`
	Scope      string        `json:"scope,omitempty"`
	Limit      int64         `json:"limit,omitempty"`
	Used       int64         `json:"used,omitempty"`
	Remaining  int64         `json:"remaining,omitempty"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
	ResetTime  time.Time     `json:"reset_time,omitempty"`

	// Suggestions for resolution
	Suggestions []string `json:"suggestions,omitempty"`
}

// Error implements the error interface
func (e *AdvancedRateLimitError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Is implements error matching for errors.Is()
func (e *AdvancedRateLimitError) Is(target error) bool {
	if t, ok := target.(*AdvancedRateLimitError); ok {
		return e.Code == t.Code
	}
	return false
}

// Unwrap implements error unwrapping for errors.Unwrap()
func (e *AdvancedRateLimitError) Unwrap() error {
	return e.Cause
}

// WithContext adds context information to the error
func (e *AdvancedRateLimitError) WithContext(key string, value interface{}) *AdvancedRateLimitError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithSuggestion adds a suggestion for error resolution
func (e *AdvancedRateLimitError) WithSuggestion(suggestion string) *AdvancedRateLimitError {
	e.Suggestions = append(e.Suggestions, suggestion)
	return e
}

// IsRetryable returns whether the error condition is retryable
func (e *AdvancedRateLimitError) IsRetryable() bool {
	switch e.Code {
	case ErrCodeRateLimitExceeded, ErrCodeQuotaExceeded, ErrCodeTimeout,
		ErrCodeStoreUnavailable, ErrCodeRedisTimeout, ErrCodeUnavailable:
		return true
	default:
		return false
	}
}

// ShouldCircuitBreak returns whether this error should trigger circuit breaker
func (e *AdvancedRateLimitError) ShouldCircuitBreak() bool {
	switch e.Code {
	case ErrCodeStoreUnavailable, ErrCodeRedisConnection, ErrCodeUnavailable:
		return true
	default:
		return false
	}
}

// HTTPStatusCode returns the appropriate HTTP status code for this error
func (e *AdvancedRateLimitError) HTTPStatusCode() int {
	switch e.Code {
	case ErrCodeRateLimitExceeded, ErrCodeQuotaExceeded:
		return 429 // Too Many Requests
	case ErrCodeInvalidEntity, ErrCodeInvalidScope, ErrCodeInvalidConfig:
		return 400 // Bad Request
	case ErrCodeRedisAuth:
		return 401 // Unauthorized
	case ErrCodeTimeout, ErrCodeStoreUnavailable, ErrCodeUnavailable:
		return 503 // Service Unavailable
	case ErrCodeFrameworkNotSupported:
		return 501 // Not Implemented
	default:
		return 500 // Internal Server Error
	}
}

// Error constructor functions

// NewAdvancedRateLimitError creates a new rate limit error
func NewAdvancedRateLimitError(code ErrorCode, message string) *AdvancedRateLimitError {
	return &AdvancedRateLimitError{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
	}
}

// NewRateLimitExceededError creates a rate limit exceeded error
func NewRateLimitExceededError(entity, scope string, limit, used int64, retryAfter time.Duration) *AdvancedRateLimitError {
	err := &AdvancedRateLimitError{
		Code:       ErrCodeRateLimitExceeded,
		Message:    fmt.Sprintf("Rate limit exceeded for %s in scope %s", entity, scope),
		Timestamp:  time.Now(),
		Entity:     entity,
		Scope:      scope,
		Limit:      limit,
		Used:       used,
		Remaining:  max(0, limit-used),
		RetryAfter: retryAfter,
		ResetTime:  time.Now().Add(retryAfter),
	}

	// Add helpful suggestions
	if retryAfter < time.Minute {
		err.WithSuggestion(fmt.Sprintf("Wait %v before retrying", retryAfter))
	} else {
		err.WithSuggestion("Consider upgrading to a higher tier for increased limits")
	}

	if scope != "global" {
		err.WithSuggestion("Try using a different endpoint with separate limits")
	}

	return err
}

// NewConfigError creates a configuration error
func NewConfigError(code ErrorCode, message, details string) *AdvancedRateLimitError {
	err := NewAdvancedRateLimitError(code, message)
	err.Details = details

	switch code {
	case ErrCodeInvalidLimit:
		err.WithSuggestion("Use format like '100/minute', '10/second', '1000/hour'")
		err.WithSuggestion("Supported units: second, minute, hour, day")
	case ErrCodeInvalidAlgorithm:
		err.WithSuggestion("Supported algorithms: token_bucket, sliding_window")
	case ErrCodeInvalidConfig:
		err.WithSuggestion("Check the builder configuration for missing required fields")
	}

	return err
}

// NewRedisError creates a Redis-related error
func NewRedisError(code ErrorCode, message string, cause error) *AdvancedRateLimitError {
	err := NewAdvancedRateLimitError(code, message)
	err.Cause = cause

	switch code {
	case ErrCodeRedisConnection:
		err.WithSuggestion("Check Redis server is running and accessible")
		err.WithSuggestion("Verify connection string format: 'localhost:6379'")
		err.WithSuggestion("Check network connectivity and firewall rules")
	case ErrCodeRedisAuth:
		err.WithSuggestion("Verify Redis password is correct")
		err.WithSuggestion("Ensure Redis is configured to require authentication")
	case ErrCodeRedisTimeout:
		err.WithSuggestion("Increase timeout value or check Redis performance")
		err.WithSuggestion("Consider using connection pooling")
	}

	return err
}

// NewInternalError creates an internal error
func NewInternalError(message string, cause error) *AdvancedRateLimitError {
	return &AdvancedRateLimitError{
		Code:      ErrCodeInternalError,
		Message:   message,
		Cause:     cause,
		Timestamp: time.Now(),
	}
}

// Predefined common errors
var (
	ErrInvalidLimitFormat = NewConfigError(ErrCodeInvalidLimit,
		"Invalid limit format",
		"Limit must be in format 'number/unit' like '100/minute'")

	ErrAlgorithmNotSupported = NewConfigError(ErrCodeInvalidAlgorithm,
		"Algorithm not supported",
		"Supported algorithms: token_bucket, sliding_window")

	ErrRedisNotAvailable = NewRedisError(ErrCodeStoreUnavailable,
		"Redis store not available", nil).
		WithSuggestion("Check Redis connection").
		WithSuggestion("Consider using in-memory store for development")

	ErrNotInitialized = NewAdvancedRateLimitError(ErrCodeNotInitialized,
		"Rate limiter not properly initialized")

	ErrFrameworkNotSupported = NewAdvancedRateLimitError(ErrCodeFrameworkNotSupported,
		"Web framework not supported").
		WithSuggestion("Use universal middleware: limiter.Middleware()").
		WithSuggestion("Supported frameworks: Gin, Echo, Fiber, Chi, net/http")
)

// Error checking utilities

// IsRateLimitExceeded checks if error is due to rate limit exceeded
func IsRateLimitExceeded(err error) bool {
	var rateLimitErr *AdvancedRateLimitError
	return errors.As(err, &rateLimitErr) && rateLimitErr.Code == ErrCodeRateLimitExceeded
}

// IsConfigError checks if error is a configuration error
func IsConfigError(err error) bool {
	var rateLimitErr *AdvancedRateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr.Code == ErrCodeInvalidConfig ||
			rateLimitErr.Code == ErrCodeInvalidLimit ||
			rateLimitErr.Code == ErrCodeInvalidAlgorithm ||
			rateLimitErr.Code == ErrCodeMissingConfig
	}
	return false
}

// IsConnectionError checks if error is a connection-related error
func IsConnectionError(err error) bool {
	var rateLimitErr *AdvancedRateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr.Code == ErrCodeRedisConnection ||
			rateLimitErr.Code == ErrCodeRedisTimeout ||
			rateLimitErr.Code == ErrCodeRedisAuth ||
			rateLimitErr.Code == ErrCodeStoreUnavailable
	}
	return false
}

// IsRetryable checks if an error condition is retryable
func IsRetryable(err error) bool {
	var rateLimitErr *AdvancedRateLimitError
	if errors.As(err, &rateLimitErr) {
		return rateLimitErr.IsRetryable()
	}
	return false
}

// GetRetryAfter extracts retry-after duration from rate limit errors
func GetRetryAfter(err error) (time.Duration, bool) {
	var rateLimitErr *AdvancedRateLimitError
	if errors.As(err, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		return rateLimitErr.RetryAfter, true
	}
	return 0, false
}

// ErrorHandler defines how errors should be handled
type ErrorHandler func(error)

// DefaultErrorHandler provides basic console error handling
func DefaultErrorHandler(err error) {
	var rateLimitErr *AdvancedRateLimitError
	if errors.As(err, &rateLimitErr) {
		fmt.Printf("[ERROR %s] %s\n", rateLimitErr.Code, rateLimitErr.Message)
		if rateLimitErr.Details != "" {
			fmt.Printf("  Details: %s\n", rateLimitErr.Details)
		}
		if len(rateLimitErr.Suggestions) > 0 {
			fmt.Println("  Suggestions:")
			for _, suggestion := range rateLimitErr.Suggestions {
				fmt.Printf("    - %s\n", suggestion)
			}
		}
	} else {
		fmt.Printf("[ERROR] %v\n", err)
	}
}

// ErrorRecovery provides error recovery strategies
type ErrorRecovery struct {
	maxRetries     int
	retryDelay     time.Duration
	circuitBreaker bool
}

// NewErrorRecovery creates a new error recovery handler
func NewErrorRecovery(maxRetries int, retryDelay time.Duration) *ErrorRecovery {
	return &ErrorRecovery{
		maxRetries:     maxRetries,
		retryDelay:     retryDelay,
		circuitBreaker: true,
	}
}

// RetryWithBackoff retries an operation with exponential backoff
func (er *ErrorRecovery) RetryWithBackoff(operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < er.maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !IsRetryable(err) {
			return err
		}

		// Check for circuit breaker condition
		var rateLimitErr *AdvancedRateLimitError
		if er.circuitBreaker && errors.As(err, &rateLimitErr) && rateLimitErr.ShouldCircuitBreak() {
			return fmt.Errorf("circuit breaker opened due to: %w", err)
		}

		// Wait before retry with exponential backoff
		delay := er.retryDelay * time.Duration(1<<uint(attempt))
		if retryAfter, hasRetryAfter := GetRetryAfter(err); hasRetryAfter {
			// Use the retry-after from rate limit error if available
			delay = retryAfter
		}

		time.Sleep(delay)
	}

	return fmt.Errorf("operation failed after %d attempts: %w", er.maxRetries, lastErr)
}

// Helper functions

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
