// stores/memory.go
package stores

import (
	"context"
	"sync"
	"time"
)

// MemoryConfig configures memory store settings
type MemoryConfig struct {
	MaxKeys         int           `yaml:"max_keys" json:"max_keys" mapstructure:"max_keys"`                         // Maximum number of keys to store (0 for unlimited)
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval" mapstructure:"cleanup_interval"` // How often to clean up expired keys
	DefaultTTL      time.Duration `yaml:"default_ttl" json:"default_ttl" mapstructure:"default_ttl"`                // Default TTL for keys without explicit expiration
}

// MemoryItem represents a stored item with metadata
type MemoryItem struct {
	Value     []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

// IsExpired checks if the item has expired
func (mi *MemoryItem) IsExpired() bool {
	return !mi.ExpiresAt.IsZero() && time.Now().After(mi.ExpiresAt)
}

// MemoryStore implements the Store interface using in-memory storage
type MemoryStore struct {
	mu             sync.RWMutex
	data           map[string]*MemoryItem
	config         MemoryConfig
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupRunning bool

	// Statistics (protected by separate mutex to avoid read/write lock conflicts)
	statsMu sync.Mutex
	stats   struct {
		gets    int64
		sets    int64
		deletes int64
		hits    int64
		misses  int64
		expired int64
		evicted int64
	}
}

// NewMemoryStore creates a new in-memory store
func NewMemoryStore(config MemoryConfig) (*MemoryStore, error) {
	// Set defaults
	if config.MaxKeys == 0 {
		config.MaxKeys = 1000000 // 1M keys default limit
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 5 * time.Minute // Cleanup every 5 minutes
	}
	if config.DefaultTTL == 0 {
		config.DefaultTTL = time.Hour // 1 hour default TTL
	}

	store := &MemoryStore{
		data:        make(map[string]*MemoryItem),
		config:      config,
		cleanupStop: make(chan struct{}),
	}

	// Start cleanup goroutine
	store.startCleanup()

	return store, nil
}

// Get retrieves a value from memory
func (m *MemoryStore) Get(ctx context.Context, key string) ([]byte, error) {
	// Update stats first
	m.statsMu.Lock()
	m.stats.gets++
	m.statsMu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		m.statsMu.Lock()
		m.stats.misses++
		m.statsMu.Unlock()
		return nil, NewStoreError(
			"store",
			"key not found",
			nil,
		)
	}

	// Check if expired
	if item.IsExpired() {
		m.statsMu.Lock()
		m.stats.misses++
		m.stats.expired++
		m.statsMu.Unlock()
		// Note: We don't delete expired items here to avoid lock upgrade
		// They'll be cleaned up by the cleanup goroutine
		return nil, NewStoreError(
			"store",
			"key not found",
			nil,
		)
	}

	m.statsMu.Lock()
	m.stats.hits++
	m.statsMu.Unlock()

	// Return a copy to prevent external modification
	result := make([]byte, len(item.Value))
	copy(result, item.Value)
	return result, nil
}

// Set stores a value in memory with optional expiration
func (m *MemoryStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	// Update stats
	m.statsMu.Lock()
	m.stats.sets++
	m.statsMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we need to evict items due to max keys limit
	if len(m.data) >= m.config.MaxKeys {
		if err := m.evictLRU(); err != nil {
			return err
		}
	}

	// Calculate expiration time
	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	} else if m.config.DefaultTTL > 0 {
		expiresAt = time.Now().Add(m.config.DefaultTTL)
	}

	// Store a copy to prevent external modification
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	m.data[key] = &MemoryItem{
		Value:     valueCopy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	return nil
}

// Increment atomically increments a counter and returns the new value
func (m *MemoryStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return m.IncrementBy(ctx, key, 1, expiration)
}

