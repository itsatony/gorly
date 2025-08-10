// +build redis

// test/redis/integration_test.go
package redis_test

import (
	"context"
	"testing"
	"time"

	ratelimit "github.com/itsatony/gorly"
	"github.com/itsatony/gorly/middleware"
)

func TestRedisIntegration(t *testing.T) {
	// Create Redis configuration
	config := ratelimit.DefaultConfig()
	config.Store = "redis"
	config.Redis = ratelimit.RedisConfig{
		Address:  "localhost:6379",
		Password: "",
		Database: 0,
	}

	// Test basic rate limiter functionality
	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create Redis rate limiter: %v", err)
	}
	defer limiter.Close()

	// Test basic rate limiting
	entity := ratelimit.NewDefaultAuthEntity("test-user", ratelimit.EntityTypeUser, ratelimit.TierFree)
	ctx := context.Background()

	// First request should be allowed
	result, err := limiter.Allow(ctx, entity, ratelimit.ScopeGlobal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result.Allowed {
		t.Error("First request should be allowed")
	}

	t.Logf("Redis rate limit result: limit=%d, remaining=%d, used=%d", 
		result.Limit, result.Remaining, result.Used)
}

func TestRedisMiddleware(t *testing.T) {
	// Create Redis configuration with custom limits
	config := &ratelimit.Config{
		Enabled:   true,
		Algorithm: "sliding_window",
		Store:     "redis",
		Redis: ratelimit.RedisConfig{
			Address:  "localhost:6379",
			Password: "",
			Database: 1, // Use different DB for middleware tests
		},
		// Set low limits for faster testing
		DefaultLimits: map[string]ratelimit.RateLimit{
			ratelimit.ScopeGlobal: {Requests: 5, Window: time.Minute},
		},
		EnableMetrics:  false,
		OperationTimeout: 5 * time.Second,
	}

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create Redis rate limiter: %v", err)
	}
	defer limiter.Close()

	// Test middleware configuration
	middlewareConfig := middleware.DefaultConfig()
	middlewareConfig.Limiter = limiter

	// Test ProcessRequest function with Redis backend
	req := &middleware.RequestInfo{
		Method:     "GET",
		Path:       "/api/test",
		RemoteAddr: "127.0.0.1:12345",
		Context:    context.Background(),
		Requests:   1,
		Headers:    make(map[string][]string),
		Metadata:   make(map[string]interface{}),
	}

	// Process multiple requests to test Redis persistence
	for i := 0; i < 6; i++ {
		result, err := middleware.ProcessRequest(req, middlewareConfig)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}

		t.Logf("Request %d: allowed=%t, remaining=%d, used=%d", 
			i+1, result.Allowed, result.Remaining, result.Used)

		if i < 5 {
			if !result.Allowed {
				t.Errorf("Request %d should be allowed (within limit of 5)", i+1)
			}
		} else {
			if result.Allowed {
				t.Errorf("Request %d should be denied (exceeds limit of 5)", i+1)
			}
		}

		// Small delay between requests
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRedisPluginRegistry(t *testing.T) {
	// Verify all plugins are registered and can work with Redis
	plugins := []string{"gin", "echo", "fiber", "chi"}

	config := ratelimit.DefaultConfig()
	config.Store = "redis"
	config.Redis = ratelimit.RedisConfig{
		Address:  "localhost:6379",
		Password: "",
		Database: 2, // Different DB for plugin tests
	}

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create Redis rate limiter: %v", err)
	}
	defer limiter.Close()

	for _, pluginName := range plugins {
		t.Run("Plugin_"+pluginName, func(t *testing.T) {
			plugin, exists := middleware.Get(pluginName)
			if !exists {
				t.Fatalf("Plugin %s not found", pluginName)
			}

			// Create middleware with Redis backend
			middlewareConfig := middleware.DefaultConfig()
			middlewareConfig.Limiter = limiter
			
			middleware := plugin.CreateMiddleware(limiter, middlewareConfig)
			if middleware == nil {
				t.Fatalf("Plugin %s returned nil middleware", pluginName)
			}

			t.Logf("✅ Plugin %s successfully created middleware with Redis backend", pluginName)
		})
	}
}

func TestRedisHealthCheck(t *testing.T) {
	config := ratelimit.DefaultConfig()
	config.Store = "redis"
	config.Redis = ratelimit.RedisConfig{
		Address:  "localhost:6379",
		Password: "",
		Database: 0,
	}

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create Redis rate limiter: %v", err)
	}
	defer limiter.Close()

	// Test health check
	ctx := context.Background()
	err = limiter.Health(ctx)
	if err != nil {
		t.Fatalf("Redis health check failed: %v", err)
	}

	t.Log("✅ Redis health check passed")
}

func TestRedisStats(t *testing.T) {
	config := ratelimit.DefaultConfig()
	config.Store = "redis"
	config.Redis = ratelimit.RedisConfig{
		Address:  "localhost:6379",
		Password: "",
		Database: 3, // Different DB for stats tests
	}

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create Redis rate limiter: %v", err)
	}
	defer limiter.Close()

	entity := ratelimit.NewDefaultAuthEntity("stats-test-user", ratelimit.EntityTypeUser, ratelimit.TierFree)
	ctx := context.Background()

	// Make some requests to generate stats
	for i := 0; i < 3; i++ {
		_, err := limiter.Allow(ctx, entity, ratelimit.ScopeGlobal)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Get stats
	stats, err := limiter.Stats(ctx, entity)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	t.Logf("Redis stats: total_requests=%d, scopes=%d", 
		stats.TotalRequests, len(stats.Scopes))

	if stats.TotalRequests < 3 {
		t.Errorf("Expected at least 3 total requests, got %d", stats.TotalRequests)
	}

	if len(stats.Scopes) == 0 {
		t.Error("Expected at least one scope in stats")
	}

	// Check scope stats
	if scopeStats, exists := stats.Scopes[ratelimit.ScopeGlobal]; exists {
		t.Logf("Global scope stats: requests=%d, denied=%d", 
			scopeStats.RequestCount, scopeStats.DeniedCount)
	} else {
		t.Error("Expected global scope stats")
	}
}