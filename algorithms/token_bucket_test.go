// algorithms/token_bucket_test.go
package algorithms

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockStore implements the Store interface for testing
type mockStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		data: make(map[string][]byte),
	}
}

func (m *mockStore) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, exists := m.data[key]; exists {
		return data, nil
	}

	// Return error to simulate key not found
	return nil, NewRateLimitError("store", "key not found", nil)
}

func (m *mockStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return nil
}

func (m *mockStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

func TestNewTokenBucketAlgorithm(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()

	if algorithm == nil {
		t.Fatal("Expected algorithm to be created")
	}

	if algorithm.Name() != "token_bucket" {
		t.Errorf("Expected algorithm name to be 'token_bucket', got %s", algorithm.Name())
	}
}

func TestTokenBucketAlgorithm_Allow_FirstRequest(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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
		t.Errorf("Expected 99 tokens remaining, got %d", result.Remaining)
	}

	if result.Limit != 100 {
		t.Errorf("Expected limit to be 100, got %d", result.Limit)
	}

	if result.Window != time.Hour {
		t.Errorf("Expected window to be 1 hour, got %v", result.Window)
	}

	if result.Algorithm != "token_bucket" {
		t.Errorf("Expected algorithm to be 'token_bucket', got %s", result.Algorithm)
	}
}

func TestTokenBucketAlgorithm_Allow_MultipleRequests(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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
			t.Errorf("Expected %d tokens remaining after request %d, got %d",
				expectedRemaining, i+1, result.Remaining)
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
		t.Errorf("Expected 4 tokens remaining, got %d", result.Remaining)
	}
}

func TestTokenBucketAlgorithm_Allow_ExceedLimit(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(3)
	window := time.Minute

	// Exhaust all tokens
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
		t.Errorf("Expected 0 tokens remaining, got %d", result.Remaining)
	}

	if result.RetryAfter == 0 {
		t.Error("Expected retry after time to be set")
	}
}

func TestTokenBucketAlgorithm_Allow_BulkRequest(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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
		t.Errorf("Expected 5 tokens remaining, got %d", result.Remaining)
	}

	// Request 6 tokens - should be denied
	result, err = algorithm.Allow(ctx, store, key, limit, window, 6)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Allowed {
		t.Error("Expected bulk request exceeding available tokens to be denied")
	}
}

func TestTokenBucketAlgorithm_Allow_InvalidRequestCount(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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

func TestTokenBucketAlgorithm_Reset(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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
		t.Errorf("Expected 1 token remaining before reset, got %d", result.Remaining)
	}

	// Reset the bucket
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
		t.Errorf("Expected 4 tokens remaining after reset, got %d", result.Remaining)
	}
}

func TestTokenBucketAlgorithm_GetBucketInfo(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Minute

	// Make some requests to populate data
	algorithm.Allow(ctx, store, key, limit, window, 3) // Allowed: 7 remaining
	algorithm.Allow(ctx, store, key, limit, window, 4) // Allowed: 3 remaining
	algorithm.Allow(ctx, store, key, limit, window, 5) // Denied: not enough tokens

	info, err := algorithm.GetBucketInfo(ctx, store, key, limit, window)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if info == nil {
		t.Fatal("Expected bucket info to be returned")
	}

	// Check basic fields
	if info["algorithm"] != "token_bucket" {
		t.Errorf("Expected algorithm to be 'token_bucket', got %v", info["algorithm"])
	}

	if info["capacity"] != limit {
		t.Errorf("Expected capacity to be %d, got %v", limit, info["capacity"])
	}

	if info["window"] != window {
		t.Errorf("Expected window to be %v, got %v", window, info["window"])
	}

	// Check that we have current tokens
	currentTokens, exists := info["current_tokens"]
	if !exists {
		t.Error("Expected current_tokens to be present")
	}

	tokensFloat, ok := currentTokens.(float64)
	if !ok {
		t.Errorf("Expected current_tokens to be float64, got %T", currentTokens)
	}

	// Allow for small floating point precision differences
	// Should have 3 tokens remaining after using 7 (3 + 4)
	if tokensFloat < 2.9 || tokensFloat > 3.1 {
		t.Errorf("Expected approximately 3 current tokens, got %f", tokensFloat)
	}

	// Check statistics
	totalRequests, exists := info["total_requests"]
	if !exists {
		t.Error("Expected total_requests to be present")
	}

	// Total successful requests: 3 + 4 = 7
	if totalRequests != int64(7) {
		t.Errorf("Expected 7 total requests, got %v", totalRequests)
	}

	deniedRequests, exists := info["denied_requests"]
	if !exists {
		t.Error("Expected denied_requests to be present")
	}

	// One request for 5 tokens was denied
	if deniedRequests != int64(5) {
		t.Errorf("Expected 5 denied requests, got %v", deniedRequests)
	}
}

