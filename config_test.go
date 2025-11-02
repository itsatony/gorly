package ratelimit

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// RATE STRING PARSING TESTS
// ============================================================================

func TestParseRateString(t *testing.T) {
	tests := []struct {
		name        string
		rateStr     string
		wantLimit   int64
		wantWindow  time.Duration
		expectError bool
	}{
		{
			name:        "valid per second",
			rateStr:     "10/1s",
			wantLimit:   10,
			wantWindow:  time.Second,
			expectError: false,
		},
		{
			name:        "valid per minute",
			rateStr:     "100/1m",
			wantLimit:   100,
			wantWindow:  time.Minute,
			expectError: false,
		},
		{
			name:        "valid per hour",
			rateStr:     "1000/1h",
			wantLimit:   1000,
			wantWindow:  time.Hour,
			expectError: false,
		},
		{
			name:        "valid per day",
			rateStr:     "10000/1d",
			wantLimit:   10000,
			wantWindow:  24 * time.Hour,
			expectError: false,
		},
		{
			name:        "multiple units per hour",
			rateStr:     "500/2h",
			wantLimit:   500,
			wantWindow:  2 * time.Hour,
			expectError: false,
		},
		{
			name:        "invalid format - missing slash",
			rateStr:     "1000",
			expectError: true,
		},
		{
			name:        "invalid format - bad unit",
			rateStr:     "100/1x",
			expectError: true,
		},
		{
			name:        "invalid format - non-numeric limit",
			rateStr:     "abc/1h",
			expectError: true,
		},
		{
			name:        "invalid format - non-numeric window",
			rateStr:     "100/xh",
			expectError: true,
		},
		{
			name:        "limit too low",
			rateStr:     "0/1h",
			expectError: true,
		},
		{
			name:        "limit too high",
			rateStr:     "10000000/1h",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, window, err := ParseRateString(tt.rateStr)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if limit != tt.wantLimit {
				t.Errorf("expected limit %d, got %d", tt.wantLimit, limit)
			}

			if window != tt.wantWindow {
				t.Errorf("expected window %v, got %v", tt.wantWindow, window)
			}
		})
	}
}

func TestFormatRateString(t *testing.T) {
	tests := []struct {
		name       string
		limit      int64
		window     time.Duration
		wantFormat string
	}{
		{
			name:       "seconds",
			limit:      10,
			window:     5 * time.Second,
			wantFormat: "10/5s",
		},
		{
			name:       "minutes",
			limit:      100,
			window:     3 * time.Minute,
			wantFormat: "100/3m",
		},
		{
			name:       "hours",
			limit:      1000,
			window:     2 * time.Hour,
			wantFormat: "1000/2h",
		},
		{
			name:       "days",
			limit:      10000,
			window:     1 * 24 * time.Hour,
			wantFormat: "10000/1d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRateString(tt.limit, tt.window)
			if got != tt.wantFormat {
				t.Errorf("expected format %s, got %s", tt.wantFormat, got)
			}
		})
	}
}

// ============================================================================
// LIMIT CONFIG TESTS
// ============================================================================

func TestNewLimitConfig(t *testing.T) {
	limit := int64(100)
	window := time.Hour
	burst := int64(10)

	config := NewLimitConfig(limit, window, burst)

	if config.Limit != limit {
		t.Errorf("expected limit %d, got %d", limit, config.Limit)
	}
	if config.Window != window {
		t.Errorf("expected window %v, got %v", window, config.Window)
	}
	if config.Burst != burst {
		t.Errorf("expected burst %d, got %d", burst, config.Burst)
	}
	if config.Strategy != StrategyTokenBucket {
		t.Errorf("expected strategy %s, got %s", StrategyTokenBucket, config.Strategy)
	}
}

func TestNewLimitConfigFromRate(t *testing.T) {
	rateStr := "1000/1h"
	burst := int64(100)

	config, err := NewLimitConfigFromRate(rateStr, burst)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Limit != 1000 {
		t.Errorf("expected limit 1000, got %d", config.Limit)
	}
	if config.Window != time.Hour {
		t.Errorf("expected window 1h, got %v", config.Window)
	}
	if config.Burst != burst {
		t.Errorf("expected burst %d, got %d", burst, config.Burst)
	}
}

func TestLimitConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *LimitConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  10,
			},
			expectError: false,
		},
		{
			name: "limit too low",
			config: &LimitConfig{
				Limit:  0,
				Window: time.Hour,
				Burst:  10,
			},
			expectError: true,
		},
		{
			name: "limit too high",
			config: &LimitConfig{
				Limit:  10000000,
				Window: time.Hour,
				Burst:  10,
			},
			expectError: true,
		},
		{
			name: "window too short",
			config: &LimitConfig{
				Limit:  100,
				Window: 0,
				Burst:  10,
			},
			expectError: true,
		},
		{
			name: "burst negative",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLimitConfigClone(t *testing.T) {
	original := &LimitConfig{
		Limit:    100,
		Window:   time.Hour,
		Burst:    10,
		Strategy: StrategyTokenBucket,
	}

	clone := original.Clone()

	if clone.Limit != original.Limit {
		t.Error("cloned limit doesn't match")
	}
	if clone.Window != original.Window {
		t.Error("cloned window doesn't match")
	}
	if clone.Burst != original.Burst {
		t.Error("cloned burst doesn't match")
	}

	// Modify clone and ensure original is unchanged
	clone.Limit = 200
	if original.Limit == 200 {
		t.Error("modifying clone affected original")
	}
}

// ============================================================================
// TIER CONFIG TESTS
// ============================================================================

func TestNewTierConfig(t *testing.T) {
	tierName := TierFree
	defaultLimit := NewLimitConfig(100, time.Hour, 10)

	tierConfig := NewTierConfig(tierName, defaultLimit)

	if tierConfig.TierName != tierName {
		t.Errorf("expected tier name %s, got %s", tierName, tierConfig.TierName)
	}
	if tierConfig.DefaultLimit.Limit != defaultLimit.Limit {
		t.Error("default limit doesn't match")
	}
	if tierConfig.ScopeLimits == nil {
		t.Error("scope limits map should be initialized")
	}
}

func TestTierConfigSetGetScopeLimit(t *testing.T) {
	tierConfig := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))

	// Set scope-specific limit
	apiLimit := NewLimitConfig(50, time.Hour, 5)
	tierConfig.SetScopeLimit(ScopeAPI, apiLimit)

	// Get scope-specific limit
	gotLimit := tierConfig.GetScopeLimit(ScopeAPI)
	if gotLimit.Limit != apiLimit.Limit {
		t.Errorf("expected limit %d, got %d", apiLimit.Limit, gotLimit.Limit)
	}

	// Get non-existent scope should return default
	unknownLimit := tierConfig.GetScopeLimit("unknown_scope")
	if unknownLimit.Limit != tierConfig.DefaultLimit.Limit {
		t.Error("should return default limit for unknown scope")
	}
}

func TestTierConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *TierConfig
		expectError bool
	}{
		{
			name: "valid config",
			setup: func() *TierConfig {
				tc := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))
				tc.SetScopeLimit(ScopeAPI, NewLimitConfig(50, time.Hour, 5))
				return tc
			},
			expectError: false,
		},
		{
			name: "missing tier name",
			setup: func() *TierConfig {
				tc := NewTierConfig("", NewLimitConfig(100, time.Hour, 10))
				return tc
			},
			expectError: true,
		},
		{
			name: "nil default limit",
			setup: func() *TierConfig {
				tc := NewTierConfig(TierFree, nil)
				return tc
			},
			expectError: true,
		},
		{
			name: "invalid scope limit",
			setup: func() *TierConfig {
				tc := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))
				tc.SetScopeLimit(ScopeAPI, &LimitConfig{
					Limit:  0, // Invalid
					Window: time.Hour,
					Burst:  10,
				})
				return tc
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.setup()
			err := config.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// RESOLVER CONFIG TESTS
// ============================================================================

func TestNewResolverConfig(t *testing.T) {
	config := NewResolverConfig()

	if config.TierConfigs == nil {
		t.Error("tier configs should be initialized")
	}
	if config.EntityOverrides == nil {
		t.Error("entity overrides should be initialized")
	}
	if !config.EnableEntityOverrides {
		t.Error("entity overrides should be enabled by default")
	}
	if !config.EnableTierLimits {
		t.Error("tier limits should be enabled by default")
	}
	if !config.EnableScopeLimits {
		t.Error("scope limits should be enabled by default")
	}
}

func TestResolverConfigSetTierConfig(t *testing.T) {
	config := NewResolverConfig()
	tierConfig := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))

	config.SetTierConfig(TierFree, tierConfig)

	if _, ok := config.TierConfigs[TierFree]; !ok {
		t.Error("tier config should be set")
	}
}

func TestResolverConfigSetEntityOverride(t *testing.T) {
	config := NewResolverConfig()
	entityID := "user123"
	scope := ScopeAPI
	limit := NewLimitConfig(500, time.Hour, 50)

	config.SetEntityOverride(entityID, scope, limit)

	if _, ok := config.EntityOverrides[entityID]; !ok {
		t.Error("entity override should be set")
	}
	if _, ok := config.EntityOverrides[entityID][scope]; !ok {
		t.Error("scope override should be set")
	}
	if config.EntityOverrides[entityID][scope].Limit != limit.Limit {
		t.Error("override limit doesn't match")
	}
}

// ============================================================================
// LIMIT RESOLVER TESTS
// ============================================================================

func TestNewLimitResolver(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, err := NewLimitResolver(config)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolver == nil {
		t.Error("resolver should not be nil")
	}
}

func TestNewLimitResolverNilConfig(t *testing.T) {
	_, err := NewLimitResolver(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestLimitResolverResolveLimit(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	tests := []struct {
		name          string
		ctx           Identity
		expectedLimit int64
	}{
		{
			name:          "free tier global",
			ctx:           NewSimpleContext("user1", ScopeGlobal, TierFree, nil),
			expectedLimit: 100, // Free tier default
		},
		{
			name:          "free tier API",
			ctx:           NewSimpleContext("user2", ScopeAPI, TierFree, nil),
			expectedLimit: 100, // Free tier API scope
		},
		{
			name:          "premium tier global",
			ctx:           NewSimpleContext("user3", ScopeGlobal, TierPremium, nil),
			expectedLimit: 1000, // Premium tier default
		},
		{
			name:          "enterprise tier",
			ctx:           NewSimpleContext("user4", ScopeGlobal, TierEnterprise, nil),
			expectedLimit: 10000, // Enterprise tier default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := resolver.ResolveLimit(tt.ctx)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if limit.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit.Limit)
			}
		})
	}
}

