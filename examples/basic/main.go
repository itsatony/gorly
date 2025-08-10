// examples/basic/main.go - Basic rate limiting demonstration
package main

import (
	"fmt"
	"log"
	"net/http"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🚀 Basic Rate Limiting Example")
	fmt.Println("==============================")

	// Create a basic HTTP server with rate limiting
	mux := http.NewServeMux()

	// Simple endpoint
	mux.HandleFunc("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"message": "Success! Request allowed.",
			"endpoint": "/api/test",
			"timestamp": "` + fmt.Sprintf("%d", r.Context().Value("timestamp")) + `"
		}`))
	})

	// Information endpoint
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"service": "Gorly Basic Example",
			"rate_limit": "3 requests per minute per IP",
			"endpoints": {
				"/api/test": "Rate limited endpoint",
				"/info": "Service information (no rate limit)"
			},
			"instructions": [
				"Make 3 requests to /api/test - all should succeed",
				"Make a 4th request - should get HTTP 429",
				"Wait 1 minute and try again"
			]
		}`))
	})

	// One-liner rate limiting! 🎯
	limiter := ratelimit.IPLimit("3/minute")

	// Apply rate limiting only to /api/* endpoints
	rateLimitedMux := http.NewServeMux()

	// Rate limited endpoints
	rateLimitedMux.Handle("/api/", limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
		http.StripPrefix("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add timestamp to context
			r = r.WithContext(r.Context())
			mux.ServeHTTP(w, r)
		})),
	))

	// Non-rate limited endpoints
	rateLimitedMux.Handle("/info", mux)
	rateLimitedMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/info", http.StatusFound)
	}))

	fmt.Println("\n✅ Server Configuration:")
	fmt.Println("   Rate Limit: 3 requests per minute per IP")
	fmt.Println("   Algorithm: Token Bucket (default)")
	fmt.Println("   Storage: In-Memory (default)")

	fmt.Println("\n🔗 Available Endpoints:")
	fmt.Println("   http://localhost:8080/info      - Service info (no rate limit)")
	fmt.Println("   http://localhost:8080/api/test  - Rate limited endpoint")

	fmt.Println("\n🧪 Try these commands:")
	fmt.Println("   curl http://localhost:8080/info")
	fmt.Println("   curl http://localhost:8080/api/test  # Try 4 times quickly!")

	fmt.Println("\n🚀 Server starting on :8080...")

	log.Fatal(http.ListenAndServe(":8080", rateLimitedMux))
}
