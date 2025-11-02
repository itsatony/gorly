package ratelimit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// LIMIT CONFIGURATION
// ============================================================================

// LimitConfig represents a single rate limit configuration
type LimitConfig struct {
	// Limit is the number of requests allowed
	Limit int64

	// Window is the time window for the limit
	Window time.Duration

	// Burst is the burst size (for token bucket)
	Burst int64

	// Strategy is the rate limiting strategy to use
	Strategy string
}

// NewLimitConfig creates a new limit configuration
func NewLimitConfig(limit int64, window time.Duration, burst int64) *LimitConfig {
	return &LimitConfig{
		Limit:    limit,
		Window:   window,
		Burst:    burst,
		Strategy: StrategyTokenBucket,
	}
}

// ParseRateString parses a rate string like "1000/1h" into limit and window
// Supported formats:
//   - "1000/1h" - 1000 requests per hour
//   - "100/1m" - 100 requests per minute
//   - "10/1s" - 10 requests per second
//   - "5000/1d" - 5000 requests per day
//
// SECURITY: This function includes comprehensive validation to prevent DOS attacks:
//   - Input length limited to 32 characters
//   - Only accepts positive non-zero values
//   - Overflow protection for all calculations
//   - Strict bounds checking on all parameters
func ParseRateString(rateStr string) (int64, time.Duration, error) {
	// SECURITY: Prevent DOS via oversized input strings
	if len(rateStr) > 32 {
		return 0, 0, WrapConfigError(nil, "rate string exceeds maximum length",
			"rate_string_length", len(rateStr),
			"max_length", 32,
			"recommendation", "use shorter rate strings like '1000/1h'")
	}

	// SECURITY: Validate basic structure before regex
	if rateStr == "" {
		return 0, 0, WrapConfigError(nil, "rate string cannot be empty",
			"expected_format", "N/Xt where N=requests (1-1000000), X=multiplier (1-999999), t=s|m|h|d")
	}

	if !strings.Contains(rateStr, "/") {
		return 0, 0, WrapConfigError(nil, "rate string must contain '/'",
			"rate_string", rateStr,
			"expected_format", "N/Xt where N=requests, X=multiplier, t=s|m|h|d")
	}

	// SECURITY: Stricter regex requiring non-zero positive values
	// [1-9]\d{0,9} = 1-9999999999 (prevents leading zeros and zero values)
	// [1-9]\d{0,5} = 1-999999 (reasonable window multipliers)
	re := regexp.MustCompile(`^([1-9]\d{0,9})/([1-9]\d{0,5})(s|m|h|d)$`)
	matches := re.FindStringSubmatch(rateStr)

	if len(matches) != 4 {
		return 0, 0, WrapConfigError(nil, "invalid rate string format",
			"rate_string", rateStr,
			"expected_format", "N/Xt where N=1-9999999999, X=1-999999, t=s|m|h|d",
			"examples", []string{"100/1h", "1000/1m", "50/30s", "5000/1d"},
			"note", "limit and multiplier must be positive non-zero integers")
	}

	// Parse limit
	limit, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, 0, WrapConfigError(err, "invalid limit in rate string",
			"rate_string", rateStr)
	}

	// Parse window multiplier
	multiplier, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return 0, 0, WrapConfigError(err, "invalid window multiplier in rate string",
			"rate_string", rateStr)
	}

	// SECURITY: Validate before calculations to prevent overflow
	// Check that limit is within acceptable bounds
	if limit < MinLimit {
		return 0, 0, WrapConfigError(nil, "limit too small",
			"limit", limit,
			"min", MinLimit,
			"recommendation", fmt.Sprintf("use limit >= %d", MinLimit))
	}

	if limit > MaxLimit {
		return 0, 0, WrapConfigError(nil, "limit exceeds maximum",
			"limit", limit,
			"max", MaxLimit,
			"recommendation", fmt.Sprintf("use limit <= %d", MaxLimit))
	}

	// SECURITY: Check multiplier bounds to prevent time.Duration overflow
	// time.Duration is int64 nanoseconds, max ~290 years
	// We limit to 24 hours = 86400 seconds to be safe
	if multiplier > MaxWindowSeconds {
		return 0, 0, WrapConfigError(nil, "window multiplier exceeds maximum",
			"multiplier", multiplier,
			"max", MaxWindowSeconds,
			"recommendation", fmt.Sprintf("use multiplier <= %d (24 hours max)", MaxWindowSeconds))
	}

	// Parse unit with overflow protection
	var window time.Duration
	switch matches[3] {
	case "s":
		// SECURITY: Verify multiplication won't overflow
		if multiplier > MaxWindowSeconds {
			return 0, 0, WrapConfigError(nil, "window duration too large",
				"multiplier", multiplier,
				"unit", "seconds",
				"max_seconds", MaxWindowSeconds)
		}
		window = time.Duration(multiplier) * time.Second

	case "m":
		// SECURITY: Check minutes don't exceed max seconds
		if multiplier > MaxWindowSeconds/60 {
			return 0, 0, WrapConfigError(nil, "window duration too large",
				"multiplier", multiplier,
				"unit", "minutes",
				"max_minutes", MaxWindowSeconds/60,
				"recommendation", fmt.Sprintf("use <= %d minutes", MaxWindowSeconds/60))
		}
		window = time.Duration(multiplier) * time.Minute

	case "h":
		// SECURITY: Check hours don't exceed max seconds
		if multiplier > MaxWindowSeconds/3600 {
			return 0, 0, WrapConfigError(nil, "window duration too large",
				"multiplier", multiplier,
				"unit", "hours",
				"max_hours", MaxWindowSeconds/3600,
				"recommendation", fmt.Sprintf("use <= %d hours (24 hours max)", MaxWindowSeconds/3600))
		}
		window = time.Duration(multiplier) * time.Hour

	case "d":
		// SECURITY: Check days don't exceed max seconds
		if multiplier > MaxWindowSeconds/86400 {
			return 0, 0, WrapConfigError(nil, "window duration too large",
				"multiplier", multiplier,
				"unit", "days",
				"max_days", MaxWindowSeconds/86400,
				"recommendation", "use <= 1 day (24 hours max)")
		}
		window = time.Duration(multiplier) * 24 * time.Hour

	default:
		return 0, 0, WrapConfigError(nil, "unsupported time unit",
			"rate_string", rateStr,
			"unit", matches[3],
			"supported_units", []string{"s (seconds)", "m (minutes)", "h (hours)", "d (days)"})
	}

	// Final validation: Ensure window is within acceptable range
	minWindow := time.Duration(MinWindowSeconds) * time.Second
	maxWindow := time.Duration(MaxWindowSeconds) * time.Second

	if window < minWindow {
		return 0, 0, WrapConfigError(nil, "window duration too short",
			"window", window,
			"min", minWindow,
			"recommendation", fmt.Sprintf("use window >= %v", minWindow))
	}

	if window > maxWindow {
		return 0, 0, WrapConfigError(nil, "window duration too long",
			"window", window,
			"max", maxWindow,
			"recommendation", fmt.Sprintf("use window <= %v (24 hours)", maxWindow))
	}

	return limit, window, nil
}

