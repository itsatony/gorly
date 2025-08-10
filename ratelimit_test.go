// ratelimit_test.go
package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Expected rate limiting to be enabled by default")
	}

	if config.Algorithm != "token_bucket" {
		t.Errorf("Expected default algorithm to be 'token_bucket', got %s", config.Algorithm)
	}

	if config.Store != "redis" {
		t.Errorf("Expected default store to be 'redis', got %s", config.Store)
	}

	if config.KeyPrefix != "m3mo:ratelimit" {
		t.Errorf("Expected default key prefix to be 'm3mo:ratelimit', got %s", config.KeyPrefix)
	}

	// Check default limits
	if config.DefaultLimits == nil {
		t.Error("Expected default limits to be configured")
	}

	globalLimit, exists := config.DefaultLimits[ScopeGlobal]
	if !exists {
		t.Error("Expected global scope to have default limits")
	}

	if globalLimit.Requests != 1000 {
		t.Errorf("Expected global limit to be 1000 requests, got %d", globalLimit.Requests)
	}

	if globalLimit.Window != time.Hour {
		t.Errorf("Expected global window to be 1 hour, got %v", globalLimit.Window)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name:        "Valid config",
			config:      DefaultConfig(),
			expectError: false,
		},
		{
			name: "Invalid algorithm",
			config: &Config{
				Enabled:   true,
				Algorithm: "invalid_algorithm",
				Store:     "redis",
			},
			expectError: true,
		},
		{
			name: "Invalid store",
			config: &Config{
				Enabled:   true,
				Algorithm: "token_bucket",
				Store:     "invalid_store",
			},
			expectError: true,
		},
		{
			name: "Empty key prefix (should be valid)",
			config: &Config{
				Enabled:   true,
				Algorithm: "token_bucket",
				Store:     "redis",
				KeyPrefix: "",
				Redis: RedisConfig{
					Address: "localhost:6379",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError && err == nil {
				t.Error("Expected validation error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error but got: %v", err)
			}
		})
	}
}

func TestDefaultAuthEntity(t *testing.T) {
	entity := NewDefaultAuthEntity("user123", EntityTypeUser, TierPremium)

	if entity.ID() != "user123" {
		t.Errorf("Expected ID to be 'user123', got %s", entity.ID())
	}

	if entity.Type() != EntityTypeUser {
		t.Errorf("Expected type to be %s, got %s", EntityTypeUser, entity.Type())
	}

	if entity.Tier() != TierPremium {
		t.Errorf("Expected tier to be %s, got %s", TierPremium, entity.Tier())
	}

	metadata := entity.Metadata()
	if metadata == nil {
		t.Error("Expected metadata map to be initialized")
	}
}

func TestKeyBuilder(t *testing.T) {
	tests := []struct {
		name        string
		prefix      string
		entity      AuthEntity
		scope       string
		expectedKey string
	}{
		{
			name:        "With prefix",
			prefix:      "test:rl",
			entity:      NewDefaultAuthEntity("user123", EntityTypeUser, TierFree),
			scope:       ScopeGlobal,
			expectedKey: "test:rl:user:user123:global",
		},
		{
			name:        "Without prefix",
			prefix:      "",
			entity:      NewDefaultAuthEntity("api-key-456", EntityTypeAPIKey, TierEnterprise),
			scope:       ScopeMemory,
			expectedKey: "api_key:api-key-456:memory",
		},
		{
			name:        "IP entity",
			prefix:      "m3mo:rl",
			entity:      NewDefaultAuthEntity("192.168.1.1", EntityTypeIP, TierFree),
			scope:       ScopeSearch,
			expectedKey: "m3mo:rl:ip:192.168.1.1:search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kb := NewKeyBuilder(tt.prefix)
			key := kb.BuildKey(tt.entity, tt.scope)

			if key != tt.expectedKey {
				t.Errorf("Expected key %s, got %s", tt.expectedKey, key)
			}
		})
	}
}

func TestKeyBuilderStats(t *testing.T) {
	kb := NewKeyBuilder("test:rl")
	entity := NewDefaultAuthEntity("user123", EntityTypeUser, TierFree)

	statsKey := kb.BuildStatsKey(entity)
	expectedStatsKey := "test:rl:stats:user:user123"

	if statsKey != expectedStatsKey {
		t.Errorf("Expected stats key %s, got %s", expectedStatsKey, statsKey)
	}

	globalStatsKey := kb.BuildGlobalStatsKey()
	expectedGlobalStatsKey := "test:rl:stats:global"

	if globalStatsKey != expectedGlobalStatsKey {
		t.Errorf("Expected global stats key %s, got %s", expectedGlobalStatsKey, globalStatsKey)
	}
}

