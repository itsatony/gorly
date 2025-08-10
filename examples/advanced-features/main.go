// examples/advanced-features/main.go - Demonstrates advanced Gorly features
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🚀 Gorly Advanced Features Demonstration")
	fmt.Println("========================================")

	// Demonstrate all advanced features
	demonstrateTypedErrors()
	demonstrateHotReload()
	demonstrateObservability()
	demonstrateTestingUtilities()
	demonstrateProductionSetup()
}

func demonstrateTypedErrors() {
	fmt.Println("\n🔥 Advanced Error Handling")
	fmt.Println("-------------------------")

	// Create a limiter with invalid configuration to show error handling
	ctx := context.Background()

	// Example 1: Configuration errors
	fmt.Println("1. Configuration Error Example:")

	builder := ratelimit.New().Limit("global", "invalid-limit")
	limiter, buildErr := builder.Build()
	if buildErr != nil {
		fmt.Printf("   Build Error: %v\n", buildErr)
		return
	}
	_, err := limiter.Check(ctx, "user1", "global")

	if err != nil {
		// Check if it's a rate limit error
		if rateLimitErr, ok := err.(*ratelimit.AdvancedRateLimitError); ok {
			fmt.Printf("   Error Code: %s\n", rateLimitErr.Code)
			fmt.Printf("   Message: %s\n", rateLimitErr.Message)
			fmt.Printf("   HTTP Status: %d\n", rateLimitErr.HTTPStatusCode())
			fmt.Printf("   Retryable: %t\n", rateLimitErr.IsRetryable())

			if len(rateLimitErr.Suggestions) > 0 {
				fmt.Printf("   Suggestions:\n")
				for _, suggestion := range rateLimitErr.Suggestions {
					fmt.Printf("     - %s\n", suggestion)
				}
			}
		}
	}

	// Example 2: Rate limit exceeded error
	fmt.Println("\n2. Rate Limit Exceeded Example:")

	validLimiter := ratelimit.IPLimit("2/minute")

	// Make requests to trigger rate limit
	for i := 1; i <= 4; i++ {
		result, err := validLimiter.Check(ctx, "192.168.1.100", "global")

		if err != nil {
			if ratelimit.IsRateLimitExceeded(err) {
				if rateLimitErr, ok := err.(*ratelimit.AdvancedRateLimitError); ok {
					fmt.Printf("   Request %d: DENIED\n", i)
					fmt.Printf("     Retry after: %v\n", rateLimitErr.RetryAfter)
					fmt.Printf("     Used: %d/%d\n", rateLimitErr.Used, rateLimitErr.Limit)
					fmt.Printf("     Suggestions: %v\n", rateLimitErr.Suggestions)
				}
			}
		} else if result.Allowed {
			fmt.Printf("   Request %d: ALLOWED (%d remaining)\n", i, result.Remaining)
		}
	}

	// Example 3: Error recovery
	fmt.Println("\n3. Error Recovery Example:")

	recovery := ratelimit.NewErrorRecovery(3, time.Second)

	operation := func() error {
		_, err := validLimiter.Check(ctx, "recovery-test", "global")
		return err
	}

	if err := recovery.RetryWithBackoff(operation); err != nil {
		fmt.Printf("   Operation failed after retries: %v\n", err)
	} else {
		fmt.Printf("   ✅ Operation succeeded with retry\n")
	}
}