// NewLimitConfigFromRate creates a limit config from a rate string
func NewLimitConfigFromRate(rateStr string, burst int64) (*LimitConfig, error) {
	limit, window, err := ParseRateString(rateStr)
	if err != nil {
		return nil, err
	}

	return &LimitConfig{
		Limit:    limit,
		Window:   window,
		Burst:    burst,
		Strategy: StrategyTokenBucket,
	}, nil
}

// Clone creates a copy of the limit configuration
func (lc *LimitConfig) Clone() *LimitConfig {
	if lc == nil {
		return nil
	}
	return &LimitConfig{
		Limit:    lc.Limit,
		Window:   lc.Window,
		Burst:    lc.Burst,
		Strategy: lc.Strategy,
	}
}

// Validate validates the limit configuration
func (lc *LimitConfig) Validate() error {
	if lc == nil {
		return WrapConfigError(nil, "limit config is nil")
	}

	if lc.Limit < MinLimit || lc.Limit > MaxLimit {
		return WrapConfigError(nil, "limit out of bounds",
			"limit", lc.Limit,
			"min", MinLimit,
			"max", MaxLimit)
	}

	if lc.Window < time.Duration(MinWindowSeconds)*time.Second ||
		lc.Window > time.Duration(MaxWindowSeconds)*time.Second {
		return WrapConfigError(nil, "window out of bounds",
			"window", lc.Window,
			"min", time.Duration(MinWindowSeconds)*time.Second,
			"max", time.Duration(MaxWindowSeconds)*time.Second)
	}

	if lc.Burst < MinBurst || lc.Burst > MaxBurst {
		return WrapConfigError(nil, "burst out of bounds",
			"burst", lc.Burst,
			"min", MinBurst,
			"max", MaxBurst)
	}

	// Validate burst is reasonable relative to limit
	// Burst should not exceed limit unless limit is very small
	if lc.Burst > lc.Limit && lc.Limit > 10 {
		return WrapConfigError(nil, "burst size exceeds limit",
			"burst", lc.Burst,
			"limit", lc.Limit,
			"recommendation", "burst should typically be <= limit")
	}

	// Validate strategy if specified
	if lc.Strategy != "" {
		validStrategies := map[string]bool{
			StrategyTokenBucket:   true,
			StrategySlidingWindow: true,
			StrategyFixedWindow:   true,
			StrategyLeakyBucket:   true,
		}
		if !validStrategies[lc.Strategy] {
			return WrapConfigError(nil, "invalid strategy",
				"strategy", lc.Strategy,
				"valid_strategies", []string{
					StrategyTokenBucket,
					StrategySlidingWindow,
					StrategyFixedWindow,
					StrategyLeakyBucket,
				})
		}
	}

	return nil
}

