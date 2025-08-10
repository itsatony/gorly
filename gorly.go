// gorly.go - The world's most elegant Go rate limiting library
package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/itsatony/gorly/internal/core"
	"github.com/itsatony/gorly/internal/middleware"
)

// Framework constants for explicit framework targeting
const (
	Gin   = middleware.FrameworkGin
	Echo  = middleware.FrameworkEcho
	Fiber = middleware.FrameworkFiber
	Chi   = middleware.FrameworkChi
	HTTP  = middleware.FrameworkHTTP
	Auto  = middleware.FrameworkAuto
)

// Limiter represents a rate limiter that can be used as middleware
type Limiter interface {
	// Middleware returns a middleware function that automatically detects the framework
	Middleware() interface{}

	// For returns middleware for a specific framework type
	// Example: limiter.For(ratelimit.Gin) for Gin-specific middleware
	For(framework middleware.FrameworkType) interface{}

	// Check performs a rate limit check for the given entity and scope
	Check(ctx context.Context, entity string, scope ...string) (*LimitResult, error)

	// Allow is an alias for Check that returns only if the request is allowed
	Allow(ctx context.Context, entity string, scope ...string) (bool, error)

	// Stats returns usage statistics
	Stats(ctx context.Context) (*LimitStats, error)

	// Health checks if the rate limiter is healthy
	Health(ctx context.Context) error

	// Close cleans up resources
	Close() error
}

// Result contains the result of a rate limit check
type LimitResult struct {
	Allowed    bool          `json:"allowed"`
	Remaining  int64         `json:"remaining"`
	Limit      int64         `json:"limit"`
	Used       int64         `json:"used"`
	RetryAfter time.Duration `json:"retry_after"`
	Window     time.Duration `json:"window"`
	ResetTime  time.Time     `json:"reset_time"`
}

// LimitStats contains usage statistics
type LimitStats struct {
	TotalRequests int64                       `json:"total_requests"`
	TotalDenied   int64                       `json:"total_denied"`
	ByScope       map[string]*LimitScopeStats `json:"by_scope"`
	ByEntity      map[string]*EntityStats     `json:"by_entity"`
}

// LimitScopeStats contains statistics for a specific scope
type LimitScopeStats struct {
	Scope    string    `json:"scope"`
	Requests int64     `json:"requests"`
	Denied   int64     `json:"denied"`
	LastUsed time.Time `json:"last_used"`
}

// EntityStats contains statistics for a specific entity
type EntityStats struct {
	Entity   string    `json:"entity"`
	Requests int64     `json:"requests"`
	Denied   int64     `json:"denied"`
	LastUsed time.Time `json:"last_used"`
}

// =============================================================================
// Version & Library Information
// =============================================================================

// Version returns the current library version
// Example: fmt.Println("Using Gorly", ratelimit.Version())
var VersionString = GetVersion

// Info returns comprehensive version information
// Example: fmt.Printf("%s\n", ratelimit.Info().String())
var Info = GetVersionInfo

// =============================================================================
// One-liner convenience functions - the magic starts here! ✨
// =============================================================================

// IPLimit creates a rate limiter that limits by IP address
// Example: app.Use(gorly.IPLimit("100/hour"))
func IPLimit(limit string) Limiter {
	limiter, err := New().
		ExtractorFunc(extractIP).
		Limit("global", limit).Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to create IP limiter: %v", err))
	}
	return limiter
}

// APIKeyLimit creates a rate limiter that limits by API key
// Looks for API key in Authorization header (Bearer token) or X-API-Key header
// Example: app.Use(gorly.APIKeyLimit("1000/hour"))
func APIKeyLimit(limit string) Limiter {
	limiter, err := New().
		ExtractorFunc(extractAPIKey).
		Limit("global", limit).Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to create API key limiter: %v", err))
	}
	return limiter
}

// UserLimit creates a rate limiter that limits by user ID
// Looks for user ID in X-User-ID header or extracts from JWT
// Example: app.Use(gorly.UserLimit("500/hour"))
func UserLimit(limit string) Limiter {
	limiter, err := New().
		ExtractorFunc(extractUserID).
		Limit("global", limit).Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to create user limiter: %v", err))
	}
	return limiter
}

// PathLimit creates a rate limiter with per-path limits
// Example: gorly.PathLimit(map[string]string{"/upload": "5/minute", "/search": "50/minute"})
func PathLimit(limits map[string]string) Limiter {
	builder := New().ExtractorFunc(extractIP)
	for path, limit := range limits {
		builder = builder.Limit(path, limit)
	}
	limiter, err := builder.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to create path limiter: %v", err))
	}
	return limiter
}