func TestTokenBucketAlgorithm_RefillOverTime(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "test:user123"
	limit := int64(10)
	window := time.Second // 10 tokens per second for fast refill

	// Consume all tokens
	for i := 0; i < 10; i++ {
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
		t.Error("Expected request to be denied when bucket is empty")
	}

	// Wait for tokens to refill (simulate time passing by manually updating the state)
	// In a real implementation, we'd wait for actual time to pass
	// For testing, we manipulate the stored state to simulate time passage

	// Get current state
	data, _ := store.Get(ctx, key)
	var state TokenBucketState
	json.Unmarshal(data, &state)

	// Manually set last refill to 0.5 seconds ago
	state.LastRefill = time.Now().Add(-500 * time.Millisecond)

	// Save updated state
	updatedData, _ := json.Marshal(state)
	store.Set(ctx, key, updatedData, time.Minute)

	// Now request should have some tokens available due to refill
	result, err = algorithm.Allow(ctx, store, key, limit, window, 1)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// We should have approximately 5 tokens refilled (0.5 seconds * 10 tokens/second)
	// So the request should be allowed
	if !result.Allowed {
		t.Error("Expected request to be allowed after refill")
	}
}

func TestTokenBucketAlgorithm_ValidateConfig(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()

	tests := []struct {
		name        string
		limit       int64
		window      time.Duration
		burstSize   int64
		expectError bool
	}{
		{
			name:        "Valid config",
			limit:       100,
			window:      time.Minute,
			burstSize:   50,
			expectError: false,
		},
		{
			name:        "Zero limit",
			limit:       0,
			window:      time.Minute,
			burstSize:   10,
			expectError: true,
		},
		{
			name:        "Negative limit",
			limit:       -1,
			window:      time.Minute,
			burstSize:   10,
			expectError: true,
		},
		{
			name:        "Zero window",
			limit:       100,
			window:      0,
			burstSize:   10,
			expectError: true,
		},
		{
			name:        "Negative window",
			limit:       100,
			window:      -time.Minute,
			burstSize:   10,
			expectError: true,
		},
		{
			name:        "Zero burst size",
			limit:       100,
			window:      time.Minute,
			burstSize:   0,
			expectError: true,
		},
		{
			name:        "Burst size exceeds limit",
			limit:       100,
			window:      time.Minute,
			burstSize:   150,
			expectError: true,
		},
		{
			name:        "Very high refill rate",
			limit:       100000,
			window:      time.Second,
			burstSize:   50,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := algorithm.ValidateConfig(tt.limit, tt.window, tt.burstSize)

			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}

func TestTokenBucketAlgorithm_GetMetrics(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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

	if metrics.BucketKey != key {
		t.Errorf("Expected bucket key to be %s, got %s", key, metrics.BucketKey)
	}

	if metrics.Capacity != limit {
		t.Errorf("Expected capacity to be %d, got %d", limit, metrics.Capacity)
	}

	// Total successful requests: 3 + 2 = 5
	if metrics.TotalRequests != 5 {
		t.Errorf("Expected 5 total requests, got %d", metrics.TotalRequests)
	}

	// One request for 10 tokens was denied
	if metrics.DeniedRequests != 10 {
		t.Errorf("Expected 10 denied requests, got %d", metrics.DeniedRequests)
	}

	// Current tokens should be approximately 5 (started with 10, used 5)
	if metrics.CurrentTokens < 4.9 || metrics.CurrentTokens > 5.1 {
		t.Errorf("Expected approximately 5 current tokens, got %f", metrics.CurrentTokens)
	}
}

func TestTokenBucketAlgorithm_ConcurrentAccess(t *testing.T) {
	algorithm := NewTokenBucketAlgorithm()
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

	// All concurrent requests should be allowed since we have 100 tokens
	if allowed != concurrentRequests {
		t.Errorf("Expected %d requests to be allowed, got %d", concurrentRequests, allowed)
	}
}

// Benchmark tests
func BenchmarkTokenBucketAlgorithm_Allow(b *testing.B) {
	algorithm := NewTokenBucketAlgorithm()
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

func BenchmarkTokenBucketAlgorithm_GetBucketInfo(b *testing.B) {
	algorithm := NewTokenBucketAlgorithm()
	store := newMockStore()
	ctx := context.Background()

	key := "benchmark:info"
	limit := int64(1000)
	window := time.Hour

	// Setup initial state
	algorithm.Allow(ctx, store, key, limit, window, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		algorithm.GetBucketInfo(ctx, store, key, limit, window)
	}
}

func BenchmarkTokenBucketAlgorithm_ConcurrentAllow(b *testing.B) {
	algorithm := NewTokenBucketAlgorithm()
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
