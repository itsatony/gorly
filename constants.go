package ratelimit

import "time"

// ============================================================================
// ID PREFIXES - For prefixed nanoIDs using go-nuts
// ============================================================================

const (
	// IDPrefixRateLimiter is the prefix for rate limiter instance IDs
	IDPrefixRateLimiter = "rl"

	// IDPrefixContext is the prefix for rate limit context IDs
	IDPrefixContext = "rlc"

	// IDPrefixResult is the prefix for result IDs (if needed)
	IDPrefixResult = "rlr"
)

// ============================================================================
// STRATEGY NAMES - Rate limiting algorithm identifiers
// ============================================================================

const (
	// StrategyTokenBucket implements token bucket algorithm
	// Allows burst traffic, tokens regenerate over time
	StrategyTokenBucket = "token_bucket"

	// StrategySlidingWindow implements sliding window algorithm
	// More precise than token bucket, no burst allowance
	StrategySlidingWindow = "sliding_window"

	// StrategyFixedWindow implements fixed window algorithm
	// Simple window-based counting, fast performance
	StrategyFixedWindow = "fixed_window"

	// StrategyLeakyBucket implements leaky bucket algorithm
	// Smooth rate limiting, queue-based processing
	StrategyLeakyBucket = "leaky_bucket"
)

// ============================================================================
// STORE TYPES - Storage backend identifiers
// ============================================================================

const (
	// StoreTypeMemory uses in-memory storage (default)
	// Good for: single instance, development, testing
	StoreTypeMemory = "memory"

	// StoreTypeRedis uses Redis for distributed storage
	// Good for: multi-instance, high availability, shared limits
	StoreTypeRedis = "redis"

	// StoreTypeMock uses mock store for testing
	StoreTypeMock = "mock"
)

// ============================================================================
// TIER NAMES - Service tier identifiers
// ============================================================================

const (
	// TierFree is the free service tier
	TierFree = "free"

	// TierPremium is the premium/paid service tier
	TierPremium = "premium"

	// TierEnterprise is the enterprise service tier
	TierEnterprise = "enterprise"

	// TierInternal is for internal/system requests
	TierInternal = "internal"

	// TierDefault is used when no tier is specified
	TierDefault = "default"
)

// ============================================================================
// SCOPE NAMES - Rate limit scope identifiers
// ============================================================================

const (
	// ScopeGlobal applies to all operations
	ScopeGlobal = "global"

	// ScopeAPI applies to API requests
	ScopeAPI = "api"

	// ScopeSearch applies to search operations
	ScopeSearch = "search"

	// ScopeUpload applies to file upload operations
	ScopeUpload = "upload"

	// ScopeDownload applies to file download operations
	ScopeDownload = "download"

	// ScopeDatabase applies to database query operations
	ScopeDatabase = "database"

	// ScopeEvents applies to event processing
	ScopeEvents = "events"

	// ScopeAdmin applies to admin operations
	ScopeAdmin = "admin"

	// ScopeMetadata applies to metadata operations
	ScopeMetadata = "metadata"

	// ScopeAnalytics applies to analytics operations
	ScopeAnalytics = "analytics"
)

// ============================================================================
// METADATA KEYS - Keys for context metadata map
// ============================================================================

const (
	// MetadataKeyIP stores the client IP address
	MetadataKeyIP = "ip"

	// MetadataKeyUserAgent stores the client user agent
	MetadataKeyUserAgent = "user_agent"

	// MetadataKeyResource stores the resource being accessed
	MetadataKeyResource = "resource"

	// MetadataKeyMethod stores the HTTP method
	MetadataKeyMethod = "method"

	// MetadataKeyPath stores the URL path
	MetadataKeyPath = "path"

	// MetadataKeyTimestamp stores the request timestamp
	MetadataKeyTimestamp = "timestamp"

	// MetadataKeyRequestID stores the request ID
	MetadataKeyRequestID = "request_id"

	// MetadataKeyUserID stores the user ID
	MetadataKeyUserID = "user_id"

	// MetadataKeyTenant stores the tenant ID
	MetadataKeyTenant = "tenant"

	// MetadataKeyRegion stores the region
	MetadataKeyRegion = "region"
)

// ============================================================================
// ERROR CODES - Error identifiers for go-cuserr
// ============================================================================

