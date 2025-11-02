package stores

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	ratelimit "github.com/itsatony/gorly"
	nuts "github.com/vaudience/go-nuts"
)

// ============================================================================
// MEMORY STORE - Thread-safe in-memory store with sharding
// ============================================================================
//
// ⚠️  WARNING: NOT FOR PRODUCTION USE ⚠️
//
// The MemoryStore has CRITICAL LIMITATIONS that make it unsuitable for production:
//
// 1. RACE CONDITIONS (P0 Security): Non-atomic read-modify-write operations
//    can allow concurrent requests to bypass rate limits in high-traffic scenarios
//
// 2. NO SHARED STATE (P0 Availability): Each server instance maintains separate
//    state, multiplying effective rate limits by instance count (3 servers = 3x limit)
//
// 3. NO PERSISTENCE (P0 Availability): All rate limit state is lost on restart,
//    allowing users to reset quotas by waiting for restarts
//
// 4. MEMORY LEAKS (P1 Stability): High cardinality keys (many unique IPs/users)
//    can cause unbounded memory growth leading to OOM
//
// 5. NO ATOMIC OPERATIONS (P0 Security): Race condition window between Get/Set
//    operations enables rate limit bypasses under load
//
// ✅ USE REDIS STORE FOR PRODUCTION
//
// The Redis store provides:
// - Atomic Lua script execution (eliminates race conditions)
// - Shared state across all server instances
// - Persistence and crash recovery
// - Automatic TTL-based expiration
// - Battle-tested scalability
//
// See stores/PRODUCTION_SAFETY.md for detailed analysis and migration guide.
//
// ✅ ACCEPTABLE USE CASES FOR MEMORY STORE:
// - Development and testing
// - Single-instance, low-traffic applications (<100 req/s)
// - Non-critical rate limiting (e.g., internal tools)
// - Emergency fallback when Redis is unavailable
//
// ============================================================================

// MemoryStore implements Store interface using in-memory storage with sharding
// Thread-safe with automatic expiration cleanup
//
// WARNING: See package documentation above for critical production limitations
type MemoryStore struct {
	shards        []*shard
	shardCount    int
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
	closed        bool
	closeMu       sync.RWMutex
	logger        ratelimit.Logger
	id            string
}

// shard represents a single shard of the memory store
type shard struct {
	mu      sync.RWMutex
	data    map[string][]byte
	expires map[string]time.Time
}