// TierLimit creates a rate limiter with tier-based limits
// Example: gorly.TierLimit(map[string]string{"free": "100/hour", "premium": "10000/hour"})
func TierLimit(limits map[string]string) Limiter {
	limiter, err := New().
		ExtractorFunc(extractTier).
		TierLimits(limits).Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to create tier limiter: %v", err))
	}
	return limiter
}

// =============================================================================
// Builder pattern for advanced configuration
// =============================================================================

// Builder provides a fluent interface for configuring rate limiters
type Builder struct {
	config *core.Config
}

// New creates a new rate limiter builder with sensible defaults
func New() *Builder {
	return &Builder{
		config: &core.Config{
			Store:         "memory", // Default to memory for simplicity
			Algorithm:     "sliding_window",
			Limits:        make(map[string]string),
			TierLimits:    make(map[string]map[string]string),
			ExtractorFunc: extractIP, // Default to IP-based limiting
		},
	}
}

// Redis configures the limiter to use Redis as the backend store
// Example: gorly.New().Redis("localhost:6379")
func (b *Builder) Redis(address string, options ...RedisOption) *Builder {
	b.config.Store = "redis"
	b.config.RedisAddress = address

	// Apply options
	for _, opt := range options {
		opt(b.config)
	}
	return b
}

// Memory configures the limiter to use in-memory storage (default)
// Example: gorly.New().Memory()
func (b *Builder) Memory() *Builder {
	b.config.Store = "memory"
	return b
}

// Algorithm sets the rate limiting algorithm
// Options: "token_bucket", "sliding_window" (default), "gcra"
// Example: gorly.New().Algorithm("token_bucket")
func (b *Builder) Algorithm(algo string) *Builder {
	b.config.Algorithm = algo
	return b
}

// Limit sets a rate limit for a specific scope
// Example: gorly.New().Limit("global", "1000/hour").Limit("upload", "10/minute")
func (b *Builder) Limit(scope, limit string) *Builder {
	b.config.Limits[scope] = limit
	return b
}

// Limits sets multiple rate limits at once
// Example: gorly.New().Limits(map[string]string{"global": "1000/hour", "upload": "10/minute"})
func (b *Builder) Limits(limits map[string]string) *Builder {
	for scope, limit := range limits {
		b.config.Limits[scope] = limit
	}
	return b
}

// TierLimits sets tier-based rate limits
// Example: gorly.New().TierLimits(map[string]string{"free": "100/hour", "premium": "10000/hour"})
func (b *Builder) TierLimits(tierLimits map[string]string) *Builder {
	if b.config.TierLimits["global"] == nil {
		b.config.TierLimits["global"] = make(map[string]string)
	}
	for tier, limit := range tierLimits {
		b.config.TierLimits["global"][tier] = limit
	}
	return b
}

// ExtractorFunc sets a custom function to extract the entity from HTTP requests
// Example: gorly.New().ExtractorFunc(func(r *http.Request) string { return r.Header.Get("X-API-Key") })
func (b *Builder) ExtractorFunc(fn func(*http.Request) string) *Builder {
	b.config.ExtractorFunc = fn
	return b
}

// ScopeFunc sets a custom function to determine the scope from HTTP requests
// Example: gorly.New().ScopeFunc(func(r *http.Request) string { return strings.TrimPrefix(r.URL.Path, "/api/") })
func (b *Builder) ScopeFunc(fn func(*http.Request) string) *Builder {
	b.config.ScopeFunc = fn
	return b
}

// OnError sets a custom error handler
// Example: gorly.New().OnError(func(err error) { log.Printf("Rate limit error: %v", err) })
func (b *Builder) OnError(fn func(error)) *Builder {
	b.config.ErrorHandler = fn
	return b
}

// OnDenied sets a custom handler for when requests are rate limited
// Example: gorly.New().OnDenied(func(w http.ResponseWriter, r *http.Request, result *LimitResult) { ... })
func (b *Builder) OnDenied(fn func(http.ResponseWriter, *http.Request, *LimitResult)) *Builder {
	// Convert the user's handler to work with internal CoreResult
	b.config.DeniedHandler = func(w http.ResponseWriter, r *http.Request, coreResult *core.CoreResult) {
		// Convert CoreResult to LimitResult
		limitResult := &LimitResult{
			Allowed:    coreResult.Allowed,
			Remaining:  coreResult.Remaining,
			Limit:      coreResult.Limit,
			Used:       coreResult.Used,
			RetryAfter: coreResult.RetryAfter,
			Window:     coreResult.Window,
			ResetTime:  coreResult.ResetTime,
		}
		fn(w, r, limitResult)
	}
	return b
}

