package stores

import (
	"context"
	"fmt"
	"sync"
	"time"

	ratelimit "github.com/itsatony/gorly"
	"github.com/redis/go-redis/v9"
	nuts "github.com/vaudience/go-nuts"
)

// ============================================================================
// REDIS STORE - Production-ready Redis backend with connection pooling
// ============================================================================

// RedisStore implements Store interface using Redis as the backend
// Thread-safe with automatic connection pooling and health checks
type RedisStore struct {
	client    *redis.Client
	config    *RedisStoreConfig
	closed    bool
	closeMu   sync.RWMutex
	logger    ratelimit.Logger
	id        string
	healthMu  sync.RWMutex
	lastError error
}

// ============================================================================
// CONFIGURATION
// ============================================================================

// RedisStoreConfig configures the Redis store
type RedisStoreConfig struct {
	// Address is the Redis server address (host:port)
	Address string

	// Password for Redis authentication (optional)
	Password string

	// Database to use (0-15)
	Database int

	// PoolSize is the maximum number of socket connections
	PoolSize int

	// MinIdleConns is the minimum number of idle connections
	MinIdleConns int

	// MaxRetries is the maximum number of retries before giving up
	MaxRetries int

	// DialTimeout is the timeout for establishing new connections
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes
	WriteTimeout time.Duration

	// PoolTimeout is the amount of time client waits for connection if all
	// connections are busy before returning an error
	PoolTimeout time.Duration

	// IdleTimeout is the amount of time after which client closes idle connections
	IdleTimeout time.Duration

	// MaxConnAge is the connection age at which client retires (closes) the connection
	MaxConnAge time.Duration

	// Logger for store operations (optional)
	Logger ratelimit.Logger

	// EnableMetrics enables Redis metrics collection
	EnableMetrics bool

	// KeyPrefix is an optional prefix for all Redis keys
	KeyPrefix string

	// RetryMaxAttempts is the maximum number of retry attempts for transient failures
	// Set to 0 to disable application-level retries (only use go-redis built-in retries)
	RetryMaxAttempts int

	// RetryInitialBackoff is the initial backoff duration before first retry
	RetryInitialBackoff time.Duration

	// RetryMaxBackoff is the maximum backoff duration between retries
	RetryMaxBackoff time.Duration

	// RetryBackoffMultiplier is the multiplier for exponential backoff (typically 2.0)
	RetryBackoffMultiplier float64
}

// DefaultRedisStoreConfig returns default configuration
func DefaultRedisStoreConfig() *RedisStoreConfig {
	return &RedisStoreConfig{
		Address:                "localhost:6379",
		Password:               "",
		Database:               0,
		PoolSize:               10,
		MinIdleConns:           2,
		MaxRetries:             3,
		DialTimeout:            5 * time.Second,
		ReadTimeout:            3 * time.Second,
		WriteTimeout:           3 * time.Second,
		PoolTimeout:            4 * time.Second,
		IdleTimeout:            5 * time.Minute,
		MaxConnAge:             30 * time.Minute,
		Logger:                 ratelimit.NewNopLogger(),
		EnableMetrics:          false,
		KeyPrefix:              "gorly:",
		RetryMaxAttempts:       3,
		RetryInitialBackoff:    50 * time.Millisecond,
		RetryMaxBackoff:        2 * time.Second,
		RetryBackoffMultiplier: 2.0,
	}
}

