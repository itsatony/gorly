// middleware/gin.go
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/itsatony/gorly"
)

// GinPlugin implements MiddlewarePlugin for Gin framework
type GinPlugin struct{}

// Name returns the plugin name
func (p *GinPlugin) Name() string {
	return "gin"
}

// Version returns the supported Gin version
func (p *GinPlugin) Version() string {
	return ">=1.9.0"
}

// CreateMiddleware creates Gin middleware function
func (p *GinPlugin) CreateMiddleware(limiter ratelimit.RateLimiter, config *Config) interface{} {
	if config == nil {
		config = DefaultConfig()
	}
	config.Limiter = limiter

	return func(c *gin.Context) {
		// Extract request information
		reqInfo, err := p.ExtractRequest(c)
		if err != nil {
			if config.Logger != nil {
				config.Logger.Error("Failed to extract request info", err, map[string]interface{}{
					"path":   c.Request.URL.Path,
					"method": c.Request.Method,
				})
			}
			c.AbortWithStatusJSON(config.ResponseConfig.ErrorStatusCode, gin.H{
				"error": "Failed to process request",
			})
			return
		}

		// Process rate limiting
		result, err := ProcessRequest(reqInfo, config)
		if err != nil {
			if config.Logger != nil {
				config.Logger.Error("Rate limiting failed", err, map[string]interface{}{
					"entity_id": reqInfo.EntityID,
					"scope":     reqInfo.Scope,
				})
			}
			c.AbortWithStatusJSON(config.ResponseConfig.ErrorStatusCode, gin.H{
				"error": "Rate limiting failed",
			})
			return
		}

		// Add rate limit headers
		headers := BuildResponseHeaders(result, &config.ResponseConfig)
		for key, value := range headers {
			c.Header(key, value)
		}

		// Check if request is allowed
		if !result.Allowed {
			if config.Logger != nil {
				config.Logger.Info("Request rate limited", map[string]interface{}{
					"entity_id":   reqInfo.EntityID,
					"scope":       reqInfo.Scope,
					"limit":       result.Limit,
					"remaining":   result.Remaining,
					"retry_after": result.RetryAfter.Seconds(),
				})
			}

			// Send rate limited response
			c.AbortWithStatusJSON(config.ResponseConfig.RateLimitedStatusCode, gin.H{
				"error":               "Rate limit exceeded",
				"limit":               result.Limit,
				"remaining":           result.Remaining,
				"retry_after_seconds": int64(result.RetryAfter.Seconds()),
				"window_seconds":      int64(result.Window.Seconds()),
			})
			return
		}

		// Add rate limit info to context for downstream handlers
		c.Set("ratelimit_result", result)
		c.Set("ratelimit_entity_id", reqInfo.EntityID)
		c.Set("ratelimit_scope", reqInfo.Scope)

		// Continue to next handler
		c.Next()
	}
}

// ExtractRequest extracts request information from Gin context
func (p *GinPlugin) ExtractRequest(frameworkRequest interface{}) (*RequestInfo, error) {
	c, ok := frameworkRequest.(*gin.Context)
	if !ok {
		return nil, fmt.Errorf("expected *gin.Context, got %T", frameworkRequest)
	}

	// Extract headers
	headers := make(map[string][]string)
	for key, values := range c.Request.Header {
		headers[key] = values
	}

	// Extract request information
	reqInfo := &RequestInfo{
		Method:     c.Request.Method,
		Path:       c.Request.URL.Path,
		RemoteAddr: c.ClientIP(), // Gin handles X-Forwarded-For automatically
		UserAgent:  c.Request.UserAgent(),
		Headers:    headers,
		Context:    c.Request.Context(),
		Metadata:   make(map[string]interface{}),
	}

	// Add query parameters to metadata
	if len(c.Request.URL.RawQuery) > 0 {
		reqInfo.Metadata["query"] = c.Request.URL.RawQuery
	}

	// Add route pattern to metadata if available
	if c.FullPath() != "" {
		reqInfo.Metadata["route"] = c.FullPath()
	}

	// Extract custom request count if specified
	if reqCountStr := c.GetHeader("X-Request-Count"); reqCountStr != "" {
		if reqCount, err := strconv.ParseInt(reqCountStr, 10, 64); err == nil && reqCount > 0 {
			reqInfo.Requests = reqCount
		}
	}

	return reqInfo, nil
}

// SendResponse sends response using Gin context
func (p *GinPlugin) SendResponse(frameworkResponse interface{}, status int, headers map[string]string, body []byte) error {
	c, ok := frameworkResponse.(*gin.Context)
	if !ok {
		return fmt.Errorf("expected *gin.Context, got %T", frameworkResponse)
	}

	// Set headers
	for key, value := range headers {
		c.Header(key, value)
	}

	// Send response
	c.Data(status, "application/json", body)
	return nil
}

