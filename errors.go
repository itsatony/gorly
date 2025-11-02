package ratelimit

import (
	"errors"
	"fmt"

	"github.com/itsatony/go-cuserr"
)

// ============================================================================
// SENTINEL ERRORS - Predefined error constants
// ============================================================================

// Core sentinel errors using standard errors package
// These are wrapped with cuserr when additional context is needed
var (
	// ErrInvalidConfig indicates the rate limiter configuration is invalid
	ErrInvalidConfig = errors.New(ErrMsgInvalidConfig)

	// ErrInvalidContext indicates the rate limit context is invalid
	ErrInvalidContext = errors.New(ErrMsgInvalidContext)

	// ErrStorageFailure indicates a storage backend failure
	ErrStorageFailure = errors.New(ErrMsgStorageFailure)

	// ErrStrategyFailure indicates a rate limiting strategy failure
	ErrStrategyFailure = errors.New(ErrMsgStrategyFailure)

	// ErrResolverFailure indicates a config resolver failure
	ErrResolverFailure = errors.New(ErrMsgResolverFailure)

	// ErrLimitExceeded indicates the rate limit has been exceeded
	ErrLimitExceeded = errors.New(ErrMsgLimitExceeded)

	// ErrInvalidLimit indicates an invalid rate limit value
	ErrInvalidLimit = errors.New(ErrMsgInvalidLimit)

	// ErrInvalidWindow indicates an invalid time window value
	ErrInvalidWindow = errors.New(ErrMsgInvalidWindow)

	// ErrInvalidBurst indicates an invalid burst value
	ErrInvalidBurst = errors.New(ErrMsgInvalidBurst)

	// ErrClosed indicates the rate limiter has been closed
	ErrClosed = errors.New(ErrMsgClosed)

	// ErrKeyNotFound indicates a key was not found in the store
	ErrKeyNotFound = errors.New(ErrMsgKeyNotFound)

	// ErrConnectionFailed indicates a connection failure
	ErrConnectionFailed = errors.New(ErrMsgConnectionFailed)

	// ErrTimeout indicates an operation timeout
	ErrTimeout = errors.New(ErrMsgTimeout)

	// ErrScriptNotSupported indicates the store doesn't support script execution
	ErrScriptNotSupported = errors.New(ErrMsgScriptNotSupported)

	// ErrKeyTooLong indicates the key exceeds maximum allowed length
	ErrKeyTooLong = errors.New(ErrMsgKeyTooLong)
)

// ============================================================================
// ERROR WRAPPING HELPERS - Convenience functions for error wrapping
// ============================================================================

// WrapConfigError wraps a configuration error with additional context
func WrapConfigError(err error, message string, keyValues ...interface{}) error {
	if err == nil {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidConfig,
			message,
		)
	}

	cusErr := cuserr.NewCustomError(ErrInvalidConfig, err, message)
	addMetadata(cusErr, keyValues...)
	return cusErr
}

// WrapContextError wraps a context error with additional context
func WrapContextError(err error, message string, keyValues ...interface{}) error {
	if err == nil {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidContext,
			message,
		)
	}

	cusErr := cuserr.NewCustomError(ErrInvalidContext, err, message)
	addMetadata(cusErr, keyValues...)
	return cusErr
}

// WrapStorageError wraps a storage error with additional context
func WrapStorageError(err error, operation string, keyValues ...interface{}) error {
	message := fmt.Sprintf("storage operation failed: %s", operation)
	metadata := append([]interface{}{"operation", operation}, keyValues...)

	if err == nil {
		cusErr := cuserr.NewInternalError("storage", nil)
		cusErr.Message = message
		addMetadata(cusErr, metadata...)
		return cusErr
	}

	cusErr := cuserr.NewCustomError(ErrStorageFailure, err, message)
	addMetadata(cusErr, metadata...)
	return cusErr
}

// WrapStrategyError wraps a strategy error with additional context
func WrapStrategyError(err error, strategyName string, keyValues ...interface{}) error {
	message := fmt.Sprintf("strategy execution failed: %s", strategyName)
	metadata := append([]interface{}{"strategy", strategyName}, keyValues...)

	if err == nil {
		cusErr := cuserr.NewInternalError("strategy", nil)
		cusErr.Message = message
		addMetadata(cusErr, metadata...)
		return cusErr
	}

	cusErr := cuserr.NewCustomError(ErrStrategyFailure, err, message)
	addMetadata(cusErr, metadata...)
	return cusErr
}

// WrapResolverError wraps a resolver error with additional context
func WrapResolverError(err error, message string, keyValues ...interface{}) error {
	if err == nil {
		cusErr := cuserr.NewInternalError("resolver", nil)
		cusErr.Message = message
		addMetadata(cusErr, keyValues...)
		return cusErr
	}

	cusErr := cuserr.NewCustomError(ErrResolverFailure, err, message)
	addMetadata(cusErr, keyValues...)
	return cusErr
}

// NewLimitExceededError creates a rate limit exceeded error with context
func NewLimitExceededError(identity, scope string, limit, used int64, keyValues ...interface{}) error {
	message := fmt.Sprintf("rate limit exceeded for %s in scope %s (%d/%d used)",
		identity, scope, used, limit)

	metadata := append([]interface{}{
		"identity", identity,
		"scope", scope,
		"limit", limit,
		"used", used,
	}, keyValues...)

	cusErr := cuserr.NewCustomErrorWithCategory(
		cuserr.ErrorCategoryValidation,
		ErrCodeLimitExceeded,
		message,
	)
	addMetadata(cusErr, metadata...)
	return cusErr
}