// ============================================================================
// TIER CONFIGURATION
// ============================================================================

// TierConfig represents rate limit configuration for a service tier
type TierConfig struct {
	// TierName is the name of the tier
	TierName string

	// ScopeLimits maps scope names to their specific limits
	// Example: "global" -> 1000/1h, "search" -> 100/1m
	ScopeLimits map[string]*LimitConfig

	// DefaultLimit is the default limit for scopes not explicitly configured
	DefaultLimit *LimitConfig

	// Strategy is the default strategy for this tier
	Strategy string

	// Description is a human-readable description of this tier
	Description string
}

// NewTierConfig creates a new tier configuration
func NewTierConfig(tierName string, defaultLimit *LimitConfig) *TierConfig {
	return &TierConfig{
		TierName:     tierName,
		ScopeLimits:  make(map[string]*LimitConfig),
		DefaultLimit: defaultLimit,
		Strategy:     StrategyTokenBucket,
	}
}

// SetScopeLimit sets the limit for a specific scope
func (tc *TierConfig) SetScopeLimit(scope string, limit *LimitConfig) {
	if tc.ScopeLimits == nil {
		tc.ScopeLimits = make(map[string]*LimitConfig)
	}
	tc.ScopeLimits[scope] = limit
}

// GetScopeLimit gets the limit for a specific scope, or default if not found
func (tc *TierConfig) GetScopeLimit(scope string) *LimitConfig {
	if limit, ok := tc.ScopeLimits[scope]; ok {
		return limit
	}
	return tc.DefaultLimit
}

// Clone creates a copy of the tier configuration
func (tc *TierConfig) Clone() *TierConfig {
	if tc == nil {
		return nil
	}

	clone := &TierConfig{
		TierName:     tc.TierName,
		ScopeLimits:  make(map[string]*LimitConfig),
		DefaultLimit: tc.DefaultLimit.Clone(),
		Strategy:     tc.Strategy,
		Description:  tc.Description,
	}

	for scope, limit := range tc.ScopeLimits {
		clone.ScopeLimits[scope] = limit.Clone()
	}

	return clone
}