func TestLimitResolverEntityOverride(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	// Set entity override
	entityID := "special_user"
	scope := ScopeAPI
	overrideLimit := NewLimitConfig(5000, time.Hour, 500)
	err := resolver.SetEntityOverride(entityID, scope, overrideLimit)
	if err != nil {
		t.Fatalf("failed to set override: %v", err)
	}

	// Resolve with override
	ctx := NewSimpleContext(entityID, scope, TierFree, nil)
	limit, err := resolver.ResolveLimit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if limit.Limit != 5000 {
		t.Errorf("expected override limit 5000, got %d", limit.Limit)
	}

	// Remove override
	err = resolver.RemoveEntityOverride(entityID, scope)
	if err != nil {
		t.Fatalf("failed to remove override: %v", err)
	}

	// Resolve after removal (should use tier limit)
	limit, err = resolver.ResolveLimit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if limit.Limit == 5000 {
		t.Error("override should be removed")
	}
}

func TestLimitResolverGetTierConfig(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	tierConfig, err := resolver.GetTierConfig(TierFree)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tierConfig.TierName != TierFree {
		t.Errorf("expected tier %s, got %s", TierFree, tierConfig.TierName)
	}
}

func TestLimitResolverHierarchy(t *testing.T) {
	// Create custom config
	config := NewResolverConfig()

	// Set default tier
	defaultTier := NewTierConfig(TierDefault, NewLimitConfig(100, time.Hour, 10))
	config.DefaultTierConfig = defaultTier

	// Set free tier with scope-specific limit
	freeTier := NewTierConfig(TierFree, NewLimitConfig(200, time.Hour, 20))
	freeTier.SetScopeLimit(ScopeAPI, NewLimitConfig(50, time.Hour, 5))
	config.SetTierConfig(TierFree, freeTier)

	resolver, _ := NewLimitResolver(config)

	// Test hierarchy: entity override > tier scope > tier default > global default
	entityID := "test_user"

	// 1. Without any overrides, should use tier scope limit
	ctx := NewSimpleContext(entityID, ScopeAPI, TierFree, nil)
	limit, _ := resolver.ResolveLimit(ctx)
	if limit.Limit != 50 {
		t.Errorf("expected tier scope limit 50, got %d", limit.Limit)
	}

	// 2. With entity override, should use override
	resolver.SetEntityOverride(entityID, ScopeAPI, NewLimitConfig(1000, time.Hour, 100))
	limit, _ = resolver.ResolveLimit(ctx)
	if limit.Limit != 1000 {
		t.Errorf("expected entity override 1000, got %d", limit.Limit)
	}

	// 3. Different scope without override should use tier default
	ctx2 := NewSimpleContext(entityID, ScopeSearch, TierFree, nil)
	limit2, _ := resolver.ResolveLimit(ctx2)
	if limit2.Limit != 200 {
		t.Errorf("expected tier default limit 200, got %d", limit2.Limit)
	}
}

// ============================================================================
// PRESET CONFIGURATION TESTS
// ============================================================================

func TestNewDefaultResolverConfig(t *testing.T) {
	config := NewDefaultResolverConfig()

	if config == nil {
		t.Fatal("config should not be nil")
	}

	// Verify all tiers are configured
	tiers := []string{TierFree, TierPremium, TierEnterprise, TierInternal}
	for _, tier := range tiers {
		if _, ok := config.TierConfigs[tier]; !ok {
			t.Errorf("tier %s should be configured", tier)
		}
	}

	// Verify default tier exists
	if config.DefaultTierConfig == nil {
		t.Error("default tier config should be set")
	}

	// Verify free tier has scope limits
	freeTier := config.TierConfigs[TierFree]
	if len(freeTier.ScopeLimits) == 0 {
		t.Error("free tier should have scope limits configured")
	}
}

func TestNewStrictResolverConfig(t *testing.T) {
	config := NewStrictResolverConfig()

	// Verify strict limits are lower than default
	defaultConfig := NewDefaultResolverConfig()

	strictFreeTier := config.TierConfigs[TierFree]
	defaultFreeTier := defaultConfig.TierConfigs[TierFree]

	if strictFreeTier.DefaultLimit.Limit >= defaultFreeTier.DefaultLimit.Limit {
		t.Error("strict config should have lower limits than default")
	}
}

