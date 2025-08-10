// middleware/http.go
package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/itsatony/gorly"
)

// HTTPMiddleware provides rate limiting middleware for standard net/http
type HTTPMiddleware struct {
	config    *HTTPMiddlewareConfig
	limiter   ratelimit.RateLimiter
	extractor HTTPEntityExtractor
}

// HTTPMiddlewareConfig configures the HTTP middleware
type HTTPMiddlewareConfig struct {
	// Limiter is the rate limiter to use
	Limiter ratelimit.RateLimiter

	// EntityExtractor extracts the auth entity from the request
	EntityExtractor HTTPEntityExtractor

	// ScopeExtractor extracts the scope from the request (optional)
	ScopeExtractor HTTPScopeExtractor

	// ErrorHandler handles rate limit errors (optional)
	ErrorHandler HTTPErrorHandler

	// SkipSuccessfulRequests only counts failed requests toward rate limit
	SkipSuccessfulRequests bool

	// SkipPaths contains paths to skip rate limiting
	SkipPaths []string

	// Headers to add to responses
	AddHeaders bool

	// Custom response when rate limited
	CustomResponse *HTTPRateLimitResponse
}

// HTTPEntityExtractor extracts an AuthEntity from an HTTP request
type HTTPEntityExtractor func(r *http.Request) (ratelimit.AuthEntity, error)

// HTTPScopeExtractor extracts the scope from an HTTP request
type HTTPScopeExtractor func(r *http.Request) string

// HTTPErrorHandler handles rate limit errors
type HTTPErrorHandler func(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result)

// HTTPRateLimitResponse defines a custom rate limit response
type HTTPRateLimitResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
}

// NewHTTPMiddleware creates a new HTTP middleware
func NewHTTPMiddleware(config *HTTPMiddlewareConfig) (*HTTPMiddleware, error) {
	if config == nil {
		return nil, fmt.Errorf("middleware config is required")
	}

	if config.Limiter == nil {
		return nil, fmt.Errorf("rate limiter is required")
	}

	if config.EntityExtractor == nil {
		// Default to IP-based extraction
		config.EntityExtractor = DefaultIPEntityExtractor
	}

	if config.ErrorHandler == nil {
		config.ErrorHandler = DefaultHTTPErrorHandler
	}

	return &HTTPMiddleware{
		config:    config,
		limiter:   config.Limiter,
		extractor: config.EntityExtractor,
	}, nil
}

// Middleware returns the HTTP middleware function
func (m *HTTPMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if path should be skipped
		if m.shouldSkipPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract entity
		entity, err := m.extractor(r)
		if err != nil {
			m.config.ErrorHandler(w, r, err, nil)
			return
		}

		// Extract scope
		scope := ratelimit.ScopeGlobal
		if m.config.ScopeExtractor != nil {
			scope = m.config.ScopeExtractor(r)
		}

		// Check rate limit
		result, err := m.limiter.Allow(r.Context(), entity, scope)
		if err != nil {
			m.config.ErrorHandler(w, r, err, result)
			return
		}

		// Add rate limit headers if enabled
		if m.config.AddHeaders {
			m.addRateLimitHeaders(w, result)
		}

		// Check if request is allowed
		if !result.Allowed {
			m.handleRateLimit(w, r, result)
			return
		}

		// Continue with the request
		next.ServeHTTP(w, r)
	})
}

// MiddlewareFunc returns the HTTP middleware function for use with mux.Router.Use()
func (m *HTTPMiddleware) MiddlewareFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.Middleware(http.HandlerFunc(next)).ServeHTTP
}

// shouldSkipPath checks if the path should skip rate limiting
func (m *HTTPMiddleware) shouldSkipPath(path string) bool {
	for _, skipPath := range m.config.SkipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// addRateLimitHeaders adds standard rate limit headers to the response
func (m *HTTPMiddleware) addRateLimitHeaders(w http.ResponseWriter, result *ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
	w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetTime.Unix(), 10))

	if !result.Allowed && result.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
}

// handleRateLimit handles rate limit exceeded responses
func (m *HTTPMiddleware) handleRateLimit(w http.ResponseWriter, r *http.Request, result *ratelimit.Result) {
	if m.config.CustomResponse != nil {
		// Use custom response
		for key, value := range m.config.CustomResponse.Headers {
			w.Header().Set(key, value)
		}

		w.WriteHeader(m.config.CustomResponse.StatusCode)

		if m.config.CustomResponse.Body != nil {
			if bodyBytes, ok := m.config.CustomResponse.Body.([]byte); ok {
				w.Write(bodyBytes)
			} else {
				json.NewEncoder(w).Encode(m.config.CustomResponse.Body)
			}
		}
		return
	}

	// Default rate limit response
	m.addRateLimitHeaders(w, result)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":       "Rate limit exceeded",
		"limit":       result.Limit,
		"window":      result.Window.String(),
		"retry_after": result.RetryAfter.Seconds(),
	}

	json.NewEncoder(w).Encode(response)
}