// Validate validates the tier configuration
func (tc *TierConfig) Validate() error {
	if tc == nil {
		return WrapConfigError(nil, "tier config is nil")
	}

	// Validate tier name
	if err := validateIdentifierString(tc.TierName, "tier name"); err != nil {
		return err
	}

	if tc.DefaultLimit == nil {
		return WrapConfigError(nil, "default limit is required",
			"tier", tc.TierName)
	}

	if err := tc.DefaultLimit.Validate(); err != nil {
		return WrapConfigError(err, "invalid default limit",
			"tier", tc.TierName)
	}

	// Validate strategy if specified
	if tc.Strategy != "" {
		validStrategies := map[string]bool{
			StrategyTokenBucket:   true,
			StrategySlidingWindow: true,
			StrategyFixedWindow:   true,
			StrategyLeakyBucket:   true,
		}
		if !validStrategies[tc.Strategy] {
			return WrapConfigError(nil, "invalid strategy",
				"tier", tc.TierName,
				"strategy", tc.Strategy)
		}
	}

	// Validate scope limits
	for scope, limit := range tc.ScopeLimits {
		// Validate scope name
		if err := validateIdentifierString(scope, "scope"); err != nil {
			return WrapConfigError(err, "invalid scope in tier config",
				"tier", tc.TierName,
				"scope", scope)
		}

		if err := limit.Validate(); err != nil {
			return WrapConfigError(err, "invalid scope limit",
				"tier", tc.TierName,
				"scope", scope)
		}
	}

	return nil
}

// ============================================================================
// RESOLVER CONFIGURATION
// ============================================================================

// ResolverConfig represents configuration for the limit resolver
type ResolverConfig struct {
	// TierConfigs maps tier names to their configurations
	TierConfigs map[string]*TierConfig

	// EntityOverrides maps entity IDs to their specific limit configurations
	// This takes highest precedence in resolution
	EntityOverrides map[string]map[string]*LimitConfig // entity_id -> scope -> limit

	// DefaultTierConfig is used when no tier-specific config is found
	DefaultTierConfig *TierConfig

	// EnableEntityOverrides enables entity-specific overrides
	EnableEntityOverrides bool

	// EnableTierLimits enables tier-based limits
	EnableTierLimits bool

	// EnableScopeLimits enables scope-based limits
	EnableScopeLimits bool
}

// NewResolverConfig creates a new resolver configuration
func NewResolverConfig() *ResolverConfig {
	return &ResolverConfig{
		TierConfigs:           make(map[string]*TierConfig),
		EntityOverrides:       make(map[string]map[string]*LimitConfig),
		EnableEntityOverrides: true,
		EnableTierLimits:      true,
		EnableScopeLimits:     true,
	}
}

// SetTierConfig sets the configuration for a tier
func (rc *ResolverConfig) SetTierConfig(tier string, config *TierConfig) {
	if rc.TierConfigs == nil {
		rc.TierConfigs = make(map[string]*TierConfig)
	}
	rc.TierConfigs[tier] = config
}

// SetEntityOverride sets a limit override for a specific entity and scope
func (rc *ResolverConfig) SetEntityOverride(entityID, scope string, limit *LimitConfig) {
	if rc.EntityOverrides == nil {
		rc.EntityOverrides = make(map[string]map[string]*LimitConfig)
	}
	if rc.EntityOverrides[entityID] == nil {
		rc.EntityOverrides[entityID] = make(map[string]*LimitConfig)
	}
	rc.EntityOverrides[entityID][scope] = limit
}

// Validate validates the resolver configuration
func (rc *ResolverConfig) Validate() error {
	if rc == nil {
		return WrapConfigError(nil, "resolver config is nil")
	}

	// Validate default tier config
	if rc.DefaultTierConfig != nil {
		if err := rc.DefaultTierConfig.Validate(); err != nil {
			return WrapConfigError(err, "invalid default tier config")
		}
	}

	// Validate tier configs
	for tier, config := range rc.TierConfigs {
		// Validate tier name
		if err := validateIdentifierString(tier, "tier"); err != nil {
			return WrapConfigError(err, "invalid tier name in tier configs")
		}

		if config == nil {
			return WrapConfigError(nil, "tier config is nil",
				"tier", tier)
		}

		if err := config.Validate(); err != nil {
			return WrapConfigError(err, "invalid tier config",
				"tier", tier)
		}
	}

	// Validate entity overrides
	for entityID, overrides := range rc.EntityOverrides {
		// Validate entity ID
		if err := validateIdentifierString(entityID, "entity ID"); err != nil {
			return WrapConfigError(err, "invalid entity ID in overrides")
		}

		if overrides == nil || len(overrides) == 0 {
			return WrapConfigError(nil, "entity has no overrides",
				"entity", entityID,
				"recommendation", "remove empty entity override entries")
		}

		for scope, limit := range overrides {
			// Validate scope name
			if err := validateIdentifierString(scope, "scope"); err != nil {
				return WrapConfigError(err, "invalid scope in entity override",
					"entity", entityID)
			}

			if limit == nil {
				return WrapConfigError(nil, "limit is nil in entity override",
					"entity", entityID,
					"scope", scope)
			}

			if err := limit.Validate(); err != nil {
				return WrapConfigError(err, "invalid entity override",
					"entity", entityID,
					"scope", scope)
			}
		}
	}

	return nil
}

