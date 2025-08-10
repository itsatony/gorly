// examples/testing/main.go - Comprehensive testing examples for Gorly
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🧪 Gorly Testing Utilities Demonstration")
	fmt.Println("========================================")

	runBasicTests()
	runScenarioTests()
	runConcurrentTests()
	runBenchmarkTests()
	runHTTPTests()
	runAssertionTests()
	runRealWorldTests()
}

func runBasicTests() {
	fmt.Println("\n📊 Basic Rate Limit Testing")
	fmt.Println("----------------------------")

	limiter := ratelimit.IPLimit("5/minute")
	helper := ratelimit.NewTestHelper(limiter)

	ctx := context.Background()
	entity := "192.168.1.100"
	scope := "global"

	// Test basic limit enforcement
	result := helper.TestLimit(ctx, entity, scope, 8, time.Millisecond*100)

	fmt.Printf("✅ Basic Test Results:\n")
	fmt.Printf("   Requests: 8, Allowed: %d, Denied: %d\n",
		result.ActualAllow, result.ActualDeny)
	fmt.Printf("   Duration: %v, Avg Latency: %v\n",
		result.Duration, result.AverageLatency)

	// Verify expected behavior
	if result.ActualAllow <= 5 && result.ActualDeny >= 2 {
		fmt.Printf("   ✅ Rate limiting working correctly!\n")
	} else {
		fmt.Printf("   ⚠️  Unexpected behavior - check configuration\n")
	}
}