func demonstrateHotReload() {
	fmt.Println("\n🔄 Hot Reload Configuration")
	fmt.Println("---------------------------")

	// Create a configuration source (file-based for demo)
	configSource := ratelimit.NewHotReloadFileConfigSource("config.json")

	// Create base limiter
	baseLimiter, err := ratelimit.New().
		Limit("global", "100/minute").
		Build()
	if err != nil {
		fmt.Printf("   Error creating base limiter: %v\n", err)
		return
	}

	// Create hot-reloadable limiter
	hotLimiter, err := ratelimit.NewHotReloadableLimiter(baseLimiter, configSource)
	if err != nil {
		fmt.Printf("   Error creating hot-reloadable limiter: %v\n", err)
		return
	}
	defer hotLimiter.Close()

	// Get the hot reload manager
	manager := hotLimiter.GetManager()

	// Set callbacks for configuration updates
	manager.SetUpdateCallback(func(config *ratelimit.HotReloadConfig) {
		fmt.Printf("   ✅ Configuration updated to version %s\n", config.Version)
		fmt.Printf("      New limits: %v\n", config.Limits)
		fmt.Printf("      Algorithm: %s\n", config.Algorithm)
		fmt.Printf("      Updated by: %s at %v\n", config.UpdatedBy, config.UpdatedAt.Format("15:04:05"))
	})

	manager.SetErrorCallback(func(err error) {
		fmt.Printf("   ❌ Hot reload error: %v\n", err)
	})

	// Demonstrate current configuration
	currentConfig := manager.GetCurrentConfig()
	if currentConfig != nil {
		fmt.Printf("   Current configuration version: %s\n", currentConfig.Version)
		fmt.Printf("   Limits: %v\n", currentConfig.Limits)
	}

	// Force a reload to demonstrate the process
	fmt.Println("   🔄 Forcing configuration reload...")
	if err := manager.ForceReload(); err != nil {
		fmt.Printf("   Error during forced reload: %v\n", err)
	}

	// Demonstrate validation rules
	fmt.Println("\n   Configuration validation:")
	rules := ratelimit.DefaultValidationRules()
	testConfig := &ratelimit.HotReloadConfig{
		Limits: map[string]string{
			"global": "1000000/minute", // Too high
		},
		Algorithm: "invalid_algorithm",
	}

	if err := rules.ValidateWithRules(testConfig); err != nil {
		if rateLimitErr, ok := err.(*ratelimit.AdvancedRateLimitError); ok {
			fmt.Printf("   ❌ Validation failed: %s\n", rateLimitErr.Message)
		}
	}
}

func demonstrateObservability() {
	fmt.Println("\n📊 Advanced Observability")
	fmt.Println("-------------------------")

	// Create base limiter
	baseLimiter, err := ratelimit.APIGateway().
		Limits(map[string]string{
			"global": "100/minute",
			"upload": "10/minute",
			"search": "50/minute",
		}).
		Build()
	if err != nil {
		fmt.Printf("   Error creating base limiter: %v\n", err)
		return
	}

	// Create observability configuration
	observabilityConfig := ratelimit.DefaultObservabilityConfig()
	observabilityConfig.LogLevel = ratelimit.LogLevelInfo

	// Create observable limiter
	limiter := ratelimit.NewObservableLimiter(baseLimiter, observabilityConfig)

	ctx := context.Background()

	// Make some requests to generate metrics
	fmt.Println("   🔄 Generating test traffic...")

	entities := []string{"user1", "user2", "user3"}
	scopes := []string{"global", "upload", "search"}

	for i := 0; i < 20; i++ {
		entity := entities[i%len(entities)]
		scope := scopes[i%len(scopes)]

		result, err := limiter.Check(ctx, entity, scope)
		if err != nil {
			fmt.Printf("   Error: %v\n", err)
		} else {
			status := "ALLOWED"
			if !result.Allowed {
				status = "DENIED"
			}
			fmt.Printf("   %s -> %s: %s (%d remaining)\n", entity, scope, status, result.Remaining)
		}

		time.Sleep(time.Millisecond * 50)
	}

	// Display health status
	fmt.Println("\n   📋 Health Status:")
	healthStatus := limiter.GetHealthStatus(ctx)
	fmt.Printf("   Overall: %s (%t)\n", healthStatus.Status, healthStatus.Healthy)
	fmt.Printf("   Duration: %v\n", healthStatus.Duration)

	for name, check := range healthStatus.Checks {
		status := "✅"
		if !check.Healthy {
			status = "❌"
		}
		fmt.Printf("   %s %s: %v (%v)\n", status, name, check.Healthy, check.Duration)
	}

	// Display metrics
	fmt.Println("\n   📈 Metrics Summary:")
	metrics := limiter.GetMetrics()

	if requestTotal, ok := metrics["request_total"].(map[string]int64); ok {
		fmt.Printf("   Request totals:\n")
		for key, count := range requestTotal {
			fmt.Printf("     %s: %d\n", key, count)
		}
	}

	if avgDuration, ok := metrics["avg_request_duration"].(time.Duration); ok {
		fmt.Printf("   Average latency: %v\n", avgDuration)
	}

	// Demonstrate alerting
	fmt.Println("\n   🚨 Alert Management:")
	alertManager := ratelimit.NewAlertManager()
	alertManager.SetThreshold("error_rate", 30.0)
	alertManager.AddHandler(ratelimit.ConsoleAlertHandler)

	alertManager.CheckMetrics(metrics)
	alerts := alertManager.GetAlerts()

	if len(alerts) > 0 {
		fmt.Printf("   Active alerts: %d\n", len(alerts))
		for _, alert := range alerts {
			fmt.Printf("     - %s: %s\n", alert.Name, alert.Message)
		}
	} else {
		fmt.Printf("   ✅ No alerts active\n")
	}
}