// IncrementBy atomically increments a counter by the given amount
func (m *MemoryStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	var currentValue int64 = 0

	// If item exists and not expired, try to parse its value
	if exists && !item.IsExpired() {
		if len(item.Value) == 8 {
			// Assume it's a 64-bit integer stored in binary format
			for i := 0; i < 8; i++ {
				currentValue |= int64(item.Value[i]) << (8 * (7 - i))
			}
		}
	}

	// Increment the value
	newValue := currentValue + amount

	// Convert to bytes (big-endian)
	valueBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		valueBytes[i] = byte(newValue >> (8 * (7 - i)))
	}

	// Store the new value
	if err := m.setWithLock(key, valueBytes, expiration); err != nil {
		return 0, err
	}

	return newValue, nil
}

// setWithLock is an internal method that assumes the mutex is already held
func (m *MemoryStore) setWithLock(key string, value []byte, expiration time.Duration) error {
	// Check if we need to evict items due to max keys limit
	if len(m.data) >= m.config.MaxKeys {
		if err := m.evictLRU(); err != nil {
			return err
		}
	}

	// Calculate expiration time
	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	} else if m.config.DefaultTTL > 0 {
		expiresAt = time.Now().Add(m.config.DefaultTTL)
	}

	// Store a copy to prevent external modification
	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)

	m.data[key] = &MemoryItem{
		Value:     valueCopy,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}

	return nil
}

// Delete removes a key from memory
func (m *MemoryStore) Delete(ctx context.Context, key string) error {
	// Update stats
	m.statsMu.Lock()
	m.stats.deletes++
	m.statsMu.Unlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// Exists checks if a key exists in memory
func (m *MemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return false, nil
	}

	// Check if expired
	if item.IsExpired() {
		return false, nil
	}

	return true, nil
}

// Health checks the health of the memory store (always healthy)
func (m *MemoryStore) Health(ctx context.Context) error {
	return nil
}

// Close cleans up resources used by the memory store
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop cleanup goroutine
	m.stopCleanup()

	// Clear all data
	m.data = nil

	return nil
}

// MultiGet retrieves multiple values at once
func (m *MemoryStore) MultiGet(ctx context.Context, keys []string) (map[string][]byte, error) {
	if len(keys) == 0 {
		return make(map[string][]byte), nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]byte)
	for _, key := range keys {
		item, exists := m.data[key]
		if exists && !item.IsExpired() {
			// Return a copy to prevent external modification
			valueCopy := make([]byte, len(item.Value))
			copy(valueCopy, item.Value)
			result[key] = valueCopy
		}
	}

	return result, nil
}

// MultiSet sets multiple values at once
func (m *MemoryStore) MultiSet(ctx context.Context, keyValues map[string][]byte, expiration time.Duration) error {
	if len(keyValues) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, value := range keyValues {
		if err := m.setWithLock(key, value, expiration); err != nil {
			return err
		}
	}

	return nil
}

// IncrementMulti atomically increments multiple counters
func (m *MemoryStore) IncrementMulti(ctx context.Context, keys []string, amounts []int64, expiration time.Duration) (map[string]int64, error) {
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

	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]int64)
	for i, key := range keys {
		item, exists := m.data[key]
		var currentValue int64 = 0

		// If item exists and not expired, try to parse its value
		if exists && !item.IsExpired() {
			if len(item.Value) == 8 {
				// Assume it's a 64-bit integer stored in binary format
				for j := 0; j < 8; j++ {
					currentValue |= int64(item.Value[j]) << (8 * (7 - j))
				}
			}
		}

		// Increment the value
		newValue := currentValue + amounts[i]

		// Convert to bytes (big-endian)
		valueBytes := make([]byte, 8)
		for j := 0; j < 8; j++ {
			valueBytes[j] = byte(newValue >> (8 * (7 - j)))
		}

		// Store the new value
		if err := m.setWithLock(key, valueBytes, expiration); err != nil {
			return nil, err
		}

		result[key] = newValue
	}

	return result, nil
}

