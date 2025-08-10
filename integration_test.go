// integration_test.go
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/itsatony/gorly/algorithms"
)

// mockRedisStore implements Store interface for testing without actual Redis
type mockRedisStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockRedisStore() *mockRedisStore {
	return &mockRedisStore{
		data: make(map[string][]byte),
	}
}

func (m *mockRedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, exists := m.data[key]; exists {
		return data, nil
	}

	return nil, fmt.Errorf("key not found: %s", key)
}

func (m *mockRedisStore) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return nil
}

func (m *mockRedisStore) Increment(ctx context.Context, key string, expiration time.Duration) (int64, error) {
	return m.IncrementBy(ctx, key, 1, expiration)
}

func (m *mockRedisStore) IncrementBy(ctx context.Context, key string, amount int64, expiration time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simple implementation - in a real Redis store this would be atomic
	current := int64(0)
	if data, exists := m.data[key]; exists {
		// For simplicity, just treat as int64 bytes
		if len(data) == 8 {
			current = int64(data[0])
		}
	}

	current += amount
	m.data[key] = []byte{byte(current)}
	return current, nil
}

func (m *mockRedisStore) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

func (m *mockRedisStore) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[key]
	return exists, nil
}

func (m *mockRedisStore) Health(ctx context.Context) error {
	return nil
}

func (m *mockRedisStore) Close() error {
	return nil
}

