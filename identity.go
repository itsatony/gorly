package ratelimit

import (
	"fmt"
	"strings"

	nuts "github.com/vaudience/go-nuts"
)

// ============================================================================
// RATE LIMIT CONTEXT INTERFACE
// ============================================================================

// Identity represents the complete context for a rate limit decision
// It replaces the old AuthEntity concept with something more flexible and generic
type Identity interface {
	// Identity returns the unique identifier for this rate limit subject
	// Examples: user ID, API key, IP address, tenant ID, connection ID
	Identity() string

	// Scope returns the rate limit scope
	// Examples: "api", "search", "upload", "db_query", "events"
	Scope() string

	// Tier returns the service tier
	// Examples: "free", "premium", "enterprise", "internal"
	Tier() string

	// Metadata returns additional context for rate limiting decisions
	// Can include: IP address, user agent, resource being accessed, etc.
	Metadata() map[string]interface{}

	// Key generates the storage key for this context
	// Format: "gorly:tier:scope:identity"
	Key() string
}

// ============================================================================
// SIMPLE CONTEXT IMPLEMENTATION
// ============================================================================

// SimpleContext is a basic implementation of Identity
type SimpleContext struct {
	identity string
	scope    string
	tier     string
	metadata map[string]interface{}
}

// NewSimpleContext creates a new simple rate limit context
func NewSimpleContext(identity, scope, tier string, metadata map[string]interface{}) *SimpleContext {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	return &SimpleContext{
		identity: identity,
		scope:    scope,
		tier:     tier,
		metadata: metadata,
	}
}

// Identity returns the identity
func (sc *SimpleContext) Identity() string {
	return sc.identity
}

// Scope returns the scope
func (sc *SimpleContext) Scope() string {
	return sc.scope
}

// Tier returns the tier
func (sc *SimpleContext) Tier() string {
	return sc.tier
}

// Metadata returns the metadata
func (sc *SimpleContext) Metadata() map[string]interface{} {
	return sc.metadata
}

// Key generates the storage key
func (sc *SimpleContext) Key() string {
	return fmt.Sprintf("%s%s%s%s%s%s%s",
		StorageKeyPrefixDefault,
		StorageKeySeparator,
		sc.tier,
		StorageKeySeparator,
		sc.scope,
		StorageKeySeparator,
		sc.identity)
}

// ============================================================================
// CONTEXT BUILDER - Fluent API for context construction
// ============================================================================

// ContextBuilder provides a fluent API for building rate limit contexts
type ContextBuilder struct {
	identity string
	scope    string
	tier     string
	metadata map[string]interface{}
}

// NewContextBuilder creates a new context builder
func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{
		scope:    ScopeGlobal,
		tier:     TierDefault,
		metadata: make(map[string]interface{}),
	}
}

// WithIdentity sets the identity
func (cb *ContextBuilder) WithIdentity(identity string) *ContextBuilder {
	cb.identity = identity
	return cb
}

// WithScope sets the scope
func (cb *ContextBuilder) WithScope(scope string) *ContextBuilder {
	cb.scope = scope
	return cb
}

// WithTier sets the tier
func (cb *ContextBuilder) WithTier(tier string) *ContextBuilder {
	cb.tier = tier
	return cb
}

// WithMetadata adds metadata (replaces existing)
func (cb *ContextBuilder) WithMetadata(metadata map[string]interface{}) *ContextBuilder {
	cb.metadata = metadata
	return cb
}

// AddMetadata adds a single metadata entry
func (cb *ContextBuilder) AddMetadata(key string, value interface{}) *ContextBuilder {
	if cb.metadata == nil {
		cb.metadata = make(map[string]interface{})
	}
	cb.metadata[key] = value
	return cb
}

// WithIP adds IP address to metadata
func (cb *ContextBuilder) WithIP(ip string) *ContextBuilder {
	return cb.AddMetadata(MetadataKeyIP, ip)
}

// WithUserAgent adds user agent to metadata
func (cb *ContextBuilder) WithUserAgent(ua string) *ContextBuilder {
	return cb.AddMetadata(MetadataKeyUserAgent, ua)
}

// WithMethod adds HTTP method to metadata
func (cb *ContextBuilder) WithMethod(method string) *ContextBuilder {
	return cb.AddMetadata(MetadataKeyMethod, method)
}

// WithPath adds URL path to metadata
func (cb *ContextBuilder) WithPath(path string) *ContextBuilder {
	return cb.AddMetadata(MetadataKeyPath, path)
}

// Build creates the rate limit context
func (cb *ContextBuilder) Build() (Identity, error) {
	if cb.identity == "" {
		return nil, WrapContextError(nil, "identity is required")
	}

	if cb.scope == "" {
		cb.scope = ScopeGlobal
	}

	if cb.tier == "" {
		cb.tier = TierDefault
	}

	return NewSimpleContext(cb.identity, cb.scope, cb.tier, cb.metadata), nil
}

