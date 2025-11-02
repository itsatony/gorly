// middleware/http.go
package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/itsatony/gorly"
	"github.com/itsatony/gorly/stores"
)

// HTTPMiddleware provides rate limiting middleware for standard net/http
type HTTPMiddleware struct {
	config    *HTTPMiddlewareConfig
	limiter   ratelimit.RateLimiter
	extractor HTTPContextExtractor
	formatter ErrorFormatter
}

// HTTPMiddlewareConfig configures the HTTP middleware
type HTTPMiddlewareConfig struct {
	// Limiter is the rate limiter to use
	Limiter ratelimit.RateLimiter

	// ContextExtractor extracts the rate limit context from the request
	ContextExtractor HTTPContextExtractor

	// ErrorHandler handles rate limit errors (optional)
	ErrorHandler HTTPErrorHandler

	// ErrorFormatter formats errors securely for client responses (optional)
	// Defaults to DefaultErrorFormatter with ExposeDetails=false for security
	ErrorFormatter ErrorFormatter

	// SkipSuccessfulRequests only counts failed requests toward rate limit
	SkipSuccessfulRequests bool

	// SkipPaths contains paths to skip rate limiting
	SkipPaths []string

	// Headers to add to responses
	AddHeaders bool

	// Custom response when rate limited
	CustomResponse *HTTPRateLimitResponse
}

// HTTPContextExtractor extracts a Identity from an HTTP request
type HTTPContextExtractor func(r *http.Request) (ratelimit.Identity, error)

// HTTPErrorHandler handles rate limit errors
type HTTPErrorHandler func(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result)

// HTTPRateLimitResponse defines a custom rate limit response
type HTTPRateLimitResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       interface{}       `json:"body"`
}

// ============================================================================
// ERROR FORMATTING - Secure error message handling
// ============================================================================

// ErrorFormatter formats errors for HTTP responses in a secure way
// It should never expose internal implementation details, stack traces,
// or sensitive information like connection strings or file paths
type ErrorFormatter interface {
	// FormatError converts an internal error into a safe client-facing message
	// The original error is provided for logging/debugging but should NOT be
	// included in the returned message
	FormatError(err error, errorType ErrorType) string
}

// ErrorType categorizes errors for appropriate error messages
type ErrorType string

const (
	// ErrorTypeExtraction indicates failure to extract rate limit context
	ErrorTypeExtraction ErrorType = "extraction"

	// ErrorTypeRateLimit indicates a rate limiting operation error
	ErrorTypeRateLimit ErrorType = "rate_limit"

	// ErrorTypeConfiguration indicates a configuration error
	ErrorTypeConfiguration ErrorType = "configuration"

	// ErrorTypeInternal indicates an unexpected internal error
	ErrorTypeInternal ErrorType = "internal"
)

// DefaultErrorFormatter provides safe, generic error messages
type DefaultErrorFormatter struct {
	// ExposeDetails controls whether to include error details (ONLY for development!)
	// SECURITY WARNING: Never enable this in production!
	ExposeDetails bool

	// Logger for internal error logging (optional)
	Logger *log.Logger
}

// FormatError converts errors to safe client messages
func (f *DefaultErrorFormatter) FormatError(err error, errorType ErrorType) string {
	// Always log the real error internally for debugging
	if f.Logger != nil {
		f.Logger.Printf("[%s] Internal error: %v", errorType, err)
	}

	// SECURITY: Never expose internal errors in production
	if f.ExposeDetails {
		// ONLY for development/debugging - includes actual error
		return fmt.Sprintf("Rate limiting error: %s", err.Error())
	}

	// Production-safe generic messages
	switch errorType {
	case ErrorTypeExtraction:
		return "Unable to process request"
	case ErrorTypeRateLimit:
		return "Rate limiting service unavailable"
	case ErrorTypeConfiguration:
		return "Service temporarily unavailable"
	case ErrorTypeInternal:
		return "Internal error occurred"
	default:
		return "An error occurred"
	}
}

