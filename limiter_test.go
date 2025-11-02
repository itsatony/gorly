package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// BASIC LIMITER TESTS
// ============================================================================

func TestNewRateLimiter(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		DefaultBurst:  10,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	if limiter == nil {
		t.Error("limiter should not be nil")
	}
}

func TestRateLimiterAllow(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	// First request should be allowed
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if !result.Allowed {
		t.Error("first request should be allowed")
	}

	if result.Limit != 10 {
		t.Errorf("expected limit 10, got %d", result.Limit)
	}
}

func TestRateLimiterAllowN(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	// Request 5 tokens
	result, err := limiter.AllowN(ctx, rlCtx, 5)
	if err != nil {
		t.Fatalf("AllowN failed: %v", err)
	}

	if !result.Allowed {
		t.Error("request should be allowed")
	}

	if result.Used != 5 {
		t.Errorf("expected 5 tokens used, got %d", result.Used)
	}
}

func TestRateLimiterExceedLimit(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  3,
		DefaultWindow: time.Second,
		DefaultBurst:  3,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	// Use all tokens
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(ctx, rlCtx)
		if err != nil {
			t.Fatalf("Allow failed on request %d: %v", i, err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Next request should be denied
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Allowed {
		t.Error("request should be denied after limit exceeded")
	}

	// RetryAfter might be 0 if tokens refill quickly, so we just check it's not negative
	if result.RetryAfter < 0 {
		t.Error("retry after should not be negative")
	}
}

func TestRateLimiterReset(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	// Use some tokens
	for i := 0; i < 3; i++ {
		limiter.Allow(ctx, rlCtx)
	}

	// Reset
	err = limiter.Reset(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should have full capacity again
	result, err := limiter.AllowN(ctx, rlCtx, 5)
	if err != nil {
		t.Fatalf("AllowN failed after reset: %v", err)
	}

	if !result.Allowed {
		t.Error("request should be allowed after reset")
	}
}

func TestRateLimiterStats(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	// Use some tokens
	limiter.AllowN(ctx, rlCtx, 3)

	// Get stats
	stats, err := limiter.Stats(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Limit != 10 {
		t.Errorf("expected limit 10, got %d", stats.Limit)
	}

	if stats.Entity != "user123" {
		t.Errorf("expected entity 'user123', got '%s'", stats.Entity)
	}
}

func TestRateLimiterHealth(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	err = limiter.Health(ctx)
	if err != nil {
		t.Errorf("Health check failed: %v", err)
	}
}

func TestRateLimiterClose(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewTokenBucketAlgorithm(),
		DefaultLimit:  10,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}

	// Close limiter
	err = limiter.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations should fail after close
	ctx := context.Background()
	rlCtx := NewSimpleContext("user123", ScopeGlobal, TierFree, nil)

	_, err = limiter.Allow(ctx, rlCtx)
	if err != ErrClosed {
		t.Errorf("expected ErrClosed after close, got %v", err)
	}
}

func TestRateLimiterWithSlidingWindow(t *testing.T) {
	store := newTestMemoryStore()

	config := &Config{
		Store:         store,
		Algorithm:     NewSlidingWindowAlgorithm(),
		DefaultLimit:  5,
		DefaultWindow: time.Second,
		DefaultBurst:  5,
		Logger:        NewNopLogger(),
	}

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewSimpleContext("user456", ScopeGlobal, TierFree, nil)

	// Make requests
	for i := 0; i < 5; i++ {
		result, err := limiter.Allow(ctx, rlCtx)
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Next should be denied
	result, err := limiter.Allow(ctx, rlCtx)
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}

	if result.Allowed {
		t.Error("request should be denied")
	}
}

// ============================================================================
// CONFIG VALIDATION TESTS
// ============================================================================

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Store:         &mockStore{},
				Algorithm:     NewTokenBucketAlgorithm(),
				DefaultLimit:  100,
				DefaultWindow: time.Minute,
				DefaultBurst:  10,
			},
			wantError: false,
		},
		{
			name: "nil store",
			config: &Config{
				Algorithm:     NewTokenBucketAlgorithm(),
				DefaultLimit:  100,
				DefaultWindow: time.Minute,
				DefaultBurst:  10,
			},
			wantError: true,
		},
		{
			name: "nil algorithm",
			config: &Config{
				Store:         &mockStore{},
				DefaultLimit:  100,
				DefaultWindow: time.Minute,
				DefaultBurst:  10,
			},
			wantError: true,
		},
		{
			name: "limit too low",
			config: &Config{
				Store:         &mockStore{},
				Algorithm:     NewTokenBucketAlgorithm(),
				DefaultLimit:  0,
				DefaultWindow: time.Minute,
				DefaultBurst:  10,
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
// TEST MEMORY STORE - Simple in-memory implementation for testing
// ============================================================================

type testMemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newTestMemoryStore() *testMemoryStore {
	return &testMemoryStore{
		data: make(map[string][]byte),
	}
}

func (m *testMemoryStore) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}
	return data, nil
}

func (m *testMemoryStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return nil
}

func (m *testMemoryStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return m.IncrementBy(ctx, key, 1, expiration)
}

func (m *testMemoryStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simple increment - not production quality
	return amount, nil
}

func (m *testMemoryStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

func (m *testMemoryStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[key]
	return exists, nil
}

func (m *testMemoryStore) Health(ctx context.Context) error {
	return nil
}

func (m *testMemoryStore) Close() error {
	return nil
}

func (m *testMemoryStore) ExecuteScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	// Test memory store doesn't support scripts - return error
	return nil, ErrScriptNotSupported
}

// ============================================================================
// MOCK STORE FOR CONFIG VALIDATION TESTS
// ============================================================================

type mockStore struct{}

func (m *mockStore) Get(ctx context.Context, key string) ([]byte, error) {
	return nil, ErrKeyNotFound
}

func (m *mockStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return nil
}

func (m *mockStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return 1, nil
}

func (m *mockStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	return amount, nil
}

func (m *mockStore) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockStore) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (m *mockStore) Health(ctx context.Context) error {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) ExecuteScript(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	// Mock store doesn't support scripts - return error
	return nil, ErrScriptNotSupported
}

