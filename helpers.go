// Package ratelimit provides helper utilities for common rate limiting patterns
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Common extractor functions for testing and development

// ExtractIP extracts IP address from request with proxy support
func ExtractIP(r *http.Request) string {
	// Try X-Forwarded-For first (supports multiple proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Try X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr as fallback
	parts := strings.Split(r.RemoteAddr, ":")
	return parts[0]
}

// ExtractAPIKey extracts API key from various headers and query parameters
func ExtractAPIKey(r *http.Request) string {
	// Try Authorization header (Bearer token)
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		if strings.HasPrefix(auth, "Token ") {
			return strings.TrimPrefix(auth, "Token ")
		}
	}

	// Try custom API key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	// Try query parameter
	if apiKey := r.URL.Query().Get("api_key"); apiKey != "" {
		return apiKey
	}

	return ExtractIP(r) // Fallback to IP
}

// ExtractUserID extracts user ID from JWT, session, or headers
func ExtractUserID(r *http.Request) string {
	// Try custom header first
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Try session cookie (simplified)
	if cookie, err := r.Cookie("session_id"); err == nil {
		return "session:" + cookie.Value
	}

	// Try JWT from Authorization header (simplified)
	if auth := r.Header.Get("Authorization"); auth != "" && strings.HasPrefix(auth, "Bearer ") {
		token := strings.TrimPrefix(auth, "Bearer ")
		return "jwt:" + token[:min(len(token), 16)] // Use first 16 chars as ID
	}

	return ExtractIP(r) // Fallback to IP
}

// ExtractUserTier extracts user tier information
func ExtractUserTier(r *http.Request) string {
	// Try explicit tier header
	if tier := r.Header.Get("X-User-Tier"); tier != "" {
		return tier
	}

	// Try to extract from API key
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		if strings.HasPrefix(apiKey, "free-") {
			return "free"
		}
		if strings.HasPrefix(apiKey, "premium-") {
			return "premium"
		}
		if strings.HasPrefix(apiKey, "enterprise-") {
			return "enterprise"
		}
	}

	return "free" // Default tier
}

// ExtractScope extracts scope based on URL path patterns
func ExtractScope(r *http.Request) string {
	path := r.URL.Path

	// Authentication endpoints
	if strings.Contains(path, "/auth/") || strings.Contains(path, "/login") || strings.Contains(path, "/register") {
		return "auth"
	}

	// Upload endpoints
	if strings.Contains(path, "/upload") || strings.Contains(path, "/files") {
		return "upload"
	}

	// Download endpoints
	if strings.Contains(path, "/download") || strings.Contains(path, "/content") {
		return "download"
	}

	// Search endpoints
	if strings.Contains(path, "/search") || strings.Contains(path, "/query") {
		return "search"
	}

	// Admin endpoints
	if strings.HasPrefix(path, "/admin/") {
		return "admin"
	}

	// API versioned endpoints
	if strings.HasPrefix(path, "/api/v1/") {
		return "api_v1"
	}
	if strings.HasPrefix(path, "/api/v2/") {
		return "api_v2"
	}

	return "global"
}

// Common entity extractors that combine multiple factors

// ExtractEntityWithTier creates entity ID that includes tier information
func ExtractEntityWithTier(r *http.Request) string {
	tier := ExtractUserTier(r)

	// Try to get user-specific identifier
	if userID := ExtractUserID(r); !strings.HasPrefix(userID, "ip:") && userID != ExtractIP(r) {
		return tier + ":" + userID
	}

	// Fall back to IP with tier
	return tier + ":" + ExtractIP(r)
}

// ExtractServiceID extracts service identifier for microservice scenarios
func ExtractServiceID(r *http.Request) string {
	// Try service ID header
	if serviceID := r.Header.Get("X-Service-ID"); serviceID != "" {
		return serviceID
	}

	// Try service name header
	if serviceName := r.Header.Get("X-Service-Name"); serviceName != "" {
		return serviceName
	}

	// Try to extract from User-Agent
	if userAgent := r.Header.Get("User-Agent"); userAgent != "" && strings.Contains(userAgent, "service/") {
		return userAgent
	}

	return "external:" + ExtractIP(r) // External traffic
}

// Timing utilities for rate limiting

// NextWindow returns the time until the next rate limit window
func NextWindow(windowDuration time.Duration) time.Duration {
	now := time.Now()
	windowStart := now.Truncate(windowDuration)
	nextWindow := windowStart.Add(windowDuration)
	return nextWindow.Sub(now)
}

// WindowStart returns the start time of the current window
func WindowStart(windowDuration time.Duration) time.Time {
	return time.Now().Truncate(windowDuration)
}