// NewHTTPMiddleware creates a new HTTP middleware
func NewHTTPMiddleware(config *HTTPMiddlewareConfig) (*HTTPMiddleware, error) {
	if config == nil {
		return nil, fmt.Errorf("middleware config is required")
	}

	if config.Limiter == nil {
		return nil, fmt.Errorf("rate limiter is required")
	}

	if config.ContextExtractor == nil {
		// Default to IP-based extraction
		config.ContextExtractor = DefaultIPContextExtractor
	}

	if config.ErrorFormatter == nil {
		// Default to secure error formatter (no details exposed)
		config.ErrorFormatter = &DefaultErrorFormatter{
			ExposeDetails: false, // SECURITY: Never expose details in production
		}
	}

	if config.ErrorHandler == nil {
		// Use new secure error handler with formatter
		config.ErrorHandler = NewSecureHTTPErrorHandler(config.ErrorFormatter)
	}

	return &HTTPMiddleware{
		config:    config,
		limiter:   config.Limiter,
		extractor: config.ContextExtractor,
		formatter: config.ErrorFormatter,
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

		// Extract rate limit context
		rlCtx, err := m.extractor(r)
		if err != nil {
			m.config.ErrorHandler(w, r, err, nil)
			return
		}

		// Check rate limit
		result, err := m.limiter.Allow(r.Context(), rlCtx)
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
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

	if !result.Allowed && result.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(result.RetryAfter.Seconds()), 10))
	}
}

// handleRateLimit handles rate limit exceeded responses
func (m *HTTPMiddleware) handleRateLimit(w http.ResponseWriter, r *http.Request, result *ratelimit.Result) {
	// P0-2 FIX: Always add standard rate limit headers FIRST
	// This ensures headers are present even with custom responses (fixes API contract violation)
	// Standard headers: X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset, Retry-After
	m.addRateLimitHeaders(w, result)

	if m.config.CustomResponse != nil {
		// Use custom response - headers were already added above
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

	// Default rate limit response - headers already added above
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

// DefaultIPContextExtractor extracts context based on IP address
func DefaultIPContextExtractor(r *http.Request) (ratelimit.Identity, error) {
	ip := getClientIP(r)
	if ip == "" {
		// SECURITY: Don't expose that we're looking for IP address
		return nil, fmt.Errorf("unable to process request")
	}

	return ratelimit.NewIPContext(ip), nil
}

// APIKeyContextExtractor creates a context extractor that uses API keys from headers
func APIKeyContextExtractor(headerName string, getUserTier func(apiKey string) string) HTTPContextExtractor {
	return func(r *http.Request) (ratelimit.Identity, error) {
		apiKey := r.Header.Get(headerName)
		if apiKey == "" {
			// SECURITY: Don't expose which header we're looking for
			return nil, fmt.Errorf("authentication required")
		}

		tier := ratelimit.TierFree
		if getUserTier != nil {
			tier = getUserTier(apiKey)
		}

		return ratelimit.NewAPIKeyContext(apiKey, tier), nil
	}
}

// UserContextExtractor creates a context extractor that uses user information from context
func UserContextExtractor(contextKey string) HTTPContextExtractor {
	return func(r *http.Request) (ratelimit.Identity, error) {
		userInfo := r.Context().Value(contextKey)
		if userInfo == nil {
			// SECURITY: Don't expose that we're looking for user info in context
			return nil, fmt.Errorf("authentication required")
		}

		// Expect userInfo to have ID and Tier methods or be a map
		if user, ok := userInfo.(interface {
			ID() string
			Tier() string
		}); ok {
			return ratelimit.NewUserContext(user.ID(), user.Tier()), nil
		}

		if userMap, ok := userInfo.(map[string]interface{}); ok {
			id, _ := userMap["id"].(string)
			tier, ok := userMap["tier"].(string)
			if !ok {
				tier = ratelimit.TierFree
			}

			if id == "" {
				// SECURITY: Generic error message
				return nil, fmt.Errorf("authentication required")
			}

			return ratelimit.NewUserContext(id, tier), nil
		}

		// SECURITY: Don't expose "invalid format" - that's internal implementation
		return nil, fmt.Errorf("authentication required")
	}
}

// PathScopeContextExtractor creates a context extractor with dynamic scope based on URL path
func PathScopeContextExtractor(pathMappings map[string]string, baseExtractor HTTPContextExtractor) HTTPContextExtractor {
	return func(r *http.Request) (ratelimit.Identity, error) {
		// Get base context
		baseCtx, err := baseExtractor(r)
		if err != nil {
			return nil, err
		}

		// Determine scope from path
		path := r.URL.Path
		scope := ratelimit.ScopeGlobal

		// Check for exact matches first
		if s, exists := pathMappings[path]; exists {
			scope = s
		} else {
			// Check for prefix matches
			for pattern, s := range pathMappings {
				if strings.HasPrefix(path, pattern) {
					scope = s
					break
				}
			}
		}

		// Create new context with the determined scope
		return ratelimit.NewSimpleContext(
			baseCtx.Identity(),
			scope,
			baseCtx.Tier(),
			baseCtx.Metadata(),
		), nil
	}
}

// MethodScopeContextExtractor creates a context extractor with scope based on HTTP method
func MethodScopeContextExtractor(baseExtractor HTTPContextExtractor) HTTPContextExtractor {
	return func(r *http.Request) (ratelimit.Identity, error) {
		// Get base context
		baseCtx, err := baseExtractor(r)
		if err != nil {
			return nil, err
		}

		// Determine scope from method
		var scope string
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			scope = "write"
		case http.MethodDelete:
			scope = "delete"
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			scope = "read"
		default:
			scope = ratelimit.ScopeGlobal
		}

		// Create new context with the determined scope
		return ratelimit.NewSimpleContext(
			baseCtx.Identity(),
			scope,
			baseCtx.Tier(),
			baseCtx.Metadata(),
		), nil
	}
}

// NewSecureHTTPErrorHandler creates a secure error handler using the provided formatter
// This is the recommended way to handle errors - it never exposes internal details
func NewSecureHTTPErrorHandler(formatter ErrorFormatter) HTTPErrorHandler {
	return func(w http.ResponseWriter, r *http.Request, err error, result *ratelimit.Result) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable) // 503 instead of 500 for rate limiting issues

		// Determine error type based on context
		errorType := ErrorTypeInternal
		if result == nil {
			// If we don't have a result, it's likely an extraction error
			errorType = ErrorTypeExtraction
		} else {
			// We have a result but got an error, so it's a rate limiting error
			errorType = ErrorTypeRateLimit
		}

		// Format error safely - never expose internal details
		safeMessage := formatter.FormatError(err, errorType)

		response := map[string]interface{}{
			"error": safeMessage,
		}

		json.NewEncoder(w).Encode(response)
	}
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
// This configuration uses secure error handling that never exposes internal details
func DefaultHTTPMiddlewareConfig(limiter ratelimit.RateLimiter) *HTTPMiddlewareConfig {
	formatter := &DefaultErrorFormatter{
		ExposeDetails: false, // SECURITY: Never expose details in production
	}

	return &HTTPMiddlewareConfig{
		Limiter:          limiter,
		ContextExtractor: DefaultIPContextExtractor,
		ErrorFormatter:   formatter,
		ErrorHandler:     NewSecureHTTPErrorHandler(formatter),
		AddHeaders:       true,
		SkipPaths: []string{
			"/health",
			"/metrics",
			"/ready",
		},
	}
}

