// middleware/echo.go
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/itsatony/gorly"
	"github.com/labstack/echo/v4"
)

// EchoPlugin implements MiddlewarePlugin for Echo framework
type EchoPlugin struct{}

// Name returns the plugin name
func (p *EchoPlugin) Name() string {
	return "echo"
}

// Version returns the supported Echo version
func (p *EchoPlugin) Version() string {
	return ">=4.0.0"
}

// CreateMiddleware creates Echo middleware function
func (p *EchoPlugin) CreateMiddleware(limiter ratelimit.RateLimiter, config *Config) interface{} {
	if config == nil {
		config = DefaultConfig()
	}
	config.Limiter = limiter

	return echo.MiddlewareFunc(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract request information
			reqInfo, err := p.ExtractRequest(c)
			if err != nil {
				if config.Logger != nil {
					config.Logger.Error("Failed to extract request info", err, map[string]interface{}{
						"path":   c.Request().URL.Path,
						"method": c.Request().Method,
					})
				}
				return c.JSON(config.ResponseConfig.ErrorStatusCode, echo.Map{
					"error": "Failed to process request",
				})
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
				return c.JSON(config.ResponseConfig.ErrorStatusCode, echo.Map{
					"error": "Rate limiting failed",
				})
			}

			// Add rate limit headers
			headers := BuildResponseHeaders(result, &config.ResponseConfig)
			for key, value := range headers {
				c.Response().Header().Set(key, value)
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
				return c.JSON(config.ResponseConfig.RateLimitedStatusCode, echo.Map{
					"error":               "Rate limit exceeded",
					"limit":               result.Limit,
					"remaining":           result.Remaining,
					"retry_after_seconds": int64(result.RetryAfter.Seconds()),
					"window_seconds":      int64(result.Window.Seconds()),
				})
			}

			// Add rate limit info to context for downstream handlers
			c.Set("ratelimit_result", result)
			c.Set("ratelimit_entity_id", reqInfo.EntityID)
			c.Set("ratelimit_scope", reqInfo.Scope)

			// Continue to next handler
			return next(c)
		}
	})
}

// ExtractRequest extracts request information from Echo context
func (p *EchoPlugin) ExtractRequest(frameworkRequest interface{}) (*RequestInfo, error) {
	c, ok := frameworkRequest.(echo.Context)
	if !ok {
		return nil, fmt.Errorf("expected echo.Context, got %T", frameworkRequest)
	}

	req := c.Request()

	// Extract headers
	headers := make(map[string][]string)
	for key, values := range req.Header {
		headers[key] = values
	}

	// Extract request information
	reqInfo := &RequestInfo{
		Method:     req.Method,
		Path:       req.URL.Path,
		RemoteAddr: c.RealIP(), // Echo handles X-Forwarded-For automatically
		UserAgent:  req.UserAgent(),
		Headers:    headers,
		Context:    req.Context(),
		Metadata:   make(map[string]interface{}),
	}

	// Add query parameters to metadata
	if len(req.URL.RawQuery) > 0 {
		reqInfo.Metadata["query"] = req.URL.RawQuery
	}

	// Add route pattern to metadata if available
	if route := c.Path(); route != "" {
		reqInfo.Metadata["route"] = route
	}

	// Add path parameters to metadata
	if paramNames := c.ParamNames(); len(paramNames) > 0 {
		params := make(map[string]string)
		for _, name := range paramNames {
			params[name] = c.Param(name)
		}
		reqInfo.Metadata["params"] = params
	}

	// Extract custom request count if specified
	if reqCountStr := req.Header.Get("X-Request-Count"); reqCountStr != "" {
		if reqCount, err := strconv.ParseInt(reqCountStr, 10, 64); err == nil && reqCount > 0 {
			reqInfo.Requests = reqCount
		}
	}

	return reqInfo, nil
}

// SendResponse sends response using Echo context
func (p *EchoPlugin) SendResponse(frameworkResponse interface{}, status int, headers map[string]string, body []byte) error {
	c, ok := frameworkResponse.(echo.Context)
	if !ok {
		return fmt.Errorf("expected echo.Context, got %T", frameworkResponse)
	}

	// Set headers
	for key, value := range headers {
		c.Response().Header().Set(key, value)
	}

	// Send response
	return c.Blob(status, "application/json", body)
}

// ============================================================================
// Echo-specific helper functions
// ============================================================================

// EchoMiddleware creates an Echo middleware with default configuration
func EchoMiddleware(limiter ratelimit.RateLimiter) echo.MiddlewareFunc {
	plugin := &EchoPlugin{}
	middleware := plugin.CreateMiddleware(limiter, DefaultConfig())
	return middleware.(echo.MiddlewareFunc)
}

// EchoMiddlewareWithConfig creates an Echo middleware with custom configuration
func EchoMiddlewareWithConfig(limiter ratelimit.RateLimiter, config *Config) echo.MiddlewareFunc {
	plugin := &EchoPlugin{}
	middleware := plugin.CreateMiddleware(limiter, config)
	return middleware.(echo.MiddlewareFunc)
}