func TestParseRateString(t *testing.T) {
	tests := []struct {
		name           string
		rateStr        string
		expectedReqs   int64
		expectedWindow time.Duration
		expectError    bool
	}{
		{
			name:           "Valid per second",
			rateStr:        "100/1s",
			expectedReqs:   100,
			expectedWindow: time.Second,
			expectError:    false,
		},
		{
			name:           "Valid per minute",
			rateStr:        "500/5m",
			expectedReqs:   500,
			expectedWindow: 5 * time.Minute,
			expectError:    false,
		},
		{
			name:           "Valid per hour",
			rateStr:        "1000/1h",
			expectedReqs:   1000,
			expectedWindow: time.Hour,
			expectError:    false,
		},
		{
			name:           "Valid per day",
			rateStr:        "10000/1d",
			expectedReqs:   10000,
			expectedWindow: 24 * time.Hour,
			expectError:    false,
		},
		{
			name:           "Valid per week",
			rateStr:        "50000/1w",
			expectedReqs:   50000,
			expectedWindow: 168 * time.Hour,
			expectError:    false,
		},
		{
			name:        "Invalid format - no slash",
			rateStr:     "100per1m",
			expectError: true,
		},
		{
			name:        "Invalid requests number",
			rateStr:     "abc/1m",
			expectError: true,
		},
		{
			name:        "Invalid duration",
			rateStr:     "100/xyz",
			expectError: true,
		},
		{
			name:        "Empty rate string",
			rateStr:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests, window, err := ParseRateString(tt.rateStr)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if requests != tt.expectedReqs {
				t.Errorf("Expected %d requests, got %d", tt.expectedReqs, requests)
			}

			if window != tt.expectedWindow {
				t.Errorf("Expected %v window, got %v", tt.expectedWindow, window)
			}
		})
	}
}

func TestRateLimitError(t *testing.T) {
	originalErr := context.DeadlineExceeded
	rateLimitErr := NewRateLimitError(ErrorTypeTimeout, "operation timed out", originalErr)

	if rateLimitErr.Type != ErrorTypeTimeout {
		t.Errorf("Expected error type %s, got %s", ErrorTypeTimeout, rateLimitErr.Type)
	}

	if rateLimitErr.Message != "operation timed out" {
		t.Errorf("Expected message 'operation timed out', got %s", rateLimitErr.Message)
	}

	if rateLimitErr.Unwrap() != originalErr {
		t.Error("Expected wrapped error to match original error")
	}

	errorStr := rateLimitErr.Error()
	expectedStr := "operation timed out: context deadline exceeded"
	if errorStr != expectedStr {
		t.Errorf("Expected error string '%s', got '%s'", expectedStr, errorStr)
	}
}

func TestScopeConstants(t *testing.T) {
	expectedScopes := []string{
		ScopeGlobal,
		ScopeMemory,
		ScopeSearch,
		ScopeMetadata,
		ScopeAnalytics,
		ScopeAdmin,
	}

	actualScopes := []string{
		"global",
		"memory",
		"search",
		"metadata",
		"analytics",
		"admin",
	}

	for i, expected := range expectedScopes {
		if expected != actualScopes[i] {
			t.Errorf("Expected scope constant %s to equal %s", expected, actualScopes[i])
		}
	}
}

func TestEntityTypeConstants(t *testing.T) {
	expectedTypes := []string{
		EntityTypeAPIKey,
		EntityTypeUser,
		EntityTypeTenant,
		EntityTypeIP,
		EntityTypeCustom,
	}

	actualTypes := []string{
		"api_key",
		"user",
		"tenant",
		"ip",
		"custom",
	}

	for i, expected := range expectedTypes {
		if expected != actualTypes[i] {
			t.Errorf("Expected entity type constant %s to equal %s", expected, actualTypes[i])
		}
	}
}

func TestTierConstants(t *testing.T) {
	expectedTiers := []string{
		TierFree,
		TierPremium,
		TierEnterprise,
		TierCustom,
	}

	actualTiers := []string{
		"free",
		"premium",
		"enterprise",
		"custom",
	}

	for i, expected := range expectedTiers {
		if expected != actualTiers[i] {
			t.Errorf("Expected tier constant %s to equal %s", expected, actualTiers[i])
		}
	}
}

func TestConfigGetRateLimit(t *testing.T) {
	config := DefaultConfig()
	entity := NewDefaultAuthEntity("user123", EntityTypeUser, TierFree)

	// Test getting rate limit for free tier user
	rateLimit := config.GetRateLimit(entity, ScopeGlobal)

	// Should get the free tier limits
	expectedLimit := config.TierLimits[TierFree].DefaultLimits[ScopeGlobal]

	if rateLimit.Requests != expectedLimit.Requests {
		t.Errorf("Expected %d requests for free tier, got %d", expectedLimit.Requests, rateLimit.Requests)
	}

	if rateLimit.Window != expectedLimit.Window {
		t.Errorf("Expected %v window for free tier, got %v", expectedLimit.Window, rateLimit.Window)
	}
}

// Benchmark tests
func BenchmarkKeyBuilder(b *testing.B) {
	kb := NewKeyBuilder("test:rl")
	entity := NewDefaultAuthEntity("user123", EntityTypeUser, TierFree)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kb.BuildKey(entity, ScopeGlobal)
	}
}

func BenchmarkParseRateString(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseRateString("1000/1h")
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	config := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		config.Validate()
	}
}