// NewConnectionError creates a connection error with context
func NewConnectionError(storeType, address string, err error, keyValues ...interface{}) error {
	message := fmt.Sprintf("failed to connect to %s at %s", storeType, address)
	metadata := append([]interface{}{
		"store_type", storeType,
		"address", address,
	}, keyValues...)

	if err == nil {
		cusErr := cuserr.NewExternalError(storeType, "connect", nil)
		cusErr.Message = message
		addMetadata(cusErr, metadata...)
		return cusErr
	}

	cusErr := cuserr.NewExternalError(storeType, "connect", err)
	cusErr.Message = message
	addMetadata(cusErr, metadata...)
	return cusErr
}

// NewTimeoutError creates a timeout error with context
func NewTimeoutError(operation string, duration interface{}, keyValues ...interface{}) error {
	message := fmt.Sprintf("operation timed out: %s", operation)
	metadata := append([]interface{}{
		"operation", operation,
		"timeout", duration,
	}, keyValues...)

	cusErr := cuserr.NewCustomErrorWithCategory(
		cuserr.ErrorCategoryValidation,
		ErrCodeTimeout,
		message,
	)
	addMetadata(cusErr, metadata...)
	return cusErr
}

// addMetadata is a helper to add key-value metadata to CustomError
func addMetadata(err *cuserr.CustomError, keyValues ...interface{}) {
	if err == nil || len(keyValues) == 0 {
		return
	}

	for i := 0; i < len(keyValues)-1; i += 2 {
		key, ok := keyValues[i].(string)
		if !ok {
			continue
		}
		value := keyValues[i+1]
		// WithMetadata expects string values, so convert to string
		valueStr := fmt.Sprintf("%v", value)
		err.WithMetadata(key, valueStr)
	}
}

// ============================================================================
// ERROR VALIDATION HELPERS
// ============================================================================

// ValidateLimitValue validates a rate limit value
func ValidateLimitValue(limit int64) error {
	if limit < MinLimit {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidLimit,
			fmt.Sprintf("limit %d is below minimum %d", limit, MinLimit),
		)
	}
	if limit > MaxLimit {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidLimit,
			fmt.Sprintf("limit %d exceeds maximum %d", limit, MaxLimit),
		)
	}
	return nil
}

// ValidateWindowSeconds validates a window duration in seconds
func ValidateWindowSeconds(windowSec int64) error {
	if windowSec < MinWindowSeconds {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidWindow,
			fmt.Sprintf("window %ds is below minimum %ds", windowSec, MinWindowSeconds),
		)
	}
	if windowSec > MaxWindowSeconds {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidWindow,
			fmt.Sprintf("window %ds exceeds maximum %ds", windowSec, MaxWindowSeconds),
		)
	}
	return nil
}

// ValidateBurstValue validates a burst value
func ValidateBurstValue(burst int64) error {
	if burst < MinBurst {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidBurst,
			fmt.Sprintf("burst %d is below minimum %d", burst, MinBurst),
		)
	}
	if burst > MaxBurst {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeInvalidBurst,
			fmt.Sprintf("burst %d exceeds maximum %d", burst, MaxBurst),
		)
	}
	return nil
}

// ValidateKeyLength validates a storage key length
// Returns ErrKeyTooLong if the key exceeds MaxKeyLength bytes
func ValidateKeyLength(key string) error {
	if len(key) > MaxKeyLength {
		return cuserr.NewCustomErrorWithCategory(
			cuserr.ErrorCategoryValidation,
			ErrCodeKeyTooLong,
			fmt.Sprintf("key length %d exceeds maximum %d bytes", len(key), MaxKeyLength),
		)
	}
	return nil
}

// ============================================================================
// ERROR CHECKING HELPERS
// ============================================================================

// IsRateLimitExceeded checks if error is a rate limit exceeded error
func IsRateLimitExceeded(err error) bool {
	return errors.Is(err, ErrLimitExceeded) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeLimitExceeded)
}

// IsStorageFailure checks if error is a storage failure
func IsStorageFailure(err error) bool {
	return errors.Is(err, ErrStorageFailure) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeStorageFailure)
}

// IsConfigError checks if error is a configuration error
func IsConfigError(err error) bool {
	return errors.Is(err, ErrInvalidConfig) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeInvalidConfig)
}

// IsContextError checks if error is a context error
func IsContextError(err error) bool {
	return errors.Is(err, ErrInvalidContext) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeInvalidContext)
}

// IsConnectionError checks if error is a connection error
func IsConnectionError(err error) bool {
	return errors.Is(err, ErrConnectionFailed) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeConnectionFailed)
}

// IsTimeoutError checks if error is a timeout error
func IsTimeoutError(err error) bool {
	return errors.Is(err, ErrTimeout) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeTimeout)
}

// IsClosed checks if error indicates limiter is closed
func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeClosed)
}

// IsScriptNotSupported checks if error indicates scripts are not supported
func IsScriptNotSupported(err error) bool {
	return errors.Is(err, ErrScriptNotSupported) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeScriptNotSupported)
}

// IsKeyTooLong checks if error indicates key length exceeded maximum
func IsKeyTooLong(err error) bool {
	return errors.Is(err, ErrKeyTooLong) ||
		(cuserr.IsCompatibleError(err) && cuserr.GetErrorCode(err) == ErrCodeKeyTooLong)
}