const (
	// ErrCodeInvalidConfig indicates invalid configuration
	ErrCodeInvalidConfig = "GORLY_INVALID_CONFIG"

	// ErrCodeInvalidContext indicates invalid rate limit context
	ErrCodeInvalidContext = "GORLY_INVALID_CONTEXT"

	// ErrCodeStorageFailure indicates storage backend failure
	ErrCodeStorageFailure = "GORLY_STORAGE_FAILURE"

	// ErrCodeStrategyFailure indicates strategy execution failure
	ErrCodeStrategyFailure = "GORLY_STRATEGY_FAILURE"

	// ErrCodeResolverFailure indicates config resolver failure
	ErrCodeResolverFailure = "GORLY_RESOLVER_FAILURE"

	// ErrCodeLimitExceeded indicates rate limit exceeded
	ErrCodeLimitExceeded = "GORLY_LIMIT_EXCEEDED"

	// ErrCodeInvalidLimit indicates invalid limit value
	ErrCodeInvalidLimit = "GORLY_INVALID_LIMIT"

	// ErrCodeInvalidWindow indicates invalid time window
	ErrCodeInvalidWindow = "GORLY_INVALID_WINDOW"

	// ErrCodeInvalidBurst indicates invalid burst value
	ErrCodeInvalidBurst = "GORLY_INVALID_BURST"

	// ErrCodeClosed indicates limiter is closed
	ErrCodeClosed = "GORLY_CLOSED"

	// ErrCodeKeyNotFound indicates key not found in store
	ErrCodeKeyNotFound = "GORLY_KEY_NOT_FOUND"

	// ErrCodeConnectionFailed indicates connection failure
	ErrCodeConnectionFailed = "GORLY_CONNECTION_FAILED"

	// ErrCodeTimeout indicates operation timeout
	ErrCodeTimeout = "GORLY_TIMEOUT"

	// ErrCodeScriptNotSupported indicates script execution is not supported
	ErrCodeScriptNotSupported = "GORLY_SCRIPT_NOT_SUPPORTED"

	// ErrCodeKeyTooLong indicates the key exceeds maximum allowed length
	ErrCodeKeyTooLong = "GORLY_KEY_TOO_LONG"
)

// ============================================================================
// ERROR MESSAGES - Human-readable error messages
// ============================================================================

const (
	// ErrMsgInvalidConfig is returned when configuration is invalid
	ErrMsgInvalidConfig = "invalid rate limiter configuration"

	// ErrMsgInvalidContext is returned when context is invalid
	ErrMsgInvalidContext = "invalid rate limit context"

	// ErrMsgStorageFailure is returned when storage operation fails
	ErrMsgStorageFailure = "storage backend failure"

	// ErrMsgStrategyFailure is returned when strategy execution fails
	ErrMsgStrategyFailure = "rate limiting strategy failure"

	// ErrMsgResolverFailure is returned when config resolver fails
	ErrMsgResolverFailure = "rate limit resolver failure"

	// ErrMsgLimitExceeded is returned when rate limit is exceeded
	ErrMsgLimitExceeded = "rate limit exceeded"

	// ErrMsgInvalidLimit is returned when limit value is invalid
	ErrMsgInvalidLimit = "invalid rate limit value"

	// ErrMsgInvalidWindow is returned when window value is invalid
	ErrMsgInvalidWindow = "invalid time window"

	// ErrMsgInvalidBurst is returned when burst value is invalid
	ErrMsgInvalidBurst = "invalid burst value"

	// ErrMsgClosed is returned when limiter is closed
	ErrMsgClosed = "rate limiter is closed"

	// ErrMsgKeyNotFound is returned when key not found
	ErrMsgKeyNotFound = "key not found"

	// ErrMsgConnectionFailed is returned when connection fails
	ErrMsgConnectionFailed = "connection failed"

	// ErrMsgTimeout is returned when operation times out
	ErrMsgTimeout = "operation timeout"

	// ErrMsgScriptNotSupported is returned when store doesn't support script execution
	ErrMsgScriptNotSupported = "script execution not supported by this store"

	// ErrMsgKeyTooLong is returned when key exceeds maximum length
	ErrMsgKeyTooLong = "key length exceeds maximum allowed"
)

// ============================================================================
// DEFAULT VALUES - Default configuration values
// ============================================================================

const (
	// DefaultLimit is the default rate limit (requests per window)
	DefaultLimit = int64(1000)

	// DefaultWindowSeconds is the default time window in seconds
	DefaultWindowSeconds = int64(3600) // 1 hour

	// DefaultWindow is the default time window duration
	DefaultWindow = time.Duration(DefaultWindowSeconds) * time.Second

	// DefaultBurst is the default burst size
	DefaultBurst = int64(100)

	// DefaultCleanupIntervalSeconds is the default cleanup interval for memory store
	DefaultCleanupIntervalSeconds = int64(300) // 5 minutes

	// DefaultCleanupInterval is the default cleanup interval duration
	DefaultCleanupInterval = time.Duration(DefaultCleanupIntervalSeconds) * time.Second

	// DefaultMaxKeys is the default maximum keys in memory store
	DefaultMaxKeys = int64(100000)

	// DefaultShardCount is the default number of shards for memory store
	DefaultShardCount = int(32)

	// DefaultRedisPoolSize is the default Redis connection pool size
	DefaultRedisPoolSize = int(10)

	// DefaultRedisTimeout is the default Redis operation timeout
	DefaultRedisTimeout = 5 * time.Second

	// DefaultRedisMaxRetries is the default number of Redis retries
	DefaultRedisMaxRetries = int(3)
)