func demonstrateTestingUtilities() {
	fmt.Println("\n🧪 Testing Utilities")
	fmt.Println("-------------------")

	// Create test limiter
	limiter, err := ratelimit.New().
		Limit("global", "5/minute").
		Limit("upload", "2/minute").
		Build()
	if err != nil {
		fmt.Printf("   Error creating test limiter: %v\n", err)
		return
	}

	// Testing with helper
	helper := ratelimit.NewTestHelper(limiter)

	fmt.Println("   1. Basic Testing:")
	ctx := context.Background()
	result := helper.TestLimit(ctx, "test-user", "global", 8, time.Millisecond*100)
	fmt.Printf("      Requests: 8, Allowed: %d, Denied: %d, Duration: %v\n",
		result.ActualAllow, result.ActualDeny, result.Duration)

	// Scenario testing
	fmt.Println("\n   2. Scenario Testing:")
	scenario := ratelimit.TestScenario{
		Name:        "Upload limits test",
		Entity:      "upload-user",
		Scope:       "upload",
		Requests:    5,
		Interval:    time.Millisecond * 100,
		ExpectAllow: 2,
		ExpectDeny:  3,
	}

	scenarioResult := helper.RunScenario(ctx, scenario)
	fmt.Printf("      %s: Success=%t, Allowed=%d, Denied=%d\n",
		scenario.Name, scenarioResult.Success,
		scenarioResult.ActualAllow, scenarioResult.ActualDeny)

	if !scenarioResult.Success && scenarioResult.Error != "" {
		fmt.Printf("      Error: %s\n", scenarioResult.Error)
	}

	// Concurrent testing
	fmt.Println("\n   3. Concurrent Testing:")
	concurrentResult := helper.RunConcurrentTest(ctx, "concurrent-user", "global", 3, 5)
	fmt.Printf("      Goroutines: %d, Requests each: 5\n", concurrentResult.Goroutines)
	fmt.Printf("      Total allowed: %d, Total denied: %d, Duration: %v\n",
		concurrentResult.TotalAllowed, concurrentResult.TotalDenied, concurrentResult.Duration)

	// HTTP testing
	fmt.Println("\n   4. HTTP Middleware Testing:")
	httpTest := ratelimit.NewMockHTTPTest(limiter)
	httpResult := httpTest.TestHTTPRequests(6, map[string]string{
		"X-User-ID": "http-test-user",
	})

	fmt.Printf("      Total: %d, Allowed: %d (200), Denied: %d (429)\n",
		httpResult.TotalRequests, httpResult.Allowed, httpResult.Denied)

	// Assertions
	fmt.Println("\n   5. Assertion Testing:")
	assert := ratelimit.NewAssertLimitBehavior(limiter)

	// Test allowance
	if err := assert.AssertAllowed(ctx, "assert-user", "global"); err != nil {
		fmt.Printf("      ❌ Assertion failed: %v\n", err)
	} else {
		fmt.Printf("      ✅ First request allowed as expected\n")
	}

	// Get statistics
	stats := helper.GetStats()
	fmt.Printf("\n   📊 Test Statistics:\n")
	fmt.Printf("      Total requests: %d\n", stats.TotalRequests)
	fmt.Printf("      Success rate: %.2f%%\n", stats.SuccessRate)
	fmt.Printf("      Average latency: %v\n", stats.AverageLatency)
}