// ============================================================================
// ONE-LINER HELPERS - Dead simple middleware creation
// ============================================================================

// IPLimit creates IP-based rate limiting middleware in one line
// Perfect for quick prototyping and simple use cases
//
// Example:
//
//	http.Handle("/api/", middleware.IPLimit(limiter, myHandler))
func IPLimit(limiter ratelimit.RateLimiter, next http.Handler) http.Handler {
	m, _ := NewHTTPMiddleware(&HTTPMiddlewareConfig{
		Limiter:          limiter,
		ContextExtractor: DefaultIPContextExtractor,
		AddHeaders:       true,
	})
	return m.Middleware(next)
}

// APIKeyLimit creates API key-based rate limiting middleware in one line
// Extracts API key from the specified header
//
// Example:
//
//	http.Handle("/api/", middleware.APIKeyLimit(limiter, "X-API-Key", getTierFunc, myHandler))
func APIKeyLimit(limiter ratelimit.RateLimiter, headerName string, getTier func(string) string, next http.Handler) http.Handler {
	m, _ := NewHTTPMiddleware(&HTTPMiddlewareConfig{
		Limiter:          limiter,
		ContextExtractor: APIKeyContextExtractor(headerName, getTier),
		AddHeaders:       true,
	})
	return m.Middleware(next)
}

// QuickLimit creates a complete rate limiter + middleware in one line
// Creates an in-memory store, limiter, and middleware all at once
// WARNING: In-memory store - not suitable for multi-instance deployments
//
// Example:
//
//	http.Handle("/api/", middleware.QuickLimit(100, time.Minute, myHandler))
func QuickLimit(limit int64, window time.Duration, next http.Handler) http.Handler {
	store, _ := stores.NewMemoryStore(nil)
	limiter, _ := ratelimit.NewSimple(store, limit, window)
	return IPLimit(limiter, next)
}