// TTL returns the time-to-live for a key
func (m *MemoryStore) TTL(ctx context.Context, key string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists || item.IsExpired() {
		return -2 * time.Second, nil // Redis convention: -2 means key doesn't exist
	}

	if item.ExpiresAt.IsZero() {
		return -1 * time.Second, nil // Redis convention: -1 means no expiration
	}

	remaining := time.Until(item.ExpiresAt)
	if remaining <= 0 {
		return -2 * time.Second, nil // Already expired
	}

	return remaining, nil
}

// Expire sets an expiration time for a key
func (m *MemoryStore) Expire(ctx context.Context, key string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists || item.IsExpired() {
		return NewStoreError(
			"store",
			"key not found",
			nil,
		)
	}

	// Update expiration time
	item.ExpiresAt = time.Now().Add(expiration)
	return nil
}

// Stats returns memory store statistics
func (m *MemoryStore) Stats() map[string]interface{} {
	m.mu.RLock()
	totalKeys := len(m.data)
	m.mu.RUnlock()

	m.statsMu.Lock()
	statsCopy := m.stats
	m.statsMu.Unlock()

	return map[string]interface{}{
		"total_keys":       totalKeys,
		"gets":             statsCopy.gets,
		"sets":             statsCopy.sets,
		"deletes":          statsCopy.deletes,
		"hits":             statsCopy.hits,
		"misses":           statsCopy.misses,
		"expired":          statsCopy.expired,
		"evicted":          statsCopy.evicted,
		"max_keys":         m.config.MaxKeys,
		"cleanup_interval": m.config.CleanupInterval.String(),
		"default_ttl":      m.config.DefaultTTL.String(),
	}
}

// startCleanup starts the background cleanup goroutine
func (m *MemoryStore) startCleanup() {
	if m.config.CleanupInterval <= 0 {
		return // Cleanup disabled
	}

	m.cleanupTicker = time.NewTicker(m.config.CleanupInterval)
	m.cleanupRunning = true

	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				m.cleanupExpired()
			case <-m.cleanupStop:
				return
			}
		}
	}()
}

// stopCleanup stops the background cleanup goroutine
func (m *MemoryStore) stopCleanup() {
	if m.cleanupRunning {
		m.cleanupRunning = false
		close(m.cleanupStop)
		if m.cleanupTicker != nil {
			m.cleanupTicker.Stop()
		}
	}
}

// cleanupExpired removes expired items from memory
func (m *MemoryStore) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expiredCount := int64(0)
	for key, item := range m.data {
		if !item.ExpiresAt.IsZero() && now.After(item.ExpiresAt) {
			delete(m.data, key)
			expiredCount++
		}
	}

	// Update stats if any items were expired
	if expiredCount > 0 {
		m.statsMu.Lock()
		m.stats.expired += expiredCount
		m.statsMu.Unlock()
	}
}

// evictLRU evicts the least recently used items to make room for new ones
func (m *MemoryStore) evictLRU() error {
	// Find the oldest item by CreatedAt
	var oldestKey string
	var oldestTime time.Time

	for key, item := range m.data {
		if oldestKey == "" || item.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = item.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(m.data, oldestKey)
		m.statsMu.Lock()
		m.stats.evicted++
		m.statsMu.Unlock()
	}

	return nil
}

// Clear removes all items from the store (useful for testing)
func (m *MemoryStore) Clear() {
	m.mu.Lock()
	m.data = make(map[string]*MemoryItem)
	m.mu.Unlock()

	// Reset stats
	m.statsMu.Lock()
	m.stats.gets = 0
	m.stats.sets = 0
	m.stats.deletes = 0
	m.stats.hits = 0
	m.stats.misses = 0
	m.stats.expired = 0
	m.stats.evicted = 0
	m.statsMu.Unlock()
}

// Size returns the current number of items in the store
func (m *MemoryStore) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}
