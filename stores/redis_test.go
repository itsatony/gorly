package stores

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

// ============================================================================
// TEST UTILITIES
// ============================================================================

// skipIfNoRedis skips the test if Redis is not available
func skipIfNoRedis(t *testing.T, store *RedisStore) {
	if store == nil {
		t.Skip("Redis not available, skipping test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := store.Health(ctx); err != nil {
		t.Skip("Redis not available, skipping test")
	}
}

// setupRedisTest creates a test Redis store
func setupRedisTest(t *testing.T) *RedisStore {
	config := &RedisStoreConfig{
		Address:      "localhost:6379",
		Password:     "",
		Database:     15, // Use DB 15 for testing
		PoolSize:     5,
		MinIdleConns: 1,
		MaxRetries:   3,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
		PoolTimeout:  2 * time.Second,
		IdleTimeout:  1 * time.Minute,
		MaxConnAge:   5 * time.Minute,
		Logger:       ratelimit.NewNopLogger(),
		KeyPrefix:    "test:",
	}

	store, err := NewRedisStore(config)
	if err != nil {
		t.Logf("Cannot create Redis store: %v", err)
		return nil
	}

	// Clean test database
	ctx := context.Background()
	_ = store.FlushDB(ctx)

	return store
}

// cleanupRedisTest cleans up after test
func cleanupRedisTest(t *testing.T, store *RedisStore) {
	if store == nil {
		return
	}

	ctx := context.Background()
	_ = store.FlushDB(ctx)
	_ = store.Close()
}

// ============================================================================
// CONFIGURATION TESTS
// ============================================================================

func TestDefaultRedisStoreConfig(t *testing.T) {
	config := DefaultRedisStoreConfig()

	if config.Address == "" {
		t.Error("Address should not be empty")
	}
	if config.PoolSize < 1 {
		t.Error("PoolSize should be at least 1")
	}
	if config.DialTimeout < time.Second {
		t.Error("DialTimeout should be at least 1 second")
	}
}

func TestRedisStoreConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *RedisStoreConfig
		wantError bool
	}{
		{
			name:      "valid config",
			config:    DefaultRedisStoreConfig(),
			wantError: false,
		},
		{
			name: "empty address",
			config: &RedisStoreConfig{
				Address:     "",
				PoolSize:    5,
				DialTimeout: 2 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid database - negative",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				Database:    -1,
				PoolSize:    5,
				DialTimeout: 2 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid database - too high",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				Database:    16,
				PoolSize:    5,
				DialTimeout: 2 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid pool size",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				PoolSize:    0,
				DialTimeout: 2 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid dial timeout",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				PoolSize:    5,
				DialTimeout: 500 * time.Millisecond,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// ============================================================================
// BASIC FUNCTIONALITY TESTS
// ============================================================================

func TestNewRedisStore(t *testing.T) {
	store := setupRedisTest(t)
	if store == nil {
		t.Skip("Redis not available")
	}
	defer cleanupRedisTest(t, store)

	if store.id == "" {
		t.Error("Store ID should not be empty")
	}
	if store.client == nil {
		t.Error("Redis client should not be nil")
	}
}

func TestRedisStoreSetGet(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "test_key"
	value := []byte("test_value")

	// Set
	err := store.Set(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	retrieved, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(retrieved) != string(value) {
		t.Errorf("expected %s, got %s", value, retrieved)
	}
}

func TestRedisStoreGetNonExistent(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	_, err := store.Get(ctx, "nonexistent_key")

	if err != ratelimit.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestRedisStoreDelete(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "test_key"
	value := []byte("test_value")

	// Set then delete
	store.Set(ctx, key, value, 0)
	err := store.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, key)
	if err != ratelimit.ErrKeyNotFound {
		t.Error("key should not exist after deletion")
	}
}

func TestRedisStoreExists(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "test_key"

	// Should not exist initially
	exists, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("key should not exist")
	}

	// Set and check again
	store.Set(ctx, key, []byte("value"), 0)
	exists, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("key should exist")
	}
}

// ============================================================================
// INCREMENT TESTS
// ============================================================================

func TestRedisStoreIncrement(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "counter"

	// Increment from 0
	val, err := store.Increment(ctx, key, 0)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Increment again
	val, err = store.Increment(ctx, key, 0)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 2 {
		t.Errorf("expected 2, got %d", val)
	}
}

func TestRedisStoreIncrementBy(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "counter"

	// Increment by 5
	val, err := store.IncrementBy(ctx, key, 5, 0)
	if err != nil {
		t.Fatalf("IncrementBy failed: %v", err)
	}
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	// Increment by 3
	val, err = store.IncrementBy(ctx, key, 3, 0)
	if err != nil {
		t.Fatalf("IncrementBy failed: %v", err)
	}
	if val != 8 {
		t.Errorf("expected 8, got %d", val)
	}
}

// ============================================================================
// EXPIRATION TESTS
// ============================================================================

func TestRedisStoreExpiration(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "expiring_key"
	value := []byte("expiring_value")

	// Set with short expiration
	err := store.Set(ctx, key, value, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should exist immediately
	retrieved, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retrieved) != string(value) {
		t.Error("value mismatch")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not exist after expiration
	_, err = store.Get(ctx, key)
	if err != ratelimit.ErrKeyNotFound {
		t.Error("key should have expired")
	}
}

func TestRedisStoreIncrementExpiration(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "expiring_counter"

	// Increment with expiration (use 1s minimum as Redis client requires)
	val, err := store.Increment(ctx, key, 1*time.Second)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Wait for expiration
	time.Sleep(1200 * time.Millisecond)

	// Next increment should start from 1 again
	val, err = store.Increment(ctx, key, 1*time.Second)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1 after expiration, got %d", val)
	}
}

// ============================================================================
// HEALTH & LIFECYCLE TESTS
// ============================================================================

func TestRedisStoreHealth(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	err := store.Health(ctx)
	if err != nil {
		t.Errorf("healthy store should return nil: %v", err)
	}
}

func TestRedisStoreClose(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)

	ctx := context.Background()
	store.Set(ctx, "key", []byte("value"), 0)

	// Close store
	err := store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations should fail after close
	_, err = store.Get(ctx, "key")
	if err != ratelimit.ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}

	// Double close should be safe
	err = store.Close()
	if err != nil {
		t.Errorf("double close should not error: %v", err)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestRedisStoreConcurrentWrites(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	var wg sync.WaitGroup
	goroutines := 50

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("concurrent_key_%d", id)
			value := []byte(fmt.Sprintf("value_%d", id))
			store.Set(ctx, key, value, 0)
		}(i)
	}

	wg.Wait()

	// Verify all keys were written
	for i := 0; i < goroutines; i++ {
		key := fmt.Sprintf("concurrent_key_%d", i)
		exists, err := store.Exists(ctx, key)
		if err != nil {
			t.Errorf("Exists failed for key %s: %v", key, err)
		}
		if !exists {
			t.Errorf("key %s should exist", key)
		}
	}
}

