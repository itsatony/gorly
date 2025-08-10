// middleware/plugin.go
package middleware

import (
	"context"
	"fmt"

	"github.com/itsatony/gorly"
)

// MiddlewarePlugin defines the interface for framework-specific middleware implementations
type MiddlewarePlugin interface {
	// Name returns the name of the framework this plugin supports
	Name() string

	// Version returns the supported framework version range
	Version() string

	// CreateMiddleware creates middleware function for the specific framework
	CreateMiddleware(limiter ratelimit.RateLimiter, config *Config) interface{}

	// ExtractRequest extracts the necessary information from framework-specific request
	ExtractRequest(frameworkRequest interface{}) (*RequestInfo, error)

	// SendResponse sends rate limit response using framework-specific response writer
	SendResponse(frameworkResponse interface{}, status int, headers map[string]string, body []byte) error
}

// RequestInfo contains extracted information from any HTTP framework request
type RequestInfo struct {
	// HTTP request information
	Method     string
	Path       string
	RemoteAddr string
	UserAgent  string
	Headers    map[string][]string

	// Rate limiting specific information
	EntityID   string                 // Extracted entity ID (user ID, API key, IP, etc.)
	EntityType string                 // Type of entity (user, apikey, ip)
	Tier       string                 // User tier if available
	Scope      string                 // Rate limit scope to apply
	Requests   int64                  // Number of requests (default 1)
	Metadata   map[string]interface{} // Additional metadata

	// Context for the request
	Context context.Context
}

// Config contains configuration for middleware behavior
type Config struct {
	// Core rate limiter configuration
	Limiter ratelimit.RateLimiter

	// Entity extraction configuration
	EntityExtractor EntityExtractor
	ScopeExtractor  ScopeExtractor
	TierExtractor   TierExtractor

	// Response configuration
	ResponseConfig ResponseConfig

	// Skip conditions
	SkipFunc SkipFunc

	// Error handling
	ErrorHandler ErrorHandler

	// Logging
	Logger Logger

	// Metrics
	MetricsEnabled bool
	MetricsPrefix  string
}

// EntityExtractor extracts entity information from request
type EntityExtractor interface {
	Extract(req *RequestInfo) (entityID, entityType string, err error)
}

// ScopeExtractor extracts scope information from request
type ScopeExtractor interface {
	Extract(req *RequestInfo) (scope string, err error)
}

// TierExtractor extracts tier information from request
type TierExtractor interface {
	Extract(req *RequestInfo) (tier string, err error)
}

// SkipFunc determines if rate limiting should be skipped for this request
type SkipFunc func(req *RequestInfo) bool

// ErrorHandler handles rate limiting errors
type ErrorHandler func(req *RequestInfo, err error) bool // return true to continue, false to stop

// Logger logs rate limiting events
type Logger interface {
	Info(msg string, fields map[string]interface{})
	Warn(msg string, fields map[string]interface{})
	Error(msg string, err error, fields map[string]interface{})
	Debug(msg string, fields map[string]interface{})
}

// ResponseConfig configures rate limit responses
type ResponseConfig struct {
	// Status codes
	RateLimitedStatusCode int // Default: 429
	ErrorStatusCode       int // Default: 500

	// Response headers
	IncludeHeaders bool   // Include rate limit headers
	HeaderPrefix   string // Header prefix (default: "X-RateLimit-")
	CustomHeaders  map[string]string

	// Response body
	RateLimitedResponse []byte // Custom rate limited response
	ErrorResponse       []byte // Custom error response

	// Content type
	ContentType string // Default: "application/json"
}

// DefaultConfig returns default middleware configuration
func DefaultConfig() *Config {
	return &Config{
		EntityExtractor: &DefaultEntityExtractor{},
		ScopeExtractor:  &DefaultScopeExtractor{},
		TierExtractor:   &DefaultTierExtractor{},
		ResponseConfig: ResponseConfig{
			RateLimitedStatusCode: 429,
			ErrorStatusCode:       500,
			IncludeHeaders:        true,
			HeaderPrefix:          "X-RateLimit-",
			ContentType:           "application/json",
			RateLimitedResponse:   []byte(`{"error":"Rate limit exceeded","retry_after_seconds":60}`),
			ErrorResponse:         []byte(`{"error":"Internal server error"}`),
		},
		Logger:         &NoOpLogger{},
		MetricsEnabled: false,
		MetricsPrefix:  "gorly_middleware_",
	}
}

// ============================================================================
// Default Extractors
// ============================================================================

// DefaultEntityExtractor extracts entity information from common sources
type DefaultEntityExtractor struct {
	// Priority order for entity extraction
	APIKeyHeaders []string // Headers to check for API keys
	UserIDHeaders []string // Headers to check for user IDs
	UseIPFallback bool     // Fall back to IP address if no other entity found
}

