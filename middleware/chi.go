// middleware/chi.go
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/itsatony/gorly"
)

// ChiPlugin implements MiddlewarePlugin for Chi router
type ChiPlugin struct{}

// Name returns the plugin name
func (p *ChiPlugin) Name() string {
	return "chi"
}

// Version returns the supported Chi version
func (p *ChiPlugin) Version() string {
	return ">=5.0.0"
}

// CreateMiddleware creates Chi middleware function
func (p *ChiPlugin) CreateMiddleware(limiter ratelimit.RateLimiter, config *Config) interface{} {
	if config == nil {
		config = DefaultConfig()
	}
	config.Limiter = limiter

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a context-aware request wrapper for Chi
			chiRequest := &ChiRequest{
				Request: r,
				Writer:  w,
			}

			// Extract request information
			reqInfo, err := p.ExtractRequest(chiRequest)
			if err != nil {
				if config.Logger != nil {
					config.Logger.Error("Failed to extract request info", err, map[string]interface{}{
						"path":   r.URL.Path,
						"method": r.Method,
					})
				}
				p.sendErrorResponse(w, config.ResponseConfig.ErrorStatusCode, "Failed to process request")
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
				p.sendErrorResponse(w, config.ResponseConfig.ErrorStatusCode, "Rate limiting failed")
				return
			}

			// Add rate limit headers
			headers := BuildResponseHeaders(result, &config.ResponseConfig)
			for key, value := range headers {
				w.Header().Set(key, value)
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
				p.sendRateLimitedResponse(w, result, &config.ResponseConfig)
				return
			}

			// Add rate limit info to context for downstream handlers
			ctx := context.WithValue(r.Context(), "ratelimit_result", result)
			ctx = context.WithValue(ctx, "ratelimit_entity_id", reqInfo.EntityID)
			ctx = context.WithValue(ctx, "ratelimit_scope", reqInfo.Scope)

			// Continue to next handler with enhanced context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ChiRequest wraps http.Request and http.ResponseWriter for Chi-specific handling
type ChiRequest struct {
	*http.Request
	Writer http.ResponseWriter
}

// ExtractRequest extracts request information from Chi request
func (p *ChiPlugin) ExtractRequest(frameworkRequest interface{}) (*RequestInfo, error) {
	chiReq, ok := frameworkRequest.(*ChiRequest)
	if !ok {
		return nil, fmt.Errorf("expected *ChiRequest, got %T", frameworkRequest)
	}

	r := chiReq.Request

	// Extract headers
	headers := make(map[string][]string)
	for key, values := range r.Header {
		headers[key] = values
	}

	// Extract request information
	reqInfo := &RequestInfo{
		Method:     r.Method,
		Path:       r.URL.Path,
		RemoteAddr: p.getRealIP(r),
		UserAgent:  r.UserAgent(),
		Headers:    headers,
		Context:    r.Context(),
		Metadata:   make(map[string]interface{}),
	}

	// Add query parameters to metadata
	if len(r.URL.RawQuery) > 0 {
		reqInfo.Metadata["query"] = r.URL.RawQuery
	}

	// Add Chi URL parameters to metadata
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if rctx.RoutePattern() != "" {
			reqInfo.Metadata["route"] = rctx.RoutePattern()
		}

		// Add URL parameters
		if len(rctx.URLParams.Keys) > 0 {
			params := make(map[string]string)
			for i, key := range rctx.URLParams.Keys {
				if i < len(rctx.URLParams.Values) {
					params[key] = rctx.URLParams.Values[i]
				}
			}
			reqInfo.Metadata["params"] = params
		}
	}

	// Extract custom request count if specified
	if reqCountStr := r.Header.Get("X-Request-Count"); reqCountStr != "" {
		if reqCount, err := strconv.ParseInt(reqCountStr, 10, 64); err == nil && reqCount > 0 {
			reqInfo.Requests = reqCount
		}
	}

	return reqInfo, nil
}

