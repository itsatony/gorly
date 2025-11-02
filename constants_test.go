package ratelimit

import (
	"testing"
	"time"
)

// ============================================================================
// ID PREFIX TESTS
// ============================================================================

func TestIDPrefixes(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"RateLimiter prefix", IDPrefixRateLimiter, "rl"},
		{"Context prefix", IDPrefixContext, "rlc"},
		{"Result prefix", IDPrefixResult, "rlr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// TIER TESTS
// ============================================================================

func TestTierConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Free tier", TierFree, "free"},
		{"Premium tier", TierPremium, "premium"},
		{"Enterprise tier", TierEnterprise, "enterprise"},
		{"Internal tier", TierInternal, "internal"},
		{"Default tier", TierDefault, "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// SCOPE TESTS
// ============================================================================

func TestScopeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Global scope", ScopeGlobal, "global"},
		{"API scope", ScopeAPI, "api"},
		{"Search scope", ScopeSearch, "search"},
		{"Upload scope", ScopeUpload, "upload"},
		{"Download scope", ScopeDownload, "download"},
		{"Database scope", ScopeDatabase, "database"},
		{"Events scope", ScopeEvents, "events"},
		{"Admin scope", ScopeAdmin, "admin"},
		{"Metadata scope", ScopeMetadata, "metadata"},
		{"Analytics scope", ScopeAnalytics, "analytics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// STRATEGY TESTS
// ============================================================================

func TestStrategyConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Token bucket strategy", StrategyTokenBucket, "token_bucket"},
		{"Leaky bucket strategy", StrategyLeakyBucket, "leaky_bucket"},
		{"Fixed window strategy", StrategyFixedWindow, "fixed_window"},
		{"Sliding window strategy", StrategySlidingWindow, "sliding_window"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// STORE TYPE TESTS
// ============================================================================

func TestStoreTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Memory store", StoreTypeMemory, "memory"},
		{"Redis store", StoreTypeRedis, "redis"},
		{"Mock store", StoreTypeMock, "mock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// METADATA KEY TESTS
// ============================================================================

func TestMetadataKeyConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"IP metadata key", MetadataKeyIP, "ip"},
		{"User agent metadata key", MetadataKeyUserAgent, "user_agent"},
		{"User ID metadata key", MetadataKeyUserID, "user_id"},
		{"Tenant metadata key", MetadataKeyTenant, "tenant"},
		{"Method metadata key", MetadataKeyMethod, "method"},
		{"Path metadata key", MetadataKeyPath, "path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("expected %s = %q, got %q", tt.name, tt.expected, tt.constant)
			}
		})
	}
}

// ============================================================================
// LIMIT/WINDOW TESTS
// ============================================================================

func TestLimitConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    int64
		min      int64
		max      int64
		expected int64
	}{
		{"Min limit", MinLimit, 0, 10, 1},
		{"Max limit", MaxLimit, 1000000, 100000000, 10000000},
		{"Default limit", DefaultLimit, 10, 10000, 100},
		{"Min burst", MinBurst, 0, 10, 1},
		{"Max burst", MaxBurst, 1000, 100000, 10000},
		{"Default burst", DefaultBurst, 1, 1000, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value < tt.min || tt.value > tt.max {
				t.Errorf("%s value %d is out of reasonable range [%d, %d]",
					tt.name, tt.value, tt.min, tt.max)
			}
		})
	}
}

func TestWindowConstants(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		min   int64
		max   int64
	}{
		{"Min window seconds", MinWindowSeconds, 0, 10},
		{"Max window seconds", MaxWindowSeconds, 3600, 31536000}, // 1 hour to 1 year
		{"Default window seconds", DefaultWindowSeconds, 1, 7200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value < tt.min || tt.value > tt.max {
				t.Errorf("%s value %d is out of reasonable range [%d, %d]",
					tt.name, tt.value, tt.min, tt.max)
			}
		})
	}
}

func TestDefaultWindow(t *testing.T) {
	// DefaultWindow should be a reasonable duration
	if DefaultWindow < 1*time.Second {
		t.Errorf("DefaultWindow should be at least 1 second, got %v", DefaultWindow)
	}
	if DefaultWindow > 1*time.Hour {
		t.Errorf("DefaultWindow should be at most 1 hour, got %v", DefaultWindow)
	}
}

// ============================================================================
// ERROR CODE/MESSAGE TESTS
// ============================================================================

func TestErrorCodes(t *testing.T) {
	codes := []string{
		ErrCodeInvalidConfig,
		ErrCodeInvalidContext,
		ErrCodeStorageFailure,
		ErrCodeStrategyFailure,
		ErrCodeResolverFailure,
		ErrCodeLimitExceeded,
		ErrCodeInvalidLimit,
		ErrCodeInvalidWindow,
		ErrCodeInvalidBurst,
		ErrCodeClosed,
		ErrCodeKeyNotFound,
		ErrCodeConnectionFailed,
		ErrCodeTimeout,
	}

	// Ensure all error codes are non-empty and unique
	seen := make(map[string]bool)
	for _, code := range codes {
		if code == "" {
			t.Error("found empty error code")
		}
		if seen[code] {
			t.Errorf("duplicate error code: %s", code)
		}
		seen[code] = true
	}

	if len(codes) != len(seen) {
		t.Errorf("expected %d unique error codes, got %d", len(codes), len(seen))
	}
}

