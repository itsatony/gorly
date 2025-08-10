// stores/memory_test.go
package stores

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewMemoryStore(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("Expected store to be created")
	}

	// Test health check
	if err := store.Health(context.Background()); err != nil {
		t.Errorf("Expected health check to pass, got error: %v", err)
	}
}

func TestMemoryStore_SetAndGet(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test:key"
	value := []byte("test value")

	// Test Set
	if err := store.Set(ctx, key, value, time.Hour); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Test Get
	retrieved, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	if string(retrieved) != string(value) {
		t.Errorf("Expected value %s, got %s", string(value), string(retrieved))
	}

	// Test modification isolation (ensure copies are made)
	value[0] = 'X'
	retrieved[0] = 'Y'

	// Get again to verify original value is unchanged
	retrieved2, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get value again: %v", err)
	}

	if string(retrieved2) != "test value" {
		t.Errorf("Expected original value to be unchanged, got %s", string(retrieved2))
	}
}

func TestMemoryStore_GetNonExistentKey(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent key")
	}

	storeErr, ok := err.(*StoreError)
	if !ok {
		t.Errorf("Expected StoreError, got %T", err)
	} else if storeErr.Type != "store" || storeErr.Message != "key not found" {
		t.Errorf("Expected store error with 'key not found', got %s: %s", storeErr.Type, storeErr.Message)
	}
}

func TestMemoryStore_Expiration(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: 50 * time.Millisecond, // Fast cleanup for testing
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test:expiring"
	value := []byte("expiring value")

	// Set with short expiration
	if err := store.Set(ctx, key, value, 100*time.Millisecond); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Should be retrievable immediately
	_, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get value before expiration: %v", err)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not be retrievable after expiration
	_, err = store.Get(ctx, key)
	if err == nil {
		t.Error("Expected error for expired key")
	}

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	// Check that key is actually removed
	stats := store.Stats()
	if stats["expired"].(int64) == 0 {
		t.Error("Expected expired counter to be greater than 0")
	}
}

func TestMemoryStore_TTL(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test:ttl"
	value := []byte("ttl value")

	// Test TTL for nonexistent key
	ttl, err := store.TTL(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}
	if ttl != -2*time.Second {
		t.Errorf("Expected TTL -2s for nonexistent key, got %v", ttl)
	}

	// Set with expiration
	expiration := 5 * time.Second
	if err := store.Set(ctx, key, value, expiration); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Check TTL
	ttl, err = store.TTL(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}

	// Should be approximately the expiration time (allowing for some variance)
	if ttl < 4*time.Second || ttl > expiration {
		t.Errorf("Expected TTL around %v, got %v", expiration, ttl)
	}

	// Set without expiration (using default TTL)
	key2 := "test:no_expiration"
	if err := store.Set(ctx, key2, value, 0); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Check TTL - should use default TTL
	ttl, err = store.TTL(ctx, key2)
	if err != nil {
		t.Fatalf("Failed to get TTL: %v", err)
	}

	// Should be approximately the default TTL
	if ttl < 55*time.Minute || ttl > time.Hour {
		t.Errorf("Expected TTL around %v, got %v", time.Hour, ttl)
	}
}

func TestMemoryStore_Increment(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test:counter"

	// First increment should create counter with value 1
	value, err := store.Increment(ctx, key, time.Hour)
	if err != nil {
		t.Fatalf("Failed to increment: %v", err)
	}
	if value != 1 {
		t.Errorf("Expected first increment to be 1, got %d", value)
	}

	// Second increment should be 2
	value, err = store.Increment(ctx, key, time.Hour)
	if err != nil {
		t.Fatalf("Failed to increment: %v", err)
	}
	if value != 2 {
		t.Errorf("Expected second increment to be 2, got %d", value)
	}

	// IncrementBy with larger amount
	value, err = store.IncrementBy(ctx, key, 5, time.Hour)
	if err != nil {
		t.Fatalf("Failed to increment by 5: %v", err)
	}
	if value != 7 {
		t.Errorf("Expected increment by 5 to be 7, got %d", value)
	}
}