func demonstrateProductionSetup() {
	fmt.Println("\n🏭 Production-Ready Setup")
	fmt.Println("------------------------")

	// Create production-grade limiter with all features
	baseLimiter, err := ratelimit.APIGateway().
		Redis("localhost:6379"). // In production, use actual Redis cluster
		Algorithm("sliding_window").
		Limits(map[string]string{
			"auth":     "100/hour",
			"search":   "5000/hour",
			"upload":   "500/hour",
			"download": "2000/hour",
			"admin":    "unlimited",
		}).
		TierLimits(map[string]string{
			"free":       "1000/hour",
			"starter":    "10000/hour",
			"business":   "100000/hour",
			"enterprise": "1000000/hour",
		}).
		ExtractorFunc(ratelimit.ExtractEntityWithTier).
		ScopeFunc(ratelimit.ExtractScope).
		OnDenied(func(w http.ResponseWriter, r *http.Request, result *ratelimit.LimitResult) {
			// Custom production error response
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetTime.Unix()))
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", result.RetryAfter.Seconds()))

			w.WriteHeader(http.StatusTooManyRequests)

			response := map[string]interface{}{
				"error": map[string]interface{}{
					"code":          "RATE_LIMIT_EXCEEDED",
					"message":       "API rate limit exceeded",
					"limit":         result.Limit,
					"used":          result.Used,
					"remaining":     result.Remaining,
					"reset_time":    result.ResetTime.Unix(),
					"retry_after":   int(result.RetryAfter.Seconds()),
					"documentation": "https://api.example.com/docs/rate-limits",
					"upgrade_info":  "https://api.example.com/upgrade",
				},
			}

			json.NewEncoder(w).Encode(response)
		}).
		Build()
	if err != nil {
		fmt.Printf("   ❌ Failed to create production limiter: %v\n", err)
		return
	}

	// Wrap with observability
	observabilityConfig := ratelimit.DefaultObservabilityConfig()
	observabilityConfig.EnableMetrics = true
	observabilityConfig.EnableLogging = true
	observabilityConfig.EnableHealthCheck = true
	observabilityConfig.LogLevel = ratelimit.LogLevelWarn // Only warnings and errors in production

	observableLimiter := ratelimit.NewObservableLimiter(baseLimiter, observabilityConfig)

	// Add hot reload capability
	configSource := ratelimit.NewHTTPConfigSource("https://config.example.com/api/rate-limits")
	hotReloadLimiter, err := ratelimit.NewHotReloadableLimiter(observableLimiter, configSource)
	if err != nil {
		fmt.Printf("   ❌ Failed to create hot-reload limiter: %v\n", err)
		return
	}
	defer hotReloadLimiter.Close()

	fmt.Println("   ✅ Production setup created with:")
	fmt.Println("      - Redis-backed storage")
	fmt.Println("      - Sliding window algorithm")
	fmt.Println("      - Multi-scope rate limiting")
	fmt.Println("      - Tier-based limits")
	fmt.Println("      - Smart entity extraction")
	fmt.Println("      - Custom error responses")
	fmt.Println("      - Full observability")
	fmt.Println("      - Hot reload capability")
	fmt.Println("      - Production error handling")

	// Create monitoring server for production
	monitoringServer := ratelimit.NewMonitoringServer(observableLimiter)

	fmt.Println("\n   🖥️  Production monitoring endpoints:")
	fmt.Println("      /health     - Health checks")
	fmt.Println("      /metrics    - JSON metrics")
	fmt.Println("      /metrics/prometheus - Prometheus format")
	fmt.Println("      /stats      - Rate limiting statistics")
	fmt.Println("      /debug      - Debug information")

	// In production, you would start the monitoring server:
	// go http.ListenAndServe(":9090", monitoringServer)

	fmt.Println("\n   🚀 Ready for production deployment!")
	fmt.Println("      - Deploy with Kubernetes health checks")
	fmt.Println("      - Connect Prometheus to /metrics/prometheus")
	fmt.Println("      - Set up alerts based on error rates")
	fmt.Println("      - Configure log aggregation")
	fmt.Println("      - Enable hot reload from config service")

	// Simulate some production traffic
	fmt.Println("\n   📈 Simulating production traffic...")
	ctx := context.Background()

	testScenarios := []struct {
		entity string
		scope  string
		count  int
	}{
		{"free:user123", "search", 3},
		{"business:corp456", "upload", 2},
		{"enterprise:bigcorp789", "admin", 5},
		{"starter:startup101", "auth", 4},
	}

	for _, scenario := range testScenarios {
		allowed := 0
		denied := 0

		for i := 0; i < scenario.count; i++ {
			result, err := hotReloadLimiter.Check(ctx, scenario.entity, scenario.scope)
			if err != nil {
				fmt.Printf("   Error: %v\n", err)
				continue
			}

			if result.Allowed {
				allowed++
			} else {
				denied++
			}
		}

		fmt.Printf("   %s (%s): %d allowed, %d denied\n",
			scenario.entity, scenario.scope, allowed, denied)
	}

	fmt.Println("\n   ✅ Production demonstration complete!")

	_ = monitoringServer // Avoid unused variable
}

func init() {
	// Set up proper logging for examples
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