func runScenarioTests() {
	fmt.Println("\n🎯 Scenario Testing")
	fmt.Println("-------------------")

	limiter, err := ratelimit.New().
		Limits(map[string]string{
			"global": "10/minute",
			"upload": "3/minute",
			"search": "20/minute",
		}).
		Build()
	if err != nil {
		log.Fatalf("Failed to build limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)

	scenarios := []ratelimit.TestScenario{
		{
			Name:        "Global limit enforcement",
			Entity:      "test-user-1",
			Scope:       "global",
			Requests:    15,
			Interval:    time.Millisecond * 50,
			ExpectAllow: 10,
			ExpectDeny:  5,
		},
		{
			Name:        "Upload strict limits",
			Entity:      "test-user-2",
			Scope:       "upload",
			Requests:    6,
			Interval:    time.Millisecond * 100,
			ExpectAllow: 3,
			ExpectDeny:  3,
		},
		{
			Name:        "Search high throughput",
			Entity:      "test-user-3",
			Scope:       "search",
			Requests:    25,
			Interval:    time.Millisecond * 30,
			ExpectAllow: 20,
			ExpectDeny:  5,
		},
	}

	for _, scenario := range scenarios {
		fmt.Printf("🔍 Running: %s\n", scenario.Name)

		result := helper.RunScenario(context.Background(), scenario)

		fmt.Printf("   Results: %d allowed, %d denied (expected: %d/%d)\n",
			result.ActualAllow, result.ActualDeny,
			scenario.ExpectAllow, scenario.ExpectDeny)

		if result.Success {
			fmt.Printf("   ✅ Scenario passed!\n")
		} else {
			fmt.Printf("   ⚠️  Scenario failed: %s\n", result.Error)
		}

		fmt.Printf("   Duration: %v\n\n", result.Duration)
	}
}

func runConcurrentTests() {
	fmt.Println("\n⚡ Concurrent Load Testing")
	fmt.Println("-------------------------")

	limiter := ratelimit.IPLimit("50/minute")
	helper := ratelimit.NewTestHelper(limiter)

	// Test concurrent access patterns
	result := helper.RunConcurrentTest(
		context.Background(),
		"concurrent-user",
		"global",
		5,  // 5 goroutines
		10, // 10 requests each
	)

	fmt.Printf("✅ Concurrent Test Results:\n")
	fmt.Printf("   Goroutines: %d, Requests each: %d\n",
		result.Goroutines, result.RequestsPerGoroutine)
	fmt.Printf("   Total allowed: %d, Total denied: %d\n",
		result.TotalAllowed, result.TotalDenied)
	fmt.Printf("   Duration: %v\n", result.Duration)

	// Check for race conditions
	totalRequests := result.Goroutines * result.RequestsPerGoroutine
	actualTotal := result.TotalAllowed + result.TotalDenied

	if actualTotal == totalRequests {
		fmt.Printf("   ✅ No requests lost (race condition free!)\n")
	} else {
		fmt.Printf("   ⚠️  Requests lost: %d (possible race condition)\n",
			totalRequests-actualTotal)
	}
}

func runBenchmarkTests() {
	fmt.Println("\n🚀 Performance Benchmarks")
	fmt.Println("-------------------------")

	limiters := map[string]ratelimit.Limiter{
		"Simple IP": ratelimit.IPLimit("1000/minute"),
		"API Key":   ratelimit.APIKeyLimit("1000/minute"),
		"Complex Multi": func() ratelimit.Limiter {
			lim, err := ratelimit.New().
				Limits(map[string]string{
					"global": "1000/minute",
					"upload": "100/minute",
					"search": "500/minute",
				}).
				TierLimits(map[string]string{
					"free":    "100/minute",
					"premium": "2000/minute",
				}).
				Build()
			if err != nil {
				log.Fatalf("Failed to build complex limiter: %v", err)
			}
			return lim
		}(),
	}

	for name, limiter := range limiters {
		fmt.Printf("🔧 Benchmarking: %s\n", name)

		helper := ratelimit.NewTestHelper(limiter)
		result := helper.BenchmarkLimiter(
			context.Background(),
			"bench-entity",
			"global",
			time.Second*2,
		)

		fmt.Printf("   Duration: %v\n", result.Duration)
		fmt.Printf("   Total requests: %d\n", result.TotalRequests)
		fmt.Printf("   RPS: %.2f\n", result.RequestsPerSecond)
		fmt.Printf("   Average latency: %v\n", result.AverageLatency)

		// Performance expectations
		if result.RequestsPerSecond > 1000 {
			fmt.Printf("   ✅ Excellent performance!\n")
		} else if result.RequestsPerSecond > 500 {
			fmt.Printf("   ✅ Good performance\n")
		} else {
			fmt.Printf("   ⚠️  Performance below expectations\n")
		}

		fmt.Println()
	}
}

func runHTTPTests() {
	fmt.Println("\n🌐 HTTP Middleware Testing")
	fmt.Println("--------------------------")

	limiter, err := ratelimit.New().
		ExtractorFunc(func(r *http.Request) string {
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				return "api:" + apiKey
			}
			return "ip:" + ratelimit.ExtractIP(r)
		}).
		Limit("global", "5/minute").
		Build()
	if err != nil {
		log.Fatalf("Failed to build HTTP limiter: %v", err)
	}

	httpTest := ratelimit.NewMockHTTPTest(limiter)

	// Test different request patterns
	testCases := []struct {
		name    string
		count   int
		headers map[string]string
	}{
		{
			name:    "Anonymous requests (IP-based)",
			count:   8,
			headers: nil,
		},
		{
			name:  "API key requests",
			count: 8,
			headers: map[string]string{
				"X-API-Key": "test-key-123",
			},
		},
		{
			name:  "Premium API key",
			count: 8,
			headers: map[string]string{
				"X-API-Key": "premium-key-456",
			},
		},
	}

	for _, tc := range testCases {
		fmt.Printf("🔍 Testing: %s\n", tc.name)

		result := httpTest.TestHTTPRequests(tc.count, tc.headers)

		fmt.Printf("   Total: %d, Allowed: %d (200), Denied: %d (429)\n",
			result.TotalRequests, result.Allowed, result.Denied)

		// Check rate limiting headers
		for i, response := range result.Responses {
			if i < 3 { // Show first few responses
				fmt.Printf("   Response %d: %d", i+1, response.StatusCode)
				if limit, exists := response.Headers["X-RateLimit-Limit"]; exists {
					fmt.Printf(" (Limit: %s", limit)
					if remaining, exists := response.Headers["X-RateLimit-Remaining"]; exists {
						fmt.Printf(", Remaining: %s", remaining)
					}
					fmt.Printf(")")
				}
				fmt.Println()
			}
		}

		fmt.Println()
	}
}

func runAssertionTests() {
	fmt.Println("\n✅ Assertion Testing")
	fmt.Println("--------------------")

	limiter := ratelimit.IPLimit("3/minute")
	assert := ratelimit.NewAssertLimitBehavior(limiter)

	ctx := context.Background()
	entity := "assertion-test-user"
	scope := "global"

	fmt.Printf("🔍 Testing assertion utilities:\n")

	// First 3 requests should be allowed
	for i := 1; i <= 3; i++ {
		if err := assert.AssertAllowed(ctx, entity, scope); err != nil {
			fmt.Printf("   ❌ Request %d failed (should be allowed): %v\n", i, err)
		} else {
			fmt.Printf("   ✅ Request %d allowed as expected\n", i)
		}
	}

	// 4th request should be denied
	if err := assert.AssertDenied(ctx, entity, scope); err != nil {
		fmt.Printf("   ❌ Request 4 failed (should be denied): %v\n", err)
	} else {
		fmt.Printf("   ✅ Request 4 denied as expected\n")
	}
}

func runRealWorldTests() {
	fmt.Println("\n🌍 Real-World Scenario Testing")
	fmt.Println("------------------------------")

	// Simulate production API gateway configuration
	limiter, err := ratelimit.APIGateway().
		Limits(map[string]string{
			"auth":     "20/hour",
			"search":   "500/hour",
			"upload":   "50/hour",
			"download": "200/hour",
			"global":   "2000/hour",
		}).
		TierLimits(map[string]string{
			"free":       "100/hour",
			"premium":    "5000/hour",
			"enterprise": "50000/hour",
		}).
		Build()
	if err != nil {
		log.Fatalf("Failed to build API gateway limiter: %v", err)
	}

	helper := ratelimit.NewTestHelper(limiter)

	fmt.Printf("🏢 API Gateway Load Test:\n")

	// Simulate different user tiers and endpoints
	scenarios := []struct {
		name     string
		entity   string
		scope    string
		requests int
		interval time.Duration
	}{
		{"Free user - search", "free:user1", "search", 30, time.Millisecond * 50},
		{"Premium user - upload", "premium:user2", "upload", 20, time.Millisecond * 100},
		{"Enterprise - bulk download", "enterprise:user3", "download", 50, time.Millisecond * 20},
		{"Anonymous - auth attempts", "ip:192.168.1.10", "auth", 15, time.Millisecond * 200},
	}

	ctx := context.Background()

	for _, scenario := range scenarios {
		fmt.Printf("   🔍 %s:\n", scenario.name)

		result := helper.TestLimit(ctx, scenario.entity, scenario.scope,
			scenario.requests, scenario.interval)

		successRate := float64(result.ActualAllow) / float64(scenario.requests) * 100

		fmt.Printf("      Allowed: %d/%d (%.1f%%), Latency: %v\n",
			result.ActualAllow, scenario.requests, successRate, result.AverageLatency)

		// Performance check
		if result.AverageLatency < time.Millisecond {
			fmt.Printf("      ✅ Excellent response time\n")
		} else if result.AverageLatency < time.Millisecond*10 {
			fmt.Printf("      ✅ Good response time\n")
		} else {
			fmt.Printf("      ⚠️  High latency detected\n")
		}
	}

	// Overall statistics
	fmt.Printf("\n📈 Overall Test Statistics:\n")
	stats := helper.GetStats()
	fmt.Printf("   Total requests: %d\n", stats.TotalRequests)
	fmt.Printf("   Success rate: %.2f%%\n", stats.SuccessRate)
	fmt.Printf("   Average latency: %v\n", stats.AverageLatency)
}

func init() {
	// Suppress detailed logs for cleaner demo output
	log.SetFlags(0)
}
