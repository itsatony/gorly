// presets.go - Smart preset configurations for common use cases
package ratelimit

import (
	"net/http"
	"strings"
)

// APIGateway creates a rate limiter optimized for API gateway scenarios
// Features: IP-based limiting, different limits for different endpoint types
func APIGateway() *Builder {
	return New().
		ExtractorFunc(extractIP).
		ScopeFunc(extractAPIScope).
		Limits(map[string]string{
			"global": "10000/hour", // General API access
			"auth":   "100/hour",   // Authentication endpoints
			"search": "1000/hour",  // Search endpoints
			"upload": "50/hour",    // Upload endpoints
			"admin":  "500/hour",   // Admin endpoints
		}).
		EnableMetrics()
}

// SaaSApp creates a rate limiter optimized for multi-tenant SaaS applications
// Features: User-based limiting with tier support
func SaaSApp() *Builder {
	return New().
		ExtractorFunc(extractUserWithTier).
		ScopeFunc(extractAPIScope).
		TierLimits(map[string]string{
			"free":       "1000/hour",
			"premium":    "10000/hour",
			"enterprise": "100000/hour",
		}).
		Limits(map[string]string{
			"upload": "10/hour", // Base upload limit (multiplied by tier)
		}).
		EnableMetrics()
}

// PublicAPI creates a rate limiter for public APIs with API key authentication
// Features: API key-based limiting with generous limits for authenticated users
func PublicAPI() *Builder {
	return New().
		ExtractorFunc(extractAPIKeyOrIP).
		ScopeFunc(extractPublicAPIScope).
		Limits(map[string]string{
			"global":    "5000/hour", // Authenticated users
			"global:ip": "100/hour",  // Unauthenticated (IP-based)
			"search":    "2000/hour", // Search operations
			"write":     "500/hour",  // Write operations
			"heavy":     "50/hour",   // Resource-intensive operations
		}).
		EnableMetrics()
}

// Microservice creates a rate limiter for service-to-service communication
// Features: Service-based limiting with circuit breaker patterns
func Microservice() *Builder {
	return New().
		ExtractorFunc(extractServiceID).
		ScopeFunc(extractServiceScope).
		Limits(map[string]string{
			"global":   "50000/hour",  // High throughput for services
			"external": "5000/hour",   // External service calls
			"database": "10000/hour",  // Database operations
			"cache":    "100000/hour", // Cache operations
		}).
		EnableMetrics()
}

// WebApp creates a rate limiter for web applications
// Features: Session-based limiting with different limits for different user types
func WebApp() *Builder {
	return New().
		ExtractorFunc(extractSessionOrIP).
		ScopeFunc(extractWebScope).
		TierLimits(map[string]string{
			"guest":   "200/hour",
			"user":    "2000/hour",
			"premium": "10000/hour",
			"admin":   "50000/hour",
		}).
		Limits(map[string]string{
			"global":   "1000/hour", // Default global limit
			"login":    "10/hour",   // Login attempts
			"register": "5/hour",    // Registration attempts
			"upload":   "20/hour",   // File uploads
		})
}

// =============================================================================
// Preset-specific extractors and scope functions
// =============================================================================

// extractAPIScope extracts scope for API gateway scenarios
func extractAPIScope(r *http.Request) string {
	path := strings.ToLower(r.URL.Path)

	// Authentication endpoints
	if strings.Contains(path, "/auth") || strings.Contains(path, "/login") || strings.Contains(path, "/token") {
		return "auth"
	}

	// Search endpoints
	if strings.Contains(path, "/search") || strings.Contains(path, "/query") {
		return "search"
	}

	// Upload endpoints
	if strings.Contains(path, "/upload") || strings.Contains(path, "/files") {
		return "upload"
	}

	// Admin endpoints
	if strings.Contains(path, "/admin") || strings.Contains(path, "/manage") {
		return "admin"
	}

	return "global"
}

// extractPublicAPIScope extracts scope for public API scenarios
func extractPublicAPIScope(r *http.Request) string {
	path := strings.ToLower(r.URL.Path)
	method := strings.ToUpper(r.Method)

	// Search operations
	if strings.Contains(path, "/search") || strings.Contains(path, "/query") {
		return "search"
	}

	// Write operations
	if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
		return "write"
	}

	// Heavy operations (analytics, reports, etc.)
	if strings.Contains(path, "/analytics") || strings.Contains(path, "/reports") || strings.Contains(path, "/export") {
		return "heavy"
	}

	return "global"
}

// extractServiceScope extracts scope for microservice scenarios
func extractServiceScope(r *http.Request) string {
	path := strings.ToLower(r.URL.Path)

	// External service calls
	if strings.Contains(path, "/external") || strings.Contains(path, "/webhook") {
		return "external"
	}

	// Database operations
	if strings.Contains(path, "/db") || strings.Contains(path, "/data") {
		return "database"
	}

	// Cache operations
	if strings.Contains(path, "/cache") {
		return "cache"
	}

	return "global"
}

// extractWebScope extracts scope for web application scenarios
func extractWebScope(r *http.Request) string {
	path := strings.ToLower(r.URL.Path)

	// Authentication endpoints
	if strings.Contains(path, "/login") {
		return "login"
	}

	if strings.Contains(path, "/register") || strings.Contains(path, "/signup") {
		return "register"
	}

	// Upload endpoints
	if strings.Contains(path, "/upload") {
		return "upload"
	}

	return "global"
}

// extractUserWithTier extracts user ID and includes tier information
func extractUserWithTier(r *http.Request) string {
	// Try to get user ID from header
	userID := extractUserID(r)
	if userID != "" && userID != extractIP(r) {
		// Also get tier information
		tier := extractTier(r)
		return tier + ":" + userID
	}

	// Fall back to IP with "free" tier
	return "free:" + extractIP(r)
}

// extractAPIKeyOrIP extracts API key or falls back to IP with different scopes
func extractAPIKeyOrIP(r *http.Request) string {
	apiKey := extractAPIKey(r)

	// If we have an API key, use it directly
	if apiKey != "" && apiKey != extractIP(r) {
		return "key:" + apiKey
	}

	// Fall back to IP with special marker
	return "ip:" + extractIP(r)
}

// extractServiceID extracts service identifier from headers
func extractServiceID(r *http.Request) string {
	// Check for service ID in headers
	if serviceID := r.Header.Get("X-Service-ID"); serviceID != "" {
		return "service:" + serviceID
	}

	if serviceID := r.Header.Get("X-Client-ID"); serviceID != "" {
		return "service:" + serviceID
	}

	// Fall back to IP
	return "service:" + extractIP(r)
}

// extractSessionOrIP extracts session ID or falls back to IP
func extractSessionOrIP(r *http.Request) string {
	// Try to get session from cookie
	if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
		// Determine user tier from additional headers/cookies
		tier := "user" // default for authenticated users
		if tierCookie, err := r.Cookie("user_tier"); err == nil {
			tier = tierCookie.Value
		}
		return tier + ":" + cookie.Value
	}

	// Check for session in header
	if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
		tier := extractTier(r)
		if tier == "free" {
			tier = "user"
		}
		return tier + ":" + sessionID
	}

	// Fall back to IP as guest
	return "guest:" + extractIP(r)
}