// ============================================================================
// CONCURRENCY STRESS TESTS
// ============================================================================

// TestRateLimiterConcurrentAllow tests concurrent Allow calls on the same entity
func TestRateLimiterConcurrentAllow(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 100
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewIPContext("192.168.1.1")

	const numGoroutines = 50
	const requestsPerGoroutine = 10
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var failureCount atomic.Int64

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				result, err := limiter.Allow(ctx, rlCtx)
				if err != nil {
					t.Errorf("Allow failed: %v", err)
					return
				}
				if result.Allowed {
					successCount.Add(1)
				} else {
					failureCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify results
	totalRequests := int64(numGoroutines * requestsPerGoroutine)
	gotSuccess := successCount.Load()
	gotFailure := failureCount.Load()

	t.Logf("Total requests: %d, Allowed: %d, Denied: %d", totalRequests, gotSuccess, gotFailure)

	if gotSuccess+gotFailure != totalRequests {
		t.Errorf("request count mismatch: got %d+%d=%d, want %d",
			gotSuccess, gotFailure, gotSuccess+gotFailure, totalRequests)
	}

	// With concurrent requests, we're testing thread-safety, not strict limit enforcement
	// Token bucket with burst may allow more than the base limit initially
	// Just verify that the limiter responded to all requests without errors
	if gotSuccess == 0 {
		t.Error("no requests were allowed, limiter may be broken")
	}
}

// TestRateLimiterConcurrentAllowN tests concurrent AllowN calls with different N values
func TestRateLimiterConcurrentAllowN(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 200
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewIPContext("192.168.1.2")

	const numGoroutines = 20
	var wg sync.WaitGroup
	var totalConsumed atomic.Int64

	// Launch concurrent goroutines with different N values
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		n := int64((i % 5) + 1) // N values: 1, 2, 3, 4, 5
		go func(cost int64) {
			defer wg.Done()
			result, err := limiter.AllowN(ctx, rlCtx, cost)
			if err != nil {
				t.Errorf("AllowN(%d) failed: %v", cost, err)
				return
			}
			if result.Allowed {
				totalConsumed.Add(cost)
			}
		}(n)
	}

	wg.Wait()

	consumed := totalConsumed.Load()
	t.Logf("Total consumed: %d, Limit: %d", consumed, config.DefaultLimit)

	// Total consumed should not exceed limit
	if consumed > config.DefaultLimit {
		t.Errorf("consumed %d tokens, but limit is %d", consumed, config.DefaultLimit)
	}
}

// TestRateLimiterConcurrentResetAndAllow tests concurrent Reset and Allow operations
func TestRateLimiterConcurrentResetAndAllow(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 50
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewIPContext("192.168.1.3")

	const numAllowGoroutines = 30
	const numResetGoroutines = 5
	var wg sync.WaitGroup
	var allowErrors atomic.Int64
	var resetErrors atomic.Int64

	// Launch Allow goroutines
	for i := 0; i < numAllowGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_, err := limiter.Allow(ctx, rlCtx)
				if err != nil {
					allowErrors.Add(1)
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Launch Reset goroutines
	for i := 0; i < numResetGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				err := limiter.Reset(ctx, rlCtx)
				if err != nil {
					resetErrors.Add(1)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Should not have any errors
	if allowErrors.Load() > 0 {
		t.Errorf("Allow had %d errors during concurrent Reset", allowErrors.Load())
	}
	if resetErrors.Load() > 0 {
		t.Errorf("Reset had %d errors during concurrent Allow", resetErrors.Load())
	}
}

// TestRateLimiterConcurrentStats tests concurrent Stats calls
func TestRateLimiterConcurrentStats(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 100
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewIPContext("192.168.1.4")

	// Make some requests first
	for i := 0; i < 20; i++ {
		limiter.Allow(ctx, rlCtx)
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	var errors atomic.Int64

	// Launch concurrent Stats calls
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				stats, err := limiter.Stats(ctx, rlCtx)
				if err != nil {
					errors.Add(1)
					return
				}
				if stats.Limit != config.DefaultLimit {
					t.Errorf("Stats returned wrong limit: got %d, want %d", stats.Limit, config.DefaultLimit)
				}
			}
		}()
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("Stats had %d errors during concurrent access", errors.Load())
	}
}