// ============================================================================
// VALIDATION LIMITS - Min/Max values for validation
// ============================================================================

const (
	// MinLimit is the minimum allowed rate limit
	MinLimit = int64(1)

	// MaxLimit is the maximum allowed rate limit
	MaxLimit = int64(1000000)

	// MinWindowSeconds is the minimum window duration in seconds
	MinWindowSeconds = int64(1)

	// MaxWindowSeconds is the maximum window duration in seconds (24 hours)
	MaxWindowSeconds = int64(86400)

	// MinBurst is the minimum burst size
	MinBurst = int64(0)

	// MaxBurst is the maximum burst size
	MaxBurst = int64(100000)

	// MinCleanupIntervalSeconds is the minimum cleanup interval
	MinCleanupIntervalSeconds = int64(10)

	// MaxCleanupIntervalSeconds is the maximum cleanup interval
	MaxCleanupIntervalSeconds = int64(3600)

	// MaxKeyLength is the maximum allowed length for store keys (in bytes)
	// This prevents DOS attacks via oversized keys and ensures compatibility
	// with Redis (512 MB max key size) while being reasonable for rate limiting
	MaxKeyLength = 256
)

// ============================================================================
// HTTP HEADERS - Standard rate limit headers
// ============================================================================

const (
	// HeaderRateLimitLimit is the header for rate limit total
	HeaderRateLimitLimit = "X-RateLimit-Limit"

	// HeaderRateLimitRemaining is the header for remaining requests
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"

	// HeaderRateLimitReset is the header for reset timestamp
	HeaderRateLimitReset = "X-RateLimit-Reset"

	// HeaderRateLimitRetryAfter is the header for retry delay
	HeaderRateLimitRetryAfter = "Retry-After"

	// HeaderRateLimitUsed is the header for used requests
	HeaderRateLimitUsed = "X-RateLimit-Used"
)

// ============================================================================
// STORAGE KEY PREFIXES - Prefixes for storage keys
// ============================================================================

const (
	// StorageKeyPrefixDefault is the default prefix for storage keys
	StorageKeyPrefixDefault = "gorly"

	// StorageKeySeparator is the separator for key components
	StorageKeySeparator = ":"
)

// ============================================================================
// LOG MESSAGES - Structured log messages
// ============================================================================

const (
	// LogMsgLimiterCreated is logged when limiter is created
	LogMsgLimiterCreated = "rate limiter created"

	// LogMsgLimiterClosed is logged when limiter is closed
	LogMsgLimiterClosed = "rate limiter closed"

	// LogMsgCheckAllowed is logged when request is allowed
	LogMsgCheckAllowed = "request allowed"

	// LogMsgCheckDenied is logged when request is denied
	LogMsgCheckDenied = "request denied"

	// LogMsgReset is logged when rate limit is reset
	LogMsgReset = "rate limit reset"

	// LogMsgStoreConnected is logged when store connects
	LogMsgStoreConnected = "store connected"

	// LogMsgStoreClosed is logged when store closes
	LogMsgStoreClosed = "store closed"

	// LogMsgCleanupStarted is logged when cleanup starts
	LogMsgCleanupStarted = "cleanup started"

	// LogMsgCleanupCompleted is logged when cleanup completes
	LogMsgCleanupCompleted = "cleanup completed"
)

// ============================================================================
// RATE STRING PATTERNS - Supported rate string formats
// ============================================================================

const (
	// RateUnitSecond represents per-second rate
	RateUnitSecond = "second"

	// RateUnitMinute represents per-minute rate
	RateUnitMinute = "minute"

	// RateUnitHour represents per-hour rate
	RateUnitHour = "hour"

	// RateUnitDay represents per-day rate
	RateUnitDay = "day"

	// RateUnitShortSecond is the short form for second
	RateUnitShortSecond = "s"

	// RateUnitShortMinute is the short form for minute
	RateUnitShortMinute = "m"

	// RateUnitShortHour is the short form for hour
	RateUnitShortHour = "h"

	// RateUnitShortDay is the short form for day
	RateUnitShortDay = "d"
)
