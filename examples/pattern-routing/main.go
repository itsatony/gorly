// Package main demonstrates pattern-based rate limiting with gorly.
//
// This example shows how to configure different rate limits for different
// API endpoints using exact matching, glob patterns, and regular expressions.
//
// Run with: go run main.go
// Test with: curl http://localhost:8080/api/payment/process
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/itsatony/gorly"
	"github.com/itsatony/gorly/middleware"
	"github.com/itsatony/gorly/routing"
	"github.com/itsatony/gorly/stores"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	fmt.Println("🚀 Starting Gorly Pattern-Based Rate Limiting Example")
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println()

	// ========================================================================
	// 1. CREATE STORE AND LIMITER
	// ========================================================================

	// Create in-memory store (use Redis in production!)
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		log.Fatal("Failed to create store:", err)
	}
	fmt.Println("✓ Created in-memory store")

	// Create rate limiter with configuration
	config := ratelimit.NewDefaultResolverConfig()

	// Configure different limits for different tiers and scopes
	premiumTier := config.TierConfigs[ratelimit.TierPremium]
	premiumTier.SetScopeLimit("payment_critical", ratelimit.NewLimitConfig(5, time.Minute, 1))
	premiumTier.SetScopeLimit("payment", ratelimit.NewLimitConfig(20, time.Minute, 5))
	premiumTier.SetScopeLimit("api_users", ratelimit.NewLimitConfig(100, time.Minute, 20))
	premiumTier.SetScopeLimit("api_admin", ratelimit.NewLimitConfig(50, time.Hour, 10))
	premiumTier.SetScopeLimit("api_versioned", ratelimit.NewLimitConfig(200, time.Minute, 50))
	premiumTier.SetScopeLimit("api_default", ratelimit.NewLimitConfig(1000, time.Hour, 100))

	freeTier := config.TierConfigs[ratelimit.TierFree]
	freeTier.SetScopeLimit("payment_critical", ratelimit.NewLimitConfig(1, time.Hour, 1))
	freeTier.SetScopeLimit("payment", ratelimit.NewLimitConfig(5, time.Hour, 1))
	freeTier.SetScopeLimit("api_users", ratelimit.NewLimitConfig(20, time.Hour, 5))
	freeTier.SetScopeLimit("api_admin", ratelimit.NewLimitConfig(0, time.Hour, 0)) // Blocked for free tier
	freeTier.SetScopeLimit("api_versioned", ratelimit.NewLimitConfig(50, time.Hour, 10))
	freeTier.SetScopeLimit("api_default", ratelimit.NewLimitConfig(100, time.Hour, 20))

	limiter, err := ratelimit.NewWithTiers(store, config)
	if err != nil {
		log.Fatal("Failed to create rate limiter:", err)
	}
	fmt.Println("✓ Created rate limiter with tier-based configuration")
	fmt.Println()

	// ========================================================================
	// 2. CONFIGURE ROUTE RESOLVER WITH PATTERNS
	// ========================================================================

	fmt.Println("📋 Configuring route patterns with metrics:")
	fmt.Println()

	resolver := routing.NewBuilder().
		// Enable Prometheus metrics for pattern matching observability
		WithMetrics(routing.WithPrometheusMetrics()).
		// Exact matches (highest priority = 100)
		AddExact("/api/payment/process", "payment_critical", 100).
		AddExact("/api/payment/refund", "payment_critical", 100).
		// Glob patterns for payment endpoints (priority = 50)
		AddGlob("/api/payment/*", "payment", 50).
		// Glob patterns for user endpoints
		AddGlob("/api/users/*", "api_users", 50).
		AddGlob("/api/users/*/profile", "api_users", 60). // More specific
		// Admin section (high priority due to sensitivity)
		AddGlob("/api/admin/**", "api_admin", 80).
		// API versioning with regex (priority = 30)
		AddRegex("^/api/v[0-9]+/.*$", "api_versioned", 30).
		// Catch-all prefix (lowest priority = 10)
		AddPrefix("/api/", "api_default", 10).
		MustBuild()

	// Print configured patterns
	patterns := resolver.GetPatterns()
	fmt.Println("  Configured Patterns (in priority order):")
	for _, p := range patterns {
		fmt.Printf("    [%3d] %-12s  %-30s → %s\n", p.Priority, p.MatchType.String(), p.Pattern, p.Scope)
	}
	fmt.Println()

	// ========================================================================
	// 3. CREATE HTTP MIDDLEWARE
	// ========================================================================

	// Base extractor gets API key from header
	baseExtractor := middleware.APIKeyContextExtractor("X-API-Key", func(apiKey string) string {
		// Simulate tier lookup (in production, look up from database)
		switch apiKey {
		case "premium-key-123":
			return ratelimit.TierPremium
		case "free-key-456":
			return ratelimit.TierFree
		default:
			return ratelimit.TierFree
		}
	})

	// Wrap with route-aware extractor
	contextExtractor := middleware.RouteAwareContextExtractor(
		resolver,
		baseExtractor,
		ratelimit.ScopeGlobal, // Default scope if no pattern matches
	)

	// Create middleware
	mw, err := middleware.NewHTTPMiddleware(&middleware.HTTPMiddlewareConfig{
		Limiter:          limiter,
		ContextExtractor: contextExtractor,
		AddHeaders:       true, // Add X-RateLimit-* headers
	})
	if err != nil {
		log.Fatal("Failed to create middleware:", err)
	}
	fmt.Println("✓ Created HTTP middleware with route-aware context extraction")
	fmt.Println()

	// ========================================================================
	// 4. SET UP HTTP ROUTES
	// ========================================================================

	mux := http.NewServeMux()

	// Metrics endpoint (Prometheus format)
	mux.Handle("/metrics", promhttp.Handler())

	// Payment endpoints (will match patterns)
	mux.HandleFunc("/api/payment/process", createHandler("Process Payment", "payment_critical"))
	mux.HandleFunc("/api/payment/refund", createHandler("Refund Payment", "payment_critical"))
	mux.HandleFunc("/api/payment/list", createHandler("List Payments", "payment"))
	mux.HandleFunc("/api/payment/history", createHandler("Payment History", "payment"))

	// User endpoints
	mux.HandleFunc("/api/users/123", createHandler("Get User", "api_users"))
	mux.HandleFunc("/api/users/123/profile", createHandler("Get User Profile", "api_users"))
	mux.HandleFunc("/api/users/create", createHandler("Create User", "api_users"))

	// Admin endpoints
	mux.HandleFunc("/api/admin/dashboard", createHandler("Admin Dashboard", "api_admin"))
	mux.HandleFunc("/api/admin/users/list", createHandler("Admin User List", "api_admin"))
	mux.HandleFunc("/api/admin/reports/generate", createHandler("Generate Report", "api_admin"))

	// Versioned API endpoints (will match regex)
	mux.HandleFunc("/api/v1/data", createHandler("Get Data (v1)", "api_versioned"))
	mux.HandleFunc("/api/v2/data", createHandler("Get Data (v2)", "api_versioned"))
	mux.HandleFunc("/api/v10/data", createHandler("Get Data (v10)", "api_versioned"))

	// Other API endpoints (will match default)
	mux.HandleFunc("/api/status", createHandler("API Status", "api_default"))
	mux.HandleFunc("/api/health", createHandler("Health Check", "api_default"))

	// Info endpoint (shows how to test)
	mux.HandleFunc("/", infoHandler)

	// Wrap with rate limiting middleware
	handler := mw.Middleware(mux)

	// ========================================================================
	// 5. START SERVER
	// ========================================================================

	fmt.Println("🌐 Server Configuration:")
	fmt.Println("  Address:  http://localhost:8080")
	fmt.Println("  Metrics:  http://localhost:8080/metrics (Prometheus format)")
	fmt.Println()
	fmt.Println("📝 Example curl commands:")
	fmt.Println()
	fmt.Println("  # Test with premium key (higher limits):")
	fmt.Println("  curl -H \"X-API-Key: premium-key-123\" http://localhost:8080/api/payment/process")
	fmt.Println()
	fmt.Println("  # Test with free key (lower limits):")
	fmt.Println("  curl -H \"X-API-Key: free-key-456\" http://localhost:8080/api/users/123")
	fmt.Println()
	fmt.Println("  # Test admin endpoint (blocked for free tier):")
	fmt.Println("  curl -H \"X-API-Key: free-key-456\" http://localhost:8080/api/admin/dashboard")
	fmt.Println()
	fmt.Println("  # Test versioned API:")
	fmt.Println("  curl -H \"X-API-Key: premium-key-123\" http://localhost:8080/api/v1/data")
	fmt.Println()
	fmt.Println("  # View rate limit headers:")
	fmt.Println("  curl -v -H \"X-API-Key: premium-key-123\" http://localhost:8080/api/payment/process")
	fmt.Println()
	fmt.Println("  # View Prometheus metrics:")
	fmt.Println("  curl http://localhost:8080/metrics")
	fmt.Println()
	fmt.Println("=" + string(make([]byte, 60)))
	fmt.Println("🚀 Server started! Press Ctrl+C to stop")
	fmt.Println()

	log.Fatal(http.ListenAndServe(":8080", handler))
}