func TestRateLimiterIntegration(t *testing.T) {
	// Create a configuration for testing
	config := &Config{
		Enabled:   true,
		Algorithm: "token_bucket",
		Store:     "redis", // We'll mock this
		KeyPrefix: "test:rl",
		DefaultLimits: map[string]RateLimit{
			ScopeGlobal: {
				Requests:  10,
				Window:    time.Minute,
				BurstSize: 5,
			},
		},
		TierLimits: map[string]TierConfig{
			TierFree: {
				DefaultLimits: map[string]RateLimit{
					ScopeGlobal: {
						Requests:  5,
						Window:    time.Minute,
						BurstSize: 3,
					},
				},
			},
			TierPremium: {
				DefaultLimits: map[string]RateLimit{
					ScopeGlobal: {
						Requests:  20,
						Window:    time.Minute,
						BurstSize: 10,
					},
				},
			},
		},
		EnableMetrics: true,
		MetricsPrefix: "test_m3mo_ratelimit",
	}

	// Create a rate limiter with mock store
	limiter := &rateLimiter{
		config:     config,
		store:      newMockRedisStore(),
		algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
		keyBuilder: NewKeyBuilder(config.KeyPrefix),
		metrics:    NewMetrics(config.MetricsPrefix),
	}

	ctx := context.Background()

	t.Run("Basic Rate Limiting", func(t *testing.T) {
		entity := NewDefaultAuthEntity("user123", EntityTypeUser, TierFree)
		scope := ScopeGlobal

		// Free tier should allow 5 requests per minute with burst of 3

		// First 3 requests should be allowed (burst)
		for i := 0; i < 3; i++ {
			result, err := limiter.Allow(ctx, entity, scope)
			if err != nil {
				t.Fatalf("Unexpected error on request %d: %v", i+1, err)
			}
			if !result.Allowed {
				t.Errorf("Expected request %d to be allowed", i+1)
			}
		}

		// 4th request should still be allowed (within limit)
		result, err := limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected 4th request to be allowed")
		}

		// 5th request should still be allowed
		result, err = limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected 5th request to be allowed")
		}

		// 6th request should be denied (exceeds free tier limit)
		result, err = limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("Expected 6th request to be denied")
		}

		if result.RetryAfter == 0 {
			t.Error("Expected retry after time to be set")
		}
	})

	t.Run("Different Tiers", func(t *testing.T) {
		freeEntity := NewDefaultAuthEntity("free_user", EntityTypeUser, TierFree)
		premiumEntity := NewDefaultAuthEntity("premium_user", EntityTypeUser, TierPremium)
		scope := ScopeGlobal

		// Free tier gets 5 requests
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, freeEntity, scope)
			if err != nil {
				t.Fatalf("Unexpected error for free user: %v", err)
			}
			if !result.Allowed {
				t.Errorf("Expected request %d to be allowed for free user", i+1)
			}
		}

		// 6th request for free user should be denied
		result, err := limiter.Allow(ctx, freeEntity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("Expected 6th request to be denied for free user")
		}

		// Premium tier gets 20 requests - test first 10
		for i := 0; i < 10; i++ {
			result, err := limiter.Allow(ctx, premiumEntity, scope)
			if err != nil {
				t.Fatalf("Unexpected error for premium user: %v", err)
			}
			if !result.Allowed {
				t.Errorf("Expected request %d to be allowed for premium user", i+1)
			}
		}

		// Premium user should still have requests available
		result, err = limiter.Allow(ctx, premiumEntity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected 11th request to be allowed for premium user")
		}
	})

	t.Run("Different Scopes", func(t *testing.T) {
		entity := NewDefaultAuthEntity("scope_user", EntityTypeUser, TierFree)

		// Global scope - should use free tier limits (5 requests)
		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, entity, ScopeGlobal)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Errorf("Expected global request %d to be allowed", i+1)
			}
		}

		// 6th global request should be denied
		result, err := limiter.Allow(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("Expected 6th global request to be denied")
		}

		// Memory scope should have its own bucket - should be allowed
		result, err = limiter.Allow(ctx, entity, ScopeMemory)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected memory scope request to be allowed (separate bucket)")
		}
	})

	t.Run("AllowN Bulk Requests", func(t *testing.T) {
		entity := NewDefaultAuthEntity("bulk_user", EntityTypeUser, TierFree)
		scope := ScopeGlobal

		// Request 3 tokens at once
		result, err := limiter.AllowN(ctx, entity, scope, 3)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected bulk request of 3 to be allowed")
		}
		if result.Remaining != 2 {
			t.Errorf("Expected 2 tokens remaining, got %d", result.Remaining)
		}

		// Request 3 more tokens - should be denied (only 2 remaining)
		result, err = limiter.AllowN(ctx, entity, scope, 3)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("Expected bulk request exceeding available tokens to be denied")
		}

		// Request 2 tokens - should be allowed
		result, err = limiter.AllowN(ctx, entity, scope, 2)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected bulk request of 2 to be allowed")
		}
	})

	t.Run("Reset Functionality", func(t *testing.T) {
		entity := NewDefaultAuthEntity("reset_user", EntityTypeUser, TierFree)
		scope := ScopeGlobal

		// Exhaust the bucket
		for i := 0; i < 5; i++ {
			limiter.Allow(ctx, entity, scope)
		}

		// Verify bucket is exhausted
		result, err := limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result.Allowed {
			t.Error("Expected request to be denied before reset")
		}

		// Reset the bucket
		err = limiter.Reset(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error during reset: %v", err)
		}

		// Should be able to make requests again
		result, err = limiter.Allow(ctx, entity, scope)
		if err != nil {
			t.Fatalf("Unexpected error after reset: %v", err)
		}
		if !result.Allowed {
			t.Error("Expected request to be allowed after reset")
		}
	})

	t.Run("Statistics", func(t *testing.T) {
		entity := NewDefaultAuthEntity("stats_user", EntityTypeUser, TierFree)

		// Make some requests across different scopes
		limiter.Allow(ctx, entity, ScopeGlobal)
		limiter.Allow(ctx, entity, ScopeGlobal)
		limiter.Allow(ctx, entity, ScopeMemory)

		// Get overall stats
		stats, err := limiter.Stats(ctx, entity)
		if err != nil {
			t.Fatalf("Unexpected error getting stats: %v", err)
		}

		if stats == nil {
			t.Fatal("Expected stats to be returned")
		}

		if stats.Entity.ID() != entity.ID() {
			t.Errorf("Expected entity ID %s, got %s", entity.ID(), stats.Entity.ID())
		}

		// Get scope-specific stats
		scopeStats, err := limiter.ScopeStats(ctx, entity, ScopeGlobal)
		if err != nil {
			t.Fatalf("Unexpected error getting scope stats: %v", err)
		}

		if scopeStats == nil {
			t.Fatal("Expected scope stats to be returned")
		}

		if scopeStats.Scope != ScopeGlobal {
			t.Errorf("Expected scope to be %s, got %s", ScopeGlobal, scopeStats.Scope)
		}
	})

	t.Run("Health Check", func(t *testing.T) {
		err := limiter.Health(ctx)
		if err != nil {
			t.Fatalf("Expected health check to pass: %v", err)
		}
	})

	t.Run("Disabled Rate Limiting", func(t *testing.T) {
		// Create disabled config
		disabledConfig := *config
		disabledConfig.Enabled = false

		disabledLimiter := &rateLimiter{
			config:     &disabledConfig,
			store:      newMockRedisStore(),
			algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
			keyBuilder: NewKeyBuilder(disabledConfig.KeyPrefix),
		}

		entity := NewDefaultAuthEntity("disabled_user", EntityTypeUser, TierFree)

		// All requests should be allowed when disabled
		for i := 0; i < 100; i++ {
			result, err := disabledLimiter.Allow(ctx, entity, ScopeGlobal)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !result.Allowed {
				t.Error("Expected all requests to be allowed when rate limiting is disabled")
			}
		}
	})
}

