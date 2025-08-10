// examples/presets/main.go - Smart preset demonstrations
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("🎯 Smart Presets Demonstration")
	fmt.Println("==============================")

	// API Gateway preset - perfect for high-traffic gateways
	apiGateway := ratelimit.APIGateway().OnDenied(customDeniedHandler)

	// SaaS App preset - multi-tenant with user tiers
	saasApp := ratelimit.SaaSApp().OnDenied(customDeniedHandler)

	// Public API preset - authentication-based limiting
	publicAPI := ratelimit.PublicAPI().OnDenied(customDeniedHandler)

	// Microservice preset - service-to-service communication
	microservice := ratelimit.Microservice().OnDenied(customDeniedHandler)

	// Web App preset - session-based limiting
	webApp := ratelimit.WebApp().OnDenied(customDeniedHandler)

	// Build all limiters
	apiGatewayLimiter, _ := apiGateway.Build()
	saasAppLimiter, _ := saasApp.Build()
	publicAPILimiter, _ := publicAPI.Build()
	microserviceLimiter, _ := microservice.Build()
	webAppLimiter, _ := webApp.Build()

	// Create different servers showcasing each preset
	mux := http.NewServeMux()

	// API Gateway example (port 8080)
	mux.HandleFunc("/gateway/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"preset": "API Gateway",
			"description": "High-throughput gateway with multiple scopes",
			"features": ["IP-based limiting", "Multiple scopes", "High limits"],
			"scopes": {
				"global": "10000/hour",
				"auth": "100/hour", 
				"search": "1000/hour"
			}
		}`))
	})

	// SaaS App example
	mux.HandleFunc("/saas/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"preset": "SaaS Application",  
			"description": "Multi-tenant application with user tiers",
			"features": ["User-based limiting", "Tier-based limits", "Per-tenant isolation"],
			"tiers": {
				"free": "100/hour",
				"premium": "5000/hour",
				"enterprise": "50000/hour"
			}
		}`))
	})

	// Public API example
	mux.HandleFunc("/public/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"preset": "Public API",
			"description": "Authentication-based rate limiting",
			"features": ["API key limiting", "Flexible authentication", "Public access tiers"],
			"auth_methods": ["API Key", "Bearer Token", "IP fallback"]
		}`))
	})

	// Microservice example
	mux.HandleFunc("/micro/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"preset": "Microservice",
			"description": "Service-to-service communication limits",
			"features": ["Service identity", "Internal/external limits", "High performance"],
			"service_types": {
				"internal": "50000/minute",
				"external": "1000/minute"
			}
		}`))
	})

	// Web App example
	mux.HandleFunc("/webapp/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"preset": "Web Application",
			"description": "Session-based limiting for web apps",
			"features": ["Session tracking", "User tiers", "Action-specific limits"],
			"actions": {
				"login": "10/hour",
				"register": "5/hour",  
				"upload": "20/hour"
			}
		}`))
	})

	// Preset comparison endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"service": "Gorly Smart Presets Demo",
			"description": "Demonstrates 5 smart presets for common scenarios",
			"presets": {
				"api_gateway": {
					"url": "http://localhost:8080/gateway/",
					"description": "High-throughput API gateway",
					"use_case": "Large-scale API management"
				},
				"saas_app": {
					"url": "http://localhost:8080/saas/", 
					"description": "Multi-tenant SaaS application",
					"use_case": "SaaS platforms with user tiers"
				},
				"public_api": {
					"url": "http://localhost:8080/public/",
					"description": "Public API with authentication",  
					"use_case": "REST APIs with API keys"
				},
				"microservice": {
					"url": "http://localhost:8080/micro/",
					"description": "Microservice communication",
					"use_case": "Service mesh rate limiting"
				},
				"web_app": {
					"url": "http://localhost:8080/webapp/",
					"description": "Web application sessions",
					"use_case": "Traditional web applications"
				}
			},
			"test_commands": [
				"curl http://localhost:8080/gateway/ -H 'X-Scope: auth'",
				"curl http://localhost:8080/saas/ -H 'X-User-Tier: premium'",
				"curl http://localhost:8080/public/ -H 'X-API-Key: test-key-123'",
				"curl http://localhost:8080/micro/ -H 'X-Service-ID: user-service'",
				"curl http://localhost:8080/webapp/ -H 'X-Session-ID: sess-abc123'"
			]
		}`))
	})

	// Create route handlers with different presets
	rateLimitedMux := http.NewServeMux()

	// Apply different presets to different routes
	rateLimitedMux.Handle("/gateway/",
		apiGatewayLimiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
			http.StripPrefix("/gateway", mux)))

	rateLimitedMux.Handle("/saas/",
		saasAppLimiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
			http.StripPrefix("/saas", mux)))

	rateLimitedMux.Handle("/public/",
		publicAPILimiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
			http.StripPrefix("/public", mux)))

	rateLimitedMux.Handle("/micro/",
		microserviceLimiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
			http.StripPrefix("/micro", mux)))

	rateLimitedMux.Handle("/webapp/",
		webAppLimiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
			http.StripPrefix("/webapp", mux)))

	// Main info endpoint (no rate limiting)
	rateLimitedMux.Handle("/", mux)

	fmt.Println("\n✅ Smart Presets Loaded:")
	fmt.Println("   🌐 API Gateway    - /gateway/  (IP-based, multi-scope)")
	fmt.Println("   🏢 SaaS App       - /saas/     (User tiers, multi-tenant)")
	fmt.Println("   🔑 Public API     - /public/   (API key based)")
	fmt.Println("   🔧 Microservice   - /micro/    (Service-to-service)")
	fmt.Println("   🌍 Web App        - /webapp/   (Session-based)")

	fmt.Println("\n🧪 Test Each Preset:")
	fmt.Println("   curl http://localhost:8080/                              # Overview")
	fmt.Println("   curl http://localhost:8080/gateway/ -H 'X-Scope: auth'   # API Gateway")
	fmt.Println("   curl http://localhost:8080/saas/ -H 'X-User-Tier: free'  # SaaS App")
	fmt.Println("   curl http://localhost:8080/public/ -H 'X-API-Key: test'  # Public API")

	fmt.Println("\n🎯 Each preset optimized for its use case!")
	fmt.Println("🚀 Server starting on :8080...")

	log.Fatal(http.ListenAndServe(":8080", rateLimitedMux))
}

func customDeniedHandler(w http.ResponseWriter, r *http.Request, result *ratelimit.LimitResult) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	// Determine preset based on path
	preset := "Unknown"
	if path := r.URL.Path; len(path) > 1 {
		parts := path[1:]
		if idx := strings.Index(parts, "/"); idx > 0 {
			preset = strings.ToUpper(string(parts[0])) + strings.ToLower(parts[1:idx])
		}
	}

	response := map[string]interface{}{
		"error":       "Rate limit exceeded",
		"preset":      preset,
		"limit":       result.Limit,
		"used":        result.Used,
		"remaining":   result.Remaining,
		"retry_after": result.RetryAfter.Seconds(),
		"window":      result.Window.String(),
		"advice":      "Each preset has different limits. Try a different endpoint or wait for reset.",
		"next_reset":  result.ResetTime.Format("2006-01-02T15:04:05Z07:00"),
	}

	json.NewEncoder(w).Encode(response)
}