func (e *DefaultEntityExtractor) Extract(req *RequestInfo) (entityID, entityType string, err error) {
	// Initialize with defaults if not set
	if e.APIKeyHeaders == nil {
		e.APIKeyHeaders = []string{"X-API-Key", "Authorization", "X-Api-Key"}
	}
	if e.UserIDHeaders == nil {
		e.UserIDHeaders = []string{"X-User-ID", "X-User-Id", "X-UserId"}
	}

	// Try to extract API key
	for _, header := range e.APIKeyHeaders {
		if values, exists := req.Headers[header]; exists && len(values) > 0 {
			apiKey := values[0]
			if len(apiKey) > 0 {
				// Handle Authorization header specially
				if header == "Authorization" {
					if len(apiKey) > 7 && apiKey[:7] == "Bearer " {
						apiKey = apiKey[7:]
					}
				}
				return "apikey:" + apiKey, ratelimit.EntityTypeAPIKey, nil
			}
		}
	}

	// Try to extract user ID
	for _, header := range e.UserIDHeaders {
		if values, exists := req.Headers[header]; exists && len(values) > 0 {
			userID := values[0]
			if len(userID) > 0 {
				return "user:" + userID, ratelimit.EntityTypeUser, nil
			}
		}
	}

	// Fall back to IP address if enabled
	if e.UseIPFallback {
		ip := extractIPAddress(req)
		return "ip:" + ip, ratelimit.EntityTypeIP, nil
	}

	return "", "", nil
}

// DefaultScopeExtractor extracts scope based on request path and method
type DefaultScopeExtractor struct {
	// Path-based scope mapping
	PathScopes map[string]string

	// Method-based scope mapping
	MethodScopes map[string]string

	// Default scope if no specific mapping found
	DefaultScope string
}

func (s *DefaultScopeExtractor) Extract(req *RequestInfo) (scope string, err error) {
	// Initialize defaults if not set
	if s.DefaultScope == "" {
		s.DefaultScope = ratelimit.ScopeGlobal
	}

	// Check path-based scopes first
	if s.PathScopes != nil {
		for pathPrefix, scopeName := range s.PathScopes {
			if len(req.Path) >= len(pathPrefix) && req.Path[:len(pathPrefix)] == pathPrefix {
				return scopeName, nil
			}
		}
	}

	// Check method-based scopes
	if s.MethodScopes != nil {
		if scopeName, exists := s.MethodScopes[req.Method]; exists {
			return scopeName, nil
		}
	}

	return s.DefaultScope, nil
}

// DefaultTierExtractor extracts tier information from headers or returns default
type DefaultTierExtractor struct {
	// Headers to check for tier information
	TierHeaders []string

	// Default tier if no tier found
	DefaultTier string
}

