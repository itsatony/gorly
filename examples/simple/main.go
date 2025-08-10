// examples/simple/main.go - Simple example of the new Gorly API
package main

import (
	"log"
	"net/http"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	// Example 1: Dead simple IP-based rate limiting
	// This is all you need for basic rate limiting!
	mux := http.NewServeMux()

	// Add a simple endpoint
	mux.HandleFunc("/api/simple", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Hello from simple rate limited API!"}`))
	})

	// Wrap with rate limiting - just one line!
	limiter := ratelimit.IPLimit("10/minute")

	// Get the middleware and demonstrate basic usage
	middleware := limiter.Middleware()
	_ = middleware // Just verify it works for now

	// For this example, we'll just use the basic HTTP server without middleware integration
	handler := mux

	log.Println("Starting simple server on :8080")
	log.Println("Try: curl http://localhost:8080/api/simple")
	log.Println("Rate limit: 10 requests per minute per IP")

	// Start server with basic rate limiting
	log.Fatal(http.ListenAndServe(":8080", handler))
}

// Example of more advanced usage (commented out)
/*
func advancedExample() {
	// Example 2: Advanced configuration with fluent API
	limiter := gorly.New().
		Redis("localhost:6379").
		Algorithm("sliding_window").
		Limits(map[string]string{
			"global": "1000/hour",
			"upload": "10/minute",
			"search": "100/minute",
		}).
		TierLimits(map[string]string{
			"free":    "100/hour",
			"premium": "10000/hour",
		}).
		ExtractorFunc(func(r *http.Request) string {
			// Custom entity extraction
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				return "key:" + apiKey
			}
			return "ip:" + extractIP(r)
		}).
		ScopeFunc(func(r *http.Request) string {
			// Custom scope extraction
			if strings.HasPrefix(r.URL.Path, "/api/upload") {
				return "upload"
			}
			if strings.HasPrefix(r.URL.Path, "/api/search") {
				return "search"
			}
			return "global"
		}).
		OnDenied(func(w http.ResponseWriter, r *http.Request, result *gorly.Result) {
			// Custom denied response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(429)
			w.Write([]byte(fmt.Sprintf(`{
				"error": "Rate limit exceeded",
				"limit": %d,
				"remaining": %d,
				"retry_after": %d
			}`, result.Limit, result.Remaining, int64(result.RetryAfter.Seconds()))))
		}).
		EnableMetrics()

	// Use with any framework
	middleware := limiter.Middleware()

	// The middleware auto-detects the framework and adapts accordingly
}

func presetExamples() {
	// Example 3: Smart presets for common scenarios

	// API Gateway scenario
	apiGateway := gorly.APIGateway()

	// SaaS application with tiers
	saasApp := gorly.SaaSApp()

	// Public API with authentication
	publicAPI := gorly.PublicAPI()

	// Microservice communication
	microservice := gorly.Microservice()

	// Web application
	webApp := gorly.WebApp()

	// All presets return builders, so you can further customize:
	customSaaS := gorly.SaaSApp().
		Redis("localhost:6379").
		TierLimits(map[string]string{
			"startup": "5000/hour",   // Custom tier
			"enterprise": "1000000/hour",
		})
}
*/