// ============================================================================
// LIMIT RESOLVER INTERFACE
// ============================================================================

// LimitResolver resolves rate limit configuration for a given context
type LimitResolver interface {
	// ResolveLimit resolves the rate limit for the given context
	// Returns the resolved limit configuration based on the hierarchy:
	// 1. Entity-specific override (if enabled)
	// 2. Tier-specific limit for scope (if enabled)
	// 3. Tier default limit (if enabled)
	// 4. Global default limit
	ResolveLimit(rlCtx Identity) (*LimitConfig, error)

	// SetEntityOverride sets a limit override for a specific entity
	SetEntityOverride(entityID, scope string, limit *LimitConfig) error

	// RemoveEntityOverride removes a limit override for an entity
	RemoveEntityOverride(entityID, scope string) error

	// GetTierConfig returns the configuration for a tier
	GetTierConfig(tier string) (*TierConfig, error)
}

// ============================================================================
// DEFAULT LIMIT RESOLVER IMPLEMENTATION
// ============================================================================

// defaultResolver implements the LimitResolver interface
type defaultResolver struct {
	config *ResolverConfig
}

// NewLimitResolver creates a new limit resolver with the given configuration
func NewLimitResolver(config *ResolverConfig) (LimitResolver, error) {
	if config == nil {
		return nil, WrapConfigError(nil, "resolver config is required")
	}

	if err := config.Validate(); err != nil {
		return nil, WrapConfigError(err, "invalid resolver config")
	}

	return &defaultResolver{
		config: config,
	}, nil
}

// ResolveLimit resolves the rate limit for the given context
func (r *defaultResolver) ResolveLimit(rlCtx Identity) (*LimitConfig, error) {
	if rlCtx == nil {
		return nil, WrapConfigError(nil, "rate limit context is required")
	}

	identity := rlCtx.Identity()
	scope := rlCtx.Scope()
	tier := rlCtx.Tier()

	// 1. Check for entity-specific override (highest priority)
	if r.config.EnableEntityOverrides {
		if entityOverrides, ok := r.config.EntityOverrides[identity]; ok {
			if limit, ok := entityOverrides[scope]; ok {
				return limit.Clone(), nil
			}
			// Check for global scope override
			if limit, ok := entityOverrides[ScopeGlobal]; ok {
				return limit.Clone(), nil
			}
		}
	}

	// 2. Check for tier-specific configuration
	if r.config.EnableTierLimits {
		if tierConfig, ok := r.config.TierConfigs[tier]; ok {
			// 2a. Check for scope-specific limit in tier
			if r.config.EnableScopeLimits {
				if limit := tierConfig.GetScopeLimit(scope); limit != nil {
					return limit.Clone(), nil
				}
			}
			// 2b. Use tier default limit
			if tierConfig.DefaultLimit != nil {
				return tierConfig.DefaultLimit.Clone(), nil
			}
		}
	}

	// 3. Fall back to default tier configuration
	if r.config.DefaultTierConfig != nil {
		if r.config.EnableScopeLimits {
			if limit := r.config.DefaultTierConfig.GetScopeLimit(scope); limit != nil {
				return limit.Clone(), nil
			}
		}
		if r.config.DefaultTierConfig.DefaultLimit != nil {
			return r.config.DefaultTierConfig.DefaultLimit.Clone(), nil
		}
	}

	// 4. Return global default as last resort
	return NewLimitConfig(DefaultLimit, DefaultWindow, DefaultBurst), nil
}

