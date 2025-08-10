// examples/advanced/main.go - Advanced configuration demonstration
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🔧 Advanced Rate Limiting Configuration")
	fmt.Println("======================================")

	mux := http.NewServeMux()

	// Different endpoints with different behaviors
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "File upload endpoint", "scope": "upload"}`))
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Search endpoint", "scope": "search"}`))
	})

	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "General data endpoint", "scope": "global"}`))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"service": "Gorly Advanced Example",
			"features": [
				"Multi-scope rate limiting",
				"Tier-based limits (free/premium)", 
				"Custom entity extraction",
				"Custom denied responses",
				"Automatic rate limit headers"
			],
			"test_instructions": {
				"api_key_tiers": {
					"free": "curl -H 'X-API-Key: free-user-123' http://localhost:8080/api/data",
					"premium": "curl -H 'X-API-Key: premium-user-456' http://localhost:8080/api/data"
				},
				"different_scopes": {
					"upload": "curl http://localhost:8080/api/upload (5/min limit)",
					"search": "curl http://localhost:8080/api/search (20/min limit)", 
					"general": "curl http://localhost:8080/api/data (100/min limit)"
				}
			}
		}`))
	})

	// Advanced fluent configuration! 🎯
	limiter, err := ratelimit.New().
		Algorithm("sliding_window"). // Precise rate limiting
		Limits(map[string]string{    // Different limits per scope
			"global": "100/minute", // General endpoints
			"upload": "5/minute",   // File uploads (strict)
			"search": "20/minute",  // Search queries
		}).
		TierLimits(map[string]string{ // User tier overrides
			"free":    "10/minute",  // Free tier users
			"premium": "200/minute", // Premium users
		}).
		ExtractorFunc(func(r *http.Request) string { // Smart entity extraction
			// Try API key first
			if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
				// Extract tier from API key prefix
				if strings.HasPrefix(apiKey, "free-") {
					return "free:" + apiKey
				} else if strings.HasPrefix(apiKey, "premium-") {
					return "premium:" + apiKey
				}
				return "user:" + apiKey
			}

			// Fallback to IP
			return extractIP(r)
		}).
		ScopeFunc(func(r *http.Request) string { // Smart scope detection
			if strings.HasPrefix(r.URL.Path, "/api/upload") {
				return "upload"
			} else if strings.HasPrefix(r.URL.Path, "/api/search") {
				return "search"
			}
			return "global"
		}).
		OnDenied(func(w http.ResponseWriter, r *http.Request, result *ratelimit.LimitResult) {
			// Custom denied response with full details
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)

			response := map[string]interface{}{
				"error":       "Rate limit exceeded",
				"limit":       result.Limit,
				"remaining":   result.Remaining,
				"used":        result.Used,
				"retry_after": result.RetryAfter.Seconds(),
				"window":      result.Window.String(),
				"reset_time":  result.ResetTime.Format("2006-01-02T15:04:05Z07:00"),
				"scope":       extractScope(r),
				"entity":      extractEntity(r),
				"advice":      generateAdvice(result),
			}

			json.NewEncoder(w).Encode(response)
		}).Build()

	if err != nil {
		log.Fatalf("Failed to build limiter: %v", err)
	}

	// Apply rate limiting to API endpoints
	rateLimitedMux := http.NewServeMux()

	// Rate limited API endpoints
	apiHandler := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
		http.StripPrefix("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mux.ServeHTTP(w, r)
		})),
	)
	rateLimitedMux.Handle("/api/", apiHandler)

	// Non-rate limited endpoints
	rateLimitedMux.Handle("/status", mux)
	rateLimitedMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/status", http.StatusFound)
	}))

	fmt.Println("\n✅ Advanced Configuration Active:")
	fmt.Println("   Algorithm: Sliding Window (precise)")
	fmt.Println("   Scopes: global(100/min), upload(5/min), search(20/min)")
	fmt.Println("   Tiers: free(10/min), premium(200/min)")
	fmt.Println("   Custom entity extraction (API keys or IP)")
	fmt.Println("   Custom denied responses with full details")

	fmt.Println("\n🔗 Test Endpoints:")
	fmt.Println("   http://localhost:8080/status           - Service status")
	fmt.Println("   http://localhost:8080/api/data         - General data (global scope)")
	fmt.Println("   http://localhost:8080/api/upload       - Upload (strict limits)")
	fmt.Println("   http://localhost:8080/api/search       - Search (moderate limits)")

	fmt.Println("\n🧪 Try Different Scenarios:")
	fmt.Println("   # Free tier user")
	fmt.Println("   curl -H 'X-API-Key: free-user-123' http://localhost:8080/api/data")
	fmt.Println()
	fmt.Println("   # Premium user")
	fmt.Println("   curl -H 'X-API-Key: premium-user-456' http://localhost:8080/api/data")
	fmt.Println()
	fmt.Println("   # Different scopes")
	fmt.Println("   curl http://localhost:8080/api/upload   # 5/min limit")
	fmt.Println("   curl http://localhost:8080/api/search   # 20/min limit")

	fmt.Println("\n🚀 Server starting on :8080...")

	log.Fatal(http.ListenAndServe(":8080", rateLimitedMux))
}

// Helper functions
func extractIP(r *http.Request) string {
	// Try X-Forwarded-For first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	parts := strings.Split(r.RemoteAddr, ":")
	return parts[0]
}

func extractScope(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/api/upload") {
		return "upload"
	} else if strings.HasPrefix(r.URL.Path, "/api/search") {
		return "search"
	}
	return "global"
}

func extractEntity(r *http.Request) string {
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		if strings.HasPrefix(apiKey, "free-") {
			return "free:" + apiKey
		} else if strings.HasPrefix(apiKey, "premium-") {
			return "premium:" + apiKey
		}
		return "user:" + apiKey
	}
	return "ip:" + extractIP(r)
}

func generateAdvice(result *ratelimit.LimitResult) string {
	if result.RetryAfter.Seconds() < 60 {
		return "Rate limit exceeded. Wait " + strconv.Itoa(int(result.RetryAfter.Seconds())) + " seconds before retrying."
	} else if result.RetryAfter.Minutes() < 60 {
		return "Rate limit exceeded. Wait " + strconv.Itoa(int(result.RetryAfter.Minutes())) + " minutes before retrying."
	} else {
		return "Rate limit exceeded. Consider upgrading to a higher tier or try again later."
	}
}