func TestMemoryStore_MultiOperations(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Test MultiSet
	keyValues := map[string][]byte{
		"key1": []byte("value1"),
		"key2": []byte("value2"),
		"key3": []byte("value3"),
	}

	if err := store.MultiSet(ctx, keyValues, time.Hour); err != nil {
		t.Fatalf("Failed to multi set: %v", err)
	}

	// Test MultiGet
	keys := []string{"key1", "key2", "key3", "nonexistent"}
	results, err := store.MultiGet(ctx, keys)
	if err != nil {
		t.Fatalf("Failed to multi get: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	for key, expectedValue := range keyValues {
		if result, exists := results[key]; !exists {
			t.Errorf("Expected key %s to exist in results", key)
		} else if string(result) != string(expectedValue) {
			t.Errorf("Expected value %s for key %s, got %s", string(expectedValue), key, string(result))
		}
	}

	// Test IncrementMulti
	counterKeys := []string{"counter1", "counter2", "counter3"}
	amounts := []int64{1, 5, 10}

	counterResults, err := store.IncrementMulti(ctx, counterKeys, amounts, time.Hour)
	if err != nil {
		t.Fatalf("Failed to multi increment: %v", err)
	}

	if len(counterResults) != 3 {
		t.Errorf("Expected 3 counter results, got %d", len(counterResults))
	}

	for i, key := range counterKeys {
		if result, exists := counterResults[key]; !exists {
			t.Errorf("Expected key %s to exist in counter results", key)
		} else if result != amounts[i] {
			t.Errorf("Expected counter value %d for key %s, got %d", amounts[i], key, result)
		}
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test:delete"
	value := []byte("delete me")

	// Set value
	if err := store.Set(ctx, key, value, time.Hour); err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// Verify it exists
	if exists, err := store.Exists(ctx, key); err != nil || !exists {
		t.Fatalf("Expected key to exist before delete")
	}

	// Delete
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify it doesn't exist
	if exists, err := store.Exists(ctx, key); err != nil || exists {
		t.Errorf("Expected key to not exist after delete")
	}

	// Try to get deleted key
	_, err = store.Get(ctx, key)
	if err == nil {
		t.Error("Expected error when getting deleted key")
	}
}

func TestMemoryStore_MaxKeys(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         5, // Small limit for testing
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Set more keys than the limit
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key%d", i)
		value := []byte(fmt.Sprintf("value%d", i))
		if err := store.Set(ctx, key, value, time.Hour); err != nil {
			t.Fatalf("Failed to set key%d: %v", i, err)
		}
	}

	// Check that we don't exceed the limit
	if size := store.Size(); size > config.MaxKeys {
		t.Errorf("Expected store size to not exceed %d, got %d", config.MaxKeys, size)
	}

	// Check stats for evictions
	stats := store.Stats()
	if stats["evicted"].(int64) == 0 {
		t.Error("Expected some keys to be evicted")
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         10000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent writes
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("worker%d:key%d", workerID, j)
				value := []byte(fmt.Sprintf("worker%d:value%d", workerID, j))
				if err := store.Set(ctx, key, value, time.Hour); err != nil {
					t.Errorf("Worker %d failed to set %s: %v", workerID, key, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("worker%d:key%d", workerID, j)
				expectedValue := fmt.Sprintf("worker%d:value%d", workerID, j)

				value, err := store.Get(ctx, key)
				if err != nil {
					t.Errorf("Worker %d failed to get %s: %v", workerID, key, err)
					continue
				}

				if string(value) != expectedValue {
					t.Errorf("Worker %d got wrong value for %s: expected %s, got %s",
						workerID, key, expectedValue, string(value))
				}
			}
		}(i)
	}

	wg.Wait()

	// Check final stats
	stats := store.Stats()
	totalExpectedKeys := numGoroutines * numOperations
	actualKeys := stats["total_keys"].(int)

	if actualKeys != totalExpectedKeys {
		t.Errorf("Expected %d keys, got %d", totalExpectedKeys, actualKeys)
	}
}

func TestMemoryStore_Stats(t *testing.T) {
	config := MemoryConfig{
		MaxKeys:         1000,
		CleanupInterval: time.Minute,
		DefaultTTL:      time.Hour,
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Perform some operations
	store.Set(ctx, "key1", []byte("value1"), time.Hour)
	store.Set(ctx, "key2", []byte("value2"), time.Hour)
	store.Get(ctx, "key1") // hit
	store.Get(ctx, "key3") // miss
	store.Delete(ctx, "key2")

	stats := store.Stats()

	// Check that stats are tracked
	if stats["sets"].(int64) != 2 {
		t.Errorf("Expected 2 sets, got %d", stats["sets"].(int64))
	}

	if stats["gets"].(int64) != 2 {
		t.Errorf("Expected 2 gets, got %d", stats["gets"].(int64))
	}

	if stats["hits"].(int64) != 1 {
		t.Errorf("Expected 1 hit, got %d", stats["hits"].(int64))
	}

	if stats["misses"].(int64) != 1 {
		t.Errorf("Expected 1 miss, got %d", stats["misses"].(int64))
	}

	if stats["deletes"].(int64) != 1 {
		t.Errorf("Expected 1 delete, got %d", stats["deletes"].(int64))
	}
}