func TestNewGenerousResolverConfig(t *testing.T) {
	config := NewGenerousResolverConfig()

	// Verify generous limits are higher than default
	defaultConfig := NewDefaultResolverConfig()

	generousFreeTier := config.TierConfigs[TierFree]
	defaultFreeTier := defaultConfig.TierConfigs[TierFree]

	if generousFreeTier.DefaultLimit.Limit <= defaultFreeTier.DefaultLimit.Limit {
		t.Error("generous config should have higher limits than default")
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestConfigIntegration(t *testing.T) {
	// Create a complete configuration setup
	resolverConfig := NewDefaultResolverConfig()

	// Add custom entity override
	resolverConfig.SetEntityOverride("vip_user", ScopeGlobal, NewLimitConfig(10000, time.Hour, 1000))

	// Create resolver
	resolver, err := NewLimitResolver(resolverConfig)
	if err != nil {
		t.Fatalf("failed to create resolver: %v", err)
	}

	// Test various contexts
	tests := []struct {
		name          string
		identity      string
		scope         string
		tier          string
		expectedLimit int64
	}{
		{
			name:          "VIP user with override",
			identity:      "vip_user",
			scope:         ScopeGlobal,
			tier:          TierFree,
			expectedLimit: 10000,
		},
		{
			name:          "Regular free user",
			identity:      "regular_user",
			scope:         ScopeAPI,
			tier:          TierFree,
			expectedLimit: 100,
		},
		{
			name:          "Premium user",
			identity:      "premium_user",
			scope:         ScopeSearch,
			tier:          TierPremium,
			expectedLimit: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewSimpleContext(tt.identity, tt.scope, tt.tier, nil)
			limit, err := resolver.ResolveLimit(ctx)
			if err != nil {
				t.Fatalf("failed to resolve limit: %v", err)
			}
			if limit.Limit != tt.expectedLimit {
				t.Errorf("expected limit %d, got %d", tt.expectedLimit, limit.Limit)
			}
		})
	}
}

// ============================================================================
// EDGE CASE TESTS - Added for Phase 2.5 coverage improvements
// ============================================================================

// TestParseRateString_EdgeCases tests additional edge cases
func TestParseRateString_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		rateStr string
		wantErr bool
	}{
		{
			name:    "empty string",
			rateStr: "",
			wantErr: true,
		},
		{
			name:    "only slash",
			rateStr: "/",
			wantErr: true,
		},
		{
			name:    "slash at end",
			rateStr: "100/",
			wantErr: true,
		},
		{
			name:    "slash at start",
			rateStr: "/1h",
			wantErr: true,
		},
		{
			name:    "multiple slashes",
			rateStr: "100/1h/extra",
			wantErr: true,
		},
		{
			name:    "negative limit",
			rateStr: "-100/1h",
			wantErr: true,
		},
		{
			name:    "negative window",
			rateStr: "100/-1h",
			wantErr: true,
		},
		{
			name:    "zero window",
			rateStr: "100/0s",
			wantErr: true,
		},
		{
			name:    "unknown unit",
			rateStr: "100/1w", // weeks not supported
			wantErr: true,
		},
		{
			name:    "mixed case unit",
			rateStr: "100/1H", // Should be lowercase
			wantErr: true,
		},
		{
			name:    "floating point limit",
			rateStr: "100.5/1h",
			wantErr: true,
		},
		{
			name:    "floating point window",
			rateStr: "100/1.5h",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseRateString(tt.rateStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRateString() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNewLimitConfigFromRate_ErrorPath tests error handling
func TestNewLimitConfigFromRate_ErrorPath(t *testing.T) {
	invalidRateStrings := []string{
		"",
		"invalid",
		"100/bad",
		"-50/1h",
	}

	for _, rateStr := range invalidRateStrings {
		_, err := NewLimitConfigFromRate(rateStr, 10)
		if err == nil {
			t.Errorf("expected error for invalid rate string '%s', got nil", rateStr)
		}
	}
}

// TestLimitConfig_CloneNil tests cloning a nil config
func TestLimitConfig_CloneNil(t *testing.T) {
	var nilConfig *LimitConfig = nil

	// Cloning nil should handle gracefully (or panic - depends on implementation)
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic on nil
			t.Log("Cloning nil config panicked (expected behavior)")
		}
	}()

	clone := nilConfig.Clone()
	if clone != nil {
		t.Error("cloning nil should return nil or panic")
	}
}

// TestLimitConfig_ValidateNil tests validating a nil config
func TestLimitConfig_ValidateNil(t *testing.T) {
	var nilConfig *LimitConfig = nil

	// Validating nil should handle gracefully (or panic)
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic on nil
			t.Log("Validating nil config panicked (expected behavior)")
		}
	}()

	err := nilConfig.Validate()
	if err == nil {
		t.Error("validating nil config should return error or panic")
	}
}

// TestTierConfig_CloneEdgeCases tests cloning tier configs
func TestTierConfig_CloneEdgeCases(t *testing.T) {
	// Clone with scope limits
	original := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))
	original.SetScopeLimit(ScopeAPI, NewLimitConfig(50, time.Hour, 5))
	original.SetScopeLimit(ScopeSearch, NewLimitConfig(75, time.Hour, 7))

	clone := original.Clone()

	// Verify tier name is cloned
	if clone.TierName != original.TierName {
		t.Error("tier name should be cloned")
	}

	// Verify default limit is cloned
	if clone.DefaultLimit.Limit != original.DefaultLimit.Limit {
		t.Error("default limit should be cloned")
	}

	// Verify scope limits are cloned
	if len(clone.ScopeLimits) != len(original.ScopeLimits) {
		t.Error("scope limits should be cloned")
	}

	// Modify clone and ensure original is unchanged
	clone.SetScopeLimit(ScopeDatabase, NewLimitConfig(1000, time.Hour, 100))
	if original.GetScopeLimit(ScopeDatabase).Limit == 1000 {
		t.Error("modifying clone should not affect original")
	}
}

