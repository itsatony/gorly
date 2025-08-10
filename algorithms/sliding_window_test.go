// algorithms/sliding_window_test.go
package algorithms

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewSlidingWindowAlgorithm(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()

	if algorithm == nil {
		t.Fatal("Expected algorithm to be created")
	}

	if algorithm.Name() != "sliding_window" {
		t.Errorf("Expected algorithm name to be 'sliding_window', got %s", algorithm.Name())
	}
}

func TestSlidingWindowAlgorithm_Allow_FirstRequest(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	// Test first request - should be allowed
	result, err := algorithm.Allow(ctx, store, "test:user123", 100, time.Hour, 1)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected first request to be allowed")
	}

	if result.Remaining != 99 {
		t.Errorf("Expected 99 requests remaining, got %d", result.Remaining)
	}

	if result.Limit != 100 {
		t.Errorf("Expected limit to be 100, got %d", result.Limit)
	}

	if result.Window != time.Hour {
		t.Errorf("Expected window to be 1 hour, got %v", result.Window)
	}

	if result.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm to be 'sliding_window', got %s", result.Algorithm)
	}

	if result.Used != 1 {
		t.Errorf("Expected 1 request used, got %d", result.Used)
	}
}

func TestSlidingWindowAlgorithm_Allow_MultipleRequests(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Minute

	// Make 5 requests
	for i := 0; i < 5; i++ {
		result, err := algorithm.Allow(ctx, store, key, limit, window, 1)

		if err != nil {
			t.Fatalf("Unexpected error on request %d: %v", i+1, err)
		}

		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}

		expectedRemaining := limit - int64(i+1)
		if result.Remaining != expectedRemaining {
			t.Errorf("Expected %d requests remaining after request %d, got %d",
				expectedRemaining, i+1, result.Remaining)
		}

		expectedUsed := int64(i + 1)
		if result.Used != expectedUsed {
			t.Errorf("Expected %d requests used after request %d, got %d",
				expectedUsed, i+1, result.Used)
		}
	}

	// 6th request should still be allowed (we have 5 remaining)
	result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected 6th request to be allowed")
	}

	if result.Remaining != 4 {
		t.Errorf("Expected 4 requests remaining, got %d", result.Remaining)
	}

	if result.Used != 6 {
		t.Errorf("Expected 6 requests used, got %d", result.Used)
	}
}

func TestSlidingWindowAlgorithm_Allow_ExceedLimit(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(3)
	window := time.Minute

	// Exhaust all available requests
	for i := 0; i < 3; i++ {
		result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Fatalf("Expected request %d to be allowed", i+1)
		}
	}

	// 4th request should be denied
	result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected 4th request to be denied")
	}

	if result.Remaining != 0 {
		t.Errorf("Expected 0 requests remaining, got %d", result.Remaining)
	}

	if result.RetryAfter <= 0 {
		t.Error("Expected retry after time to be set")
	}

	if result.Used != 3 {
		t.Errorf("Expected 3 requests used, got %d", result.Used)
	}
}

func TestSlidingWindowAlgorithm_Allow_BulkRequest(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Minute

	// Request 5 tokens at once
	result, err := algorithm.Allow(ctx, store, key, limit, window, 5)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !result.Allowed {
		t.Error("Expected bulk request to be allowed")
	}

	if result.Remaining != 5 {
		t.Errorf("Expected 5 requests remaining, got %d", result.Remaining)
	}

	if result.Used != 5 {
		t.Errorf("Expected 5 requests used, got %d", result.Used)
	}

	// Request 6 more - should be denied
	result, err = algorithm.Allow(ctx, store, key, limit, window, 6)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected bulk request exceeding available quota to be denied")
	}

	if result.Remaining != 5 {
		t.Errorf("Expected 5 requests remaining, got %d", result.Remaining)
	}

	if result.Used != 5 {
		t.Errorf("Expected 5 requests used (unchanged), got %d", result.Used)
	}
}

func TestSlidingWindowAlgorithm_Allow_InvalidRequestCount(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	// Test with zero requests
	result, err := algorithm.Allow(ctx, store, "test:user123", 100, time.Hour, 0)

	if err == nil {
		t.Error("Expected error for zero request count")
	}

	if result.Allowed {
		t.Error("Expected zero request count to be denied")
	}

	// Test with negative requests
	result, err = algorithm.Allow(ctx, store, "test:user123", 100, time.Hour, -1)

	if err == nil {
		t.Error("Expected error for negative request count")
	}

	if result.Allowed {
		t.Error("Expected negative request count to be denied")
	}
}