func TestErrorMessages(t *testing.T) {
	messages := map[string]string{
		"ErrMsgInvalidConfig":    ErrMsgInvalidConfig,
		"ErrMsgInvalidContext":   ErrMsgInvalidContext,
		"ErrMsgStorageFailure":   ErrMsgStorageFailure,
		"ErrMsgStrategyFailure":  ErrMsgStrategyFailure,
		"ErrMsgResolverFailure":  ErrMsgResolverFailure,
		"ErrMsgLimitExceeded":    ErrMsgLimitExceeded,
		"ErrMsgInvalidLimit":     ErrMsgInvalidLimit,
		"ErrMsgInvalidWindow":    ErrMsgInvalidWindow,
		"ErrMsgInvalidBurst":     ErrMsgInvalidBurst,
		"ErrMsgClosed":           ErrMsgClosed,
		"ErrMsgKeyNotFound":      ErrMsgKeyNotFound,
		"ErrMsgConnectionFailed": ErrMsgConnectionFailed,
		"ErrMsgTimeout":          ErrMsgTimeout,
	}

	for name, msg := range messages {
		if msg == "" {
			t.Errorf("%s is empty", name)
		}
		if len(msg) < 10 {
			t.Errorf("%s is too short: %q", name, msg)
		}
	}
}

// ============================================================================
// STORAGE KEY TESTS
// ============================================================================

func TestStorageKeyConstants(t *testing.T) {
	if StorageKeyPrefixDefault == "" {
		t.Error("StorageKeyPrefixDefault is empty")
	}

	if StorageKeySeparator == "" {
		t.Error("StorageKeySeparator is empty")
	}

	// Test that separator is not commonly used in identifiers
	invalidSeparators := []string{"_", "-", "."}
	for _, sep := range invalidSeparators {
		if StorageKeySeparator == sep {
			t.Logf("Warning: StorageKeySeparator '%s' might conflict with common identifier characters", sep)
		}
	}
}

// ============================================================================
// HTTP HEADER TESTS
// ============================================================================

func TestHTTPHeaderConstants(t *testing.T) {
	headers := map[string]string{
		"HeaderRateLimitLimit":      HeaderRateLimitLimit,
		"HeaderRateLimitRemaining":  HeaderRateLimitRemaining,
		"HeaderRateLimitReset":      HeaderRateLimitReset,
		"HeaderRateLimitUsed":       HeaderRateLimitUsed,
		"HeaderRateLimitRetryAfter": HeaderRateLimitRetryAfter,
	}

	for name, header := range headers {
		if header == "" {
			t.Errorf("%s is empty", name)
		}
		// HTTP headers should start with a capital letter or X-
		if len(header) < 2 {
			t.Errorf("%s is too short: %q", name, header)
		}
	}
}

// ============================================================================
// CONSISTENCY TESTS
// ============================================================================

func TestLimitBurstConsistency(t *testing.T) {
	// Min values should be non-negative
	if MinLimit < 1 {
		t.Errorf("MinLimit should be at least 1, got %d", MinLimit)
	}
	if MinBurst < 0 {
		t.Errorf("MinBurst should be at least 0, got %d", MinBurst)
	}

	// Max values should be greater than min values
	if MaxLimit <= MinLimit {
		t.Errorf("MaxLimit (%d) should be greater than MinLimit (%d)", MaxLimit, MinLimit)
	}
	if MaxBurst <= MinBurst {
		t.Errorf("MaxBurst (%d) should be greater than MinBurst (%d)", MaxBurst, MinBurst)
	}

	// Default values should be within range
	if DefaultLimit < MinLimit || DefaultLimit > MaxLimit {
		t.Errorf("DefaultLimit (%d) should be between MinLimit (%d) and MaxLimit (%d)",
			DefaultLimit, MinLimit, MaxLimit)
	}
	if DefaultBurst < MinBurst || DefaultBurst > MaxBurst {
		t.Errorf("DefaultBurst (%d) should be between MinBurst (%d) and MaxBurst (%d)",
			DefaultBurst, MinBurst, MaxBurst)
	}
}

func TestWindowConsistency(t *testing.T) {
	// Min window should be at least 1 second
	if MinWindowSeconds < 1 {
		t.Errorf("MinWindowSeconds should be at least 1, got %d", MinWindowSeconds)
	}

	// Max window should be greater than min
	if MaxWindowSeconds <= MinWindowSeconds {
		t.Errorf("MaxWindowSeconds (%d) should be greater than MinWindowSeconds (%d)",
			MaxWindowSeconds, MinWindowSeconds)
	}

	// Default window should be within range
	if DefaultWindowSeconds < MinWindowSeconds || DefaultWindowSeconds > MaxWindowSeconds {
		t.Errorf("DefaultWindowSeconds (%d) should be between MinWindowSeconds (%d) and MaxWindowSeconds (%d)",
			DefaultWindowSeconds, MinWindowSeconds, MaxWindowSeconds)
	}
}

func TestDefaultWindowConsistency(t *testing.T) {
	// DefaultWindow should be positive
	if DefaultWindow <= 0 {
		t.Errorf("DefaultWindow should be positive, got %v", DefaultWindow)
	}

	// DefaultWindowSeconds should match DefaultWindow
	expectedSeconds := int64(DefaultWindow / time.Second)
	if DefaultWindowSeconds != expectedSeconds {
		t.Errorf("DefaultWindowSeconds (%d) should match DefaultWindow in seconds (%d)",
			DefaultWindowSeconds, expectedSeconds)
	}
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkConstantAccess(b *testing.B) {
	b.Run("TierAccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = TierFree
		}
	})

	b.Run("ScopeAccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ScopeGlobal
		}
	})

	b.Run("ErrorCodeAccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ErrCodeLimitExceeded
		}
	})
}