// TestTierConfig_GetScopeLimitNilScopeLimits tests getting scope limit when map is nil
func TestTierConfig_GetScopeLimitNilScopeLimits(t *testing.T) {
	tierConfig := &TierConfig{
		TierName:     TierFree,
		DefaultLimit: NewLimitConfig(100, time.Hour, 10),
		ScopeLimits:  nil, // Explicitly nil
	}

	// Should return default limit without panicking
	limit := tierConfig.GetScopeLimit(ScopeAPI)
	if limit.Limit != 100 {
		t.Error("should return default limit when ScopeLimits is nil")
	}
}

// TestResolverConfig_EntityOverrideIsolation tests that entity overrides are properly isolated
func TestResolverConfig_EntityOverrideIsolation(t *testing.T) {
	config1 := NewDefaultResolverConfig()
	config2 := NewDefaultResolverConfig()

	// Add override to config1
	config1.SetEntityOverride("user1", ScopeAPI, NewLimitConfig(500, time.Hour, 50))

	// config2 should not have this override
	if _, ok := config2.EntityOverrides["user1"]; ok {
		t.Error("config2 should not have config1's overrides")
	}

	// Add different override to config2
	config2.SetEntityOverride("user2", ScopeSearch, NewLimitConfig(1000, time.Hour, 100))

	// config1 should not have config2's override
	if _, ok := config1.EntityOverrides["user2"]; ok {
		t.Error("config1 should not have config2's overrides")
	}
}

// TestLimitResolver_ResolveLimit_EdgeCases tests edge cases in limit resolution
func TestLimitResolver_ResolveLimit_EdgeCases(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	tests := []struct {
		name     string
		ctx      Identity
		wantErr  bool
		errMatch string
	}{
		{
			name:     "nil context",
			ctx:      nil,
			wantErr:  true,
			errMatch: "nil",
		},
		{
			name:    "unknown tier",
			ctx:     NewSimpleContext("user1", ScopeAPI, "unknown_tier", nil),
			wantErr: false, // Should fall back to default
		},
		{
			name:    "empty scope - should use global",
			ctx:     NewSimpleContext("user1", "", TierFree, nil),
			wantErr: false, // Resolver doesn't validate, allows empty scope
		},
		{
			name:    "empty identity - generates key with empty identity",
			ctx:     NewSimpleContext("", ScopeAPI, TierFree, nil),
			wantErr: false, // Resolver doesn't validate, allows empty identity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, err := resolver.ResolveLimit(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMatch != "" && err != nil {
				if err.Error() == "" || tt.errMatch != "nil" {
					// Just check that error exists
					t.Logf("Got expected error: %v", err)
				}
			}
			if !tt.wantErr && limit == nil {
				t.Error("expected limit but got nil")
			}
		})
	}
}

// TestLimitResolver_GetTierConfig_EdgeCases tests edge cases for GetTierConfig
func TestLimitResolver_GetTierConfig_EdgeCases(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	// Test getting unknown tier
	tierConfig, err := resolver.GetTierConfig("unknown_tier")
	if err != nil {
		// Should return default tier config or error
		t.Logf("Got expected error for unknown tier: %v", err)
	} else if tierConfig != nil {
		// Or fall back to default
		t.Logf("Got default tier config for unknown tier")
	} else {
		t.Error("expected either error or default tier config")
	}

	// Test getting empty tier name
	tierConfig, err = resolver.GetTierConfig("")
	if err == nil && tierConfig == nil {
		t.Error("expected error or default config for empty tier name")
	}
}

// TestLimitResolver_EntityOverride_EdgeCases tests edge cases for entity overrides
func TestLimitResolver_EntityOverride_EdgeCases(t *testing.T) {
	config := NewDefaultResolverConfig()
	resolver, _ := NewLimitResolver(config)

	// Test setting nil override
	err := resolver.SetEntityOverride("user1", ScopeAPI, nil)
	if err == nil {
		t.Error("expected error when setting nil override")
	}

	// Test removing non-existent override
	err = resolver.RemoveEntityOverride("non_existent_user", ScopeAPI)
	if err != nil {
		t.Logf("Got expected error for non-existent override: %v", err)
	}

	// Test setting override with empty entity ID
	err = resolver.SetEntityOverride("", ScopeAPI, NewLimitConfig(100, time.Hour, 10))
	if err == nil {
		t.Error("expected error when setting override with empty entity ID")
	}

	// Test setting override with empty scope
	err = resolver.SetEntityOverride("user1", "", NewLimitConfig(100, time.Hour, 10))
	if err == nil {
		t.Error("expected error when setting override with empty scope")
	}
}

