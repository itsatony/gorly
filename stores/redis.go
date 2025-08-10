// stores/redis.go
package stores

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig configures Redis store settings
type RedisConfig struct {
	Address     string        `yaml:"address" json:"address" mapstructure:"address"`
	Password    string        `yaml:"password" json:"password" mapstructure:"password"`
	Database    int           `yaml:"database" json:"database" mapstructure:"database"`
	PoolSize    int           `yaml:"pool_size" json:"pool_size" mapstructure:"pool_size"`
	MinIdleConn int           `yaml:"min_idle_conn" json:"min_idle_conn" mapstructure:"min_idle_conn"`
	MaxRetries  int           `yaml:"max_retries" json:"max_retries" mapstructure:"max_retries"`
	Timeout     time.Duration `yaml:"timeout" json:"timeout" mapstructure:"timeout"`
	TLS         bool          `yaml:"tls" json:"tls" mapstructure:"tls"`
}

// StoreError represents an error from the store
type StoreError struct {
	Type    string
	Message string
	Err     error
}

func (e *StoreError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// NewStoreError creates a new store error
func NewStoreError(errorType, message string, err error) *StoreError {
	return &StoreError{
		Type:    errorType,
		Message: message,
		Err:     err,
	}
}

// RedisStore implements the Store interface using Redis
type RedisStore struct {
	client *redis.Client
	config RedisConfig
}

// NewRedisStore creates a new Redis store
func NewRedisStore(config RedisConfig) (*RedisStore, error) {
	// Configure Redis client options
	opts := &redis.Options{
		Addr:         config.Address,
		Password:     config.Password,
		DB:           config.Database,
		PoolSize:     config.PoolSize,
		MinIdleConns: config.MinIdleConn,
		MaxRetries:   config.MaxRetries,
		DialTimeout:  config.Timeout,
		ReadTimeout:  config.Timeout,
		WriteTimeout: config.Timeout,
	}

	// Configure TLS if enabled
	if config.TLS {
		opts.TLSConfig = &tls.Config{
			InsecureSkipVerify: false,
		}
	}

	// Create Redis client
	client := redis.NewClient(opts)

	store := &RedisStore{
		client: client,
		config: config,
	}

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	if err := store.Health(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return store, nil
}

// Get retrieves a value from Redis
func (r *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, NewStoreError(
				"store",
				"key not found",
				err,
			)
		}
		return nil, NewStoreError(
			"store",
			"failed to get value from Redis",
			err,
		)
	}
	return val, nil
}

// Set stores a value in Redis with optional expiration
func (r *RedisStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	err := r.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return NewStoreError(
			"store",
			"failed to set value in Redis",
			err,
		)
	}
	return nil
}

// Increment atomically increments a counter and returns the new value
func (r *RedisStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return r.IncrementBy(ctx, key, 1, expiration)
}

// IncrementBy atomically increments a counter by the given amount
func (r *RedisStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	// Use a Lua script for atomic increment with expiration
	luaScript := `
		local current = redis.call('INCRBY', KEYS[1], ARGV[1])
		if tonumber(ARGV[2]) > 0 then
			redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return current
	`

	expirationSeconds := int64(expiration.Seconds())
	result, err := r.client.Eval(ctx, luaScript, []string{key}, amount, expirationSeconds).Int64()
	if err != nil {
		return 0, NewStoreError(
			"store",
			"failed to increment counter in Redis",
			err,
		)
	}

	return result, nil
}

// Delete removes a key from Redis
func (r *RedisStore) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return NewStoreError(
			"store",
			"failed to delete key from Redis",
			err,
		)
	}
	return nil
}

// Exists checks if a key exists in Redis
func (r *RedisStore) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, NewStoreError(
			"store",
			"failed to check key existence in Redis",
			err,
		)
	}
	return count > 0, nil
}

// Health checks the health of the Redis connection
func (r *RedisStore) Health(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		return NewStoreError(
			"network",
			"Redis health check failed",
			err,
		)
	}
	return nil
}

// Close closes the Redis connection
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// MultiGet retrieves multiple values at once for better performance
func (r *RedisStore) MultiGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	values, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, NewStoreError(
			"store",
			"failed to get multiple values from Redis",
			err,
		)
	}

	result := make(map[string][]byte)
	for i, value := range values {
		if value != nil {
			if strValue, ok := value.(string); ok {
				result[keys[i]] = []byte(strValue)
			}
		}
	}

	return result, nil
}

// MultiSet sets multiple values at once for better performance
func (r *RedisStore) MultiSet(ctx context.Context, keyValues map[string][]byte, expiration time.Duration) error {
	if len(keyValues) == 0 {
		return nil
	}

	// Use pipeline for better performance
	pipe := r.client.Pipeline()

	for key, value := range keyValues {
		pipe.Set(ctx, key, value, expiration)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return NewStoreError(
			"store",
			"failed to set multiple values in Redis",
			err,
		)
	}

	return nil
}

// IncrementMulti atomically increments multiple counters
func (r *RedisStore) IncrementMulti(ctx context.Context, keys []string, amounts []int64, expiration time.Duration) (map[string]int64, error) {
	if len(keys) != len(amounts) {
		return nil, NewStoreError(
			"config",
			"keys and amounts arrays must have the same length",
			nil,
		)
	}

	if len(keys) == 0 {
		return make(map[string]int64), nil
	}

	// Use pipeline for better performance
	pipe := r.client.Pipeline()

	luaScript := `
		local current = redis.call('INCRBY', KEYS[1], ARGV[1])
		if tonumber(ARGV[2]) > 0 then
			redis.call('EXPIRE', KEYS[1], ARGV[2])
		end
		return current
	`

	expirationSeconds := int64(expiration.Seconds())

	for i, key := range keys {
		pipe.Eval(ctx, luaScript, []string{key}, amounts[i], expirationSeconds)
	}

	results, err := pipe.Exec(ctx)
	if err != nil {
		return nil, NewStoreError(
			"store",
			"failed to increment multiple counters in Redis",
			err,
		)
	}

	resultMap := make(map[string]int64)
	for i, result := range results {
		if cmd, ok := result.(*redis.Cmd); ok {
			if val, err := cmd.Int64(); err == nil {
				resultMap[keys[i]] = val
			} else {
				return nil, NewStoreError(
					"store",
					fmt.Sprintf("failed to parse result for key %s", keys[i]),
					err,
				)
			}
		}
	}

	return resultMap, nil
}

// TTL returns the time-to-live for a key
func (r *RedisStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	duration, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, NewStoreError(
			"store",
			"failed to get TTL from Redis",
			err,
		)
	}
	return duration, nil
}

// Expire sets an expiration time for a key
func (r *RedisStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	err := r.client.Expire(ctx, key, expiration).Err()
	if err != nil {
		return NewStoreError(
			"store",
			"failed to set expiration in Redis",
			err,
		)
	}
	return nil
}

// GetClient returns the underlying Redis client for advanced operations
func (r *RedisStore) GetClient() *redis.Client {
	return r.client
}

// Stats returns Redis connection statistics
func (r *RedisStore) Stats() map[string]interface{} {
	stats := r.client.PoolStats()
	return map[string]interface{}{
		"hits":        stats.Hits,
		"misses":      stats.Misses,
		"timeouts":    stats.Timeouts,
		"total_conns": stats.TotalConns,
		"idle_conns":  stats.IdleConns,
		"stale_conns": stats.StaleConns,
	}
}