// SetEntityOverride sets a limit override for a specific entity
func (r *defaultResolver) SetEntityOverride(entityID, scope string, limit *LimitConfig) error {
	if entityID == "" {
		return WrapConfigError(nil, "entity ID is required")
	}
	if scope == "" {
		return WrapConfigError(nil, "scope is required")
	}
	if limit == nil {
		return WrapConfigError(nil, "limit is required")
	}

	if err := limit.Validate(); err != nil {
		return WrapConfigError(err, "invalid limit configuration")
	}

	r.config.SetEntityOverride(entityID, scope, limit)
	return nil
}

// RemoveEntityOverride removes a limit override for an entity
func (r *defaultResolver) RemoveEntityOverride(entityID, scope string) error {
	if entityID == "" {
		return WrapConfigError(nil, "entity ID is required")
	}
	if scope == "" {
		return WrapConfigError(nil, "scope is required")
	}

	if entityOverrides, ok := r.config.EntityOverrides[entityID]; ok {
		delete(entityOverrides, scope)
		if len(entityOverrides) == 0 {
			delete(r.config.EntityOverrides, entityID)
		}
	}

	return nil
}

// GetTierConfig returns the configuration for a tier
func (r *defaultResolver) GetTierConfig(tier string) (*TierConfig, error) {
	if tier == "" {
		return nil, WrapConfigError(nil, "tier is required")
	}

	if config, ok := r.config.TierConfigs[tier]; ok {
		return config.Clone(), nil
	}

	if r.config.DefaultTierConfig != nil {
		return r.config.DefaultTierConfig.Clone(), nil
	}

	return nil, WrapConfigError(nil, "tier config not found", "tier", tier)
}

// ============================================================================
// PRESET CONFIGURATIONS
// ============================================================================

// NewDefaultResolverConfig creates a resolver config with sensible defaults
func NewDefaultResolverConfig() *ResolverConfig {
	config := NewResolverConfig()

	// Configure Free tier
	freeTier := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))
	freeTier.Description = "Free tier with basic rate limits"
	freeTier.SetScopeLimit(ScopeAPI, NewLimitConfig(100, time.Hour, 10))
	freeTier.SetScopeLimit(ScopeSearch, NewLimitConfig(20, time.Hour, 5))
	freeTier.SetScopeLimit(ScopeUpload, NewLimitConfig(10, time.Hour, 2))
	config.SetTierConfig(TierFree, freeTier)

	// Configure Premium tier
	premiumTier := NewTierConfig(TierPremium, NewLimitConfig(1000, time.Hour, 100))
	premiumTier.Description = "Premium tier with increased limits"
	premiumTier.SetScopeLimit(ScopeAPI, NewLimitConfig(1000, time.Hour, 100))
	premiumTier.SetScopeLimit(ScopeSearch, NewLimitConfig(500, time.Hour, 50))
	premiumTier.SetScopeLimit(ScopeUpload, NewLimitConfig(100, time.Hour, 20))
	config.SetTierConfig(TierPremium, premiumTier)

	// Configure Enterprise tier
	enterpriseTier := NewTierConfig(TierEnterprise, NewLimitConfig(10000, time.Hour, 1000))
	enterpriseTier.Description = "Enterprise tier with high limits"
	enterpriseTier.SetScopeLimit(ScopeAPI, NewLimitConfig(10000, time.Hour, 1000))
	enterpriseTier.SetScopeLimit(ScopeSearch, NewLimitConfig(5000, time.Hour, 500))
	enterpriseTier.SetScopeLimit(ScopeUpload, NewLimitConfig(1000, time.Hour, 100))
	config.SetTierConfig(TierEnterprise, enterpriseTier)

	// Configure Internal tier
	internalTier := NewTierConfig(TierInternal, NewLimitConfig(100000, time.Hour, 10000))
	internalTier.Description = "Internal tier for system requests"
	config.SetTierConfig(TierInternal, internalTier)

	// Set default tier config
	config.DefaultTierConfig = NewTierConfig(TierDefault, NewLimitConfig(DefaultLimit, DefaultWindow, DefaultBurst))
	config.DefaultTierConfig.Description = "Default tier configuration"

	return config
}