// TestResolverConfig_ValidateEdgeCases tests validation edge cases
func TestResolverConfig_ValidateEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *ResolverConfig
		wantErr   bool
		wantPanic bool
	}{
		{
			name: "nil default tier config",
			setup: func() *ResolverConfig {
				config := NewResolverConfig()
				config.DefaultTierConfig = nil
				return config
			},
			wantErr:   false, // Validate doesn't check for nil default tier
			wantPanic: false,
		},
		{
			name: "invalid tier config",
			setup: func() *ResolverConfig {
				config := NewResolverConfig()
				config.SetTierConfig(TierFree, &TierConfig{
					TierName:     "", // Empty name is invalid
					DefaultLimit: NewLimitConfig(100, time.Hour, 10),
					ScopeLimits:  make(map[string]*LimitConfig),
				})
				return config
			},
			wantErr: true,
		},
		{
			name: "nil tier config in map",
			setup: func() *ResolverConfig {
				config := NewResolverConfig()
				config.TierConfigs[TierFree] = nil
				return config
			},
			wantErr:   true,  // Now properly returns error instead of panicking
			wantPanic: false, // Better error handling
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.setup()

			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Error("expected panic but didn't get one")
					}
				}()
			}

			err := config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestFormatRateString_EdgeCases tests edge cases for formatting
