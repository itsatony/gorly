// middleware/mux.go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/itsatony/gorly"
)

// MuxMiddleware provides rate limiting middleware specifically for Gorilla Mux
type MuxMiddleware struct {
	httpMiddleware *HTTPMiddleware
	config         *MuxMiddlewareConfig
}

// MuxMiddlewareConfig extends HTTPMiddlewareConfig with Mux-specific features
type MuxMiddlewareConfig struct {
	*HTTPMiddlewareConfig

	// RouteBasedScopes maps route names to scopes
	RouteBasedScopes map[string]string

	// VariableBasedScopes extracts scopes from route variables
	VariableBasedScopes map[string]string

	// PerRouteOverrides allows different rate limits per route
	PerRouteOverrides map[string]*RouteRateLimit
}

// RouteRateLimit defines rate limiting overrides for specific routes
type RouteRateLimit struct {
	Requests  int64  `json:"requests"`
	Window    string `json:"window"` // e.g., "1h", "15m", "60s"
	BurstSize int64  `json:"burst_size,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

// NewMuxMiddleware creates a new Gorilla Mux middleware
func NewMuxMiddleware(config *MuxMiddlewareConfig) (*MuxMiddleware, error) {
	if config == nil {
		return nil, ratelimit.NewRateLimitError(
			ratelimit.ErrorTypeConfig,
			"mux middleware config is required",
			nil,
		)
	}

	if config.HTTPMiddlewareConfig == nil {
		return nil, ratelimit.NewRateLimitError(
			ratelimit.ErrorTypeConfig,
			"HTTP middleware config is required",
			nil,
		)
	}

	// Override the scope extractor to use Mux-aware extraction
	originalScopeExtractor := config.ScopeExtractor
	config.ScopeExtractor = func(r *http.Request) string {
		return extractMuxScope(r, config, originalScopeExtractor)
	}

	httpMiddleware, err := NewHTTPMiddleware(config.HTTPMiddlewareConfig)
	if err != nil {
		return nil, err
	}

	return &MuxMiddleware{
		httpMiddleware: httpMiddleware,
		config:         config,
	}, nil
}

// Middleware returns the Mux middleware function
func (m *MuxMiddleware) Middleware(next http.Handler) http.Handler {
	return m.httpMiddleware.Middleware(next)
}

// MiddlewareFunc returns the middleware as a function for use with mux.Router.Use()
func (m *MuxMiddleware) MiddlewareFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.httpMiddleware.MiddlewareFunc(next)
}

// RouteMiddleware creates middleware for specific routes with custom rate limits
func (m *MuxMiddleware) RouteMiddleware(routeName string, customLimit *RouteRateLimit) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add route-specific information to context for scope extraction
			if routeName != "" {
				r = r.WithContext(setMuxRoute(r.Context(), routeName))
			}

			if customLimit != nil {
				r = r.WithContext(setMuxCustomLimit(r.Context(), customLimit))
			}

			m.httpMiddleware.Middleware(next).ServeHTTP(w, r)
		})
	}
}

// extractMuxScope extracts scope information using Mux-specific context
func extractMuxScope(r *http.Request, config *MuxMiddlewareConfig, fallback HTTPScopeExtractor) string {
	// Check for custom route-specific limits first
	if customLimit := getMuxCustomLimit(r.Context()); customLimit != nil && customLimit.Scope != "" {
		return customLimit.Scope
	}

	// Check route name-based scopes
	if routeName := getMuxRoute(r.Context()); routeName != "" {
		if scope, exists := config.RouteBasedScopes[routeName]; exists {
			return scope
		}
	}

	// Check for Mux route from current route
	if route := mux.CurrentRoute(r); route != nil {
		if routeName := route.GetName(); routeName != "" {
			if scope, exists := config.RouteBasedScopes[routeName]; exists {
				return scope
			}
		}

		// Extract from route variables
		if vars := mux.Vars(r); len(vars) > 0 {
			for variable, scope := range config.VariableBasedScopes {
				if _, exists := vars[variable]; exists {
					return scope
				}
			}
		}
	}

	// Use HTTP method as scope
	method := strings.ToLower(r.Method)
	methodScopes := map[string]string{
		"get":    "read",
		"head":   "read",
		"post":   "write",
		"put":    "write",
		"patch":  "write",
		"delete": "delete",
	}

	if scope, exists := methodScopes[method]; exists {
		return scope
	}

	// Fall back to original scope extractor
	if fallback != nil {
		return fallback(r)
	}

	return ratelimit.ScopeGlobal
}

// MuxEntityExtractor creates an entity extractor that can use Mux route variables
func MuxEntityExtractor(variableName string, entityType string, getTierFunc func(string) string) HTTPEntityExtractor {
	return func(r *http.Request) (ratelimit.AuthEntity, error) {
		vars := mux.Vars(r)
		if vars == nil {
			return DefaultIPEntityExtractor(r)
		}

		entityID, exists := vars[variableName]
		if !exists || entityID == "" {
			return DefaultIPEntityExtractor(r)
		}

		tier := ratelimit.TierFree
		if getTierFunc != nil {
			tier = getTierFunc(entityID)
		}

		return ratelimit.NewDefaultAuthEntity(entityID, entityType, tier), nil
	}
}

// MuxUserEntityExtractor creates an entity extractor for user-based routing
func MuxUserEntityExtractor(getTierFunc func(string) string) HTTPEntityExtractor {
	return MuxEntityExtractor("userId", ratelimit.EntityTypeUser, getTierFunc)
}

// MuxAPIKeyEntityExtractor creates an entity extractor for API key-based routing
func MuxAPIKeyEntityExtractor(getTierFunc func(string) string) HTTPEntityExtractor {
	return MuxEntityExtractor("apiKey", ratelimit.EntityTypeAPIKey, getTierFunc)
}

// MuxTenantEntityExtractor creates an entity extractor for tenant-based routing
func MuxTenantEntityExtractor(getTierFunc func(string) string) HTTPEntityExtractor {
	return MuxEntityExtractor("tenantId", ratelimit.EntityTypeTenant, getTierFunc)
}

// ResourceBasedScopeExtractor creates a scope extractor based on resource types in the URL
func ResourceBasedScopeExtractor() HTTPScopeExtractor {
	return func(r *http.Request) string {
		path := strings.ToLower(r.URL.Path)

		// Common resource mappings
		resourceMappings := map[string]string{
			"memories":      ratelimit.ScopeMemory,
			"search":        ratelimit.ScopeSearch,
			"meta-memories": ratelimit.ScopeMetadata,
			"analytics":     ratelimit.ScopeAnalytics,
			"admin":         ratelimit.ScopeAdmin,
			"auth":          "auth",
			"users":         "users",
			"stats":         "stats",
		}

		for resource, scope := range resourceMappings {
			if strings.Contains(path, resource) {
				return scope
			}
		}

		return ratelimit.ScopeGlobal
	}
}

// CRUDScopeExtractor combines HTTP method with resource detection for CRUD operations
func CRUDScopeExtractor() HTTPScopeExtractor {
	resourceExtractor := ResourceBasedScopeExtractor()

	return func(r *http.Request) string {
		resource := resourceExtractor(r)
		method := strings.ToLower(r.Method)

		// Combine resource and method
		switch method {
		case "get", "head":
			return resource + ":read"
		case "post":
			return resource + ":create"
		case "put", "patch":
			return resource + ":update"
		case "delete":
			return resource + ":delete"
		default:
			return resource
		}
	}
}

// DefaultMuxMiddlewareConfig returns a default configuration for Mux middleware
func DefaultMuxMiddlewareConfig(limiter ratelimit.RateLimiter) *MuxMiddlewareConfig {
	httpConfig := DefaultHTTPMiddlewareConfig(limiter)

	return &MuxMiddlewareConfig{
		HTTPMiddlewareConfig: httpConfig,
		RouteBasedScopes: map[string]string{
			"memories":      ratelimit.ScopeMemory,
			"search":        ratelimit.ScopeSearch,
			"meta-memories": ratelimit.ScopeMetadata,
			"analytics":     ratelimit.ScopeAnalytics,
			"admin":         ratelimit.ScopeAdmin,
		},
		VariableBasedScopes: map[string]string{
			"userId":   "user",
			"tenantId": "tenant",
			"apiKey":   "api_key",
		},
		PerRouteOverrides: make(map[string]*RouteRateLimit),
	}
}

// Context helpers for storing Mux-specific information

type contextKey string

const (
	muxRouteKey       contextKey = "mux_route"
	muxCustomLimitKey contextKey = "mux_custom_limit"
)

// setMuxRoute stores the route name in context
func setMuxRoute(ctx context.Context, routeName string) context.Context {
	return context.WithValue(ctx, muxRouteKey, routeName)
}

// getMuxRoute retrieves the route name from context
func getMuxRoute(ctx context.Context) string {
	if route, ok := ctx.Value(muxRouteKey).(string); ok {
		return route
	}
	return ""
}

// setMuxCustomLimit stores custom limit in context
func setMuxCustomLimit(ctx context.Context, limit *RouteRateLimit) context.Context {
	return context.WithValue(ctx, muxCustomLimitKey, limit)
}

// getMuxCustomLimit retrieves custom limit from context
func getMuxCustomLimit(ctx context.Context) *RouteRateLimit {
	if limit, ok := ctx.Value(muxCustomLimitKey).(*RouteRateLimit); ok {
		return limit
	}
	return nil
}

// Helper method to set up common M3MO-specific middleware
func NewM3MOMuxMiddleware(limiter ratelimit.RateLimiter, authExtractor HTTPEntityExtractor) (*MuxMiddleware, error) {
	config := &MuxMiddlewareConfig{
		HTTPMiddlewareConfig: &HTTPMiddlewareConfig{
			Limiter:         limiter,
			EntityExtractor: authExtractor,
			ScopeExtractor:  CRUDScopeExtractor(),
			ErrorHandler:    DefaultHTTPErrorHandler,
			AddHeaders:      true,
			SkipPaths: []string{
				"/health",
				"/metrics",
				"/ready",
				"/api/v1/xml/", // Skip XML endpoints as they handle their own auth
			},
		},
		RouteBasedScopes: map[string]string{
			"create_memory":    ratelimit.ScopeMemory + ":create",
			"get_memory":       ratelimit.ScopeMemory + ":read",
			"update_memory":    ratelimit.ScopeMemory + ":update",
			"delete_memory":    ratelimit.ScopeMemory + ":delete",
			"search_memories":  ratelimit.ScopeSearch,
			"explore_memories": ratelimit.ScopeSearch,
			"generate_meta":    ratelimit.ScopeMetadata,
			"get_stats":        ratelimit.ScopeAnalytics,
		},
		VariableBasedScopes: map[string]string{
			"userId": ratelimit.EntityTypeUser,
		},
	}

	return NewMuxMiddleware(config)
}