// DefaultIPEntityExtractor extracts entity information based on IP address
func DefaultIPEntityExtractor(r *http.Request) (ratelimit.AuthEntity, error) {
	ip := getClientIP(r)
	if ip == "" {
		return nil, fmt.Errorf("unable to determine client IP")
	}

	return ratelimit.NewDefaultAuthEntity(
		ip,
		ratelimit.EntityTypeIP,
		ratelimit.TierFree, // Default tier for IP-based limiting
	), nil
}

// APIKeyEntityExtractor creates an entity extractor that uses API keys from headers
func APIKeyEntityExtractor(headerName string, getUserTier func(apiKey string) string) HTTPEntityExtractor {
	return func(r *http.Request) (ratelimit.AuthEntity, error) {
		apiKey := r.Header.Get(headerName)
		if apiKey == "" {
			return nil, fmt.Errorf("API key not found in header %s", headerName)
		}

		tier := ratelimit.TierFree
		if getUserTier != nil {
			tier = getUserTier(apiKey)
		}

		return ratelimit.NewDefaultAuthEntity(
			apiKey,
			ratelimit.EntityTypeAPIKey,
			tier,
		), nil
	}
}

// UserEntityExtractor creates an entity extractor that uses user information from context
func UserEntityExtractor(contextKey string) HTTPEntityExtractor {
	return func(r *http.Request) (ratelimit.AuthEntity, error) {
		userInfo := r.Context().Value(contextKey)
		if userInfo == nil {
			return nil, fmt.Errorf("user information not found in context")
		}

		// Expect userInfo to have ID and Tier methods or be a map
		if user, ok := userInfo.(interface {
			ID() string
			Tier() string
		}); ok {
			return ratelimit.NewDefaultAuthEntity(
				user.ID(),
				ratelimit.EntityTypeUser,
				user.Tier(),
			), nil
		}

		if userMap, ok := userInfo.(map[string]interface{}); ok {
			id, _ := userMap["id"].(string)
			tier, ok := userMap["tier"].(string)
			if !ok {
				tier = ratelimit.TierFree
			}

			if id == "" {
				return nil, fmt.Errorf("user ID not found in context")
			}

			return ratelimit.NewDefaultAuthEntity(
				id,
				ratelimit.EntityTypeUser,
				tier,
			), nil
		}

		return nil, fmt.Errorf("invalid user information format in context")
	}
}

// PathScopeExtractor creates a scope extractor based on URL path patterns
func PathScopeExtractor(pathMappings map[string]string) HTTPScopeExtractor {
	return func(r *http.Request) string {
		path := r.URL.Path

		// Check for exact matches first
		if scope, exists := pathMappings[path]; exists {
			return scope
		}

		// Check for prefix matches
		for pattern, scope := range pathMappings {
			if strings.HasPrefix(path, pattern) {
				return scope
			}
		}

		return ratelimit.ScopeGlobal
	}
}

// MethodScopeExtractor creates a scope extractor based on HTTP method
func MethodScopeExtractor() HTTPScopeExtractor {
	return func(r *http.Request) string {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			return "write"
		case http.MethodDelete:
			return "delete"
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return "read"
		default:
			return ratelimit.ScopeGlobal
		}
	}
}

// CombinedScopeExtractor combines multiple scope extractors
func CombinedScopeExtractor(extractors ...HTTPScopeExtractor) HTTPScopeExtractor {
	return func(r *http.Request) string {
		for _, extractor := range extractors {
			if scope := extractor(r); scope != ratelimit.ScopeGlobal {
				return scope
			}
		}
		return ratelimit.ScopeGlobal
	}
}

// DefaultHTTPErrorHandler provides a default error handler for HTTP middleware
func DefaultHTTPErrorHandler(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)

	response := map[string]interface{}{
		"error":   "Rate limiting error",
		"details": err.Error(),
	}

	json.NewEncoder(w).Encode(response)
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check common headers in order of preference
	headers := []string{
		"CF-Connecting-IP",    // Cloudflare
		"True-Client-IP",      // Cloudflare Enterprise
		"X-Real-IP",           // Nginx
		"X-Forwarded-For",     // Standard
		"X-Client-IP",         // Apache
		"X-Cluster-Client-IP", // Cluster
	}

	for _, header := range headers {
		if ip := r.Header.Get(header); ip != "" {
			// Handle comma-separated list (X-Forwarded-For can contain multiple IPs)
			if strings.Contains(ip, ",") {
				ip = strings.TrimSpace(strings.Split(ip, ",")[0])
			}
			if ip != "" && ip != "unknown" {
				return ip
			}
		}
	}

	// Fallback to RemoteAddr
	if ip := r.RemoteAddr; ip != "" {
		// Remove port if present
		if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
			return ip[:colonIndex]
		}
		return ip
	}

	return ""
}

// DefaultHTTPMiddlewareConfig returns a default configuration for HTTP middleware
func DefaultHTTPMiddlewareConfig(limiter ratelimit.RateLimiter) *HTTPMiddlewareConfig {
	return &HTTPMiddlewareConfig{
		Limiter:         limiter,
		EntityExtractor: DefaultIPEntityExtractor,
		ScopeExtractor:  nil, // Use global scope by default
		ErrorHandler:    DefaultHTTPErrorHandler,
		AddHeaders:      true,
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
		},
	}
}
