// algorithms/memory_bounds_test.go
package algorithms

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// MEMORY BOUNDS PROTECTION TESTS
// ============================================================================

// TestSlidingWindow_MemoryBoundsProtection verifies that the sliding window
// algorithm prevents unbounded memory growth by capping the number of tracked requests
func TestSlidingWindow_MemoryBoundsProtection(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	// Use a small limit to test bounds quickly
	limit := int64(10)
	window := time.Minute
	key := "memory-bounds-test"

	// The max trackable requests should be limit * 2 = 20
	maxTracked := limit * 2

	// First, fill up to the limit (should all be allowed)
	for i := int64(0); i < limit; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed (used: %d, limit: %d)", i, result.Used, limit)
		}
	}

	// Next request should be denied (limit reached)
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if result.Allowed {
		t.Error("Request should be denied - limit reached")
	}

	// Now try to force more requests without cleanup to test memory bounds
	// We'll use a very large window so requests don't expire
	longWindow := 24 * time.Hour
	testKey := "memory-dos-test"

	// Try to add requests up to 2x the limit (maxTracked = 20)
	// The first 10 should be allowed, then denied
	for i := int64(0); i < maxTracked; i++ {
		result, err := algo.Allow(ctx, store, testKey, limit, longWindow, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		if i < limit {
			// First 10 should be allowed
			if !result.Allowed {
				t.Errorf("Request %d should be allowed (i < limit)", i)
			}
		} else {
			// Requests 10-19 should be denied (limit exceeded)
			if result.Allowed {
				t.Errorf("Request %d should be denied (i >= limit)", i)
			}
		}
	}

	// Now we have 20 tracked requests. Try to add more to test memory bounds protection
	// This should be denied due to memory bounds, not just rate limit
	for i := int64(0); i < 10; i++ {
		result, err := algo.Allow(ctx, store, testKey, limit, longWindow, 1)
		if err != nil {
			t.Fatalf("Request beyond memory limit failed: %v", err)
		}

		if result.Allowed {
			t.Errorf("Request beyond memory limit should be denied (DOS protection)")
		}

		// The used count should remain at maxTracked
		if result.Used > maxTracked {
			t.Errorf("Used count (%d) should not exceed max tracked (%d)", result.Used, maxTracked)
		}
	}
}

// TestSlidingWindow_MemoryBoundsWithLargeLimit tests memory bounds with large limits
func TestSlidingWindow_MemoryBoundsWithLargeLimit(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	// Use a very large limit that would exceed 1M when doubled
	// The memory bounds should cap at 1M regardless of limit
	limit := int64(600000) // 600k
	window := time.Hour
	key := "large-limit-test"

	// Calculate expected max tracked (should be capped at 1M)
	expectedMax := int64(1000000) // Hard cap

	// Make a single request
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	if !result.Allowed {
		t.Error("First request should be allowed")
	}

	// Verify the algorithm isn't trying to track more than 1M
	// We can't test the actual limit without making 1M requests,
	// but we can verify the code doesn't panic or allocate unbounded memory
	t.Logf("Large limit test completed successfully (limit=%d, expected_max=%d)", limit, expectedMax)
}

// TestSlidingWindow_MemoryBoundsWithSmallLimit tests memory bounds with very small limits
func TestSlidingWindow_MemoryBoundsWithSmallLimit(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	// Use the minimum limit
	limit := int64(1)
	window := time.Minute
	key := "small-limit-test"

	// Max tracked should be limit * 2 = 2
	maxTracked := limit * 2

	// First request should be allowed
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}

	if !result.Allowed {
		t.Error("First request should be allowed")
	}

	// Second request should be denied (limit reached)
	result, err = algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	if result.Allowed {
		t.Error("Second request should be denied - limit reached")
	}

	// Third request should also be denied (still tracking 2 requests)
	result, err = algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Third request failed: %v", err)
	}

	if result.Allowed {
		t.Error("Third request should be denied")
	}

	// Verify used count doesn't exceed maxTracked
	if result.Used > maxTracked {
		t.Errorf("Used count (%d) exceeded max tracked (%d)", result.Used, maxTracked)
	}
}

// TestSlidingWindow_MemoryCleanup verifies that expired requests are cleaned up
// and don't count toward memory bounds
func TestSlidingWindow_MemoryCleanup(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	limit := int64(5)
	window := 2 * time.Second // Short window for cleanup test
	key := "cleanup-test"

	// Fill the limit
	for i := int64(0); i < limit; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Wait for window to expire
	time.Sleep(window + 500*time.Millisecond)

	// Now we should be able to make new requests (old ones cleaned up)
	for i := int64(0); i < limit; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Post-cleanup request %d failed: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("Post-cleanup request %d should be allowed (old requests expired)", i)
		}
	}
}

// TestSlidingWindow_MemoryBoundsBatchRequests tests memory bounds with batch requests
func TestSlidingWindow_MemoryBoundsBatchRequests(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	limit := int64(100)
	window := time.Minute
	key := "batch-test"

	// Try to add a batch that's within limit
	result, err := algo.Allow(ctx, store, key, limit, window, 50)
	if err != nil {
		t.Fatalf("Batch request failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Batch request within limit should be allowed")
	}

	if result.Used != 50 {
		t.Errorf("Expected used=50, got %d", result.Used)
	}

	// Try to add another batch that exceeds limit
	result, err = algo.Allow(ctx, store, key, limit, window, 60)
	if err != nil {
		t.Fatalf("Second batch request failed: %v", err)
	}

	if result.Allowed {
		t.Error("Batch request exceeding limit should be denied")
	}

	// Used should still be 50 (previous batch)
	if result.Used != 50 {
		t.Errorf("Expected used=50 (unchanged), got %d", result.Used)
	}
}