// SendResponse sends response using Chi (standard http.ResponseWriter)
func (p *ChiPlugin) SendResponse(frameworkResponse interface{}, status int, headers map[string]string, body []byte) error {
	chiReq, ok := frameworkResponse.(*ChiRequest)
	if !ok {
		return fmt.Errorf("expected *ChiRequest, got %T", frameworkResponse)
	}

	w := chiReq.Writer

	// Set headers
	for key, value := range headers {
		w.Header().Set(key, value)
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(body)
	return err
}

// ============================================================================
// Chi-specific helper functions
// ============================================================================

// ChiMiddleware creates a Chi middleware with default configuration
func ChiMiddleware(limiter ratelimit.RateLimiter) func(http.Handler) http.Handler {
	plugin := &ChiPlugin{}
	middleware := plugin.CreateMiddleware(limiter, DefaultConfig())
	return middleware.(func(http.Handler) http.Handler)
}

// ChiMiddlewareWithConfig creates a Chi middleware with custom configuration
func ChiMiddlewareWithConfig(limiter ratelimit.RateLimiter, config *Config) func(http.Handler) http.Handler {
	plugin := &ChiPlugin{}
	middleware := plugin.CreateMiddleware(limiter, config)
	return middleware.(func(http.Handler) http.Handler)
}

// ChiEntityExtractor creates an entity extractor configured for Chi patterns
func ChiEntityExtractor() EntityExtractor {
	return &DefaultEntityExtractor{
		APIKeyHeaders: []string{"X-API-Key", "Authorization", "X-Api-Key", "X-API-TOKEN"},
		UserIDHeaders: []string{"X-User-ID", "X-User-Id", "X-UserId", "X-Subject"},
		UseIPFallback: true,
	}
}

// ChiScopeExtractor creates a scope extractor configured for REST APIs with Chi patterns
func ChiScopeExtractor() ScopeExtractor {
	return &DefaultScopeExtractor{
		PathScopes: map[string]string{
			"/api/v1/auth":     ratelimit.ScopeGlobal,
			"/api/v1/users":    "users",
			"/api/v1/search":   ratelimit.ScopeSearch,
			"/api/v1/upload":   "upload",
			"/api/v1/admin":    "admin",
			"/api/v1/public":   "public",
			"/api/v1/webhooks": "webhooks",
			"/api/v1/graphql":  "graphql",
		},
		MethodScopes: map[string]string{
			"POST":    "write",
			"PUT":     "write",
			"PATCH":   "write",
			"DELETE":  "delete",
			"GET":     "read",
			"HEAD":    "read",
			"OPTIONS": "options",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}
}

// ChiConfig creates a Chi-optimized middleware configuration
func ChiConfig(limiter ratelimit.RateLimiter) *Config {
	return &Config{
		Limiter:         limiter,
		EntityExtractor: ChiEntityExtractor(),
		ScopeExtractor:  ChiScopeExtractor(),
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
// Advanced Chi Features
// ============================================================================

// ChiSkipHealthChecks returns a skip function that skips health check endpoints
func ChiSkipHealthChecks() SkipFunc {
	healthPaths := []string{"/health", "/healthz", "/ping", "/status", "/metrics", "/ready", "/live", "/debug/pprof"}
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

// ChiSkipByRoutePattern returns a skip function that skips based on Chi route patterns
func ChiSkipByRoutePattern(patterns ...string) SkipFunc {
	return func(req *RequestInfo) bool {
		if routePattern, exists := req.Metadata["route"]; exists {
			if route, ok := routePattern.(string); ok {
				for _, pattern := range patterns {
					if route == pattern {
						return true
					}
				}
			}
		}
		return false
	}
}

// ChiParameterBasedExtractor creates entity extractor that uses Chi URL parameters
func ChiParameterBasedExtractor(paramMapping map[string]string) EntityExtractor {
	return &ParameterBasedEntityExtractor{
		ParamMapping:      paramMapping,
		FallbackExtractor: ChiEntityExtractor(),
	}
}

// ParameterBasedEntityExtractor extracts entities from URL parameters
type ParameterBasedEntityExtractor struct {
	ParamMapping      map[string]string // parameter name -> entity type
	FallbackExtractor EntityExtractor
}

func (e *ParameterBasedEntityExtractor) Extract(req *RequestInfo) (entityID, entityType string, err error) {
	// Check if we have parameters in metadata
	if paramsInterface, exists := req.Metadata["params"]; exists {
		if params, ok := paramsInterface.(map[string]string); ok {
			for paramName, entityType := range e.ParamMapping {
				if paramValue, exists := params[paramName]; exists && paramValue != "" {
					return entityType + ":" + paramValue, entityType, nil
				}
			}
		}
	}

	// Fall back to standard extraction
	if e.FallbackExtractor != nil {
		return e.FallbackExtractor.Extract(req)
	}

	return "", "", fmt.Errorf("no entity found")
}

// ChiSubrouterConfig creates configuration for Chi subrouters with different rate limits
func ChiSubrouterConfig(limiter ratelimit.RateLimiter, routeConfigs map[string]*Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Determine which configuration to use based on route
			var config *Config

			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				routePattern := rctx.RoutePattern()
				if routeConfig, exists := routeConfigs[routePattern]; exists {
					config = routeConfig
				}
			}

			// Use default config if no specific config found
			if config == nil {
				config = ChiConfig(limiter)
			}

			// Apply the middleware logic
			plugin := &ChiPlugin{}
			middleware := plugin.CreateMiddleware(limiter, config)
			middlewareHandler := middleware.(func(http.Handler) http.Handler)
			middlewareHandler(next).ServeHTTP(w, r)
		})
	}
}

// ChiContentTypeBasedExtractor extracts different information based on Content-Type
func ChiContentTypeBasedExtractor() ScopeExtractor {
	return &ContentTypeBasedScopeExtractor{
		ContentTypeScopes: map[string]string{
			"application/json":         "json_api",
			"application/xml":          "xml_api",
			"multipart/form-data":      "upload",
			"application/octet-stream": "binary",
			"text/plain":               "text",
		},
		DefaultScope: ratelimit.ScopeGlobal,
	}
}

// ContentTypeBasedScopeExtractor extracts scope based on request Content-Type
type ContentTypeBasedScopeExtractor struct {
	ContentTypeScopes map[string]string
	DefaultScope      string
}

func (e *ContentTypeBasedScopeExtractor) Extract(req *RequestInfo) (scope string, err error) {
	// Check Content-Type header
	if contentTypeHeaders, exists := req.Headers["Content-Type"]; exists && len(contentTypeHeaders) > 0 {
		contentType := strings.Split(contentTypeHeaders[0], ";")[0] // Remove charset etc.
		contentType = strings.TrimSpace(strings.ToLower(contentType))

		if scopeName, exists := e.ContentTypeScopes[contentType]; exists {
			return scopeName, nil
		}
	}

	return e.DefaultScope, nil
}

// ============================================================================
// Helper methods for Chi plugin
// ============================================================================

// getRealIP extracts the real IP address from the request, handling proxies
func (p *ChiPlugin) getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain (original client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// sendErrorResponse sends a JSON error response
func (p *ChiPlugin) sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, `{"error":"%s","code":"ERROR"}`, message)
}

// sendRateLimitedResponse sends a rate limited response
func (p *ChiPlugin) sendRateLimitedResponse(w http.ResponseWriter, result *ratelimit.Result, config *ResponseConfig) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(config.RateLimitedStatusCode)

	response := fmt.Sprintf(`{
		"error":"Rate limit exceeded",
		"code":"RATE_LIMIT_EXCEEDED",
		"limit":%d,
		"remaining":%d,
		"retry_after_seconds":%d,
		"window_seconds":%d
	}`, result.Limit, result.Remaining, int64(result.RetryAfter.Seconds()), int64(result.Window.Seconds()))

	w.Write([]byte(response))
}

// init registers the Chi plugin
func init() {
	Register(&ChiPlugin{})
}
