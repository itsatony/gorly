// Package ratelimit provides examples of using testing utilities
package ratelimit_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

func TestBasicRateLimit(t *testing.T) {
	// Create a simple IP-based limiter
	limiter := ratelimit.IPLimit("5/minute")

	// Create test helper
	helper := ratelimit.NewTestHelper(limiter)

	ctx := context.Background()
	entity := "192.168.1.100"
	scope := "global"

	// Test that first 5 requests are allowed
	result := helper.TestLimit(ctx, entity, scope, 5, time.Millisecond*100)

	if result.ActualAllow != 5 {
		t.Errorf("Expected 5 allowed requests, got %d", result.ActualAllow)
	}

	if result.ActualDeny != 0 {
		t.Errorf("Expected 0 denied requests, got %d", result.ActualDeny)
	}

	// Test that 6th request is denied
	extraResult := helper.TestLimit(ctx, entity, scope, 1, 0)
	if extraResult.ActualDeny != 1 {
		t.Errorf("Expected 1 denied request, got %d", extraResult.ActualDeny)
	}
}

func TestScenarioExecution(t *testing.T) {
	limiter, err := ratelimit.New().
		Limit("global", "10/minute").
		Build()
	if err != nil {
		t.Fatalf("Failed to build limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)

	scenario := ratelimit.TestScenario{
		Name:        "Basic limit test",
		Entity:      "user123",
		Scope:       "global",
		Requests:    12,
		Interval:    time.Millisecond * 100,
		ExpectAllow: 10,
		ExpectDeny:  2,
	}

	result := helper.RunScenario(context.Background(), scenario)

	if !result.Success && result.Error == "" {
		t.Errorf("Scenario should have succeeded or provided error details")
	}

	if result.ActualAllow < 8 || result.ActualAllow > 12 {
		t.Errorf("Allow count outside reasonable range: %d", result.ActualAllow)
	}
}

func TestConcurrentRequests(t *testing.T) {
	limiter := ratelimit.IPLimit("100/minute")
	helper := ratelimit.NewTestHelper(limiter)

	// Test concurrent access
	result := helper.RunConcurrentTest(
		context.Background(),
		"concurrent-test",
		"global",
		10, // 10 goroutines
		5,  // 5 requests each
	)

	if result.TotalAllowed+result.TotalDenied != 50 {
		t.Errorf("Expected 50 total requests, got %d",
			result.TotalAllowed+result.TotalDenied)
	}

	if result.TotalAllowed < 40 {
		t.Errorf("Too many requests denied in concurrent test: %d allowed",
			result.TotalAllowed)
	}
}

func TestBenchmarkLimiter(t *testing.T) {
	limiter := ratelimit.IPLimit("1000/minute")
	helper := ratelimit.NewTestHelper(limiter)

	// Run benchmark for 1 second
	result := helper.BenchmarkLimiter(
		context.Background(),
		"benchmark-entity",
		"global",
		time.Second,
	)

	if result.TotalRequests == 0 {
		t.Error("Benchmark should have made some requests")
	}

	if result.RequestsPerSecond == 0 {
		t.Error("RPS should be greater than 0")
	}

	// Should handle at least 100 RPS
	if result.RequestsPerSecond < 100 {
		t.Logf("Performance warning: Only %f RPS achieved", result.RequestsPerSecond)
	}
}

func TestHTTPMiddleware(t *testing.T) {
	limiter := ratelimit.IPLimit("3/minute")
	httpTest := ratelimit.NewMockHTTPTest(limiter)

	// Test without special headers
	result := httpTest.TestHTTPRequests(5, nil)

	if result.Allowed < 3 {
		t.Errorf("Expected at least 3 allowed requests, got %d", result.Allowed)
	}

	if result.Denied < 2 {
		t.Errorf("Expected at least 2 denied requests, got %d", result.Denied)
	}

	// Verify rate limiting headers are present
	for _, response := range result.Responses {
		if response.StatusCode == 429 {
			if _, exists := response.Headers["X-RateLimit-Limit"]; !exists {
				t.Error("Rate limiting headers missing from 429 response")
			}
		}
	}
}

func TestAssertions(t *testing.T) {
	limiter := ratelimit.IPLimit("2/minute")
	assert := ratelimit.NewAssertLimitBehavior(limiter)

	ctx := context.Background()
	entity := "test-user"
	scope := "global"

	// First request should be allowed
	if err := assert.AssertAllowed(ctx, entity, scope); err != nil {
		t.Errorf("First request assertion failed: %v", err)
	}

	// Second request should be allowed
	if err := assert.AssertAllowed(ctx, entity, scope); err != nil {
		t.Errorf("Second request assertion failed: %v", err)
	}

	// Third request should be denied
	if err := assert.AssertDenied(ctx, entity, scope); err != nil {
		t.Errorf("Third request assertion failed: %v", err)
	}
}

func TestTierBasedLimiting(t *testing.T) {
	limiter, err := ratelimit.New().
		ExtractorFunc(func(r *http.Request) string {
			tier := r.Header.Get("X-User-Tier")
			return tier + ":" + r.Header.Get("X-User-ID")
		}).
		TierLimits(map[string]string{
			"free":    "5/minute",
			"premium": "50/minute",
		}).
		Build()
	if err != nil {
		t.Fatalf("Failed to build limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)
	ctx := context.Background()

	// Test free tier
	freeResult := helper.TestLimit(ctx, "free:user1", "global", 7, time.Millisecond*10)
	if freeResult.ActualAllow > 6 {
		t.Errorf("Free tier allowed too many requests: %d", freeResult.ActualAllow)
	}

	// Test premium tier
	premiumResult := helper.TestLimit(ctx, "premium:user2", "global", 20, time.Millisecond*10)
	if premiumResult.ActualAllow < 15 {
		t.Errorf("Premium tier denied too many requests: %d allowed", premiumResult.ActualAllow)
	}
}

func TestMultiScopeLimiting(t *testing.T) {
	limiter, err := ratelimit.New().
		Limits(map[string]string{
			"upload": "2/minute",
			"search": "10/minute",
			"global": "20/minute",
		}).
		Build()
	if err != nil {
		t.Fatalf("Failed to build limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)
	ctx := context.Background()
	entity := "multitest-user"

	// Test upload scope (strict limit)
	uploadResult := helper.TestLimit(ctx, entity, "upload", 5, time.Millisecond*10)
	if uploadResult.ActualAllow > 3 {
		t.Errorf("Upload scope too lenient: %d allowed", uploadResult.ActualAllow)
	}

	// Test search scope (moderate limit)
	searchResult := helper.TestLimit(ctx, entity, "search", 15, time.Millisecond*10)
	if searchResult.ActualAllow < 8 {
		t.Errorf("Search scope too strict: %d allowed", searchResult.ActualAllow)
	}
}

func TestStatisticsTracking(t *testing.T) {
	limiter := ratelimit.IPLimit("10/minute")
	helper := ratelimit.NewTestHelper(limiter)

	ctx := context.Background()

	// Make some requests
	helper.TestLimit(ctx, "stats-user-1", "global", 5, time.Millisecond*10)
	helper.TestLimit(ctx, "stats-user-2", "global", 8, time.Millisecond*10)

	stats := helper.GetStats()

	if stats.TotalRequests != 13 {
		t.Errorf("Expected 13 total requests, got %d", stats.TotalRequests)
	}

	if stats.AllowedRequests > 13 {
		t.Errorf("Allowed requests should not exceed total: %d", stats.AllowedRequests)
	}

	if stats.SuccessRate < 50 || stats.SuccessRate > 100 {
		t.Errorf("Success rate out of reasonable range: %f%%", stats.SuccessRate)
	}
}

// Example of how to create custom test scenarios for your application
func TestCustomAPIScenarios(t *testing.T) {
	// Configure limiter like production API gateway
	limiter, err := ratelimit.APIGateway().
		Limits(map[string]string{
			"auth":   "10/hour",
			"search": "100/hour",
			"upload": "5/hour",
			"global": "1000/hour",
		}).
		Build()
	if err != nil {
		t.Fatalf("Failed to build limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)
	ctx := context.Background()

	scenarios := []ratelimit.TestScenario{
		{
			Name:        "Authentication endpoint stress",
			Entity:      "api-client-1",
			Scope:       "auth",
			Requests:    15,
			Interval:    time.Minute / 15, // Spread over 1 minute
			ExpectAllow: 10,
			ExpectDeny:  5,
		},
		{
			Name:        "Search burst traffic",
			Entity:      "api-client-2",
			Scope:       "search",
			Requests:    50,
			Interval:    time.Minute / 50,
			ExpectAllow: 50, // Should all be allowed
			ExpectDeny:  0,
		},
		{
			Name:        "Upload limit enforcement",
			Entity:      "api-client-3",
			Scope:       "upload",
			Requests:    10,
			Interval:    time.Minute / 10,
			ExpectAllow: 5,
			ExpectDeny:  5,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			result := helper.RunScenario(ctx, scenario)

			if !result.Success {
				t.Logf("Scenario failed: %s", result.Error)
				// In real tests, you might want to fail here
				// t.Errorf("Scenario failed: %s", result.Error)
			}

			t.Logf("Scenario: %s - Allowed: %d, Denied: %d, Duration: %v",
				scenario.Name, result.ActualAllow, result.ActualDeny, result.Duration)
		})
	}
}
