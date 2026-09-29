package stores

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	nuts "github.com/vaudience/go-nuts"

	ratelimit "github.com/itsatony/gorly"
)

// ============================================================================
// REDIS STORE - Production-ready Redis backend with connection pooling
// ============================================================================

// RedisStore implements Store interface using Redis as the backend
// Thread-safe with automatic connection pooling and health checks
type RedisStore struct {
	client redis.UniversalClient
	config *RedisStoreConfig
	// ownsClient is true when the store built the client (NewRedisStore) and
	// therefore closes it in Close. A client handed in via
	// NewRedisStoreFromClient belongs to the caller and is never closed here.
	ownsClient bool
	address    string
	database   int
	closed     bool
	closeMu    sync.RWMutex
	logger     ratelimit.Logger
	id         string
	healthMu   sync.RWMutex
	lastError  error
}

// ============================================================================
// CONFIGURATION
// ============================================================================

// RedisStoreConfig configures the Redis store
type RedisStoreConfig struct {
	// Address is the Redis server address (host:port)
	Address string

	// Username is the Redis 6+ ACL user sent with AUTH (optional). When empty,
	// a password-only AUTH is sent (the "default" user), exactly as before.
	// Managed Redis offerings that enforce ACLs refuse a password-only AUTH.
	Username string

	// Password for Redis authentication (optional)
	Password string

	// TLSConfig enables TLS when non-nil. It is cloned at construction, so
	// later mutation by the caller has no effect on the store. When ServerName
	// is empty, crypto/tls derives it from Address. nil (the default) keeps a
	// plaintext connection, as before. MinVersion below TLS 1.2 is refused.
	//
	// To trust a certificate that carries no usable hostname (e.g. a CN-only,
	// SAN-less self-signed cert, as Scaleway's managed Redis presents), use
	// PinnedLeafTLSConfig: it pins the exact LEAF certificate.
	//
	// ⛔ Do NOT hand-roll "InsecureSkipVerify + verify the chain against a CA
	// pool" unless the pool holds only that one leaf: with the hostname check
	// off, a chain check against a CA that also signs OTHER tenants' servers
	// (a provider-wide CA) accepts every one of their certificates too.
	// InsecureSkipVerify with no verifier at all is refused (it verifies nothing).
	TLSConfig *tls.Config

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

	// KeyPrefix is the prefix for all Redis keys (default "gorly:"). For
	// NewRedisStoreFromClient an empty KeyPrefix is replaced by the default,
	// because a caller-owned client usually shares its database with the rest
	// of the service and unprefixed keys would collide with it.
	KeyPrefix string

	// RetryMaxAttempts is the maximum number of application-level retry
	// attempts for transient failures. 0 disables them (only go-redis's own
	// MaxRetries apply) — note that a partial config literal therefore gets NO
	// application-level retries unless it sets this; negative is refused.
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
	if c.DialTimeout < minDialTimeout {
		return ratelimit.WrapConfigError(nil, "Dial timeout must be at least 1 second",
			"dial_timeout", c.DialTimeout)
	}
	return validateTLSConfig(c.TLSConfig)
}

// newGoRedisClient builds the client NewRedisStore owns (a seam for tests that
// must observe that a failed construction closes it).
var newGoRedisClient = redis.NewClient

// minDialTimeout is the least DialTimeout (and, for NewRedisStoreFromClient,
// startup-ping timeout) a config may set.
const minDialTimeout = time.Second

// validateTLSConfig refuses a TLS config that would verify nothing:
// InsecureSkipVerify with neither VerifyConnection nor VerifyPeerCertificate
// accepts ANY certificate, i.e. TLS without authentication. Pinning (skip the
// hostname check, verify the chain yourself) must say so with a verifier.
func validateTLSConfig(tc *tls.Config) error {
	if tc != nil && tc.MinVersion != 0 && tc.MinVersion < tls.VersionTLS12 {
		return ratelimit.WrapConfigError(nil, "TLS MinVersion below TLS 1.2 is not allowed",
			"tls_min_version", tc.MinVersion)
	}
	// InsecureSkipVerify turns the default verification off, so something must
	// replace it on EVERY handshake. Only VerifyConnection runs on resumed
	// sessions; VerifyPeerCertificate is skipped for them, so a config relying
	// on it alone accepts any resumed session unverified (v1.3.2).
	if tc != nil && tc.InsecureSkipVerify && tc.VerifyConnection == nil {
		return ratelimit.WrapConfigError(nil,
			"TLS InsecureSkipVerify requires VerifyConnection (VerifyPeerCertificate is not called on resumed sessions); use PinnedLeafTLSConfig",
			"tls_insecure_skip_verify", true)
	}
	return nil
}