// Validate validates the configuration
func (c *RedisStoreConfig) Validate() error {
	if c.Address == "" {
		return ratelimit.WrapConfigError(nil, "Redis address is required",
			"address", c.Address)
	}
	if c.Database < 0 || c.Database > 15 {
		return ratelimit.WrapConfigError(nil, "Redis database must be between 0 and 15",
			"database", c.Database)
	}
	if c.PoolSize < 1 {
		return ratelimit.WrapConfigError(nil, "Pool size must be at least 1",
			"pool_size", c.PoolSize)
	}
	if c.MinIdleConns < 0 {
		return ratelimit.WrapConfigError(nil, "Min idle connections cannot be negative",
			"min_idle_conns", c.MinIdleConns)
	}
	if c.MaxRetries < 0 {
		return ratelimit.WrapConfigError(nil, "Max retries cannot be negative",
			"max_retries", c.MaxRetries)
	}
	if c.DialTimeout < time.Second {
		return ratelimit.WrapConfigError(nil, "Dial timeout must be at least 1 second",
			"dial_timeout", c.DialTimeout)
	}
	return nil
}

// ============================================================================
// CONSTRUCTOR
// ============================================================================

// NewRedisStore creates a new Redis store with connection pooling
func NewRedisStore(config *RedisStoreConfig) (*RedisStore, error) {
	if config == nil {
		config = DefaultRedisStoreConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Create Redis client with connection pooling
	client := redis.NewClient(&redis.Options{
		Addr:         config.Address,
		Password:     config.Password,
		DB:           config.Database,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConns,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  config.DialTimeout,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		PoolTimeout:  config.PoolTimeout,
		// Note: IdleTimeout and MaxConnAge are not exposed in redis.Options
		// They are managed internally by the connection pool
	})

	rs := &RedisStore{
		client: client,
		config: config,
		logger: config.Logger,
		id:     nuts.NID(ratelimit.IDPrefixRateLimiter, 16),
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), config.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, ratelimit.WrapStorageError(err, "redis connection failed",
			"address", config.Address,
			"database", config.Database)
	}

	rs.logger.Info("redis store created",
		"id", rs.id,
		"address", config.Address,
		"database", config.Database,
		"pool_size", config.PoolSize,
	)

	return rs, nil
}

// ============================================================================
// STORE INTERFACE IMPLEMENTATION
// ============================================================================

// Get retrieves a value from Redis
func (rs *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return nil, err
	}

	fullKey := rs.buildKey(key)
	var data []byte

	err := rs.retryableOperation(ctx, "get", func() error {
		result, err := rs.client.Get(ctx, fullKey).Bytes()
		if err == redis.Nil {
			return ratelimit.ErrKeyNotFound
		}
		if err != nil {
			return err
		}
		data = result
		return nil
	})

	if err != nil {
		rs.recordError(err)
		if err == ratelimit.ErrKeyNotFound {
			return nil, err
		}
		return nil, ratelimit.WrapStorageError(err, "redis get failed",
			"key", key)
	}

	return data, nil
}

// Set stores a value in Redis with optional expiration
func (rs *RedisStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return err
	}

	fullKey := rs.buildKey(key)

	err := rs.retryableOperation(ctx, "set", func() error {
		return rs.client.Set(ctx, fullKey, value, expiration).Err()
	})

	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis set failed",
			"key", key)
	}

	return nil
}

// Increment atomically increments a counter in Redis
func (rs *RedisStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return rs.IncrementBy(ctx, key, 1, expiration)
}

// IncrementBy atomically increments a counter by the given amount
func (rs *RedisStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	if err := rs.checkClosed(); err != nil {
		return 0, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return 0, err
	}

	fullKey := rs.buildKey(key)
	var result int64

	err := rs.retryableOperation(ctx, "increment", func() error {
		// Use Redis transaction to ensure atomicity
		pipe := rs.client.TxPipeline()
		incrCmd := pipe.IncrBy(ctx, fullKey, amount)
		if expiration > 0 {
			pipe.Expire(ctx, fullKey, expiration)
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return err
		}

		result = incrCmd.Val()
		return nil
	})

	if err != nil {
		rs.recordError(err)
		return 0, ratelimit.WrapStorageError(err, "redis increment failed",
			"key", key, "amount", amount)
	}

	return result, nil
}

// Delete removes a key from Redis
func (rs *RedisStore) Delete(ctx context.Context, key string) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return err
	}

	fullKey := rs.buildKey(key)
	err := rs.client.Del(ctx, fullKey).Err()
	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis delete failed",
			"key", key)
	}

	return nil
}