// MustBuild creates the rate limit context or panics
func (cb *ContextBuilder) MustBuild() Identity {
	ctx, err := cb.Build()
	if err != nil {
		panic(err)
	}
	return ctx
}

// ============================================================================
// CONVENIENCE CONSTRUCTORS
// ============================================================================

// NewIPContext creates a rate limit context for IP-based limiting
func NewIPContext(ip string) Identity {
	return NewSimpleContext(ip, ScopeGlobal, TierFree, map[string]interface{}{
		MetadataKeyIP: ip,
	})
}

// NewAPIKeyContext creates a rate limit context for API key-based limiting
func NewAPIKeyContext(apiKey, tier string) Identity {
	return NewSimpleContext(apiKey, ScopeAPI, tier, map[string]interface{}{
		"api_key": apiKey,
	})
}

// NewUserContext creates a rate limit context for user-based limiting
func NewUserContext(userID, tier string) Identity {
	return NewSimpleContext(userID, ScopeGlobal, tier, map[string]interface{}{
		MetadataKeyUserID: userID,
	})
}

// NewTenantContext creates a rate limit context for tenant-based limiting
func NewTenantContext(tenantID, tier string) Identity {
	return NewSimpleContext(tenantID, ScopeGlobal, tier, map[string]interface{}{
		MetadataKeyTenant: tenantID,
	})
}

// ============================================================================
// CONTEXT WITH UNIQUE ID
// ============================================================================

// IdentifiedContext wraps a Identity with a unique ID
type IdentifiedContext struct {
	id      string
	wrapped Identity
}

// NewIdentifiedContext creates a context with a unique ID
func NewIdentifiedContext(wrapped Identity) *IdentifiedContext {
	return &IdentifiedContext{
		id:      nuts.NID(IDPrefixContext, 16),
		wrapped: wrapped,
	}
}

// ID returns the unique context ID
func (ic *IdentifiedContext) ID() string {
	return ic.id
}

// Identity returns the wrapped context's identity
func (ic *IdentifiedContext) Identity() string {
	return ic.wrapped.Identity()
}

// Scope returns the wrapped context's scope
func (ic *IdentifiedContext) Scope() string {
	return ic.wrapped.Scope()
}

// Tier returns the wrapped context's tier
func (ic *IdentifiedContext) Tier() string {
	return ic.wrapped.Tier()
}

// Metadata returns the wrapped context's metadata
func (ic *IdentifiedContext) Metadata() map[string]interface{} {
	return ic.wrapped.Metadata()
}

// Key returns the wrapped context's key
func (ic *IdentifiedContext) Key() string {
	return ic.wrapped.Key()
}

// ============================================================================
// CONTEXT VALIDATION
// ============================================================================

// ValidateContext validates a rate limit context
func ValidateContext(ctx Identity) error {
	if ctx == nil {
		return WrapContextError(nil, "context is nil")
	}

	if ctx.Identity() == "" {
		return WrapContextError(nil, "context identity is empty")
	}

	if ctx.Scope() == "" {
		return WrapContextError(nil, "context scope is empty")
	}

	if ctx.Tier() == "" {
		return WrapContextError(nil, "context tier is empty")
	}

	return nil
}

// ============================================================================
// CONTEXT UTILITIES
// ============================================================================

// ParseKey parses a storage key back into components
// Expected format: "gorly:tier:scope:identity"
func ParseKey(key string) (tier, scope, identity string, err error) {
	parts := strings.Split(key, StorageKeySeparator)
	if len(parts) != 4 {
		return "", "", "", fmt.Errorf("invalid key format: expected 4 parts, got %d", len(parts))
	}

	if parts[0] != StorageKeyPrefixDefault {
		return "", "", "", fmt.Errorf("invalid key prefix: expected %s, got %s", StorageKeyPrefixDefault, parts[0])
	}

	return parts[1], parts[2], parts[3], nil
}

// ContextFromKey creates a simple context from a storage key
func ContextFromKey(key string) (Identity, error) {
	tier, scope, identity, err := ParseKey(key)
	if err != nil {
		return nil, WrapContextError(err, "failed to parse key", "key", key)
	}

	return NewSimpleContext(identity, scope, tier, nil), nil
}

// ContextsEqual checks if two contexts are equivalent for rate limiting
func ContextsEqual(a, b Identity) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Identity() == b.Identity() &&
		a.Scope() == b.Scope() &&
		a.Tier() == b.Tier()
}

// CloneContext creates a deep copy of a rate limit context
func CloneContext(ctx Identity) Identity {
	if ctx == nil {
		return nil
	}

	// Copy metadata
	metadata := make(map[string]interface{})
	for k, v := range ctx.Metadata() {
		metadata[k] = v
	}

	return NewSimpleContext(ctx.Identity(), ctx.Scope(), ctx.Tier(), metadata)
}