// entry represents a stored value with metadata
type entry struct {
	Value     []byte    `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ============================================================================
// CONFIGURATION
// ============================================================================

// MemoryStoreConfig configures the memory store
type MemoryStoreConfig struct {
	// ShardCount is the number of shards for concurrent access
	// Higher values improve concurrency but use more memory
	ShardCount int

	// CleanupInterval is how often to clean up expired entries
	CleanupInterval time.Duration

	// MaxKeys is the maximum number of keys per shard (0 = unlimited)
	MaxKeys int

	// Logger for store operations (optional)
	Logger ratelimit.Logger
}

// DefaultMemoryStoreConfig returns default configuration
func DefaultMemoryStoreConfig() *MemoryStoreConfig {
	return &MemoryStoreConfig{
		ShardCount:      int(ratelimit.DefaultShardCount),
		CleanupInterval: time.Duration(ratelimit.DefaultCleanupIntervalSeconds) * time.Second,
		MaxKeys:         int(ratelimit.DefaultMaxKeys),
		Logger:          ratelimit.NewNopLogger(),
	}
}

// Validate validates the configuration
func (c *MemoryStoreConfig) Validate() error {
	if c.ShardCount < 1 {
		return ratelimit.WrapConfigError(nil, "shard count must be at least 1",
			"shard_count", c.ShardCount)
	}
	if c.CleanupInterval < time.Second {
		return ratelimit.WrapConfigError(nil, "cleanup interval must be at least 1 second",
			"cleanup_interval", c.CleanupInterval)
	}
	return nil
}

// ============================================================================
// CONSTRUCTOR
// ============================================================================

// NewMemoryStore creates a new in-memory store
//
// ⚠️  WARNING: NOT FOR PRODUCTION USE ⚠️
//
// MemoryStore has critical limitations (race conditions, no shared state,
// no persistence) that make it unsuitable for production deployments.
//
// FOR PRODUCTION: Use NewRedisStore() instead
// See stores/PRODUCTION_SAFETY.md for detailed comparison and migration guide
//
// ONLY use MemoryStore for:
// - Development and testing
// - Single-instance, low-traffic applications
// - Non-critical internal tools
func NewMemoryStore(config *MemoryStoreConfig) (*MemoryStore, error) {
	if config == nil {
		config = DefaultMemoryStoreConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	ms := &MemoryStore{
		shardCount:  config.ShardCount,
		shards:      make([]*shard, config.ShardCount),
		stopCleanup: make(chan struct{}),
		logger:      config.Logger,
		id:          nuts.NID(ratelimit.IDPrefixRateLimiter, 16),
	}

	// Initialize shards
	for i := 0; i < config.ShardCount; i++ {
		ms.shards[i] = &shard{
			data:    make(map[string][]byte),
			expires: make(map[string]time.Time),
		}
	}

	// Start cleanup goroutine
	ms.cleanupTicker = time.NewTicker(config.CleanupInterval)
	go ms.cleanupLoop()

	ms.logger.Info("memory store created",
		"id", ms.id,
		"shards", config.ShardCount,
		"cleanup_interval", config.CleanupInterval,
	)

	return ms, nil
}

// ============================================================================
// STORE INTERFACE IMPLEMENTATION
// ============================================================================

// Get retrieves a value from the store
func (ms *MemoryStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ms.checkClosed(); err != nil {
		return nil, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return nil, err
	}

	shard := ms.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	// Check expiration
	if expiry, exists := shard.expires[key]; exists && time.Now().After(expiry) {
		return nil, ratelimit.ErrKeyNotFound
	}

	value, exists := shard.data[key]
	if !exists {
		return nil, ratelimit.ErrKeyNotFound
	}

	// Return a copy to prevent external modification
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Set stores a value in the store with optional expiration
func (ms *MemoryStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	if err := ms.checkClosed(); err != nil {
		return err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return err
	}

	shard := ms.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Store a copy to prevent external modification
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	shard.data[key] = valueCopy

	if expiration > 0 {
		shard.expires[key] = time.Now().Add(expiration)
	} else {
		delete(shard.expires, key)
	}

	return nil
}

// Increment atomically increments a counter and returns the new value
func (ms *MemoryStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return ms.IncrementBy(ctx, key, 1, expiration)
}

// IncrementBy atomically increments a counter by the given amount
func (ms *MemoryStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	if err := ms.checkClosed(); err != nil {
		return 0, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return 0, err
	}

	shard := ms.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check expiration and clean up if expired
	if expiry, exists := shard.expires[key]; exists && time.Now().After(expiry) {
		delete(shard.data, key)
		delete(shard.expires, key)
	}

	// Get current value using binary encoding (much faster than JSON for int64)
	// This entire operation is atomic due to the shard lock held above
	var current int64
	if data, exists := shard.data[key]; exists {
		// Decode int64 from binary (8 bytes, big-endian)
		if len(data) >= 8 {
			current = int64(binary.BigEndian.Uint64(data))
		} else {
			// Fallback to JSON for backward compatibility (if data was stored as JSON)
			if err := json.Unmarshal(data, &current); err != nil {
				return 0, ratelimit.WrapStorageError(err, "increment",
					"key", key, "action", "decode")
			}
		}
	}

	// Increment (still under lock, so atomic)
	current += amount

	// Store new value using binary encoding (8 bytes for int64)
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, uint64(current))
	shard.data[key] = data

	// Update expiration
	if expiration > 0 {
		shard.expires[key] = time.Now().Add(expiration)
	}

	return current, nil
}

// Delete removes a key from the store
func (ms *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ms.checkClosed(); err != nil {
		return err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return err
	}

	shard := ms.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	delete(shard.data, key)
	delete(shard.expires, key)
	return nil
}

// Exists checks if a key exists in the store
func (ms *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	if err := ms.checkClosed(); err != nil {
		return false, err
	}

	// Validate key length to prevent DOS attacks
	if err := ratelimit.ValidateKeyLength(key); err != nil {
		return false, err
	}

	shard := ms.getShard(key)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	// Check expiration
	if expiry, exists := shard.expires[key]; exists && time.Now().After(expiry) {
		return false, nil
	}

	_, exists := shard.data[key]
	return exists, nil
}

// ExecuteScript returns ErrScriptNotSupported for memory store
// Memory store does not support Lua scripts - use Redis for atomic operations
// WARNING: Memory store has race conditions in concurrent scenarios
func (ms *MemoryStore) ExecuteScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return nil, ratelimit.ErrScriptNotSupported
}

// Health checks the health of the store
func (ms *MemoryStore) Health(ctx context.Context) error {
	if err := ms.checkClosed(); err != nil {
		return err
	}
	return nil
}

// Close cleanly shuts down the store
func (ms *MemoryStore) Close() error {
	ms.closeMu.Lock()
	defer ms.closeMu.Unlock()

	if ms.closed {
		return nil
	}

	ms.closed = true

	// Stop cleanup goroutine
	close(ms.stopCleanup)
	if ms.cleanupTicker != nil {
		ms.cleanupTicker.Stop()
	}

	// Clear all shards
	for _, shard := range ms.shards {
		shard.mu.Lock()
		shard.data = nil
		shard.expires = nil
		shard.mu.Unlock()
	}

	ms.logger.Info("memory store closed", "id", ms.id)
	return nil
}

// ============================================================================
// INTERNAL HELPERS
// ============================================================================

// getShard returns the shard for the given key using consistent hashing
func (ms *MemoryStore) getShard(key string) *shard {
	// Simple hash function - sum of byte values
	hash := uint32(0)
	for i := 0; i < len(key); i++ {
		hash = hash*31 + uint32(key[i])
	}
	return ms.shards[hash%uint32(ms.shardCount)]
}

// checkClosed checks if the store is closed
func (ms *MemoryStore) checkClosed() error {
	ms.closeMu.RLock()
	defer ms.closeMu.RUnlock()

	if ms.closed {
		return ratelimit.ErrClosed
	}
	return nil
}

// cleanupLoop periodically cleans up expired entries
func (ms *MemoryStore) cleanupLoop() {
	for {
		select {
		case <-ms.cleanupTicker.C:
			ms.cleanup()
		case <-ms.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries from all shards
func (ms *MemoryStore) cleanup() {
	now := time.Now()
	totalCleaned := 0

	ms.logger.Debug("starting cleanup", "id", ms.id)

	for shardIdx, shard := range ms.shards {
		shard.mu.Lock()

		keysToDelete := make([]string, 0)
		for key, expiry := range shard.expires {
			if now.After(expiry) {
				keysToDelete = append(keysToDelete, key)
			}
		}

		for _, key := range keysToDelete {
			delete(shard.data, key)
			delete(shard.expires, key)
			totalCleaned++
		}

		shard.mu.Unlock()

		if len(keysToDelete) > 0 {
			ms.logger.Debug("cleaned shard",
				"id", ms.id,
				"shard", shardIdx,
				"cleaned", len(keysToDelete),
			)
		}
	}

	if totalCleaned > 0 {
		ms.logger.Info("cleanup completed",
			"id", ms.id,
			"total_cleaned", totalCleaned,
		)
	}
}

// ============================================================================
// STATISTICS & MONITORING
// ============================================================================

// Stats returns statistics about the memory store
type MemoryStoreStats struct {
	ID            string `json:"id"`
	ShardCount    int    `json:"shard_count"`
	TotalKeys     int    `json:"total_keys"`
	TotalExpiring int    `json:"total_expiring"`
	Closed        bool   `json:"closed"`
}

// Stats returns statistics about the store
func (ms *MemoryStore) Stats() *MemoryStoreStats {
	ms.closeMu.RLock()
	defer ms.closeMu.RUnlock()

	stats := &MemoryStoreStats{
		ID:         ms.id,
		ShardCount: ms.shardCount,
		Closed:     ms.closed,
	}

	for _, shard := range ms.shards {
		shard.mu.RLock()
		stats.TotalKeys += len(shard.data)
		stats.TotalExpiring += len(shard.expires)
		shard.mu.RUnlock()
	}

	return stats
}

// String returns a string representation of the store
func (ms *MemoryStore) String() string {
	stats := ms.Stats()
	return fmt.Sprintf("MemoryStore{id=%s, shards=%d, keys=%d, expiring=%d, closed=%v}",
		stats.ID, stats.ShardCount, stats.TotalKeys, stats.TotalExpiring, stats.Closed)
}