// ============================================================================
// Gin-specific helper functions
// ============================================================================

// GinMiddleware creates a Gin middleware with default configuration
func GinMiddleware(limiter ratelimit.RateLimiter) gin.HandlerFunc {
	plugin := &GinPlugin{}
	middleware := plugin.CreateMiddleware(limiter, DefaultConfig())
	return middleware.(gin.HandlerFunc)
}

// GinMiddlewareWithConfig creates a Gin middleware with custom configuration
func GinMiddlewareWithConfig(limiter ratelimit.RateLimiter, config *Config) gin.HandlerFunc {
	plugin := &GinPlugin{}
	middleware := plugin.CreateMiddleware(limiter, config)
	return middleware.(gin.HandlerFunc)
}

// GinEntityExtractor creates an entity extractor configured for common Gin patterns
func GinEntityExtractor() EntityExtractor {
	return &DefaultEntityExtractor{
		APIKeyHeaders: []string{"X-API-Key", "Authorization"},
		UserIDHeaders: []string{"X-User-ID"},
		UseIPFallback: true,
	}
}

// GinScopeExtractor creates a scope extractor configured for REST APIs
func GinScopeExtractor() ScopeExtractor {
	return &DefaultScopeExtractor{
		PathScopes: map[string]string{
			"/api/v1/auth":   ratelimit.ScopeGlobal,
			"/api/v1/users":  "users",
			"/api/v1/search": ratelimit.ScopeSearch,
			"/api/v1/upload": "upload",
			"/api/v1/admin":  "admin",
		},
		MethodScopes: map[string]string{
			"POST":   "write",
			"PUT":    "write",
			"DELETE": "write",
			"GET":    "read",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}
}

// GinConfig creates a Gin-optimized middleware configuration
func GinConfig(limiter ratelimit.RateLimiter) *Config {
	return &Config{
		Limiter:         limiter,
		EntityExtractor: GinEntityExtractor(),
		ScopeExtractor:  GinScopeExtractor(),
		TierExtractor:   &DefaultTierExtractor{},
		ResponseConfig: ResponseConfig{
			RateLimitedStatusCode: http.StatusTooManyRequests,
			ErrorStatusCode:       http.StatusInternalServerError,
			IncludeHeaders:        true,
			HeaderPrefix:          "X-RateLimit-",
			ContentType:           "application/json",
			RateLimitedResponse:   []byte(`{"error":"Rate limit exceeded","code":"RATE_LIMIT_EXCEEDED"}`),
			ErrorResponse:         []byte(`{"error":"Internal server error","code":"INTERNAL_ERROR"}`),
		},
		Logger:         &NoOpLogger{},
		MetricsEnabled: false,
	}
}

// ============================================================================
// Advanced Gin Features
// ============================================================================

// GinSkipHealthChecks returns a skip function that skips health check endpoints
func GinSkipHealthChecks() SkipFunc {
	healthPaths := []string{"/health", "/healthz", "/ping", "/status", "/metrics"}
	return func(req *RequestInfo) bool {
		path := strings.ToLower(req.Path)
		for _, healthPath := range healthPaths {
			if path == healthPath || strings.HasPrefix(path, healthPath+"/") {
				return true
			}
		}
		return false
	}
}

// GinSkipOptions returns a skip function that skips OPTIONS requests
func GinSkipOptions() SkipFunc {
	return func(req *RequestInfo) bool {
		return req.Method == "OPTIONS"
	}
}

// GinSkipStatic returns a skip function that skips static file requests
func GinSkipStatic(staticPrefixes ...string) SkipFunc {
	if len(staticPrefixes) == 0 {
		staticPrefixes = []string{"/static", "/assets", "/public", "/css", "/js", "/img"}
	}

	return func(req *RequestInfo) bool {
		path := strings.ToLower(req.Path)
		for _, prefix := range staticPrefixes {
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
		// Also skip common static file extensions
		staticExtensions := []string{".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg", ".woff", ".ttf"}
		for _, ext := range staticExtensions {
			if strings.HasSuffix(path, ext) {
				return true
			}
		}
		return false
	}
}

// CombineSkipFuncs combines multiple skip functions with OR logic
func CombineSkipFuncs(skipFuncs ...SkipFunc) SkipFunc {
	return func(req *RequestInfo) bool {
		for _, skipFunc := range skipFuncs {
			if skipFunc != nil && skipFunc(req) {
				return true
			}
		}
		return false
	}
}

// init registers the Gin plugin
func init() {
	Register(&GinPlugin{})
}
