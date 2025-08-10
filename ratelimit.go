// ratelimit.go
package ratelimit

import (
	"context"
	"time"
)

// AuthEntity represents a flexible authentication entity for rate limiting
// Can be API keys, users, tenants, IP addresses, or custom entities
type AuthEntity interface {
	// ID returns the unique identifier for this entity
	ID() string

	// Type returns the entity type (api_key, user, tenant, ip, custom)
	Type() string

	// Tier returns the service tier (free, premium, enterprise, custom)
	Tier() string

	// Metadata returns additional entity-specific metadata
	Metadata() map[string]interface{}
}

// DefaultAuthEntity provides a simple implementation of AuthEntity
type DefaultAuthEntity struct {
	IDValue       string                 `json:"id"`
	TypeValue     string                 `json:"type"`
	TierValue     string                 `json:"tier"`
	MetadataValue map[string]interface{} `json:"metadata,omitempty"`
}

func (e *DefaultAuthEntity) ID() string   { return e.IDValue }
func (e *DefaultAuthEntity) Type() string { return e.TypeValue }
func (e *DefaultAuthEntity) Tier() string { return e.TierValue }
func (e *DefaultAuthEntity) Metadata() map[string]interface{} {
	if e.MetadataValue == nil {
		return make(map[string]interface{})
	}
	return e.MetadataValue
}

// NewDefaultAuthEntity creates a new DefaultAuthEntity
func NewDefaultAuthEntity(id, entityType, tier string) *DefaultAuthEntity {
	return &DefaultAuthEntity{
		IDValue:       id,
		TypeValue:     entityType,
		TierValue:     tier,
		MetadataValue: make(map[string]interface{}),
	}
}

// Result represents the result of a rate limit check
type Result struct {
	// Allowed indicates whether the request is allowed
	Allowed bool `json:"allowed"`

	// Remaining indicates how many requests are remaining in the current window
	Remaining int64 `json:"remaining"`

	// RetryAfter indicates when the client can retry (for denied requests)
	RetryAfter time.Duration `json:"retry_after"`

	// ResetTime indicates when the rate limit window resets
	ResetTime time.Time `json:"reset_time"`

	// Limit indicates the total limit for this entity/scope
	Limit int64 `json:"limit"`

	// Window indicates the time window for this rate limit
	Window time.Duration `json:"window"`

	// Used indicates how many requests have been used in current window
	Used int64 `json:"used"`

	// Algorithm indicates which rate limiting algorithm was used
	Algorithm string `json:"algorithm"`
}

// Stats represents usage statistics for an entity
type Stats struct {
	// Entity information
	Entity AuthEntity `json:"entity"`

	// Per-scope statistics
	Scopes map[string]ScopeStats `json:"scopes"`

	// Overall statistics
	TotalRequests int64     `json:"total_requests"`
	TotalDenied   int64     `json:"total_denied"`
	LastRequest   time.Time `json:"last_request,omitempty"`
	FirstRequest  time.Time `json:"first_request,omitempty"`

	// Performance metrics
	AverageLatency time.Duration `json:"average_latency"`
	MaxLatency     time.Duration `json:"max_latency"`

	// Rate limiting metrics
	RateLimitHits   int64 `json:"rate_limit_hits"`
	RateLimitMisses int64 `json:"rate_limit_misses"`
}

// ScopeStats represents statistics for a specific scope
type ScopeStats struct {
	Scope        string        `json:"scope"`
	RequestCount int64         `json:"request_count"`
	DeniedCount  int64         `json:"denied_count"`
	LastRequest  time.Time     `json:"last_request,omitempty"`
	CurrentUsage int64         `json:"current_usage"`
	Limit        int64         `json:"limit"`
	Window       time.Duration `json:"window"`
	Algorithm    string        `json:"algorithm"`
}

// RateLimiter is the core interface for rate limiting functionality
type RateLimiter interface {
	// Allow checks if a request is allowed for the given entity and scope
	Allow(ctx context.Context, entity AuthEntity, scope string) (*Result, error)

	// AllowN checks if N requests are allowed for the given entity and scope
	AllowN(ctx context.Context, entity AuthEntity, scope string, n int64) (*Result, error)

	// Reset resets the rate limit for the given entity and scope
	Reset(ctx context.Context, entity AuthEntity, scope string) error

	// Stats returns usage statistics for the given entity
	Stats(ctx context.Context, entity AuthEntity) (*Stats, error)

	// ScopeStats returns statistics for a specific entity and scope
	ScopeStats(ctx context.Context, entity AuthEntity, scope string) (*ScopeStats, error)

	// Health checks the health of the rate limiter (e.g., Redis connectivity)
	Health(ctx context.Context) error

	// Close cleans up any resources used by the rate limiter
	Close() error
}

// ErrorType represents different types of rate limiting errors
type ErrorType string

const (
	ErrorTypeStore     ErrorType = "store_error"
	ErrorTypeAlgorithm ErrorType = "algorithm_error"
	ErrorTypeConfig    ErrorType = "config_error"
	ErrorTypeNetwork   ErrorType = "network_error"
	ErrorTypeTimeout   ErrorType = "timeout_error"
)

// RateLimitError represents an error in rate limiting operations
type RateLimitError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Scope   string    `json:"scope,omitempty"`
	Entity  string    `json:"entity,omitempty"`
	Err     error     `json:"-"` // Don't serialize the wrapped error
}

func (e *RateLimitError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *RateLimitError) Unwrap() error {
	return e.Err
}

// NewRateLimitError creates a new RateLimitError
func NewRateLimitError(errorType ErrorType, message string, err error) *RateLimitError {
	return &RateLimitError{
		Type:    errorType,
		Message: message,
		Err:     err,
	}
}

// Common entity types
const (
	EntityTypeAPIKey = "api_key"
	EntityTypeUser   = "user"
	EntityTypeTenant = "tenant"
	EntityTypeIP     = "ip"
	EntityTypeCustom = "custom"
)

// Common service tiers
const (
	TierFree       = "free"
	TierPremium    = "premium"
	TierEnterprise = "enterprise"
	TierCustom     = "custom"
)

// Common scopes
const (
	ScopeGlobal    = "global"
	ScopeMemory    = "memory"
	ScopeSearch    = "search"
	ScopeMetadata  = "metadata"
	ScopeAnalytics = "analytics"
	ScopeAdmin     = "admin"
)

// KeyBuilder helps build consistent keys for rate limiting
type KeyBuilder struct {
	prefix string
}

// NewKeyBuilder creates a new KeyBuilder with the given prefix
func NewKeyBuilder(prefix string) *KeyBuilder {
	return &KeyBuilder{prefix: prefix}
}

// BuildKey builds a key for the given entity and scope
func (kb *KeyBuilder) BuildKey(entity AuthEntity, scope string) string {
	if kb.prefix == "" {
		return entity.Type() + ":" + entity.ID() + ":" + scope
	}
	return kb.prefix + ":" + entity.Type() + ":" + entity.ID() + ":" + scope
}

// BuildStatsKey builds a key for statistics storage
func (kb *KeyBuilder) BuildStatsKey(entity AuthEntity) string {
	if kb.prefix == "" {
		return "stats:" + entity.Type() + ":" + entity.ID()
	}
	return kb.prefix + ":stats:" + entity.Type() + ":" + entity.ID()
}

// BuildGlobalStatsKey builds a key for global statistics
func (kb *KeyBuilder) BuildGlobalStatsKey() string {
	if kb.prefix == "" {
		return "stats:global"
	}
	return kb.prefix + ":stats:global"
}