// EnableMetrics enables Prometheus metrics collection
// Example: gorly.New().EnableMetrics()
func (b *Builder) EnableMetrics() *Builder {
	b.config.MetricsEnabled = true
	return b
}

// Build creates the rate limiter from the builder configuration
func (b *Builder) Build() (Limiter, error) {
	// Validate configuration
	if err := b.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create the core limiter
	limiter, err := core.NewLimiter(b.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create limiter: %w", err)
	}

	return &limiterImpl{
		core:   limiter,
		config: b.config,
	}, nil
}

// Middleware builds the limiter and returns middleware that auto-detects the framework
func (b *Builder) Middleware() interface{} {
	limiter, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build limiter: %v", err))
	}
	return limiter.Middleware()
}

// =============================================================================
// Redis configuration options
// =============================================================================

// RedisOption configures Redis connection options
type RedisOption func(*core.Config)

// RedisPassword sets the Redis password
func RedisPassword(password string) RedisOption {
	return func(c *core.Config) {
		c.RedisPassword = password
	}
}

// RedisDB sets the Redis database number
func RedisDB(db int) RedisOption {
	return func(c *core.Config) {
		c.RedisDB = db
	}
}

// RedisPoolSize sets the Redis connection pool size
func RedisPoolSize(size int) RedisOption {
	return func(c *core.Config) {
		c.RedisPoolSize = size
	}
}

// =============================================================================
// Default entity extractors
// =============================================================================

// extractIP extracts the client IP address from the request
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
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
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// extractAPIKey extracts the API key from Authorization header or X-API-Key header
func extractAPIKey(r *http.Request) string {
	// Check Authorization header (Bearer token)
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Check X-API-Key header
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return apiKey
	}

	// Check X-Api-Key header (alternative spelling)
	if apiKey := r.Header.Get("X-Api-Key"); apiKey != "" {
		return apiKey
	}

	// Fall back to IP if no API key found
	return extractIP(r)
}

// extractUserID extracts the user ID from X-User-ID header
func extractUserID(r *http.Request) string {
	// Check X-User-ID header
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return userID
	}

	// Check X-User-Id header (alternative spelling)
	if userID := r.Header.Get("X-User-Id"); userID != "" {
		return userID
	}

	// TODO: Extract from JWT token in Authorization header
	// For now, fall back to IP
	return extractIP(r)
}

// extractTier extracts the user tier from X-User-Tier header
func extractTier(r *http.Request) string {
	// Check X-User-Tier header
	if tier := r.Header.Get("X-User-Tier"); tier != "" {
		return tier
	}

	// Check X-Tier header
	if tier := r.Header.Get("X-Tier"); tier != "" {
		return tier
	}

	// Default to "free" tier
	return "free"
}

// =============================================================================
// Internal implementation
// =============================================================================

// limiterImpl implements the Limiter interface
type limiterImpl struct {
	core   core.Limiter
	config *core.Config
}

func (l *limiterImpl) Middleware() interface{} {
	return middleware.New(l.core, l.config)
}

func (l *limiterImpl) For(framework middleware.FrameworkType) interface{} {
	mw := middleware.New(l.core, l.config).(*middleware.UniversalMiddleware)
	return mw.For(framework)
}

func (l *limiterImpl) Check(ctx context.Context, entity string, scope ...string) (*LimitResult, error) {
	scopeName := "global"
	if len(scope) > 0 && scope[0] != "" {
		scopeName = scope[0]
	}

	result, err := l.core.Check(ctx, entity, scopeName)
	if err != nil {
		return nil, err
	}

	return &LimitResult{
		Allowed:    result.Allowed,
		Remaining:  result.Remaining,
		Limit:      result.Limit,
		Used:       result.Used,
		RetryAfter: result.RetryAfter,
		Window:     result.Window,
		ResetTime:  result.ResetTime,
	}, nil
}

func (l *limiterImpl) Allow(ctx context.Context, entity string, scope ...string) (bool, error) {
	result, err := l.Check(ctx, entity, scope...)
	if err != nil {
		return false, err
	}
	return result.Allowed, nil
}

func (l *limiterImpl) Stats(ctx context.Context) (*LimitStats, error) {
	// TODO: Implement stats collection
	return &LimitStats{
		TotalRequests: 0,
		TotalDenied:   0,
		ByScope:       make(map[string]*LimitScopeStats),
		ByEntity:      make(map[string]*EntityStats),
	}, nil
}

func (l *limiterImpl) Health(ctx context.Context) error {
	return l.core.Health(ctx)
}

func (l *limiterImpl) Close() error {
	return l.core.Close()
}