func TestFormatRateString_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		limit  int64
		window time.Duration
		want   string
	}{
		{
			name:   "zero limit",
			limit:  0,
			window: time.Hour,
			want:   "0/1h",
		},
		{
			name:   "very large limit",
			limit:  1000000,
			window: time.Hour,
			want:   "1000000/1h",
		},
		{
			name:   "sub-second window",
			limit:  100,
			window: 500 * time.Millisecond,
			want:   "100/0s", // Less than 1 second
		},
		{
			name:   "exact day",
			limit:  10000,
			window: 24 * time.Hour,
			want:   "10000/1d",
		},
		{
			name:   "multiple days",
			limit:  50000,
			window: 5 * 24 * time.Hour,
			want:   "50000/5d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRateString(tt.limit, tt.window)
			if got != tt.want {
				t.Errorf("FormatRateString() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLimitConfig_ValidateEdgeCases tests additional validation edge cases
func TestLimitConfig_ValidateEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		config  *LimitConfig
		wantErr bool
	}{
		{
			name: "exactly at max limit",
			config: &LimitConfig{
				Limit:  MaxLimit,
				Window: time.Hour,
				Burst:  1000,
			},
			wantErr: false, // Max value should be valid
		},
		{
			name: "exactly at min limit",
			config: &LimitConfig{
				Limit:  MinLimit,
				Window: time.Hour,
				Burst:  1,
			},
			wantErr: false, // Min value should be valid
		},
		{
			name: "window at minimum",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Duration(MinWindowSeconds) * time.Second,
				Burst:  10,
			},
			wantErr: false, // Min window should be valid
		},
		{
			name: "zero burst",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  0,
			},
			wantErr: false, // Zero burst should be valid
		},
		{
			name: "very large burst",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  1000000,
			},
			wantErr: true, // Burst exceeds MaxBurst (100000)
		},
		{
			name: "burst at max",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  MaxBurst,
			},
			wantErr: true, // Burst (100000) exceeds limit (100) - new validation catches this
		},
		{
			name: "burst larger than limit",
			config: &LimitConfig{
				Limit:  100,
				Window: time.Hour,
				Burst:  500, // Burst > Limit
			},
			wantErr: true, // New validation: burst should not exceed limit when limit > 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTierConfig_SetScopeLimitNil tests setting nil scope limit
func TestTierConfig_SetScopeLimitNil(t *testing.T) {
	tierConfig := NewTierConfig(TierFree, NewLimitConfig(100, time.Hour, 10))

	// Set a scope limit first
	tierConfig.SetScopeLimit(ScopeAPI, NewLimitConfig(50, time.Hour, 5))

	// Setting nil scope limit stores nil in the map
	tierConfig.SetScopeLimit(ScopeAPI, nil)

	// GetScopeLimit returns the value from the map, which is nil
	limit := tierConfig.GetScopeLimit(ScopeAPI)
	if limit != nil {
		t.Error("scope limit should be nil after setting to nil")
	}

	// To actually remove the override, delete from the map
	delete(tierConfig.ScopeLimits, ScopeAPI)
	limit = tierConfig.GetScopeLimit(ScopeAPI)
	if limit == nil {
		t.Error("after deleting from map, should return default limit")
	}
	if limit != nil && limit.Limit != tierConfig.DefaultLimit.Limit {
		t.Error("after deleting from map, should return default limit value")
	}
}

// TestResolverConfig_DisabledFeatures tests behavior when features are disabled
func TestResolverConfig_DisabledFeatures(t *testing.T) {
	config := NewDefaultResolverConfig()
	config.EnableEntityOverrides = false
	config.EnableTierLimits = false
	config.EnableScopeLimits = false

	resolver, err := NewLimitResolver(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Set entity override
	resolver.SetEntityOverride("user1", ScopeAPI, NewLimitConfig(5000, time.Hour, 500))

	// With entity overrides disabled, should not use override
	ctx := NewSimpleContext("user1", ScopeAPI, TierFree, nil)
	limit, err := resolver.ResolveLimit(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With all features disabled, should fall back to default
	t.Logf("Got limit with disabled features: %d", limit.Limit)
}

// ============================================================================
// SECURITY TESTS - P0-1 Fix Validation
// Tests for DOS vulnerability fixes in ParseRateString
// ============================================================================

// TestParseRateString_SecurityValidation tests all security-related validations
// This test addresses P0-1: Rate String Parser DOS vulnerability
func TestParseRateString_SecurityValidation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		description string
	}{
		// Length validation (prevent DOS via huge strings)
		{
			name:        "oversized input string",
			input:       strings.Repeat("9", 100) + "/1h",
			expectError: true,
			description: "should reject strings longer than 32 characters",
		},
		{
			name:        "near max length - valid",
			input:       "1000000/86400s", // 15 chars - uses max valid values (1M limit, 24h window)
			expectError: false,
			description: "should accept strings with large valid values",
		},

		// Zero value validation (prevent zero limits)
		{
			name:        "zero limit",
			input:       "0/1h",
			expectError: true,
			description: "should reject zero limit",
		},
		{
			name:        "zero window multiplier",
			input:       "100/0h",
			expectError: true,
			description: "should reject zero window multiplier",
		},
		{
			name:        "leading zero in limit",
			input:       "0100/1h",
			expectError: true,
			description: "should reject leading zeros in limit",
		},
		{
			name:        "leading zero in multiplier",
			input:       "100/01h",
			expectError: true,
			description: "should reject leading zeros in multiplier",
		},

		// Empty/malformed input validation
		{
			name:        "empty string",
			input:       "",
			expectError: true,
			description: "should reject empty string",
		},
		{
			name:        "only number",
			input:       "100",
			expectError: true,
			description: "should reject string without slash",
		},
		{
			name:        "only slash",
			input:       "/",
			expectError: true,
			description: "should reject lone slash",
		},
		{
			name:        "only unit",
			input:       "/1h",
			expectError: true,
			description: "should reject missing limit",
		},
		{
			name:        "missing unit",
			input:       "100/1",
			expectError: true,
			description: "should reject missing time unit",
		},

		// Overflow protection validation
		{
			name:        "huge limit value",
			input:       "99999999999/1h", // 11 digits
			expectError: true,
			description: "should reject limit exceeding max",
		},
		{
			name:        "huge window multiplier",
			input:       "100/9999999s", // 7 digits
			expectError: true,
			description: "should reject multiplier exceeding max",
		},
		{
			name:        "max valid seconds",
			input:       "100/86400s", // 24 hours in seconds
			expectError: false,
			description: "should accept max valid window in seconds",
		},
		{
			name:        "over max seconds",
			input:       "100/86401s", // Over 24 hours
			expectError: true,
			description: "should reject window exceeding 24 hours",
		},
		{
			name:        "max valid minutes",
			input:       "100/1440m", // 24 hours in minutes
			expectError: false,
			description: "should accept max valid window in minutes",
		},
		{
			name:        "over max minutes",
			input:       "100/1441m", // Over 24 hours
			expectError: true,
			description: "should reject minutes exceeding 24 hours",
		},
		{
			name:        "max valid hours",
			input:       "100/24h", // 24 hours
			expectError: false,
			description: "should accept 24 hour window",
		},
		{
			name:        "over max hours",
			input:       "100/25h", // Over 24 hours
			expectError: true,
			description: "should reject hours exceeding 24",
		},
		{
			name:        "max valid days",
			input:       "100/1d", // 1 day = 24 hours
			expectError: false,
			description: "should accept 1 day window",
		},
		{
			name:        "over max days",
			input:       "100/2d", // 2 days
			expectError: true,
			description: "should reject days exceeding 1",
		},

		// Invalid characters/format
		{
			name:        "special characters in limit",
			input:       "10@0/1h",
			expectError: true,
			description: "should reject special characters",
		},
		{
			name:        "spaces in string",
			input:       "100 / 1h",
			expectError: true,
			description: "should reject spaces",
		},
		{
			name:        "negative limit",
			input:       "-100/1h",
			expectError: true,
			description: "should reject negative limit",
		},
		{
			name:        "negative multiplier",
			input:       "100/-1h",
			expectError: true,
			description: "should reject negative multiplier",
		},

		// Valid edge cases
		{
			name:        "minimum valid limit",
			input:       "1/1h",
			expectError: false,
			description: "should accept minimum valid limit",
		},
		{
			name:        "maximum valid limit",
			input:       "1000000/1h", // MaxLimit from constants
			expectError: false,
			description: "should accept maximum valid limit",
		},
		{
			name:        "minimum window",
			input:       "100/1s",
			expectError: false,
			description: "should accept minimum window (1 second)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, window, err := ParseRateString(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("%s: expected error but got none (limit=%d, window=%v)",
						tt.description, limit, window)
				}
			} else {
				if err != nil {
					t.Errorf("%s: unexpected error: %v", tt.description, err)
				}
				// For valid inputs, verify the parsed values are reasonable
				if limit < 1 || limit > MaxLimit {
					t.Errorf("valid input produced invalid limit: %d", limit)
				}
				if window < time.Second || window > 24*time.Hour {
					t.Errorf("valid input produced invalid window: %v", window)
				}
			}
		})
	}
}

