package stores

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

// ============================================================================
// BASIC FUNCTIONALITY TESTS
// ============================================================================

func TestNewMemoryStore(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	stats := store.Stats()
	if stats.ShardCount != int(ratelimit.DefaultShardCount) {
		t.Errorf("expected %d shards, got %d", ratelimit.DefaultShardCount, stats.ShardCount)
	}
	if stats.TotalKeys != 0 {
		t.Errorf("expected 0 keys, got %d", stats.TotalKeys)
	}
}

func TestMemoryStoreSetGet(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test_key"
	value := []byte("test_value")

	// Set
	err = store.Set(ctx, key, value, 0)
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

func TestMemoryStoreGetNonExistent(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Get(ctx, "nonexistent")

	if err != ratelimit.ErrKeyNotFound {
		t.Errorf("expected ErrKeyNotFound, got %v", err)
	}
}

func TestMemoryStoreDelete(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "test_key"
	value := []byte("test_value")

	// Set then delete
	store.Set(ctx, key, value, 0)
	err = store.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = store.Get(ctx, key)
	if err != ratelimit.ErrKeyNotFound {
		t.Error("key should not exist after deletion")
	}
}

func TestMemoryStoreExists(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

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

func TestMemoryStoreIncrement(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

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

func TestMemoryStoreIncrementBy(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

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

func TestMemoryStoreExpiration(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "expiring_key"
	value := []byte("expiring_value")

	// Set with short expiration
	err = store.Set(ctx, key, value, 100*time.Millisecond)
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

func TestMemoryStoreIncrementExpiration(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "expiring_counter"

	// Increment with expiration
	val, err := store.Increment(ctx, key, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1, got %d", val)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Next increment should start from 1 again
	val, err = store.Increment(ctx, key, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Increment failed: %v", err)
	}
	if val != 1 {
		t.Errorf("expected 1 after expiration, got %d", val)
	}
}

// ============================================================================
// CLEANUP TESTS
// ============================================================================

func TestMemoryStoreCleanup(t *testing.T) {
	config := &MemoryStoreConfig{
		ShardCount:      4,
		CleanupInterval: 1 * time.Second,
		MaxKeys:         0,
		Logger:          ratelimit.NewNopLogger(),
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Add multiple expiring keys
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("key_%d", i)
		store.Set(ctx, key, []byte("value"), 500*time.Millisecond)
	}

	// Verify they exist
	stats := store.Stats()
	if stats.TotalKeys != 10 {
		t.Errorf("expected 10 keys, got %d", stats.TotalKeys)
	}

	// Wait for cleanup to run (expiry + cleanup interval)
	time.Sleep(1600 * time.Millisecond)

	// Keys should be cleaned up
	stats = store.Stats()
	if stats.TotalKeys != 0 {
		t.Errorf("expected 0 keys after cleanup, got %d", stats.TotalKeys)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestMemoryStoreConcurrentWrites(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	goroutines := 100

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
	stats := store.Stats()
	if stats.TotalKeys != goroutines {
		t.Errorf("expected %d keys, got %d", goroutines, stats.TotalKeys)
	}
}

func TestMemoryStoreConcurrentIncrements(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	key := "concurrent_counter"
	goroutines := 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = store.Increment(ctx, key, 0)
		}()
	}

	wg.Wait()

	// Get final count
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Decode using binary encoding (new format)
	var count int64
	if len(data) >= 8 {
		count = int64(binary.BigEndian.Uint64(data))
	} else {
		// Fallback to JSON for backward compatibility
		json.Unmarshal(data, &count)
	}

	if count != int64(goroutines) {
		t.Errorf("expected count %d, got %d", goroutines, count)
	}
}

// ============================================================================
// CLOSE TESTS
// ============================================================================

func TestMemoryStoreClose(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	store.Set(ctx, "key", []byte("value"), 0)

	// Close store
	err = store.Close()
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
// HEALTH TESTS
// ============================================================================

func TestMemoryStoreHealth(t *testing.T) {
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.Health(ctx)
	if err != nil {
		t.Errorf("healthy store should return nil: %v", err)
	}

	// Close and check health
	store.Close()
	err = store.Health(ctx)
	if err != ratelimit.ErrClosed {
		t.Errorf("closed store should return ErrClosed: %v", err)
	}
}

// ============================================================================
// STATISTICS TESTS
// ============================================================================

func TestMemoryStoreStats(t *testing.T) {
	config := &MemoryStoreConfig{
		ShardCount:      8,
		CleanupInterval: time.Minute,
		MaxKeys:         0,
		Logger:          ratelimit.NewNopLogger(),
	}

	store, err := NewMemoryStore(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Add some keys
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("stats_key_%d", i)
		var expiration time.Duration
		if i%2 == 0 {
			expiration = time.Hour // Some with expiration
		}
		store.Set(ctx, key, []byte("value"), expiration)
	}

	stats := store.Stats()
	if stats.ShardCount != 8 {
		t.Errorf("expected 8 shards, got %d", stats.ShardCount)
	}
	if stats.TotalKeys != 20 {
		t.Errorf("expected 20 keys, got %d", stats.TotalKeys)
	}
	if stats.TotalExpiring != 10 {
		t.Errorf("expected 10 expiring keys, got %d", stats.TotalExpiring)
	}
	if stats.Closed {
		t.Error("store should not be closed")
	}

	// Test String method
	str := store.String()
	if len(str) == 0 {
		t.Error("String() should not be empty")
	}
}

// ============================================================================
// CONFIGURATION TESTS
// ============================================================================

func TestMemoryStoreConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *MemoryStoreConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: &MemoryStoreConfig{
				ShardCount:      4,
				CleanupInterval: 10 * time.Second,
				MaxKeys:         1000,
				Logger:          ratelimit.NewNopLogger(),
			},
			wantError: false,
		},
		{
			name: "invalid shard count",
			config: &MemoryStoreConfig{
				ShardCount:      0,
				CleanupInterval: 10 * time.Second,
				Logger:          ratelimit.NewNopLogger(),
			},
			wantError: true,
		},
		{
			name: "invalid cleanup interval",
			config: &MemoryStoreConfig{
				ShardCount:      4,
				CleanupInterval: 500 * time.Millisecond,
				Logger:          ratelimit.NewNopLogger(),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMemoryStore(tt.config)
			if (err != nil) != tt.wantError {
				t.Errorf("NewMemoryStore() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkMemoryStoreSet(b *testing.B) {
	store, _ := NewMemoryStore(nil)
	defer store.Close()

	ctx := context.Background()
	value := []byte("benchmark_value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		store.Set(ctx, key, value, 0)
	}
}

func BenchmarkMemoryStoreGet(b *testing.B) {
	store, _ := NewMemoryStore(nil)
	defer store.Close()

	ctx := context.Background()
	key := "bench_key"
	value := []byte("benchmark_value")
	store.Set(ctx, key, value, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, key)
	}
}

func BenchmarkMemoryStoreIncrement(b *testing.B) {
	store, _ := NewMemoryStore(nil)
	defer store.Close()

	ctx := context.Background()
	key := "bench_counter"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Increment(ctx, key, 0)
	}
}

func BenchmarkMemoryStoreConcurrentSet(b *testing.B) {
	store, _ := NewMemoryStore(nil)
	defer store.Close()

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

func BenchmarkMemoryStoreConcurrentIncrement(b *testing.B) {
	store, _ := NewMemoryStore(nil)
	defer store.Close()

	ctx := context.Background()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = store.Increment(ctx, "concurrent_counter", 0)
		}
	})
}
