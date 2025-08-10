// middleware/fiber.go
package middleware

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/itsatony/gorly"
)

// FiberPlugin implements MiddlewarePlugin for Fiber framework
type FiberPlugin struct{}

// Name returns the plugin name
func (p *FiberPlugin) Name() string {
	return "fiber"
}

// Version returns the supported Fiber version
func (p *FiberPlugin) Version() string {
	return ">=2.0.0"
}

// CreateMiddleware creates Fiber middleware function
func (p *FiberPlugin) CreateMiddleware(limiter ratelimit.RateLimiter, config *Config) interface{} {
	if config == nil {
		config = DefaultConfig()
	}
	config.Limiter = limiter

	return func(c *fiber.Ctx) error {
		// Extract request information
		reqInfo, err := p.ExtractRequest(c)
		if err != nil {
			if config.Logger != nil {
				config.Logger.Error("Failed to extract request info", err, map[string]interface{}{
					"path":   c.Path(),
					"method": c.Method(),
				})
			}
			return c.Status(config.ResponseConfig.ErrorStatusCode).JSON(fiber.Map{
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
			return c.Status(config.ResponseConfig.ErrorStatusCode).JSON(fiber.Map{
				"error": "Rate limiting failed",
			})
		}

		// Add rate limit headers
		headers := BuildResponseHeaders(result, &config.ResponseConfig)
		for key, value := range headers {
			c.Set(key, value)
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
			return c.Status(config.ResponseConfig.RateLimitedStatusCode).JSON(fiber.Map{
				"error":               "Rate limit exceeded",
				"limit":               result.Limit,
				"remaining":           result.Remaining,
				"retry_after_seconds": int64(result.RetryAfter.Seconds()),
				"window_seconds":      int64(result.Window.Seconds()),
			})
		}

		// Add rate limit info to context for downstream handlers
		c.Locals("ratelimit_result", result)
		c.Locals("ratelimit_entity_id", reqInfo.EntityID)
		c.Locals("ratelimit_scope", reqInfo.Scope)

		// Continue to next handler
		return c.Next()
	}
}

// ExtractRequest extracts request information from Fiber context
func (p *FiberPlugin) ExtractRequest(frameworkRequest interface{}) (*RequestInfo, error) {
	c, ok := frameworkRequest.(*fiber.Ctx)
	if !ok {
		return nil, fmt.Errorf("expected *fiber.Ctx, got %T", frameworkRequest)
	}

	// Extract headers (Fiber uses fasthttp which handles headers differently)
	headers := make(map[string][]string)
	c.Request().Header.VisitAll(func(key, value []byte) {
		keyStr := string(key)
		valueStr := string(value)
		headers[keyStr] = append(headers[keyStr], valueStr)
	})

	// Extract request information
	reqInfo := &RequestInfo{
		Method:     c.Method(),
		Path:       c.Path(),
		RemoteAddr: c.IP(), // Fiber handles X-Forwarded-For automatically
		UserAgent:  c.Get("User-Agent"),
		Headers:    headers,
		Context:    c.Context(),
		Metadata:   make(map[string]interface{}),
	}

	// Add query parameters to metadata
	if query := c.Request().URI().QueryString(); len(query) > 0 {
		reqInfo.Metadata["query"] = string(query)
	}

	// Add route pattern to metadata if available
	if route := c.Route().Path; route != "" {
		reqInfo.Metadata["route"] = route
	}

	// Add path parameters to metadata
	if params := c.AllParams(); len(params) > 0 {
		reqInfo.Metadata["params"] = params
	}

	// Extract custom request count if specified
	if reqCountStr := c.Get("X-Request-Count"); reqCountStr != "" {
		if reqCount, err := strconv.ParseInt(reqCountStr, 10, 64); err == nil && reqCount > 0 {
			reqInfo.Requests = reqCount
		}
	}

	return reqInfo, nil
}

// SendResponse sends response using Fiber context
func (p *FiberPlugin) SendResponse(frameworkResponse interface{}, status int, headers map[string]string, body []byte) error {
	c, ok := frameworkResponse.(*fiber.Ctx)
	if !ok {
		return fmt.Errorf("expected *fiber.Ctx, got %T", frameworkResponse)
	}

	// Set headers
	for key, value := range headers {
		c.Set(key, value)
	}

	// Send response
	c.Set("Content-Type", "application/json")
	return c.Status(status).Send(body)
}

// ============================================================================
// Fiber-specific helper functions
// ============================================================================

// FiberMiddleware creates a Fiber middleware with default configuration
func FiberMiddleware(limiter ratelimit.RateLimiter) fiber.Handler {
	plugin := &FiberPlugin{}
	middleware := plugin.CreateMiddleware(limiter, DefaultConfig())
	return middleware.(fiber.Handler)
}

// FiberMiddlewareWithConfig creates a Fiber middleware with custom configuration
func FiberMiddlewareWithConfig(limiter ratelimit.RateLimiter, config *Config) fiber.Handler {
	plugin := &FiberPlugin{}
	middleware := plugin.CreateMiddleware(limiter, config)
	return middleware.(fiber.Handler)
}

// FiberEntityExtractor creates an entity extractor configured for common Fiber patterns
func FiberEntityExtractor() EntityExtractor {
	return &DefaultEntityExtractor{
		APIKeyHeaders: []string{"X-API-Key", "Authorization", "X-Api-Key"},
		UserIDHeaders: []string{"X-User-ID", "X-User-Id", "X-UserId"},
		UseIPFallback: true,
	}
}

// FiberScopeExtractor creates a scope extractor configured for REST APIs
func FiberScopeExtractor() ScopeExtractor {
	return &DefaultScopeExtractor{
		PathScopes: map[string]string{
			"/api/v1/auth":     ratelimit.ScopeGlobal,
			"/api/v1/users":    "users",
			"/api/v1/search":   ratelimit.ScopeSearch,
			"/api/v1/upload":   "upload",
			"/api/v1/admin":    "admin",
			"/api/v1/public":   "public",
			"/api/v1/webhooks": "webhooks",
		},
		MethodScopes: map[string]string{
			"POST":   "write",
			"PUT":    "write",
			"PATCH":  "write",
			"DELETE": "delete",
			"GET":    "read",
			"HEAD":   "read",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}
}

// FiberConfig creates a Fiber-optimized middleware configuration
func FiberConfig(limiter ratelimit.RateLimiter) *Config {
	return &Config{
		Limiter:         limiter,
		EntityExtractor: FiberEntityExtractor(),
		ScopeExtractor:  FiberScopeExtractor(),
		TierExtractor:   &DefaultTierExtractor{},
		ResponseConfig: ResponseConfig{
			RateLimitedStatusCode: fiber.StatusTooManyRequests,
			ErrorStatusCode:       fiber.StatusInternalServerError,
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
// Advanced Fiber Features
// ============================================================================

// FiberSkipHealthChecks returns a skip function that skips health check endpoints
func FiberSkipHealthChecks() SkipFunc {
	healthPaths := []string{"/health", "/healthz", "/ping", "/status", "/metrics", "/ready", "/live"}
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

// FiberSkipWebSocket returns a skip function that skips WebSocket upgrade requests
func FiberSkipWebSocket() SkipFunc {
	return func(req *RequestInfo) bool {
		// Check for WebSocket upgrade headers
		if connectionHeaders, exists := req.Headers["Connection"]; exists {
			for _, conn := range connectionHeaders {
				if strings.ToLower(conn) == "upgrade" {
					if upgradeHeaders, exists := req.Headers["Upgrade"]; exists {
						for _, upgrade := range upgradeHeaders {
							if strings.ToLower(upgrade) == "websocket" {
								return true
							}
						}
					}
				}
			}
		}
		return false
	}
}

// FiberBurstProtection returns a request count extractor that can handle burst requests
func FiberBurstProtection() func(req *RequestInfo) int64 {
	return func(req *RequestInfo) int64 {
		// For upload endpoints, count each MB as a separate request
		if strings.Contains(req.Path, "/upload") {
			if contentLengthStr := req.Headers["Content-Length"]; len(contentLengthStr) > 0 {
				if contentLength, err := strconv.ParseInt(contentLengthStr[0], 10, 64); err == nil {
					// Count each MB as one request (minimum 1)
					mbCount := (contentLength + 1024*1024 - 1) / (1024 * 1024)
					if mbCount < 1 {
						mbCount = 1
					}
					return mbCount
				}
			}
		}

		// For batch endpoints, extract batch size
		if strings.Contains(req.Path, "/batch") {
			if batchSizeStr := req.Headers["X-Batch-Size"]; len(batchSizeStr) > 0 {
				if batchSize, err := strconv.ParseInt(batchSizeStr[0], 10, 64); err == nil && batchSize > 0 {
					return batchSize
				}
			}
		}

		return 1 // Default single request
	}
}

// FiberDynamicSkip returns a skip function that can be configured at runtime
func FiberDynamicSkip() (SkipFunc, func(string, bool)) {
	skipPaths := make(map[string]bool)

	skipFunc := func(req *RequestInfo) bool {
		return skipPaths[req.Path]
	}

	configurator := func(path string, skip bool) {
		if skip {
			skipPaths[path] = true
		} else {
			delete(skipPaths, path)
		}
	}

	return skipFunc, configurator
}

// FiberCustomEntityExtractor creates an entity extractor with custom logic
func FiberCustomEntityExtractor(customExtract func(*RequestInfo) (string, string, error)) EntityExtractor {
	return &CustomEntityExtractor{
		CustomExtract: customExtract,
		Fallback:      FiberEntityExtractor(),
	}
}

// CustomEntityExtractor allows custom entity extraction logic
type CustomEntityExtractor struct {
	CustomExtract func(*RequestInfo) (string, string, error)
	Fallback      EntityExtractor
}

func (e *CustomEntityExtractor) Extract(req *RequestInfo) (entityID, entityType string, err error) {
	// Try custom extraction first
	if e.CustomExtract != nil {
		id, typ, err := e.CustomExtract(req)
		if err == nil && id != "" {
			return id, typ, nil
		}
	}

	// Fall back to default extraction
	if e.Fallback != nil {
		return e.Fallback.Extract(req)
	}

	return "", "", fmt.Errorf("no entity extraction method available")
}

// FiberPerformanceLogger creates a logger that tracks performance metrics
func FiberPerformanceLogger() Logger {
	return &PerformanceLogger{}
}

// PerformanceLogger logs performance metrics
type PerformanceLogger struct{}

func (l *PerformanceLogger) Info(msg string, fields map[string]interface{}) {
	// Could integrate with your logging system
	fmt.Printf("[INFO] %s: %+v\n", msg, fields)
}

func (l *PerformanceLogger) Warn(msg string, fields map[string]interface{}) {
	fmt.Printf("[WARN] %s: %+v\n", msg, fields)
}

func (l *PerformanceLogger) Error(msg string, err error, fields map[string]interface{}) {
	fmt.Printf("[ERROR] %s: %v, fields: %+v\n", msg, err, fields)
}

func (l *PerformanceLogger) Debug(msg string, fields map[string]interface{}) {
	// Debug logs can be disabled in production
	fmt.Printf("[DEBUG] %s: %+v\n", msg, fields)
}

// init registers the Fiber plugin
func init() {
	Register(&FiberPlugin{})
}