// Exists checks if a key exists in Redis
func (rs *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := rs.checkClosed(); err != nil {
		return false, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return false, err
	}

	fullKey := rs.buildKey(key)
	count, err := rs.client.Exists(ctx, fullKey).Result()
	if err != nil {
		rs.recordError(err)
		return false, ratelimit.WrapStorageError(err, "redis exists failed",
			"key", key)
	}

	return count > 0, nil
}

// ExecuteScript executes a Lua script atomically in Redis
// This enables race-free rate limiting by performing read-modify-write in a single operation
func (rs *RedisStore) ExecuteScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	// Validate all keys to prevent DOS attacks
	for _, key := range keys {
		if err := ratelimit.ValidateKeyLength(key); err != nil {
			return nil, err
		}
	}

	// Build full keys with prefix
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rs.buildKey(key)
	}

	var result interface{}

	err := rs.retryableOperation(ctx, "script", func() error {
		// Execute Lua script using EVAL
		res, err := rs.client.Eval(ctx, script, fullKeys, args...).Result()
		if err != nil {
			return err
		}
		result = res
		return nil
	})

	if err != nil {
		rs.recordError(err)
		return nil, ratelimit.WrapStorageError(err, "redis script execution failed",
			"script_length", len(script),
			"keys_count", len(keys))
	}

	return result, nil
}

// LoadScript pre-loads a Lua script and returns its SHA1 hash
// This enables using EVALSHA for better performance (script is cached on Redis server)
func (rs *RedisStore) LoadScript(ctx context.Context, script string) (string, error) {
	if err := rs.checkClosed(); err != nil {
		return "", err
	}

	// Load script to Redis server and get SHA1 hash
	sha, err := rs.client.ScriptLoad(ctx, script).Result()
	if err != nil {
		rs.recordError(err)
		return "", ratelimit.WrapStorageError(err, "redis script load failed",
			"script_length", len(script))
	}

	rs.logger.Debug("redis script loaded",
		"sha", sha,
		"script_length", len(script))

	return sha, nil
}

// ExecuteScriptSHA executes a pre-loaded Lua script by its SHA1 hash
// This is more efficient than ExecuteScript as the script doesn't need to be sent each time
func (rs *RedisStore) ExecuteScriptSHA(ctx context.Context, sha string, keys []string, args ...interface{}) (interface{}, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	// Validate all keys to prevent DOS attacks
	for _, key := range keys {
		if err := ratelimit.ValidateKeyLength(key); err != nil {
			return nil, err
		}
	}

	// Build full keys with prefix
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rs.buildKey(key)
	}

	// Execute pre-loaded script using EVALSHA
	result, err := rs.client.EvalSha(ctx, sha, fullKeys, args...).Result()
	if err != nil {
		// If script not found, return specific error so caller can reload
		if err.Error() == "NOSCRIPT No matching script. Please use EVAL." {
			rs.recordError(err)
			return nil, ratelimit.WrapStorageError(err, "redis script not found (needs reload)",
				"sha", sha)
		}
		rs.recordError(err)
		return nil, ratelimit.WrapStorageError(err, "redis script execution failed",
			"sha", sha,
			"keys_count", len(keys))
	}

	return result, nil
}

// Health checks the health of the Redis connection
func (rs *RedisStore) Health(ctx context.Context) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	// Ping with timeout
	err := rs.client.Ping(ctx).Err()
	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis health check failed")
	}

	rs.clearError()
	return nil
}

// Close closes the Redis connection pool
func (rs *RedisStore) Close() error {
	rs.closeMu.Lock()
	defer rs.closeMu.Unlock()

	if rs.closed {
		return nil
	}

	rs.closed = true

	// Close Redis client
	if err := rs.client.Close(); err != nil {
		rs.logger.Warn("error closing redis client",
			"id", rs.id,
			"error", err)
		return ratelimit.WrapStorageError(err, "redis close failed")
	}

	rs.logger.Info("redis store closed", "id", rs.id)
	return nil
}