func TestRateLimiterConcurrency(t *testing.T) {
	config := DefaultConfig()
	config.DefaultLimits[ScopeGlobal] = RateLimit{
		Requests:  100,
		Window:    time.Minute,
		BurstSize: 50,
	}

	limiter := &rateLimiter{
		config:     config,
		store:      newMockRedisStore(),
		algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
		keyBuilder: NewKeyBuilder(config.KeyPrefix),
	}

	ctx := context.Background()
	// Use a tier that gets the default limits (100 requests)
	entity := NewDefaultAuthEntity("concurrent_user", EntityTypeUser, "custom")

	var wg sync.WaitGroup
	concurrentRequests := 50
	results := make(chan bool, concurrentRequests)

	// Launch concurrent requests
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			result, err := limiter.Allow(ctx, entity, ScopeGlobal)
			if err != nil {
				t.Errorf("Unexpected error in goroutine %d: %v", id, err)
				return
			}
			results <- result.Allowed
		}(i)
	}

	wg.Wait()
	close(results)

	// Count allowed requests
	allowedCount := 0
	for allowed := range results {
		if allowed {
			allowedCount++
		}
	}

	// All requests should be allowed since we have 100 limit and only 50 concurrent requests
	if allowedCount != concurrentRequests {
		t.Errorf("Expected %d requests to be allowed, got %d", concurrentRequests, allowedCount)
	}
}

func TestRateLimiterClose(t *testing.T) {
	config := DefaultConfig()

	limiter := &rateLimiter{
		config:     config,
		store:      newMockRedisStore(),
		algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
		keyBuilder: NewKeyBuilder(config.KeyPrefix),
	}

	ctx := context.Background()
	entity := NewDefaultAuthEntity("close_user", EntityTypeUser, TierFree)

	// Should work before closing
	result, err := limiter.Allow(ctx, entity, ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error before close: %v", err)
	}
	if !result.Allowed {
		t.Error("Expected request to be allowed before close")
	}

	// Close the limiter
	err = limiter.Close()
	if err != nil {
		t.Fatalf("Unexpected error during close: %v", err)
	}

	// Should not work after closing
	result, err = limiter.Allow(ctx, entity, ScopeGlobal)
	if err == nil {
		t.Error("Expected error after close")
	}
	if result != nil {
		t.Error("Expected nil result after close")
	}
}

// Benchmark integration tests
func BenchmarkRateLimiterIntegration(b *testing.B) {
	config := DefaultConfig()
	config.DefaultLimits[ScopeGlobal] = RateLimit{
		Requests:  int64(b.N + 1000), // Ensure we don't hit limits during benchmark
		Window:    time.Hour,
		BurstSize: int64(b.N + 1000),
	}

	limiter := &rateLimiter{
		config:     config,
		store:      newMockRedisStore(),
		algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
		keyBuilder: NewKeyBuilder(config.KeyPrefix),
	}

	ctx := context.Background()
	entity := NewDefaultAuthEntity("benchmark_user", EntityTypeUser, TierFree)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx, entity, ScopeGlobal)
	}
}

func BenchmarkRateLimiterConcurrent(b *testing.B) {
	config := DefaultConfig()
	config.DefaultLimits[ScopeGlobal] = RateLimit{
		Requests:  int64(b.N + 1000),
		Window:    time.Hour,
		BurstSize: int64(b.N + 1000),
	}

	limiter := &rateLimiter{
		config:     config,
		store:      newMockRedisStore(),
		algorithm:  &tokenBucketWrapper{algorithm: algorithms.NewTokenBucketAlgorithm()},
		keyBuilder: NewKeyBuilder(config.KeyPrefix),
	}

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			entity := NewDefaultAuthEntity(fmt.Sprintf("user_%d", i%100), EntityTypeUser, TierFree)
			limiter.Allow(ctx, entity, ScopeGlobal)
			i++
		}
	})
}