func TestRedisStoreConcurrentIncrements(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "concurrent_counter"
	goroutines := 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = store.Increment(ctx, key, 0)
		}()
	}

	wg.Wait()

	// Get final count - should be exactly goroutines
	val, err := store.client.Get(ctx, store.buildKey(key)).Int64()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if val != int64(goroutines) {
		t.Errorf("expected count %d, got %d", goroutines, val)
	}
}

// ============================================================================
// BATCH OPERATION TESTS
// ============================================================================

func TestRedisStoreGetMulti(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Set multiple keys
	items := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	for key, value := range items {
		store.Set(ctx, key, value, 0)
	}

	// Get multiple keys
	keys := []string{"key1", "key2", "key3"}
	results, err := store.GetMulti(ctx, keys)
	if err != nil {
		t.Fatalf("GetMulti failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	for key, expectedValue := range items {
		if gotValue, exists := results[key]; !exists {
			t.Errorf("key %s missing from results", key)
		} else if string(gotValue) != string(expectedValue) {
			t.Errorf("key %s: expected %s, got %s", key, expectedValue, gotValue)
		}
	}
}

func TestRedisStoreSetMulti(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	items := map[string][]byte{
		"batch_key1": []byte("batch_value1"),
		"batch_key2": []byte("batch_value2"),
		"batch_key3": []byte("batch_value3"),
	}

	err := store.SetMulti(ctx, items, 0)
	if err != nil {
		t.Fatalf("SetMulti failed: %v", err)
	}

	// Verify all keys were set
	for key, expectedValue := range items {
		value, err := store.Get(ctx, key)
		if err != nil {
			t.Errorf("Get failed for key %s: %v", key, err)
		}
		if string(value) != string(expectedValue) {
			t.Errorf("key %s: expected %s, got %s", key, expectedValue, value)
		}
	}
}

func TestRedisStoreDeleteMulti(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Set multiple keys
	keys := []string{"del_key1", "del_key2", "del_key3"}
	for _, key := range keys {
		store.Set(ctx, key, []byte("value"), 0)
	}

	// Delete multiple keys
	err := store.DeleteMulti(ctx, keys)
	if err != nil {
		t.Fatalf("DeleteMulti failed: %v", err)
	}

	// Verify all keys were deleted
	for _, key := range keys {
		exists, err := store.Exists(ctx, key)
		if err != nil {
			t.Errorf("Exists failed for key %s: %v", key, err)
		}
		if exists {
			t.Errorf("key %s should not exist after deletion", key)
		}
	}
}

// ============================================================================
// ADVANCED OPERATION TESTS
// ============================================================================

func TestRedisStoreGetWithTTL(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "ttl_key"
	value := []byte("ttl_value")
	expiration := 10 * time.Second

	// Set with expiration
	err := store.Set(ctx, key, value, expiration)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get with TTL
	retrievedValue, ttl, err := store.GetWithTTL(ctx, key)
	if err != nil {
		t.Fatalf("GetWithTTL failed: %v", err)
	}

	if string(retrievedValue) != string(value) {
		t.Errorf("value mismatch: expected %s, got %s", value, retrievedValue)
	}

	if ttl <= 0 || ttl > expiration {
		t.Errorf("expected TTL between 0 and %v, got %v", expiration, ttl)
	}
}

func TestRedisStoreSetNX(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "setnx_key"
	value1 := []byte("value1")
	value2 := []byte("value2")

	// First SetNX should succeed
	success, err := store.SetNX(ctx, key, value1, 0)
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if !success {
		t.Error("first SetNX should succeed")
	}

	// Second SetNX should fail (key exists)
	success, err = store.SetNX(ctx, key, value2, 0)
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if success {
		t.Error("second SetNX should fail (key exists)")
	}

	// Verify original value is preserved
	retrievedValue, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(retrievedValue) != string(value1) {
		t.Errorf("expected %s, got %s", value1, retrievedValue)
	}
}

func TestRedisStoreScan(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Set multiple keys with a pattern
	prefix := "scan_test_"
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("%s%d", prefix, i)
		store.Set(ctx, key, []byte("value"), 0)
	}

	// Scan for keys
	pattern := prefix + "*"
	keys, err := store.Scan(ctx, pattern, 100)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(keys) != 5 {
		t.Errorf("expected 5 keys, got %d", len(keys))
	}
}