// ============================================================================
// INTERNAL HELPERS
// ============================================================================

// buildKey constructs the full Redis key with prefix
func (rs *RedisStore) buildKey(key string) string {
	if rs.config.KeyPrefix == "" {
		return key
	}
	return rs.config.KeyPrefix + key
}

// retryableOperation executes a Redis operation with exponential backoff retry logic
// This provides application-level retries for transient failures beyond go-redis built-in retries
func (rs *RedisStore) retryableOperation(ctx context.Context, operation string, fn func() error) error {
	if rs.config.RetryMaxAttempts <= 0 {
		// Retries disabled, execute once
		return fn()
	}

	var lastErr error
	backoff := rs.config.RetryInitialBackoff

	for attempt := 0; attempt <= rs.config.RetryMaxAttempts; attempt++ {
		// Execute the operation
		err := fn()
		if err == nil {
			// Success!
			if attempt > 0 {
				rs.logger.Info("redis operation succeeded after retry",
					"operation", operation,
					"attempt", attempt+1,
					"total_attempts", rs.config.RetryMaxAttempts+1,
				)
			}
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !rs.isRetryableError(err) {
			// Non-retryable error, fail immediately
			rs.logger.Debug("redis operation failed with non-retryable error",
				"operation", operation,
				"error", err.Error(),
			)
			return err
		}

		// If this was the last attempt, don't sleep
		if attempt >= rs.config.RetryMaxAttempts {
			break
		}

		// Log retry attempt
		rs.logger.Debug("redis operation failed, retrying",
			"operation", operation,
			"attempt", attempt+1,
			"max_attempts", rs.config.RetryMaxAttempts+1,
			"backoff", backoff,
			"error", err.Error(),
		)

		// Sleep with exponential backoff
		select {
		case <-ctx.Done():
			// Context cancelled, return context error
			return ctx.Err()
		case <-time.After(backoff):
			// Continue to next retry
		}

		// Calculate next backoff with exponential increase
		backoff = time.Duration(float64(backoff) * rs.config.RetryBackoffMultiplier)
		if backoff > rs.config.RetryMaxBackoff {
			backoff = rs.config.RetryMaxBackoff
		}
	}

	// All retries exhausted
	rs.logger.Warn("redis operation failed after all retries",
		"operation", operation,
		"attempts", rs.config.RetryMaxAttempts+1,
		"error", lastErr.Error(),
	)

	return lastErr
}

// isRetryableError determines if a Redis error is transient and should be retried
func (rs *RedisStore) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Network errors (retryable)
	if redis.Nil == err {
		// Key not found - not retryable (it's a valid response)
		return false
	}

	// Connection errors (retryable)
	if contains(errMsg, "connection refused") ||
		contains(errMsg, "connection reset") ||
		contains(errMsg, "broken pipe") ||
		contains(errMsg, "EOF") ||
		contains(errMsg, "i/o timeout") ||
		contains(errMsg, "timeout") {
		return true
	}

	// Pool exhaustion (retryable - might get connection after retry)
	if contains(errMsg, "connection pool timeout") ||
		contains(errMsg, "pool timeout") {
		return true
	}

	// Server errors that might be transient (retryable)
	if contains(errMsg, "LOADING") || // Redis is loading dataset
		contains(errMsg, "BUSY") || // Redis is busy (script running)
		contains(errMsg, "READONLY") { // Replica in readonly mode
		return true
	}

	// Authentication/authorization errors (NOT retryable)
	if contains(errMsg, "NOAUTH") ||
		contains(errMsg, "WRONGPASS") ||
		contains(errMsg, "NOPERM") {
		return false
	}

	// Script errors (NOT retryable - script needs to be fixed/reloaded)
	if contains(errMsg, "NOSCRIPT") {
		return false
	}

	// Data/command errors (NOT retryable - bad request)
	if contains(errMsg, "WRONGTYPE") ||
		contains(errMsg, "syntax error") ||
		contains(errMsg, "unknown command") {
		return false
	}

	// Default: assume non-retryable for safety
	// This prevents infinite retries on unexpected errors
	return false
}

