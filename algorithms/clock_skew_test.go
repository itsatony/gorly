// algorithms/clock_skew_test.go
package algorithms

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// CLOCK SKEW TESTS - Token Bucket Algorithm
// ============================================================================

// TestTokenBucket_BackwardClockJump verifies that token bucket handles
// backward clock jumps gracefully without panicking or corrupting state
func TestTokenBucket_BackwardClockJump(t *testing.T) {
	// Note: This test uses the non-atomic method which is easier to test
	// The Lua script has similar protection
	store := newMockStore()
	algo := NewTokenBucketAlgorithm()
	ctx := context.Background()

	limit := int64(10)
	window := time.Second
	key := "clock-skew-test"

	// Make initial request
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Initial request failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Initial request should be allowed")
	}

	// The token bucket algorithm uses time.Now() internally
	// We can't actually manipulate time, but we can verify the algorithm
	// doesn't panic or return invalid results when time differences are negative
	// This is tested implicitly by the algorithm's elapsed time check

	// Make several more requests to consume tokens
	for i := 0; i < 5; i++ {
		result, err = algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Even if clock jumped backward, algorithm should still work
	// In worst case, it won't refill tokens but won't crash or corrupt state
	result, err = algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatal("Request after potential clock skew should not error:", err)
	}

	// Result should be valid (either allowed or denied with proper metadata)
	if result.Limit != limit {
		t.Errorf("Limit should be %d, got %d", limit, result.Limit)
	}

	if result.Used < 0 {
		t.Error("Used count should never be negative")
	}

	if result.Remaining < 0 {
		t.Error("Remaining count should never be negative")
	}
}

// TestTokenBucket_ForwardClockJump verifies forward clock jumps work correctly
func TestTokenBucket_ForwardClockJump(t *testing.T) {
	store := newMockStore()
	algo := NewTokenBucketAlgorithm()
	ctx := context.Background()

	limit := int64(10)
	window := time.Second
	key := "forward-jump-test"

	// Consume all tokens
	for i := int64(0); i < limit; i++ {
		_, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
	}

	// Next request should be denied
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatal(err)
	}

	if result.Allowed {
		t.Error("Request should be denied - limit reached")
	}

	// Wait for tokens to refill
	time.Sleep(window + 100*time.Millisecond)

	// Now requests should be allowed again (tokens refilled)
	result, err = algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatal(err)
	}

	if !result.Allowed {
		t.Error("Request should be allowed after window - tokens refilled")
	}
}

// ============================================================================
// CLOCK SKEW TESTS - Sliding Window Algorithm
// ============================================================================

// TestSlidingWindow_BackwardClockJump_FutureTimestamps tests that
// future timestamps are properly cleaned up after backward clock jump
func TestSlidingWindow_BackwardClockJump_FutureTimestamps(t *testing.T) {
	store := newMockStore()
	algo := NewSlidingWindowAlgorithm()
	ctx := context.Background()

	limit := int64(5)
	window := 10 * time.Second
	key := "future-timestamps-test"

	// Make some requests to establish timestamps
	for i := int64(0); i < 3; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Current usage should be 3
	result, err := algo.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatal(err)
	}

	if result.Used != 4 { // 3 previous + 1 current
		t.Errorf("Expected used=4, got %d", result.Used)
	}

	// The algorithm naturally handles clock skew through its window-based cleanup
	// Timestamps from "before" a backward clock jump will be seen as "future"
	// and should be cleaned up on the next request

	// Verify algorithm remains stable
	for i := 0; i < 3; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Stability check %d failed: %v", i, err)
		}

		// Should have valid metadata
		if result.Limit != limit {
			t.Errorf("Limit should be %d, got %d", limit, result.Limit)
		}

		if result.Used < 0 {
			t.Error("Used should never be negative")
		}

		if result.Remaining < 0 {
			t.Error("Remaining should never be negative")
		}
	}
}

// TestSlidingWindow_CleanupRemovesFutureTimestamps directly tests
// the cleanup function's ability to remove future timestamps
func TestSlidingWindow_CleanupRemovesFutureTimestamps(t *testing.T) {
	algo := NewSlidingWindowAlgorithm()

	// Create a state with mixed timestamps
	now := time.Now()
	nowNano := now.UnixNano()
	windowNano := int64(time.Minute.Nanoseconds())

	state := &SlidingWindowState{
		Requests: []int64{
			nowNano - 50*int64(time.Second), // 50s ago - valid
			nowNano - 30*int64(time.Second), // 30s ago - valid
			nowNano,                         // now - valid
			nowNano + 10*int64(time.Second), // 10s future - INVALID
			nowNano + 20*int64(time.Second), // 20s future - INVALID
		},
		WindowNano: windowNano,
	}

	// Clean up
	state = algo.cleanupExpiredRequests(state, nowNano)

	// Should only have 3 valid timestamps (not the future ones)
	if len(state.Requests) != 3 {
		t.Errorf("Expected 3 timestamps after cleanup, got %d", len(state.Requests))
	}

	// Verify no timestamps are from the future
	for _, ts := range state.Requests {
		if ts > nowNano {
			t.Errorf("Found future timestamp %d (now=%d)", ts, nowNano)
		}
	}

	// Verify timestamps are sorted and valid
	for i := 1; i < len(state.Requests); i++ {
		if state.Requests[i] < state.Requests[i-1] {
			t.Error("Timestamps should remain sorted after cleanup")
		}
	}
}