// ============================================================================
// STATISTICS TESTS
// ============================================================================

func TestRedisStoreStats(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	stats := store.Stats()

	if stats.ID == "" {
		t.Error("stats ID should not be empty")
	}
	if stats.Address == "" {
		t.Error("stats Address should not be empty")
	}
	if stats.Closed {
		t.Error("stats Closed should be false for open store")
	}
	if stats.PoolStats == nil {
		t.Error("stats PoolStats should not be nil for open store")
	}
}

func TestRedisStoreString(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	str := store.String()
	if len(str) == 0 {
		t.Error("String() should not be empty")
	}
}

// ============================================================================
// SCRIPT EXECUTION TESTS
// ============================================================================

func TestRedisStoreExecuteScript(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Simple Lua script to set a value
	script := `
		redis.call('SET', KEYS[1], ARGV[1])
		return redis.call('GET', KEYS[1])
	`

	result, err := store.ExecuteScript(ctx, script, []string{"script_test_key"}, "test_value")
	if err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRedisStoreLoadScript(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	script := `return "hello from lua"`

	sha, err := store.LoadScript(ctx, script)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	if sha == "" {
		t.Error("expected non-empty SHA")
	}
}

func TestRedisStoreExecuteScriptSHA(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Load script first
	script := `return {KEYS[1], ARGV[1]}`
	sha, err := store.LoadScript(ctx, script)
	if err != nil {
		t.Fatalf("LoadScript failed: %v", err)
	}

	// Execute using SHA
	result, err := store.ExecuteScriptSHA(ctx, sha, []string{"key1"}, "arg1")
	if err != nil {
		t.Fatalf("ExecuteScriptSHA failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

// ============================================================================
// GETSET TESTS
// ============================================================================

func TestRedisStoreGetSet(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "getset_key"
	oldValue := []byte("old_value")
	newValue := []byte("new_value")

	// Set initial value
	store.Set(ctx, key, oldValue, 0)

	// GetSet - should return old value and set new value
	retrieved, err := store.GetSet(ctx, key, newValue)
	if err != nil {
		t.Fatalf("GetSet failed: %v", err)
	}

	if string(retrieved) != string(oldValue) {
		t.Errorf("expected old value %s, got %s", oldValue, retrieved)
	}

	// Verify new value was set
	current, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(current) != string(newValue) {
		t.Errorf("expected new value %s, got %s", newValue, current)
	}
}

func TestRedisStoreGetSetNonExistent(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()
	key := "getset_nonexistent"
	value := []byte("value")

	// GetSet on non-existent key should return ErrKeyNotFound
	_, err := store.GetSet(ctx, key, value)
	if err != ratelimit.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

// ============================================================================
// ERROR PATH TESTS
// ============================================================================

func TestRedisStoreOperationsAfterClose(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)

	ctx := context.Background()

	// Close the store
	store.Close()

	// All operations should return ErrClosed
	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Get",
			fn: func() error {
				_, err := store.Get(ctx, "key")
				return err
			},
		},
		{
			name: "Set",
			fn: func() error {
				return store.Set(ctx, "key", []byte("value"), 0)
			},
		},
		{
			name: "Delete",
			fn: func() error {
				return store.Delete(ctx, "key")
			},
		},
		{
			name: "Exists",
			fn: func() error {
				_, err := store.Exists(ctx, "key")
				return err
			},
		},
		{
			name: "Increment",
			fn: func() error {
				_, err := store.Increment(ctx, "key", 0)
				return err
			},
		},
		{
			name: "IncrementBy",
			fn: func() error {
				_, err := store.IncrementBy(ctx, "key", 1, 0)
				return err
			},
		},
		{
			name: "Health",
			fn: func() error {
				return store.Health(ctx)
			},
		},
		{
			name: "ExecuteScript",
			fn: func() error {
				_, err := store.ExecuteScript(ctx, "return 1", []string{})
				return err
			},
		},
		{
			name: "GetMulti",
			fn: func() error {
				_, err := store.GetMulti(ctx, []string{"key"})
				return err
			},
		},
		{
			name: "SetMulti",
			fn: func() error {
				return store.SetMulti(ctx, map[string][]byte{"key": []byte("value")}, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != ratelimit.ErrClosed {
				t.Errorf("%s: expected ErrClosed, got %v", tt.name, err)
			}
		})
	}
}

func TestRedisStoreEmptyKeys(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// GetMulti with empty keys
	results, err := store.GetMulti(ctx, []string{})
	if err != nil {
		t.Errorf("GetMulti with empty keys should not error: %v", err)
	}
	if len(results) != 0 {
		t.Error("GetMulti with empty keys should return empty map")
	}

	// SetMulti with empty items
	err = store.SetMulti(ctx, map[string][]byte{}, 0)
	if err != nil {
		t.Errorf("SetMulti with empty items should not error: %v", err)
	}

	// DeleteMulti with empty keys
	err = store.DeleteMulti(ctx, []string{})
	if err != nil {
		t.Errorf("DeleteMulti with empty keys should not error: %v", err)
	}
}

func TestRedisStoreGetWithTTLNonExistent(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	_, _, err := store.GetWithTTL(ctx, "nonexistent_ttl_key")
	if err != ratelimit.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestRedisStoreScanEmptyPattern(t *testing.T) {
	store := setupRedisTest(t)
	skipIfNoRedis(t, store)
	defer cleanupRedisTest(t, store)

	ctx := context.Background()

	// Scan with empty pattern should work
	keys, err := store.Scan(ctx, "", 10)
	if err != nil {
		t.Errorf("Scan with empty pattern should not error: %v", err)
	}
	// May return keys or empty, both are valid
	_ = keys
}

func TestRedisStoreConfigValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  *RedisStoreConfig
		wantErr bool
	}{
		{
			name: "nil logger - should use default",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				PoolSize:    5,
				DialTimeout: 2 * time.Second,
				Logger:      nil, // Should get default
			},
			wantErr: false,
		},
		{
			name: "zero min idle conns",
			config: &RedisStoreConfig{
				Address:      "localhost:6379",
				PoolSize:     5,
				MinIdleConns: 0,
				DialTimeout:  2 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "empty key prefix",
			config: &RedisStoreConfig{
				Address:     "localhost:6379",
				PoolSize:    5,
				DialTimeout: 2 * time.Second,
				KeyPrefix:   "",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedisStoreNewWithInvalidConfig(t *testing.T) {
	// nil config should use defaults and connect successfully
	store, err := NewRedisStore(nil)
	if err != nil {
		t.Logf("NewRedisStore with nil config (uses defaults): %v", err)
		// This is expected if Redis is not available - skip the test
		if store != nil {
			store.Close()
		}
	} else {
		// If Redis is available, verify it used defaults
		if store == nil {
			t.Error("NewRedisStore with nil config should return non-nil store")
		} else {
			store.Close()
		}
	}

	// Invalid config should error during validation
	invalidConfig := &RedisStoreConfig{
		Address:  "", // Invalid - empty address
		PoolSize: 5,
	}
	_, err = NewRedisStore(invalidConfig)
	if err == nil {
		t.Error("NewRedisStore with invalid config (empty address) should error")
	}
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkRedisStoreSet(b *testing.B) {
	store := setupRedisTest(&testing.T{})
	if store == nil {
		b.Skip("Redis not available")
	}
	defer cleanupRedisTest(&testing.T{}, store)

	ctx := context.Background()
	value := []byte("benchmark_value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		store.Set(ctx, key, value, 0)
	}
}

func BenchmarkRedisStoreGet(b *testing.B) {
	store := setupRedisTest(&testing.T{})
	if store == nil {
		b.Skip("Redis not available")
	}
	defer cleanupRedisTest(&testing.T{}, store)

	ctx := context.Background()
	key := "bench_key"
	value := []byte("benchmark_value")
	store.Set(ctx, key, value, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, key)
	}
}

func BenchmarkRedisStoreIncrement(b *testing.B) {
	store := setupRedisTest(&testing.T{})
	if store == nil {
		b.Skip("Redis not available")
	}
	defer cleanupRedisTest(&testing.T{}, store)

	ctx := context.Background()
	key := "bench_counter"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Increment(ctx, key, 0)
	}
}

func BenchmarkRedisStoreConcurrentSet(b *testing.B) {
	store := setupRedisTest(&testing.T{})
	if store == nil {
		b.Skip("Redis not available")
	}
	defer cleanupRedisTest(&testing.T{}, store)

	ctx := context.Background()
	value := []byte("benchmark_value")

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent_bench_key_%d", i)
			store.Set(ctx, key, value, 0)
			i++
		}
	})
}

func BenchmarkRedisStoreConcurrentIncrement(b *testing.B) {
	store := setupRedisTest(&testing.T{})
	if store == nil {
		b.Skip("Redis not available")
	}
	defer cleanupRedisTest(&testing.T{}, store)

	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = store.Increment(ctx, "concurrent_counter", 0)
		}
	})
}