func TestSlidingWindowAlgorithm_SlidingWindow(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:sliding"
	limit := int64(5)
	window := 2 * time.Second // Short window for testing

	// Fill up the window
	for i := 0; i < 5; i++ {
		result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Fatalf("Expected request %d to be allowed", i+1)
		}
	}

	// Next request should be denied
	result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("Expected request to be denied when window is full")
	}

	// Wait for window to slide (older requests should expire)
	time.Sleep(2500 * time.Millisecond) // Wait longer than the window with more margin

	// Now request should be allowed again
	result, err = algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error after sliding: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected request to be allowed after window slides")
	}

	// Should have almost full capacity again
	if result.Remaining < 4 {
		t.Errorf("Expected at least 4 requests remaining after slide, got %d", result.Remaining)
	}
}

func TestSlidingWindowAlgorithm_Reset(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(5)
	window := time.Minute

	// Make some requests
	for i := 0; i < 3; i++ {
		algorithm.Allow(ctx, store, key, limit, window, 1)
	}

	// Verify state exists
	result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.Remaining != 1 {
		t.Errorf("Expected 1 request remaining before reset, got %d", result.Remaining)
	}

	// Reset the window
	err = algorithm.Reset(ctx, store, key)
	if err != nil {
		t.Fatalf("Unexpected error during reset: %v", err)
	}

	// Next request should start fresh
	result, err = algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error after reset: %v", err)
	}

	if result.Remaining != 4 {
		t.Errorf("Expected 4 requests remaining after reset, got %d", result.Remaining)
	}

	if result.Used != 1 {
		t.Errorf("Expected 1 request used after reset, got %d", result.Used)
	}
}

func TestSlidingWindowAlgorithm_GetWindowInfo(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Minute

	// Make some requests
	algorithm.Allow(ctx, store, key, limit, window, 3) // 3 requests
	algorithm.Allow(ctx, store, key, limit, window, 2) // 2 more requests
	algorithm.Allow(ctx, store, key, limit, window, 6) // Should be denied (only 5 remaining)

	info, err := algorithm.GetWindowInfo(ctx, store, key, limit, window)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if info == nil {
		t.Fatal("Expected window info to be returned")
	}

	// Check basic fields
	if info["algorithm"] != "sliding_window" {
		t.Errorf("Expected algorithm to be 'sliding_window', got %v", info["algorithm"])
	}

	if info["limit"] != limit {
		t.Errorf("Expected limit to be %d, got %v", limit, info["limit"])
	}

	if info["window"] != window {
		t.Errorf("Expected window to be %v, got %v", window, info["window"])
	}

	// Check that we have current requests (3 + 2 = 5, since 6-request was denied)
	currentRequests, exists := info["current_requests"]
	if !exists {
		t.Error("Expected current_requests to be present")
	}

	if currentRequests != 5 {
		t.Errorf("Expected 5 current requests, got %v", currentRequests)
	}

	// Check remaining (10 - 5 = 5)
	remaining, exists := info["remaining"]
	if !exists {
		t.Error("Expected remaining to be present")
	}

	if remaining != int64(5) {
		t.Errorf("Expected 5 remaining, got %v", remaining)
	}

	// Check statistics (3 + 2 = 5 successful requests)
	totalRequests, exists := info["total_requests"]
	if !exists {
		t.Error("Expected total_requests to be present")
	}

	if totalRequests != int64(5) {
		t.Errorf("Expected 5 total requests, got %v", totalRequests)
	}

	// Check denied requests (6 requests were denied)
	deniedRequests, exists := info["denied_requests"]
	if !exists {
		t.Error("Expected denied_requests to be present")
	}

	if deniedRequests != int64(6) {
		t.Errorf("Expected 6 denied requests, got %v", deniedRequests)
	}
}

func TestSlidingWindowAlgorithm_GetMetrics(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Minute

	// Make some requests
	algorithm.Allow(ctx, store, key, limit, window, 3)
	algorithm.Allow(ctx, store, key, limit, window, 2)
	algorithm.Allow(ctx, store, key, limit, window, 10) // Should be denied

	metrics, err := algorithm.GetMetrics(ctx, store, key, limit, window)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if metrics == nil {
		t.Fatal("Expected metrics to be returned")
	}

	if metrics.WindowKey != key {
		t.Errorf("Expected window key to be %s, got %s", key, metrics.WindowKey)
	}

	if metrics.Algorithm != "sliding_window" {
		t.Errorf("Expected algorithm to be 'sliding_window', got %s", metrics.Algorithm)
	}

	if metrics.Limit != limit {
		t.Errorf("Expected limit to be %d, got %d", limit, metrics.Limit)
	}

	if metrics.CurrentRequests != 5 {
		t.Errorf("Expected 5 current requests, got %d", metrics.CurrentRequests)
	}

	if metrics.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", metrics.TotalRequests)
	}

	if metrics.DeniedRequests != 10 {
		t.Errorf("Expected 10 denied requests, got %d", metrics.DeniedRequests)
	}
}