// NewStrictResolverConfig creates a resolver config with strict limits
func NewStrictResolverConfig() *ResolverConfig {
	config := NewResolverConfig()

	// All tiers have stricter limits
	freeTier := NewTierConfig(TierFree, NewLimitConfig(50, time.Hour, 5))
	freeTier.SetScopeLimit(ScopeAPI, NewLimitConfig(50, time.Hour, 5))
	freeTier.SetScopeLimit(ScopeSearch, NewLimitConfig(10, time.Hour, 2))
	config.SetTierConfig(TierFree, freeTier)

	premiumTier := NewTierConfig(TierPremium, NewLimitConfig(500, time.Hour, 50))
	premiumTier.SetScopeLimit(ScopeAPI, NewLimitConfig(500, time.Hour, 50))
	premiumTier.SetScopeLimit(ScopeSearch, NewLimitConfig(100, time.Hour, 10))
	config.SetTierConfig(TierPremium, premiumTier)

	config.DefaultTierConfig = NewTierConfig(TierDefault, NewLimitConfig(100, time.Hour, 10))

	return config
}

// NewGenerousResolverConfig creates a resolver config with generous limits
func NewGenerousResolverConfig() *ResolverConfig {
	config := NewResolverConfig()

	// All tiers have more generous limits
	freeTier := NewTierConfig(TierFree, NewLimitConfig(500, time.Hour, 50))
	freeTier.SetScopeLimit(ScopeAPI, NewLimitConfig(500, time.Hour, 50))
	freeTier.SetScopeLimit(ScopeSearch, NewLimitConfig(100, time.Hour, 20))
	config.SetTierConfig(TierFree, freeTier)

	premiumTier := NewTierConfig(TierPremium, NewLimitConfig(5000, time.Hour, 500))
	premiumTier.SetScopeLimit(ScopeAPI, NewLimitConfig(5000, time.Hour, 500))
	premiumTier.SetScopeLimit(ScopeSearch, NewLimitConfig(2000, time.Hour, 200))
	config.SetTierConfig(TierPremium, premiumTier)

	enterpriseTier := NewTierConfig(TierEnterprise, NewLimitConfig(50000, time.Hour, 5000))
	config.SetTierConfig(TierEnterprise, enterpriseTier)

	config.DefaultTierConfig = NewTierConfig(TierDefault, NewLimitConfig(1000, time.Hour, 100))

	return config
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

// FormatRateString formats limit and window into a rate string
func FormatRateString(limit int64, window time.Duration) string {
	// Determine the best unit
	seconds := window.Seconds()

	if seconds < 60 {
		return fmt.Sprintf("%d/%ds", limit, int(seconds))
	} else if seconds < 3600 {
		minutes := int(seconds / 60)
		return fmt.Sprintf("%d/%dm", limit, minutes)
	} else if seconds < 86400 {
		hours := int(seconds / 3600)
		return fmt.Sprintf("%d/%dh", limit, hours)
	} else {
		days := int(seconds / 86400)
		return fmt.Sprintf("%d/%dd", limit, days)
	}
}

// ============================================================================
// VALIDATION HELPERS
// ============================================================================

// validateIdentifierString validates that a string is suitable as an identifier
// (tier name, scope name, entity ID, etc.)
func validateIdentifierString(s, fieldName string) error {
	if s == "" {
		return WrapConfigError(nil, fmt.Sprintf("%s cannot be empty", fieldName))
	}

	// Check for leading/trailing whitespace
	if s != strings.TrimSpace(s) {
		return WrapConfigError(nil, fmt.Sprintf("%s contains leading or trailing whitespace", fieldName),
			"value", s,
			"trimmed", strings.TrimSpace(s))
	}

	// Check for control characters
	for _, r := range s {
		if r < 32 || r == 127 {
			return WrapConfigError(nil, fmt.Sprintf("%s contains control characters", fieldName),
				"value", s)
		}
	}

	// Check for reasonable length (prevent DOS attacks with huge identifiers)
	if len(s) > MaxKeyLength {
		return WrapConfigError(nil, fmt.Sprintf("%s exceeds maximum length", fieldName),
			"value_length", len(s),
			"max_length", MaxKeyLength)
	}

	return nil
}