// EchoEntityExtractor creates an entity extractor configured for common Echo patterns
func EchoEntityExtractor() EntityExtractor {
	return &DefaultEntityExtractor{
		APIKeyHeaders: []string{"X-API-Key", "Authorization", "X-Api-Key"},
		UserIDHeaders: []string{"X-User-ID", "X-User-Id"},
		UseIPFallback: true,
	}
}

// EchoScopeExtractor creates a scope extractor configured for REST APIs
func EchoScopeExtractor() ScopeExtractor {
	return &DefaultScopeExtractor{
		PathScopes: map[string]string{
			"/api/auth":   ratelimit.ScopeGlobal,
			"/api/users":  "users",
			"/api/search": ratelimit.ScopeSearch,
			"/api/upload": "upload",
			"/api/admin":  "admin",
			"/api/public": "public",
		},
		MethodScopes: map[string]string{
			"POST":   "write",
			"PUT":    "write",
			"PATCH":  "write",
			"DELETE": "write",
			"GET":    "read",
			"HEAD":   "read",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}
}

// EchoConfig creates an Echo-optimized middleware configuration
func EchoConfig(limiter ratelimit.RateLimiter) *Config {
	return &Config{
		Limiter:         limiter,
		EntityExtractor: EchoEntityExtractor(),
		ScopeExtractor:  EchoScopeExtractor(),
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
// Advanced Echo Features
// ============================================================================

// EchoSkipHealthChecks returns a skip function that skips health check endpoints
func EchoSkipHealthChecks() SkipFunc {
	healthPaths := []string{"/health", "/healthz", "/ping", "/status", "/metrics", "/debug"}
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

// EchoSkipByRoute returns a skip function that skips requests based on route patterns
func EchoSkipByRoute(routes ...string) SkipFunc {
	return func(req *RequestInfo) bool {
		if routePattern, exists := req.Metadata["route"]; exists {
			if route, ok := routePattern.(string); ok {
				for _, skipRoute := range routes {
					if route == skipRoute {
						return true
					}
				}
			}
		}
		return false
	}
}

// EchoSkipByUserAgent returns a skip function that skips based on User-Agent
func EchoSkipByUserAgent(userAgents ...string) SkipFunc {
	return func(req *RequestInfo) bool {
		ua := strings.ToLower(req.UserAgent)
		for _, skipUA := range userAgents {
			if strings.Contains(ua, strings.ToLower(skipUA)) {
				return true
			}
		}
		return false
	}
}

// EchoPathBasedTierExtractor extracts tier based on API path (e.g., /v1/premium/, /v1/basic/)
func EchoPathBasedTierExtractor() TierExtractor {
	return &PathBasedTierExtractor{
		PathTiers: map[string]string{
			"/api/v1/premium":    ratelimit.TierPremium,
			"/api/v1/enterprise": ratelimit.TierEnterprise,
			"/api/v1/pro":        ratelimit.TierPremium,
			"/api/v1/free":       ratelimit.TierFree,
			"/api/v1/public":     ratelimit.TierFree,
		},
		DefaultTier: ratelimit.TierFree,
	}
}

// PathBasedTierExtractor extracts tier information from request path
type PathBasedTierExtractor struct {
	PathTiers   map[string]string
	DefaultTier string
}

func (t *PathBasedTierExtractor) Extract(req *RequestInfo) (tier string, err error) {
	// Check if path matches any tier pattern
	for pathPrefix, tierName := range t.PathTiers {
		if strings.HasPrefix(req.Path, pathPrefix) {
			return tierName, nil
		}
	}

	// Fall back to default tier
	if t.DefaultTier != "" {
		return t.DefaultTier, nil
	}

	return ratelimit.TierFree, nil
}

// EchoHeaderBasedEntityExtractor creates an entity extractor that can extract from custom headers
func EchoHeaderBasedEntityExtractor(entityHeaders map[string]string) EntityExtractor {
	return &HeaderBasedEntityExtractor{
		EntityHeaders: entityHeaders,
		DefaultEntityExtractor: DefaultEntityExtractor{
			APIKeyHeaders: []string{"Authorization", "X-API-Key"},
			UserIDHeaders: []string{"X-User-ID"},
			UseIPFallback: true,
		},
	}
}

// HeaderBasedEntityExtractor extracts entities based on configurable headers
type HeaderBasedEntityExtractor struct {
	EntityHeaders map[string]string // header -> entity type mapping
	DefaultEntityExtractor
}

func (e *HeaderBasedEntityExtractor) Extract(req *RequestInfo) (entityID, entityType string, err error) {
	// First check custom entity headers
	for header, entType := range e.EntityHeaders {
		if values, exists := req.Headers[header]; exists && len(values) > 0 {
			if values[0] != "" {
				return entType + ":" + values[0], entType, nil
			}
		}
	}

	// Fall back to default extraction
	return e.DefaultEntityExtractor.Extract(req)
}

// init registers the Echo plugin
func init() {
	Register(&EchoPlugin{})
}