// ============================================================================
// CONSTRUCTOR
// ============================================================================

// NewRedisStore creates a new Redis store with connection pooling. The store
// owns the client it builds and closes it in Close.
func NewRedisStore(config *RedisStoreConfig) (*RedisStore, error) {
	if config == nil {
		config = DefaultRedisStoreConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	var tlsConfig *tls.Config
	if config.TLSConfig != nil {
		tlsConfig = config.TLSConfig.Clone()
	}

	// Create Redis client with connection pooling
	client := newGoRedisClient(&redis.Options{
		Addr:         config.Address,
		Username:     config.Username,
		Password:     config.Password,
		DB:           config.Database,
		TLSConfig:    tlsConfig,
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
		client:     client,
		config:     config,
		ownsClient: true,
		address:    config.Address,
		database:   config.Database,
		logger:     config.Logger,
		id:         nuts.NID(ratelimit.IDPrefixRateLimiter, 16),
	}

	if err := rs.ping(config.DialTimeout); err != nil {
		// The store built this client; do not leak its pool on a failed boot.
		_ = client.Close()
		return nil, err
	}

	if config.Password != "" && tlsConfig == nil {
		rs.logger.Warn("redis store authenticates over PLAINTEXT: the password crosses the network unencrypted; set TLSConfig",
			"id", rs.id,
			"address", config.Address)
	}

	rs.logger.Info("redis store created",
		"id", rs.id,
		"address", config.Address,
		"database", config.Database,
		"pool_size", config.PoolSize,
		"tls", tlsConfig != nil,
		"acl_user", config.Username != "",
	)

	return rs, nil
}

// NewRedisStoreFromClient creates a store on top of a client the CALLER built
// and owns — the way to reuse one client factory (TLS, ACL user, pool, hooks)
// so the rate limiter can never open a weaker connection than the rest of the
// service. The store never closes client: Close only marks the store closed.
//
// config is optional (nil = DefaultRedisStoreConfig()). Only the store-level
// fields apply — KeyPrefix, Logger, the Retry* fields and DialTimeout (as the
// startup-ping timeout). The connection fields (Address, Username, Password,
// Database, TLSConfig, pool sizes and socket timeouts) belong to the client and
// are ignored. The client is pinged once; construction fails if it is
// unreachable.
//
// ⚠ With a *redis.ClusterClient, Scan walks only the node it lands on and
// multi-key scripts must keep their keys in one hash slot.
func NewRedisStoreFromClient(client redis.UniversalClient, config *RedisStoreConfig) (*RedisStore, error) {
	if client == nil {
		return nil, ratelimit.WrapConfigError(nil, "Redis client is required",
			"client", nil)
	}
	if config == nil {
		config = DefaultRedisStoreConfig()
	} else {
		// Copy: the store must not write defaults into the caller's struct.
		cfg := *config
		config = &cfg
	}
	if config.Logger == nil {
		config.Logger = ratelimit.NewNopLogger()
	}
	if config.DialTimeout == 0 {
		config.DialTimeout = DefaultRedisStoreConfig().DialTimeout
	}
	if config.DialTimeout < minDialTimeout {
		return nil, ratelimit.WrapConfigError(nil, "Dial timeout must be at least 1 second",
			"dial_timeout", config.DialTimeout)
	}
	if config.KeyPrefix == "" {
		// A caller-owned client usually shares its DB with the service:
		// never write unprefixed keys into it.
		config.KeyPrefix = DefaultRedisStoreConfig().KeyPrefix
	}
	// The default config's Address is not a caller statement; anything else is.
	ignoredConnFields := (config.Address != "" && config.Address != DefaultRedisStoreConfig().Address) ||
		config.Username != "" || config.Password != "" || config.TLSConfig != nil
	if config.RetryMaxAttempts < 0 {
		return nil, ratelimit.WrapConfigError(nil, "Retry max attempts cannot be negative",
			"retry_max_attempts", config.RetryMaxAttempts)
	}
	if config.RetryMaxAttempts > 0 {
		// Zero means "unset" for a partial literal: take the defaults rather
		// than retrying in a zero-backoff loop.
		def := DefaultRedisStoreConfig()
		if config.RetryInitialBackoff <= 0 {
			config.RetryInitialBackoff = def.RetryInitialBackoff
		}
		if config.RetryMaxBackoff <= 0 {
			config.RetryMaxBackoff = def.RetryMaxBackoff
		}
		if config.RetryBackoffMultiplier < 1 {
			config.RetryBackoffMultiplier = def.RetryBackoffMultiplier
		}
	}

	address, database := describeClient(client)
	plaintextPassword := clientSendsPlaintextPassword(client)
	// Stats/String read the store's own fields, not the ignored config ones.
	rs := &RedisStore{
		client:     client,
		config:     config,
		ownsClient: false,
		address:    address,
		database:   database,
		logger:     config.Logger,
		id:         nuts.NID(ratelimit.IDPrefixRateLimiter, 16),
	}

	if plaintextPassword {
		rs.logger.Warn("redis store authenticates over PLAINTEXT: the password crosses the network unencrypted; set TLSConfig",
			"id", rs.id,
			"address", address)
	}

	if ignoredConnFields {
		rs.logger.Warn("redis store: Address/Username/Password/TLSConfig are IGNORED with a caller-owned client; configure them on the client",
			"id", rs.id)
	}

	if err := rs.ping(config.DialTimeout); err != nil {
		return nil, err
	}

	rs.logger.Info("redis store created from caller-owned client",
		"id", rs.id,
		"address", address,
		"database", database,
	)

	return rs, nil
}

// describeClient reports the address and database of a client for Stats and
// logs. Only *redis.Client exposes both; other UniversalClients (Cluster,
// Failover/Sentinel, Ring) report "" / 0.
func describeClient(client redis.UniversalClient) (string, int) {
	if c, ok := client.(*redis.Client); ok {
		opts := c.Options()
		return opts.Addr, opts.DB
	}
	return "", 0
}

// clientSendsPlaintextPassword reports a *redis.Client configured with a
// password and no TLS. Other client kinds are not inspected.
func clientSendsPlaintextPassword(client redis.UniversalClient) bool {
	c, ok := client.(*redis.Client)
	if !ok {
		return false
	}
	opts := c.Options()
	return opts.Password != "" && opts.TLSConfig == nil
}

// ping verifies connectivity once at construction.
func (rs *RedisStore) ping(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := rs.client.Ping(ctx).Err(); err != nil {
		return ratelimit.WrapStorageError(err, "redis connection failed",
			"address", rs.address,
			"database", rs.database)
	}
	return nil
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

	// A caller-owned client (NewRedisStoreFromClient) is the caller's to close.
	if !rs.ownsClient {
		rs.logger.Info("redis store closed (caller-owned client left open)", "id", rs.id)
		return nil
	}

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
			// Context canceled, return context error
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
		Address:  rs.address,
		Database: rs.database,
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
			result[keys[i]] = data //nolint:gosec // G602: cmds has exactly one entry per key
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

// FlushDB clears ALL keys in the current database, not just this store's
// prefix (USE WITH CAUTION). It is only allowed on a client the store owns
// (NewRedisStore): a caller-owned client (NewRedisStoreFromClient) usually
// shares its database with the rest of the service, so FlushDB is refused
// there — delete your own keys instead. ⚠ With a cluster client FLUSHDB
// reaches only the node the command lands on.
func (rs *RedisStore) FlushDB(ctx context.Context) error {
	if err := rs.checkClosed(); err != nil {
		return err
	}
	if !rs.ownsClient {
		return ratelimit.WrapNotSupportedError(
			"FlushDB is not supported on a caller-owned client (it would wipe the whole shared database)",
			"owns_client", false)
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