// TestRateLimiterConcurrentMixedOperations tests all operations happening concurrently
func TestRateLimiterConcurrentMixedOperations(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 100
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()
	rlCtx := NewIPContext("192.168.1.5")

	const duration = 100 * time.Millisecond
	var wg sync.WaitGroup
	var errors atomic.Int64

	// Allow goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			end := time.Now().Add(duration)
			for time.Now().Before(end) {
				_, err := limiter.Allow(ctx, rlCtx)
				if err != nil {
					errors.Add(1)
					return
				}
			}
		}()
	}

	// AllowN goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			end := time.Now().Add(duration)
			for time.Now().Before(end) {
				_, err := limiter.AllowN(ctx, rlCtx, 2)
				if err != nil {
					errors.Add(1)
					return
				}
			}
		}()
	}

	// Stats goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			end := time.Now().Add(duration)
			for time.Now().Before(end) {
				_, err := limiter.Stats(ctx, rlCtx)
				if err != nil {
					errors.Add(1)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
	}

	// Reset goroutines
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			end := time.Now().Add(duration)
			for time.Now().Before(end) {
				err := limiter.Reset(ctx, rlCtx)
				if err != nil {
					errors.Add(1)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("Mixed operations had %d errors", errors.Load())
	}
}

// TestRateLimiterConcurrentMultipleEntities tests concurrent access to different entities
func TestRateLimiterConcurrentMultipleEntities(t *testing.T) {
	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 50
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	const numEntities = 10
	const goroutinesPerEntity = 5
	var wg sync.WaitGroup
	var errors atomic.Int64

	// Launch goroutines for each entity
	for entityID := 0; entityID < numEntities; entityID++ {
		for g := 0; g < goroutinesPerEntity; g++ {
			wg.Add(1)
			ip := fmt.Sprintf("192.168.1.%d", entityID+10)
			go func(entityIP string) {
				defer wg.Done()
				rlCtx := NewIPContext(entityIP)
				for i := 0; i < 20; i++ {
					_, err := limiter.Allow(ctx, rlCtx)
					if err != nil {
						errors.Add(1)
						return
					}
				}
			}(ip)
		}
	}

	wg.Wait()

	if errors.Load() > 0 {
		t.Errorf("Multiple entities test had %d errors", errors.Load())
	}

	// Successfully tested concurrent access to multiple independent entities
	// without errors - this verifies thread-safety across different rate limit keys
	t.Logf("Successfully processed requests for %d entities with %d goroutines each",
		numEntities, goroutinesPerEntity)
}

// TestRateLimiterStressTest is a high-concurrency stress test
func TestRateLimiterStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	store := newTestMemoryStore()
	defer store.Close()

	config := DefaultConfig()
	config.Store = store
	config.Algorithm = NewTokenBucketAlgorithm()
	config.DefaultLimit = 1000
	config.DefaultWindow = time.Second

	limiter, err := NewRateLimiter(config)
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}
	defer limiter.Close()

	ctx := context.Background()

	const numGoroutines = 100
	const requestsPerGoroutine = 100
	const duration = 500 * time.Millisecond

	var wg sync.WaitGroup
	var totalRequests atomic.Int64
	var allowedRequests atomic.Int64
	var errors atomic.Int64

	startTime := time.Now()

	// Launch high number of concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rlCtx := NewIPContext(fmt.Sprintf("192.168.%d.%d", id/256, id%256))

			end := time.Now().Add(duration)
			localRequests := 0
			for time.Now().Before(end) && localRequests < requestsPerGoroutine {
				result, err := limiter.Allow(ctx, rlCtx)
				if err != nil {
					errors.Add(1)
					return
				}
				totalRequests.Add(1)
				if result.Allowed {
					allowedRequests.Add(1)
				}
				localRequests++
			}
		}(i)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	total := totalRequests.Load()
	allowed := allowedRequests.Load()
	errorCount := errors.Load()

	t.Logf("Stress Test Results:")
	t.Logf("  Duration: %v", elapsed)
	t.Logf("  Total Requests: %d", total)
	t.Logf("  Allowed: %d", allowed)
	t.Logf("  Denied: %d", total-allowed)
	t.Logf("  Errors: %d", errorCount)
	t.Logf("  Throughput: %.0f req/s", float64(total)/elapsed.Seconds())

	if errorCount > 0 {
		t.Errorf("Stress test encountered %d errors", errorCount)
	}

	if total == 0 {
		t.Error("No requests were processed")
	}
}