// ParseLimit parses a limit string like "100/minute" into rate and duration
func ParseLimit(limit string) (int64, time.Duration, error) {
	parts := strings.Split(limit, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid limit format: %s (expected format: '100/minute')", limit)
	}

	// Parse rate
	var rate int64
	if _, err := fmt.Sscanf(parts[0], "%d", &rate); err != nil {
		return 0, 0, fmt.Errorf("invalid rate: %s", parts[0])
	}

	// Parse duration
	var duration time.Duration
	switch strings.ToLower(parts[1]) {
	case "second", "sec", "s":
		duration = time.Second
	case "minute", "min", "m":
		duration = time.Minute
	case "hour", "hr", "h":
		duration = time.Hour
	case "day", "d":
		duration = time.Hour * 24
	default:
		return 0, 0, fmt.Errorf("invalid duration unit: %s", parts[1])
	}

	return rate, duration, nil
}

// FormatLimit formats rate and duration back into a limit string
func FormatLimit(rate int64, duration time.Duration) string {
	switch duration {
	case time.Second:
		return fmt.Sprintf("%d/second", rate)
	case time.Minute:
		return fmt.Sprintf("%d/minute", rate)
	case time.Hour:
		return fmt.Sprintf("%d/hour", rate)
	case time.Hour * 24:
		return fmt.Sprintf("%d/day", rate)
	default:
		return fmt.Sprintf("%d/%s", rate, duration.String())
	}
}

// Development helpers

// DebugExtractor wraps an extractor to log entity extraction for debugging
func DebugExtractor(extractor func(*http.Request) string, logger func(string)) func(*http.Request) string {
	return func(r *http.Request) string {
		entity := extractor(r)
		logger(fmt.Sprintf("Extracted entity: %s from %s %s", entity, r.Method, r.URL.Path))
		return entity
	}
}

// DebugScopeFunc wraps a scope function to log scope extraction for debugging
func DebugScopeFunc(scopeFunc func(*http.Request) string, logger func(string)) func(*http.Request) string {
	return func(r *http.Request) string {
		scope := scopeFunc(r)
		logger(fmt.Sprintf("Extracted scope: %s from %s %s", scope, r.Method, r.URL.Path))
		return scope
	}
}

// MockRequest creates a mock HTTP request for testing
func MockRequest(method, path string, headers map[string]string) *http.Request {
	req, _ := http.NewRequest(method, path, nil)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return req
}

// Quick test utilities

// QuickIPTest tests IP-based rate limiting with default parameters
func QuickIPTest(limit string) func(t interface{}) {
	return func(t interface{}) {
		limiter := IPLimit(limit)
		helper := NewTestHelper(limiter)

		ctx := context.Background()
		result := helper.TestLimit(ctx, "192.168.1.100", "global", 10, time.Millisecond*10)

		if result.ActualAllow+result.ActualDeny != 10 {
			fmt.Printf("Error: Expected 10 total requests, got %d\n", result.ActualAllow+result.ActualDeny)
		}

		fmt.Printf("Limit: %s - Allowed: %d, Denied: %d\n", limit, result.ActualAllow, result.ActualDeny)
	}
}

// QuickAPIKeyTest tests API key-based rate limiting
func QuickAPIKeyTest(limit string) func(t interface{}) {
	return func(t interface{}) {
		limiter := APIKeyLimit(limit)
		helper := NewTestHelper(limiter)

		ctx := context.Background()
		result := helper.TestLimit(ctx, "test-api-key-123", "global", 10, time.Millisecond*10)

		if result.ActualAllow+result.ActualDeny != 10 {
			fmt.Printf("Error: Expected 10 total requests, got %d\n", result.ActualAllow+result.ActualDeny)
		}

		fmt.Printf("API Key Limit: %s - Allowed: %d, Denied: %d\n", limit, result.ActualAllow, result.ActualDeny)
	}
}

// Utility functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Common test scenarios

var CommonScenarios = []TestScenario{
	{
		Name:        "Burst traffic test",
		Entity:      "burst-test-user",
		Scope:       "global",
		Requests:    100,
		Interval:    0, // No interval - burst
		ExpectAllow: 50,
		ExpectDeny:  50,
	},
	{
		Name:        "Steady traffic test",
		Entity:      "steady-test-user",
		Scope:       "global",
		Requests:    20,
		Interval:    time.Second / 20, // 20 RPS
		ExpectAllow: 18,
		ExpectDeny:  2,
	},
	{
		Name:        "Upload stress test",
		Entity:      "upload-test-user",
		Scope:       "upload",
		Requests:    10,
		Interval:    time.Second,
		ExpectAllow: 5,
		ExpectDeny:  5,
	},
}