func TestSlidingWindowAlgorithm_ValidateConfig(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()

	tests := []struct {
		name        string
		limit       int64
		window      time.Duration
		maxRequests int64
		expectError bool
	}{
		{
			name:        "Valid config",
			limit:       100,
			window:      time.Minute,
			maxRequests: 1000,
			expectError: false,
		},
		{
			name:        "Zero limit",
			limit:       0,
			window:      time.Minute,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Negative limit",
			limit:       -1,
			window:      time.Minute,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Zero window",
			limit:       100,
			window:      0,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Negative window",
			limit:       100,
			window:      -time.Minute,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Very short window",
			limit:       100,
			window:      500 * time.Millisecond,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Very long window",
			limit:       100,
			window:      25 * time.Hour,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Very high limit",
			limit:       2000000,
			window:      time.Hour,
			maxRequests: 1000,
			expectError: true,
		},
		{
			name:        "Limit exceeds max requests",
			limit:       100,
			window:      time.Hour,
			maxRequests: 50,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := algorithm.ValidateConfig(tt.limit, tt.window, tt.maxRequests)

			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}

func TestSlidingWindowAlgorithm_GetRequestPattern(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:pattern"
	limit := int64(10)
	window := time.Minute

	// Make some requests with specific timing
	algorithm.Allow(ctx, store, key, limit, window, 1)
	time.Sleep(100 * time.Millisecond)
	algorithm.Allow(ctx, store, key, limit, window, 1)
	time.Sleep(200 * time.Millisecond)
	algorithm.Allow(ctx, store, key, limit, window, 1)

	pattern, err := algorithm.GetRequestPattern(ctx, store, key, limit, window)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if pattern == nil {
		t.Fatal("Expected request pattern to be returned")
	}

	if pattern.TotalRequests != 3 {
		t.Errorf("Expected 3 total requests, got %d", pattern.TotalRequests)
	}

	if pattern.RequestRate <= 0 {
		t.Errorf("Expected positive request rate, got %f", pattern.RequestRate)
	}

	if pattern.AverageInterval <= 0 {
		t.Errorf("Expected positive average interval, got %v", pattern.AverageInterval)
	}
}

func TestSlidingWindowAlgorithm_ConcurrentAccess(t *testing.T) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:concurrent"
	limit := int64(100)
	window := time.Minute

	var wg sync.WaitGroup
	concurrentRequests := 50
	successCount := make(chan bool, concurrentRequests)

	// Launch concurrent requests
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := algorithm.Allow(ctx, store, key, limit, window, 1)
			if err != nil {
				t.Errorf("Unexpected error in concurrent request: %v", err)
				return
			}
			successCount <- result.Allowed
		}()
	}

	wg.Wait()
	close(successCount)

	// Count successful requests
	allowed := 0
	for success := range successCount {
		if success {
			allowed++
		}
	}

	// All concurrent requests should be allowed since we have 100 limit
	if allowed != concurrentRequests {
		t.Errorf("Expected %d requests to be allowed, got %d", concurrentRequests, allowed)
	}
}

// Benchmark tests
func BenchmarkSlidingWindowAlgorithm_Allow(b *testing.B) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "benchmark:user"
	limit := int64(10000)
	window := time.Hour

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		algorithm.Allow(ctx, store, key, limit, window, 1)
	}
}

func BenchmarkSlidingWindowAlgorithm_GetWindowInfo(b *testing.B) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "benchmark:info"
	limit := int64(1000)
	window := time.Hour

	// Setup initial state
	algorithm.Allow(ctx, store, key, limit, window, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		algorithm.GetWindowInfo(ctx, store, key, limit, window)
	}
}

func BenchmarkSlidingWindowAlgorithm_ConcurrentAllow(b *testing.B) {
	algorithm := NewSlidingWindowAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	limit := int64(10000)
	window := time.Hour

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("benchmark:concurrent:%d", i%100) // 100 different keys
			algorithm.Allow(ctx, store, key, limit, window, 1)
			i++
		}
	})
}