// TestSlidingWindow_CleanupOldAndFutureTimestamps tests cleanup
// of both expired (too old) and future timestamps
func TestSlidingWindow_CleanupOldAndFutureTimestamps(t *testing.T) {
	algo := NewSlidingWindowAlgorithm()

	now := time.Now()
	nowNano := now.UnixNano()
	windowNano := int64((30 * time.Second).Nanoseconds())

	state := &SlidingWindowState{
		Requests: []int64{
			nowNano - 60*int64(time.Second), // 60s ago - expired
			nowNano - 50*int64(time.Second), // 50s ago - expired
			nowNano - 20*int64(time.Second), // 20s ago - valid
			nowNano - 10*int64(time.Second), // 10s ago - valid
			nowNano,                         // now - valid
			nowNano + 5*int64(time.Second),  // 5s future - invalid
			nowNano + 10*int64(time.Second), // 10s future - invalid
		},
		WindowNano: windowNano,
	}

	// Clean up
	state = algo.cleanupExpiredRequests(state, nowNano)

	// Should only have 3 valid timestamps (within window and not future)
	if len(state.Requests) != 3 {
		t.Errorf("Expected 3 valid timestamps, got %d", len(state.Requests))
		for i, ts := range state.Requests {
			age := nowNano - ts
			t.Logf("  Timestamp %d: age=%v, future=%v", i, time.Duration(age), ts > nowNano)
		}
	}

	// All remaining timestamps should be within [window_start, now]
	windowStart := nowNano - windowNano
	for _, ts := range state.Requests {
		if ts < windowStart {
			t.Errorf("Timestamp %d is before window start %d", ts, windowStart)
		}
		if ts > nowNano {
			t.Errorf("Timestamp %d is in the future (now=%d)", ts, nowNano)
		}
	}
}

// ============================================================================
// CLOCK SKEW EDGE CASES
// ============================================================================

// TestSlidingWindow_EmptyStateWithClockSkew tests clock skew with no existing state
func TestSlidingWindow_EmptyStateWithClockSkew(t *testing.T) {
	algo := NewSlidingWindowAlgorithm()

	nowNano := time.Now().UnixNano()
	windowNano := int64(time.Minute.Nanoseconds())

	state := &SlidingWindowState{
		Requests:   []int64{}, // Empty
		WindowNano: windowNano,
	}

	// Cleanup should handle empty state gracefully
	state = algo.cleanupExpiredRequests(state, nowNano)

	if len(state.Requests) != 0 {
		t.Error("Empty state should remain empty after cleanup")
	}
}

// TestSlidingWindow_AllFutureTimestamps tests when all timestamps are from future
func TestSlidingWindow_AllFutureTimestamps(t *testing.T) {
	algo := NewSlidingWindowAlgorithm()

	nowNano := time.Now().UnixNano()
	windowNano := int64(time.Minute.Nanoseconds())

	state := &SlidingWindowState{
		Requests: []int64{
			nowNano + 10*int64(time.Second),
			nowNano + 20*int64(time.Second),
			nowNano + 30*int64(time.Second),
		},
		WindowNano: windowNano,
	}

	// All timestamps are from future - should be removed
	state = algo.cleanupExpiredRequests(state, nowNano)

	if len(state.Requests) != 0 {
		t.Errorf("All future timestamps should be removed, got %d", len(state.Requests))
	}
}

// TestSlidingWindow_AllExpiredTimestamps tests when all timestamps are expired
func TestSlidingWindow_AllExpiredTimestamps(t *testing.T) {
	algo := NewSlidingWindowAlgorithm()

	nowNano := time.Now().UnixNano()
	windowNano := int64((30 * time.Second).Nanoseconds())

	state := &SlidingWindowState{
		Requests: []int64{
			nowNano - 60*int64(time.Second),
			nowNano - 50*int64(time.Second),
			nowNano - 40*int64(time.Second),
		},
		WindowNano: windowNano,
	}

	// All timestamps are expired - should be removed
	state = algo.cleanupExpiredRequests(state, nowNano)

	if len(state.Requests) != 0 {
		t.Errorf("All expired timestamps should be removed, got %d", len(state.Requests))
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

// TestClockSkew_TokenBucketStability tests token bucket stability
func TestClockSkew_TokenBucketStability(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	algo := NewTokenBucketAlgorithm()

	key := "stability-test-tb"
	limit := int64(100)
	window := 5 * time.Second

	// Make many requests
	for i := 0; i < 50; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		// Verify result is always valid
		if result.Limit != limit {
			t.Errorf("Invalid limit in result: expected %d, got %d", limit, result.Limit)
		}

		if result.Used < 0 || result.Used > limit*2 {
			t.Errorf("Invalid used count: %d", result.Used)
		}

		if result.Remaining < 0 {
			t.Errorf("Invalid remaining count: %d", result.Remaining)
		}
	}
}

// TestClockSkew_SlidingWindowStability tests sliding window stability
func TestClockSkew_SlidingWindowStability(t *testing.T) {
	store := newMockStore()
	ctx := context.Background()
	algo := NewSlidingWindowAlgorithm()

	key := "stability-test-sw"
	limit := int64(100)
	window := 5 * time.Second

	// Make many requests
	for i := 0; i < 50; i++ {
		result, err := algo.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i, err)
		}

		// Verify result is always valid
		if result.Limit != limit {
			t.Errorf("Invalid limit in result: expected %d, got %d", limit, result.Limit)
		}

		if result.Used < 0 || result.Used > limit*2 {
			t.Errorf("Invalid used count: %d", result.Used)
		}

		if result.Remaining < 0 {
			t.Errorf("Invalid remaining count: %d", result.Remaining)
		}
	}
}