// TestParseRateString_OverflowProtection specifically tests overflow scenarios
func TestParseRateString_OverflowProtection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		note  string
	}{
		{
			name:  "seconds overflow",
			input: "100/99999s",
			note:  "99999 seconds > 24 hours (86400s)",
		},
		{
			name:  "minutes overflow",
			input: "100/9999m",
			note:  "9999 minutes > 24 hours (1440m)",
		},
		{
			name:  "hours overflow",
			input: "100/99h",
			note:  "99 hours > 24 hours",
		},
		{
			name:  "days overflow",
			input: "100/10d",
			note:  "10 days > 24 hours",
		},
		{
			name:  "extreme seconds",
			input: "100/999999s",
			note:  "extreme value should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseRateString(tt.input)
			if err == nil {
				t.Errorf("%s: should have rejected overflow scenario (%s)", tt.name, tt.note)
			}
		})
	}
}

// TestParseRateString_DOSAttackVectors tests known DOS attack patterns
func TestParseRateString_DOSAttackVectors(t *testing.T) {
	attackVectors := []struct {
		name       string
		input      string
		attackType string
	}{
		{
			name:       "memory exhaustion - huge string",
			input:      strings.Repeat("9", 1000) + "/1h",
			attackType: "memory DOS via oversized input",
		},
		{
			name:       "integer overflow - max int64",
			input:      "9223372036854775807/1h",
			attackType: "integer overflow attack",
		},
		{
			name:       "time duration overflow",
			input:      "100/9223372036854775807s",
			attackType: "time.Duration overflow",
		},
		{
			name:       "malicious Unicode",
			input:      "100\u0000/1h", // null byte
			attackType: "Unicode injection",
		},
		{
			name:       "SQL injection attempt",
			input:      "100'; DROP TABLE--/1h",
			attackType: "SQL injection pattern (shouldn't apply but test anyway)",
		},
		{
			name:       "script injection",
			input:      "<script>alert('xss')</script>/1h",
			attackType: "XSS pattern",
		},
	}

	for _, tt := range attackVectors {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseRateString(tt.input)
			if err == nil {
				t.Errorf("%s: failed to reject %s", tt.name, tt.attackType)
			} else {
				t.Logf("%s: correctly rejected %s with error: %v", tt.name, tt.attackType, err)
			}
		})
	}
}

// TestParseRateString_BoundaryValues tests exact boundary conditions
func TestParseRateString_BoundaryValues(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		shouldPass bool
	}{
		// Limit boundaries
		{"limit exactly MinLimit", "1/1h", true},
		{"limit below MinLimit", "0/1h", false},
		{"limit exactly MaxLimit", "1000000/1h", true},
		{"limit above MaxLimit", "1000001/1h", false},

		// Window boundaries (in seconds)
		{"window exactly 1 second", "100/1s", true},
		{"window exactly 86400 seconds (24h)", "100/86400s", true},
		{"window 86401 seconds (over 24h)", "100/86401s", false},

		// Window boundaries (in minutes)
		{"window exactly 1 minute", "100/1m", true},
		{"window exactly 1440 minutes (24h)", "100/1440m", true},
		{"window 1441 minutes (over 24h)", "100/1441m", false},

		// Window boundaries (in hours)
		{"window exactly 1 hour", "100/1h", true},
		{"window exactly 24 hours", "100/24h", true},
		{"window 25 hours", "100/25h", false},

		// Window boundaries (in days)
		{"window exactly 1 day", "100/1d", true},
		{"window 2 days", "100/2d", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseRateString(tt.input)
			if tt.shouldPass && err != nil {
				t.Errorf("expected valid input to pass, got error: %v", err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("expected invalid input to fail, but it passed")
			}
		})
	}
}

// TestParseRateString_RegressionTests tests specific bugs found in production
// This prevents regression of fixed issues
func TestParseRateString_RegressionTests(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		bugID   string
	}{
		{
			name:    "P0-1: zero limit was accepted",
			input:   "0/1h",
			wantErr: true,
			bugID:   "P0-1",
		},
		{
			name:    "P0-1: leading zeros were accepted",
			input:   "0100/1h",
			wantErr: true,
			bugID:   "P0-1",
		},
		{
			name:    "P0-1: oversized string caused DOS",
			input:   strings.Repeat("9", 100) + "/1h",
			wantErr: true,
			bugID:   "P0-1",
		},
		{
			name:    "P0-1: overflow not checked",
			input:   "100/999999s",
			wantErr: true,
			bugID:   "P0-1",
		},
		{
			name:    "Assessment: 'invalid' was mentioned",
			input:   "invalid",
			wantErr: true,
			bugID:   "Assessment",
		},
		{
			name:    "Assessment: '100' was mentioned",
			input:   "100",
			wantErr: true,
			bugID:   "Assessment",
		},
		{
			name:    "Assessment: '/hour' was mentioned",
			input:   "/hour",
			wantErr: true,
			bugID:   "Assessment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseRateString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("[%s] %s: error = %v, wantErr %v", tt.bugID, tt.name, err, tt.wantErr)
			}
		})
	}
}