func (t *DefaultTierExtractor) Extract(req *RequestInfo) (tier string, err error) {
	// Initialize defaults if not set
	if t.TierHeaders == nil {
		t.TierHeaders = []string{"X-User-Tier", "X-Tier"}
	}
	if t.DefaultTier == "" {
		t.DefaultTier = ratelimit.TierFree
	}

	// Try to extract tier from headers
	for _, header := range t.TierHeaders {
		if values, exists := req.Headers[header]; exists && len(values) > 0 {
			tier := values[0]
			if len(tier) > 0 {
				return tier, nil
			}
		}
	}

	return t.DefaultTier, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// extractIPAddress extracts IP address from request, handling proxies
func extractIPAddress(req *RequestInfo) string {
	// Check X-Forwarded-For header
	if values, exists := req.Headers["X-Forwarded-For"]; exists && len(values) > 0 {
		// Take the first IP in the chain (original client)
		forwardedFor := values[0]
		if forwardedFor != "" {
			// Split by comma and take first IP
			if idx := len(forwardedFor); idx > 0 {
				for i, char := range forwardedFor {
					if char == ',' || char == ' ' {
						idx = i
						break
					}
				}
				return forwardedFor[:idx]
			}
		}
	}

	// Check X-Real-IP header
	if values, exists := req.Headers["X-Real-IP"]; exists && len(values) > 0 {
		if values[0] != "" {
			return values[0]
		}
	}

	// Fall back to RemoteAddr
	return req.RemoteAddr
}

// ============================================================================
// Middleware Core Logic
// ============================================================================

// ProcessRequest processes a rate limiting request using the core logic
func ProcessRequest(req *RequestInfo, config *Config) (*ratelimit.Result, error) {
	// Skip if skip function returns true
	if config.SkipFunc != nil && config.SkipFunc(req) {
		return &ratelimit.Result{Allowed: true}, nil
	}

	// Extract entity information
	entityID, entityType, err := config.EntityExtractor.Extract(req)
	if err != nil {
		if config.ErrorHandler != nil && config.ErrorHandler(req, err) {
			return &ratelimit.Result{Allowed: true}, nil // Continue on error handler's request
		}
		return nil, err
	}

	// Extract scope
	scope, err := config.ScopeExtractor.Extract(req)
	if err != nil {
		if config.ErrorHandler != nil && config.ErrorHandler(req, err) {
			return &ratelimit.Result{Allowed: true}, nil
		}
		return nil, err
	}

	// Extract tier
	tier, err := config.TierExtractor.Extract(req)
	if err != nil {
		if config.ErrorHandler != nil && config.ErrorHandler(req, err) {
			return &ratelimit.Result{Allowed: true}, nil
		}
		return nil, err
	}

	// Update request info
	req.EntityID = entityID
	req.EntityType = entityType
	req.Tier = tier
	req.Scope = scope
	if req.Requests <= 0 {
		req.Requests = 1
	}

	// Create entity
	entity := ratelimit.NewDefaultAuthEntity(entityID, entityType, tier)

	// Log request processing
	if config.Logger != nil {
		config.Logger.Debug("Processing rate limit request", map[string]interface{}{
			"entity_id":   entityID,
			"entity_type": entityType,
			"tier":        tier,
			"scope":       scope,
			"requests":    req.Requests,
			"method":      req.Method,
			"path":        req.Path,
			"user_agent":  req.UserAgent,
		})
	}

	// Check rate limit
	var result *ratelimit.Result
	if req.Requests == 1 {
		result, err = config.Limiter.Allow(req.Context, entity, scope)
	} else {
		result, err = config.Limiter.AllowN(req.Context, entity, scope, req.Requests)
	}

	if err != nil {
		if config.Logger != nil {
			config.Logger.Error("Rate limit check failed", err, map[string]interface{}{
				"entity_id": entityID,
				"scope":     scope,
				"requests":  req.Requests,
			})
		}

		if config.ErrorHandler != nil && config.ErrorHandler(req, err) {
			return &ratelimit.Result{Allowed: true}, nil
		}
		return nil, err
	}

	// Log result
	if config.Logger != nil {
		level := "Debug"
		if !result.Allowed {
			level = "Info"
		}

		logFields := map[string]interface{}{
			"entity_id":   entityID,
			"scope":       scope,
			"allowed":     result.Allowed,
			"remaining":   result.Remaining,
			"limit":       result.Limit,
			"used":        result.Used,
			"retry_after": result.RetryAfter.Seconds(),
			"algorithm":   result.Algorithm,
		}

		if level == "Info" {
			config.Logger.Info("Rate limit exceeded", logFields)
		} else {
			config.Logger.Debug("Rate limit check completed", logFields)
		}
	}

	return result, nil
}

// BuildResponseHeaders builds rate limit response headers
func BuildResponseHeaders(result *ratelimit.Result, config *ResponseConfig) map[string]string {
	if !config.IncludeHeaders {
		return config.CustomHeaders
	}

	headers := make(map[string]string)

	// Copy custom headers first
	for k, v := range config.CustomHeaders {
		headers[k] = v
	}

	prefix := config.HeaderPrefix
	if prefix == "" {
		prefix = "X-RateLimit-"
	}

	// Standard rate limit headers
	headers[prefix+"Limit"] = fmt.Sprintf("%d", result.Limit)
	headers[prefix+"Remaining"] = fmt.Sprintf("%d", result.Remaining)
	headers[prefix+"Used"] = fmt.Sprintf("%d", result.Used)

	if result.Window > 0 {
		headers[prefix+"Window"] = fmt.Sprintf("%d", int64(result.Window.Seconds()))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		headers[prefix+"Retry-After"] = fmt.Sprintf("%d", int64(result.RetryAfter.Seconds()))
		headers["Retry-After"] = fmt.Sprintf("%d", int64(result.RetryAfter.Seconds())) // Standard header
	}

	if !result.ResetTime.IsZero() {
		headers[prefix+"Reset"] = fmt.Sprintf("%d", result.ResetTime.Unix())
	}

	headers[prefix+"Algorithm"] = result.Algorithm

	return headers
}

// ============================================================================
// No-op implementations
// ============================================================================

// NoOpLogger is a logger that does nothing
type NoOpLogger struct{}

func (l *NoOpLogger) Info(msg string, fields map[string]interface{})             {}
func (l *NoOpLogger) Warn(msg string, fields map[string]interface{})             {}
func (l *NoOpLogger) Error(msg string, err error, fields map[string]interface{}) {}
func (l *NoOpLogger) Debug(msg string, fields map[string]interface{})            {}

// ============================================================================
// Plugin Registry
// ============================================================================

// PluginRegistry manages registered middleware plugins
type PluginRegistry struct {
	plugins map[string]MiddlewarePlugin
}

// GlobalRegistry is the global plugin registry
var GlobalRegistry = NewPluginRegistry()

// NewPluginRegistry creates a new plugin registry
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]MiddlewarePlugin),
	}
}

// Register registers a middleware plugin
func (r *PluginRegistry) Register(plugin MiddlewarePlugin) {
	r.plugins[plugin.Name()] = plugin
}

// Get retrieves a middleware plugin by name
func (r *PluginRegistry) Get(name string) (MiddlewarePlugin, bool) {
	plugin, exists := r.plugins[name]
	return plugin, exists
}

// List returns all registered plugin names
func (r *PluginRegistry) List() []string {
	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}
	return names
}

// Register registers a plugin with the global registry
func Register(plugin MiddlewarePlugin) {
	GlobalRegistry.Register(plugin)
}

// Get retrieves a plugin from the global registry
func Get(name string) (MiddlewarePlugin, bool) {
	return GlobalRegistry.Get(name)
}

// List lists all plugins in the global registry
func List() []string {
	return GlobalRegistry.List()
}
