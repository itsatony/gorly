package stores

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestMemoryStore_IncrementConcurrent tests thread safety of Increment under high concurrency
func TestMemoryStore_IncrementConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numGoroutines          = 100
		incrementsPerGoroutine = 1000
		expectedTotal          = numGoroutines * incrementsPerGoroutine
	)

	key := "concurrent_counter"
	var wg sync.WaitGroup

	// Launch many goroutines that all increment the same key
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				_, err := store.Increment(ctx, key, time.Hour)
				if err != nil {
					t.Errorf("Increment failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// Verify the final count is exactly what we expect
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get final value: %v", err)
	}

	// Decode the value
	var finalValue int64
	if len(data) >= 8 {
		finalValue = int64(binary.BigEndian.Uint64(data))
	} else {
		t.Fatalf("Invalid data length: %d", len(data))
	}

	if finalValue != expectedTotal {
		t.Errorf("Race condition detected! Expected %d, got %d (lost %d increments)",
			expectedTotal, finalValue, expectedTotal-finalValue)
	}
}

// TestMemoryStore_IncrementByConcurrent tests IncrementBy with variable amounts
func TestMemoryStore_IncrementByConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numGoroutines          = 50
		operationsPerGoroutine = 500
	)

	key := "concurrent_counter_by"
	var wg sync.WaitGroup
	var expectedTotal int64

	// Each goroutine increments by a different amount
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		amount := int64(i + 1) // 1, 2, 3, ..., numGoroutines
		expectedTotal += amount * operationsPerGoroutine

		go func(amt int64) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_, err := store.IncrementBy(ctx, key, amt, time.Hour)
				if err != nil {
					t.Errorf("IncrementBy failed: %v", err)
					return
				}
			}
		}(amount)
	}

	wg.Wait()

	// Verify the final count
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get final value: %v", err)
	}

	var finalValue int64
	if len(data) >= 8 {
		finalValue = int64(binary.BigEndian.Uint64(data))
	}

	if finalValue != expectedTotal {
		t.Errorf("Race condition with IncrementBy! Expected %d, got %d (diff: %d)",
			expectedTotal, finalValue, expectedTotal-finalValue)
	}
}

// TestMemoryStore_MultiKeyConc current tests concurrent access to multiple keys
func TestMemoryStore_MultiKeyConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numKeys          = 10
		numGoroutines    = 50
		incrementsPerKey = 100
	)

	var wg sync.WaitGroup

	// Concurrent operations on multiple different keys
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for k := 0; k < numKeys; k++ {
				key := fmt.Sprintf("key_%d", k)
				for j := 0; j < incrementsPerKey; j++ {
					_, err := store.Increment(ctx, key, time.Hour)
					if err != nil {
						t.Errorf("Increment failed for %s: %v", key, err)
						return
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify each key has the expected count
	expectedPerKey := int64(numGoroutines * incrementsPerKey)
	for k := 0; k < numKeys; k++ {
		key := fmt.Sprintf("key_%d", k)
		data, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Failed to get value for %s: %v", key, err)
		}

		var value int64
		if len(data) >= 8 {
			value = int64(binary.BigEndian.Uint64(data))
		}

		if value != expectedPerKey {
			t.Errorf("Race condition for %s! Expected %d, got %d",
				key, expectedPerKey, value)
		}
	}
}

// TestMemoryStore_MixedOperationsConcurrent tests mixed reads and writes
func TestMemoryStore_MixedOperationsConcurrent(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numGoroutines          = 50
		operationsPerGoroutine = 200
	)

	key := "mixed_ops_key"
	var wg sync.WaitGroup

	// Initialize with zero
	_, err = store.IncrementBy(ctx, key, 0, time.Hour)
	if err != nil {
		t.Fatalf("Failed to initialize key: %v", err)
	}

	// Launch goroutines with mixed operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// Mix of operations: increment, read, increment, read
				if j%4 == 0 || j%4 == 2 {
					_, err := store.Increment(ctx, key, time.Hour)
					if err != nil {
						t.Errorf("Increment failed: %v", err)
						return
					}
				} else {
					// Just read
					_, err := store.Get(ctx, key)
					if err != nil {
						// Key might not exist yet, that's ok
						continue
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// The final value should be deterministic
	// We did numGoroutines * (operationsPerGoroutine / 2) increments
	expectedTotal := int64(numGoroutines * (operationsPerGoroutine / 2))

	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get final value: %v", err)
	}

	var finalValue int64
	if len(data) >= 8 {
		finalValue = int64(binary.BigEndian.Uint64(data))
	}

	if finalValue != expectedTotal {
		t.Errorf("Race condition with mixed operations! Expected %d, got %d",
			expectedTotal, finalValue)
	}
}

// TestMemoryStore_StressTest is an aggressive stress test
func TestMemoryStore_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numGoroutines = 500 // Very high concurrency
		operations    = 1000
	)

	key := "stress_test_key"
	var wg sync.WaitGroup
	var errors sync.Map

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_, err := store.Increment(ctx, key, time.Hour)
				if err != nil {
					errors.Store(id, err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Check for any errors
	errorCount := 0
	errors.Range(func(key, value interface{}) bool {
		t.Errorf("Goroutine %v failed: %v", key, value)
		errorCount++
		return true
	})

	if errorCount > 0 {
		t.Fatalf("Stress test failed with %d errors", errorCount)
	}

	// Verify final count
	expectedTotal := int64(numGoroutines * operations)
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get final value: %v", err)
	}

	var finalValue int64
	if len(data) >= 8 {
		finalValue = int64(binary.BigEndian.Uint64(data))
	}

	if finalValue != expectedTotal {
		t.Errorf("CRITICAL: Race condition under stress! Expected %d, got %d (lost %d operations)",
			expectedTotal, finalValue, expectedTotal-finalValue)
	}

	// Performance metric
	opsPerSecond := float64(numGoroutines*operations) / duration.Seconds()
	t.Logf("Stress test completed: %d goroutines, %d ops each, %.0f ops/sec",
		numGoroutines, operations, opsPerSecond)
}

// TestMemoryStore_NoRaceWithExpiration tests concurrent increments with expiration
func TestMemoryStore_NoRaceWithExpiration(t *testing.T) {
	ctx := context.Background()
	store, err := NewMemoryStore(nil)
	if err != nil {
		t.Fatalf("Failed to create memory store: %v", err)
	}
	defer store.Close()

	const (
		numGoroutines = 100
		operations    = 500
	)

	key := "expiring_counter"
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Use short expiration to trigger cleanup logic
				_, err := store.IncrementBy(ctx, key, 1, 10*time.Second)
				if err != nil {
					t.Errorf("IncrementBy with expiration failed: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	// Verify count
	expectedTotal := int64(numGoroutines * operations)
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Failed to get final value: %v", err)
	}

	var finalValue int64
	if len(data) >= 8 {
		finalValue = int64(binary.BigEndian.Uint64(data))
	}

	if finalValue != expectedTotal {
		t.Errorf("Race condition with expiration! Expected %d, got %d",
			expectedTotal, finalValue)
	}
}