// createHandler creates a handler that returns info about the endpoint
func createHandler(name, expectedScope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := map[string]interface{}{
			"success":        true,
			"endpoint":       name,
			"path":           r.URL.Path,
			"expected_scope": expectedScope,
			"message":        fmt.Sprintf("Successfully called: %s", name),
			"rate_limits": map[string]string{
				"limit":     w.Header().Get("X-RateLimit-Limit"),
				"remaining": w.Header().Get("X-RateLimit-Remaining"),
				"reset":     w.Header().Get("X-RateLimit-Reset"),
			},
		}

		json.NewEncoder(w).Encode(response)
	}
}

// infoHandler shows information about testing the API
func infoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `
<!DOCTYPE html>
<html>
<head>
    <title>Gorly Pattern-Based Rate Limiting Example</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 900px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        h2 { color: #666; margin-top: 30px; }
        .pattern { background: #f5f5f5; padding: 10px; margin: 5px 0; border-left: 3px solid #4CAF50; }
        .priority { color: #FF5722; font-weight: bold; }
        .scope { color: #2196F3; font-weight: bold; }
        code { background: #eee; padding: 2px 6px; border-radius: 3px; }
        pre { background: #282c34; color: #abb2bf; padding: 15px; border-radius: 5px; overflow-x: auto; }
        .tier { background: #FFF9C4; padding: 10px; margin: 10px 0; border-radius: 5px; }
    </style>
</head>
<body>
    <h1>🚀 Gorly Pattern-Based Rate Limiting Example</h1>

    <p>This example demonstrates how to configure different rate limits for different API endpoints using pattern matching.</p>

    <h2>📋 Configured Route Patterns</h2>
    <div class="pattern">
        <strong>[100]</strong> <code>exact</code>: /api/payment/process → <span class="scope">payment_critical</span>
    </div>
    <div class="pattern">
        <strong>[100]</strong> <code>exact</code>: /api/payment/refund → <span class="scope">payment_critical</span>
    </div>
    <div class="pattern">
        <strong>[80]</strong> <code>glob</code>: /api/admin/** → <span class="scope">api_admin</span>
    </div>
    <div class="pattern">
        <strong>[60]</strong> <code>glob</code>: /api/users/*/profile → <span class="scope">api_users</span>
    </div>
    <div class="pattern">
        <strong>[50]</strong> <code>glob</code>: /api/payment/* → <span class="scope">payment</span>
    </div>
    <div class="pattern">
        <strong>[50]</strong> <code>glob</code>: /api/users/* → <span class="scope">api_users</span>
    </div>
    <div class="pattern">
        <strong>[30]</strong> <code>regex</code>: ^/api/v[0-9]+/.*$ → <span class="scope">api_versioned</span>
    </div>
    <div class="pattern">
        <strong>[10]</strong> <code>prefix</code>: /api/ → <span class="scope">api_default</span>
    </div>

    <h2>🎟️ API Keys</h2>
    <div class="tier">
        <strong>Premium Tier:</strong> <code>premium-key-123</code><br>
        Higher rate limits, access to all endpoints
    </div>
    <div class="tier">
        <strong>Free Tier:</strong> <code>free-key-456</code><br>
        Lower rate limits, admin endpoints blocked
    </div>

    <h2>🧪 Test Commands</h2>
    <pre>
# Test payment processing (critical endpoint, highest priority)
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/payment/process

# Test payment listing (general payment endpoint)
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/payment/list

# Test user endpoint
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/users/123

# Test user profile (more specific pattern)
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/users/123/profile

# Test admin endpoint with premium key (allowed)
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/admin/dashboard

# Test admin endpoint with free key (rate limited to 0)
curl -H "X-API-Key: free-key-456" http://localhost:8080/api/admin/dashboard

# Test versioned API (matches regex pattern)
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/v1/data
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/v2/data

# View rate limit headers
curl -v -H "X-API-Key: premium-key-123" http://localhost:8080/api/payment/process

# Trigger rate limit (call payment/process 6+ times quickly)
for i in {1..10}; do
  curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/payment/process
  echo ""
done
    </pre>

    <h2>📊 Rate Limit Information</h2>
    <p>Check the response headers for rate limit information:</p>
    <ul>
        <li><code>X-RateLimit-Limit</code>: Maximum requests allowed in the window</li>
        <li><code>X-RateLimit-Remaining</code>: Requests remaining in current window</li>
        <li><code>X-RateLimit-Reset</code>: Unix timestamp when the window resets</li>
        <li><code>Retry-After</code>: Seconds to wait before retrying (when rate limited)</li>
    </ul>

    <h2>🎯 Pattern Matching Priority</h2>
    <p>When multiple patterns match a path, the pattern with the highest priority wins:</p>
    <ol>
        <li><strong>Exact matches</strong> (priority 100) - most specific</li>
        <li><strong>Admin patterns</strong> (priority 80) - high security</li>
        <li><strong>Specific globs</strong> (priority 60) - like user profiles</li>
        <li><strong>General globs</strong> (priority 50) - payment/user sections</li>
        <li><strong>Regex patterns</strong> (priority 30) - API versioning</li>
        <li><strong>Prefix patterns</strong> (priority 10) - catch-all fallback</li>
    </ol>

    <h2>📊 Prometheus Metrics</h2>
    <p>This example includes Prometheus metrics for pattern matching observability:</p>
    <pre>
# View metrics in Prometheus format
curl http://localhost:8080/metrics

# Available metrics:
# - gorly_routing_matches_total{match_type} - Pattern matches by type (exact, prefix, glob, regex)
# - gorly_routing_match_duration_seconds{match_type} - Match duration histogram
# - gorly_routing_patterns_total{match_type} - Configured patterns by type
# - gorly_routing_no_matches_total - Requests that matched no pattern
    </pre>
    <p>Use these metrics to:</p>
    <ul>
        <li>Monitor pattern matching performance (sub-microsecond typical)</li>
        <li>Identify frequently matched patterns</li>
        <li>Detect patterns that never match (configuration issues)</li>
        <li>Optimize pattern priority based on usage data</li>
        <li>Alert on performance degradation</li>
    </ul>
</body>
</html>
    `

	w.Write([]byte(html))
}