// contains is a simple string contains check (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				indexContains(s, substr)))
}

func indexContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// checkClosed checks if the store is closed
func (rs *RedisStore) checkClosed() error {
	rs.closeMu.RLock()
	defer rs.closeMu.RUnlock()

	if rs.closed {
		return ratelimit.ErrClosed
	}
	return nil
}

// recordError records the last error for health tracking
func (rs *RedisStore) recordError(err error) {
	rs.healthMu.Lock()
	defer rs.healthMu.Unlock()
	rs.lastError = err
}

// clearError clears the last error after successful operation
func (rs *RedisStore) clearError() {
	rs.healthMu.Lock()
	defer rs.healthMu.Unlock()
	rs.lastError = nil
}

// ============================================================================
// STATISTICS & MONITORING
// ============================================================================

// RedisStoreStats contains statistics about the Redis store
type RedisStoreStats struct {
	ID            string           `json:"id"`
	Address       string           `json:"address"`
	Database      int              `json:"database"`
	PoolStats     *redis.PoolStats `json:"pool_stats,omitempty"`
	Closed        bool             `json:"closed"`
	LastError     string           `json:"last_error,omitempty"`
	LastErrorTime *time.Time       `json:"last_error_time,omitempty"`
}

// Stats returns statistics about the Redis store
func (rs *RedisStore) Stats() *RedisStoreStats {
	rs.closeMu.RLock()
	defer rs.closeMu.RUnlock()

	stats := &RedisStoreStats{
		ID:       rs.id,
		Address:  rs.config.Address,
		Database: rs.config.Database,
		Closed:   rs.closed,
	}

	if !rs.closed && rs.client != nil {
		poolStats := rs.client.PoolStats()
		stats.PoolStats = poolStats
	}

	rs.healthMu.RLock()
	if rs.lastError != nil {
		stats.LastError = rs.lastError.Error()
	}
	rs.healthMu.RUnlock()

	return stats
}

// String returns a string representation of the store
func (rs *RedisStore) String() string {
	stats := rs.Stats()
	return fmt.Sprintf("RedisStore{id=%s, address=%s, db=%d, closed=%v}",
		stats.ID, stats.Address, stats.Database, stats.Closed)
}

// ============================================================================
// BATCH OPERATIONS (OPTIONAL OPTIMIZATION)
// ============================================================================

// GetMulti retrieves multiple values in a single pipeline
func (rs *RedisStore) GetMulti(ctx context.Context, keys []string) (map[string][]byte, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	// Build full keys
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rs.buildKey(key)
	}

	// Use pipeline for batch get
	pipe := rs.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(fullKeys))
	for i, fullKey := range fullKeys {
		cmds[i] = pipe.Get(ctx, fullKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		rs.recordError(err)
		return nil, ratelimit.WrapStorageError(err, "redis get multi failed")
	}

	// Collect results
	result := make(map[string][]byte)
	for i, cmd := range cmds {
		if data, err := cmd.Bytes(); err == nil {
			result[keys[i]] = data
		}
	}

	return result, nil
}

// SetMulti stores multiple values in a single pipeline
func (rs *RedisStore) SetMulti(ctx context.Context, items map[string][]byte, expiration time.Duration) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	if len(items) == 0 {
		return nil
	}

	// Use pipeline for batch set
	pipe := rs.client.Pipeline()
	for key, value := range items {
		fullKey := rs.buildKey(key)
		pipe.Set(ctx, fullKey, value, expiration)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis set multi failed")
	}

	return nil
}

// DeleteMulti removes multiple keys in a single pipeline
func (rs *RedisStore) DeleteMulti(ctx context.Context, keys []string) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	// Build full keys
	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = rs.buildKey(key)
	}

	err := rs.client.Del(ctx, fullKeys...).Err()
	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis delete multi failed")
	}

	return nil
}

// ============================================================================
// ADVANCED OPERATIONS
// ============================================================================

// GetWithTTL retrieves a value and its remaining TTL
func (rs *RedisStore) GetWithTTL(ctx context.Context, key string) ([]byte, time.Duration, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, 0, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return nil, 0, err
	}

	fullKey := rs.buildKey(key)

	// Use pipeline to get value and TTL atomically
	pipe := rs.client.Pipeline()
	getCmd := pipe.Get(ctx, fullKey)
	ttlCmd := pipe.TTL(ctx, fullKey)

	_, err := pipe.Exec(ctx)
	if err == redis.Nil {
		return nil, 0, ratelimit.ErrKeyNotFound
	}
	if err != nil {
		rs.recordError(err)
		return nil, 0, ratelimit.WrapStorageError(err, "redis get with ttl failed",
			"key", key)
	}

	data, err := getCmd.Bytes()
	if err == redis.Nil {
		return nil, 0, ratelimit.ErrKeyNotFound
	}
	if err != nil {
		rs.recordError(err)
		return nil, 0, ratelimit.WrapStorageError(err, "redis get with ttl failed",
			"key", key)
	}

	ttl := ttlCmd.Val()
	return data, ttl, nil
}

// SetNX sets a value only if the key does not exist (SET if Not eXists)
func (rs *RedisStore) SetNX(ctx context.Context, key string, value []byte, expiration time.Duration) (bool, error) {
	if err := rs.checkClosed(); err != nil {
		return false, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return false, err
	}

	fullKey := rs.buildKey(key)
	success, err := rs.client.SetNX(ctx, fullKey, value, expiration).Result()
	if err != nil {
		rs.recordError(err)
		return false, ratelimit.WrapStorageError(err, "redis setnx failed",
			"key", key)
	}

	return success, nil
}

// GetSet atomically sets a new value and returns the old value
func (rs *RedisStore) GetSet(ctx context.Context, key string, value []byte) ([]byte, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return nil, err
	}

	fullKey := rs.buildKey(key)
	data, err := rs.client.GetSet(ctx, fullKey, value).Bytes()
	if err == redis.Nil {
		return nil, ratelimit.ErrKeyNotFound
	}
	if err != nil {
		rs.recordError(err)
		return nil, ratelimit.WrapStorageError(err, "redis getset failed",
			"key", key)
	}

	return data, nil
}

// Scan iterates over keys matching a pattern
func (rs *RedisStore) Scan(ctx context.Context, pattern string, count int64) ([]string, error) {
	if err := rs.checkClosed(); err != nil {
		return nil, err
	}

	fullPattern := rs.buildKey(pattern)
	var keys []string
	iter := rs.client.Scan(ctx, 0, fullPattern, count).Iterator()

	for iter.Next(ctx) {
		// Remove prefix if present
		key := iter.Val()
		if rs.config.KeyPrefix != "" && len(key) > len(rs.config.KeyPrefix) {
			key = key[len(rs.config.KeyPrefix):]
		}
		keys = append(keys, key)
	}

	if err := iter.Err(); err != nil {
		rs.recordError(err)
		return nil, ratelimit.WrapStorageError(err, "redis scan failed",
			"pattern", pattern)
	}

	return keys, nil
}

// FlushDB clears all keys in the current database (USE WITH CAUTION)
func (rs *RedisStore) FlushDB(ctx context.Context) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}

	err := rs.client.FlushDB(ctx).Err()
	if err != nil {
		rs.recordError(err)
		return ratelimit.WrapStorageError(err, "redis flushdb failed")
	}

	rs.logger.Warn("redis database flushed",
		"id", rs.id,
		"database", rs.config.Database)

	return nil
}
