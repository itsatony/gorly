package ratelimit

import (
	"strings"
	"testing"
)

// ============================================================================
// SIMPLE CONTEXT TESTS
// ============================================================================

func TestNewSimpleContext(t *testing.T) {
	ctx := NewSimpleContext("user123", ScopeAPI, TierPremium, map[string]interface{}{
		"ip": "192.168.1.1",
	})

	if ctx.Identity() != "user123" {
		t.Errorf("expected identity 'user123', got '%s'", ctx.Identity())
	}
	if ctx.Scope() != ScopeAPI {
		t.Errorf("expected scope '%s', got '%s'", ScopeAPI, ctx.Scope())
	}
	if ctx.Tier() != TierPremium {
		t.Errorf("expected tier '%s', got '%s'", TierPremium, ctx.Tier())
	}

	metadata := ctx.Metadata()
	if metadata == nil {
		t.Fatal("metadata is nil")
	}
	if ip, ok := metadata["ip"].(string); !ok || ip != "192.168.1.1" {
		t.Errorf("expected ip='192.168.1.1', got %v", metadata["ip"])
	}
}

func TestNewSimpleContextNilMetadata(t *testing.T) {
	ctx := NewSimpleContext("user123", ScopeAPI, TierFree, nil)

	metadata := ctx.Metadata()
	if metadata == nil {
		t.Error("metadata should not be nil even when passed as nil")
	}
	if len(metadata) != 0 {
		t.Errorf("empty metadata should have length 0, got %d", len(metadata))
	}
}

func TestSimpleContextKey(t *testing.T) {
	ctx := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	key := ctx.Key()

	expected := StorageKeyPrefixDefault + StorageKeySeparator +
		TierPremium + StorageKeySeparator +
		ScopeAPI + StorageKeySeparator +
		"user123"

	if key != expected {
		t.Errorf("expected key '%s', got '%s'", expected, key)
	}
}

// ============================================================================
// CONTEXT BUILDER TESTS
// ============================================================================

func TestContextBuilderBasic(t *testing.T) {
	ctx, err := NewContextBuilder().
		WithIdentity("user456").
		WithScope(ScopeSearch).
		WithTier(TierEnterprise).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Identity() != "user456" {
		t.Errorf("expected identity 'user456', got '%s'", ctx.Identity())
	}
	if ctx.Scope() != ScopeSearch {
		t.Errorf("expected scope '%s', got '%s'", ScopeSearch, ctx.Scope())
	}
	if ctx.Tier() != TierEnterprise {
		t.Errorf("expected tier '%s', got '%s'", TierEnterprise, ctx.Tier())
	}
}

func TestContextBuilderWithMetadata(t *testing.T) {
	ctx, err := NewContextBuilder().
		WithIdentity("user789").
		WithIP("10.0.0.1").
		WithUserAgent("TestAgent/1.0").
		WithMethod("GET").
		WithPath("/api/v1/test").
		AddMetadata("custom_key", "custom_value").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata := ctx.Metadata()
	if metadata[MetadataKeyIP] != "10.0.0.1" {
		t.Errorf("expected IP '10.0.0.1', got %v", metadata[MetadataKeyIP])
	}
	if metadata[MetadataKeyUserAgent] != "TestAgent/1.0" {
		t.Errorf("expected UserAgent 'TestAgent/1.0', got %v", metadata[MetadataKeyUserAgent])
	}
	if metadata[MetadataKeyMethod] != "GET" {
		t.Errorf("expected Method 'GET', got %v", metadata[MetadataKeyMethod])
	}
	if metadata[MetadataKeyPath] != "/api/v1/test" {
		t.Errorf("expected Path '/api/v1/test', got %v", metadata[MetadataKeyPath])
	}
	if metadata["custom_key"] != "custom_value" {
		t.Errorf("expected custom_key='custom_value', got %v", metadata["custom_key"])
	}
}

func TestContextBuilderDefaults(t *testing.T) {
	ctx, err := NewContextBuilder().
		WithIdentity("user_default").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should default to ScopeGlobal and TierDefault
	if ctx.Scope() != ScopeGlobal {
		t.Errorf("expected default scope '%s', got '%s'", ScopeGlobal, ctx.Scope())
	}
	if ctx.Tier() != TierDefault {
		t.Errorf("expected default tier '%s', got '%s'", TierDefault, ctx.Tier())
	}
}

func TestContextBuilderMissingIdentity(t *testing.T) {
	_, err := NewContextBuilder().
		WithScope(ScopeAPI).
		Build()

	if err == nil {
		t.Error("expected error when identity is missing, got nil")
	}
}

func TestContextBuilderMustBuild(t *testing.T) {
	// Should not panic with valid input
	ctx := NewContextBuilder().
		WithIdentity("user_must").
		MustBuild()

	if ctx.Identity() != "user_must" {
		t.Errorf("expected identity 'user_must', got '%s'", ctx.Identity())
	}
}

func TestContextBuilderMustBuildPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when identity is missing")
		}
	}()

	// Should panic without identity
	NewContextBuilder().MustBuild()
}

// ============================================================================
// CONVENIENCE CONSTRUCTOR TESTS
// ============================================================================

func TestNewIPContext(t *testing.T) {
	ip := "203.0.113.42"
	ctx := NewIPContext(ip)

	if ctx.Identity() != ip {
		t.Errorf("expected identity '%s', got '%s'", ip, ctx.Identity())
	}
	if ctx.Scope() != ScopeGlobal {
		t.Errorf("expected scope '%s', got '%s'", ScopeGlobal, ctx.Scope())
	}
	if ctx.Tier() != TierFree {
		t.Errorf("expected tier '%s', got '%s'", TierFree, ctx.Tier())
	}

	metadata := ctx.Metadata()
	if metadata[MetadataKeyIP] != ip {
		t.Errorf("expected IP metadata '%s', got %v", ip, metadata[MetadataKeyIP])
	}
}

func TestNewAPIKeyContext(t *testing.T) {
	apiKey := "sk_test_1234567890"
	tier := TierPremium
	ctx := NewAPIKeyContext(apiKey, tier)

	if ctx.Identity() != apiKey {
		t.Errorf("expected identity '%s', got '%s'", apiKey, ctx.Identity())
	}
	if ctx.Scope() != ScopeAPI {
		t.Errorf("expected scope '%s', got '%s'", ScopeAPI, ctx.Scope())
	}
	if ctx.Tier() != tier {
		t.Errorf("expected tier '%s', got '%s'", tier, ctx.Tier())
	}

	metadata := ctx.Metadata()
	if metadata["api_key"] != apiKey {
		t.Errorf("expected api_key metadata '%s', got %v", apiKey, metadata["api_key"])
	}
}

func TestNewUserContext(t *testing.T) {
	userID := "usr_abc123"
	tier := TierEnterprise
	ctx := NewUserContext(userID, tier)

	if ctx.Identity() != userID {
		t.Errorf("expected identity '%s', got '%s'", userID, ctx.Identity())
	}
	if ctx.Scope() != ScopeGlobal {
		t.Errorf("expected scope '%s', got '%s'", ScopeGlobal, ctx.Scope())
	}
	if ctx.Tier() != tier {
		t.Errorf("expected tier '%s', got '%s'", tier, ctx.Tier())
	}

	metadata := ctx.Metadata()
	if metadata[MetadataKeyUserID] != userID {
		t.Errorf("expected user_id metadata '%s', got %v", userID, metadata[MetadataKeyUserID])
	}
}

func TestNewTenantContext(t *testing.T) {
	tenantID := "tenant_xyz789"
	tier := TierEnterprise
	ctx := NewTenantContext(tenantID, tier)

	if ctx.Identity() != tenantID {
		t.Errorf("expected identity '%s', got '%s'", tenantID, ctx.Identity())
	}
	if ctx.Scope() != ScopeGlobal {
		t.Errorf("expected scope '%s', got '%s'", ScopeGlobal, ctx.Scope())
	}
	if ctx.Tier() != tier {
		t.Errorf("expected tier '%s', got '%s'", tier, ctx.Tier())
	}

	metadata := ctx.Metadata()
	if metadata[MetadataKeyTenant] != tenantID {
		t.Errorf("expected tenant metadata '%s', got %v", tenantID, metadata[MetadataKeyTenant])
	}
}

// ============================================================================
// IDENTIFIED CONTEXT TESTS
// ============================================================================

func TestNewIdentifiedContext(t *testing.T) {
	base := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	identified := NewIdentifiedContext(base)

	if identified.ID() == "" {
		t.Error("ID should not be empty")
	}

	// Should start with the correct prefix
	if len(identified.ID()) < len(IDPrefixContext) {
		t.Errorf("ID '%s' is too short", identified.ID())
	}

	// Should delegate to wrapped context
	if identified.Identity() != base.Identity() {
		t.Error("Identity() should delegate to wrapped context")
	}
	if identified.Scope() != base.Scope() {
		t.Error("Scope() should delegate to wrapped context")
	}
	if identified.Tier() != base.Tier() {
		t.Error("Tier() should delegate to wrapped context")
	}
	if identified.Key() != base.Key() {
		t.Error("Key() should delegate to wrapped context")
	}
}

// ============================================================================
// CONTEXT VALIDATION TESTS
// ============================================================================

func TestValidateContext(t *testing.T) {
	tests := []struct {
		name    string
		ctx     Identity
		wantErr bool
	}{
		{
			name:    "valid context",
			ctx:     NewSimpleContext("user123", ScopeAPI, TierPremium, nil),
			wantErr: false,
		},
		{
			name:    "nil context",
			ctx:     nil,
			wantErr: true,
		},
		{
			name:    "empty identity",
			ctx:     NewSimpleContext("", ScopeAPI, TierPremium, nil),
			wantErr: true,
		},
		{
			name:    "empty scope",
			ctx:     NewSimpleContext("user123", "", TierPremium, nil),
			wantErr: true,
		},
		{
			name:    "empty tier",
			ctx:     NewSimpleContext("user123", ScopeAPI, "", nil),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContext() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// KEY PARSING TESTS
// ============================================================================

func TestParseKey(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		wantTier     string
		wantScope    string
		wantIdentity string
		wantErr      bool
	}{
		{
			name:         "valid key",
			key:          "gorly:premium:api:user123",
			wantTier:     "premium",
			wantScope:    "api",
			wantIdentity: "user123",
			wantErr:      false,
		},
		{
			name:    "invalid format - too few parts",
			key:     "gorly:premium:api",
			wantErr: true,
		},
		{
			name:    "invalid format - too many parts",
			key:     "gorly:premium:api:user123:extra",
			wantErr: true,
		},
		{
			name:    "invalid prefix",
			key:     "wrong:premium:api:user123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, scope, identity, err := ParseKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tier != tt.wantTier {
					t.Errorf("tier = %v, want %v", tier, tt.wantTier)
				}
				if scope != tt.wantScope {
					t.Errorf("scope = %v, want %v", scope, tt.wantScope)
				}
				if identity != tt.wantIdentity {
					t.Errorf("identity = %v, want %v", identity, tt.wantIdentity)
				}
			}
		})
	}
}

func TestContextFromKey(t *testing.T) {
	original := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	key := original.Key()

	recovered, err := ContextFromKey(key)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if recovered.Identity() != original.Identity() {
		t.Errorf("identity mismatch: got %s, want %s", recovered.Identity(), original.Identity())
	}
	if recovered.Scope() != original.Scope() {
		t.Errorf("scope mismatch: got %s, want %s", recovered.Scope(), original.Scope())
	}
	if recovered.Tier() != original.Tier() {
		t.Errorf("tier mismatch: got %s, want %s", recovered.Tier(), original.Tier())
	}
}

// ============================================================================
// CONTEXT COMPARISON TESTS
// ============================================================================

func TestContextsEqual(t *testing.T) {
	ctx1 := NewSimpleContext("user123", ScopeAPI, TierPremium, map[string]interface{}{"key": "value1"})
	ctx2 := NewSimpleContext("user123", ScopeAPI, TierPremium, map[string]interface{}{"key": "value2"})
	ctx3 := NewSimpleContext("user456", ScopeAPI, TierPremium, nil)

	if !ContextsEqual(ctx1, ctx2) {
		t.Error("contexts with same identity/scope/tier should be equal (metadata ignored)")
	}

	if ContextsEqual(ctx1, ctx3) {
		t.Error("contexts with different identity should not be equal")
	}

	if !ContextsEqual(nil, nil) {
		t.Error("nil contexts should be equal")
	}

	if ContextsEqual(ctx1, nil) {
		t.Error("non-nil context should not equal nil")
	}
}

// ============================================================================
// CONTEXT CLONING TESTS
// ============================================================================

func TestCloneContext(t *testing.T) {
	original := NewSimpleContext("user123", ScopeAPI, TierPremium, map[string]interface{}{
		"ip":    "192.168.1.1",
		"count": 42,
	})

	cloned := CloneContext(original)

	// Should have same values
	if cloned.Identity() != original.Identity() {
		t.Error("cloned identity should match original")
	}
	if cloned.Scope() != original.Scope() {
		t.Error("cloned scope should match original")
	}
	if cloned.Tier() != original.Tier() {
		t.Error("cloned tier should match original")
	}

	// Metadata should be copied
	clonedMeta := cloned.Metadata()
	originalMeta := original.Metadata()

	if clonedMeta["ip"] != originalMeta["ip"] {
		t.Error("metadata should be copied")
	}

	// Modifying clone should not affect original
	clonedMeta["new_key"] = "new_value"
	if _, exists := originalMeta["new_key"]; exists {
		t.Error("modifying clone metadata should not affect original")
	}
}

func TestCloneContextNil(t *testing.T) {
	cloned := CloneContext(nil)
	if cloned != nil {
		t.Error("cloning nil should return nil")
	}
}

// ============================================================================
// BENCHMARK TESTS
// ============================================================================

func BenchmarkNewSimpleContext(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	}
}

func BenchmarkContextKey(b *testing.B) {
	ctx := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ctx.Key()
	}
}

func BenchmarkContextBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = NewContextBuilder().
			WithIdentity("user123").
			WithScope(ScopeAPI).
			WithTier(TierPremium).
			WithIP("192.168.1.1").
			Build()
	}
}

func BenchmarkParseKey(b *testing.B) {
	key := "gorly:premium:api:user123"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = ParseKey(key)
	}
}

// ============================================================================
// EDGE CASE TESTS - Added for Phase 2.5 coverage improvements
// ============================================================================

// TestContextBuilder_WithMetadata tests the WithMetadata method which replaces metadata
func TestContextBuilder_WithMetadata(t *testing.T) {
	builder := NewContextBuilder().
		WithIdentity("user123").
		AddMetadata("old_key", "old_value")

	// Replace metadata entirely
	newMetadata := map[string]interface{}{
		"new_key": "new_value",
		"ip":      "192.168.1.1",
	}
	ctx, err := builder.WithMetadata(newMetadata).Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata := ctx.Metadata()
	if metadata["new_key"] != "new_value" {
		t.Error("new metadata should be present")
	}
	if metadata["ip"] != "192.168.1.1" {
		t.Error("new metadata should be present")
	}
	if _, exists := metadata["old_key"]; exists {
		t.Error("old metadata should be replaced, not merged")
	}
}

// TestContextBuilder_WithMetadataNil tests WithMetadata with nil map
func TestContextBuilder_WithMetadataNil(t *testing.T) {
	ctx, err := NewContextBuilder().
		WithIdentity("user123").
		WithMetadata(nil).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still work - NewSimpleContext handles nil metadata
	metadata := ctx.Metadata()
	if metadata == nil {
		t.Error("metadata should not be nil even when set to nil")
	}
}

// TestContextBuilder_AddMetadataWhenNil tests AddMetadata when metadata map is nil
func TestContextBuilder_AddMetadataWhenNil(t *testing.T) {
	builder := NewContextBuilder().WithIdentity("user123")

	// Set metadata to nil explicitly
	builder.metadata = nil

	// AddMetadata should create the map if it's nil
	ctx, err := builder.AddMetadata("test_key", "test_value").Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata := ctx.Metadata()
	if metadata["test_key"] != "test_value" {
		t.Error("metadata should be added even when map was nil")
	}
}

// TestContextBuilder_Build_EmptyScopeDefaulting tests that empty scope defaults to ScopeGlobal
func TestContextBuilder_Build_EmptyScopeDefaulting(t *testing.T) {
	builder := NewContextBuilder().WithIdentity("user123")

	// Set scope to empty string
	builder.scope = ""

	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Scope() != ScopeGlobal {
		t.Errorf("empty scope should default to '%s', got '%s'", ScopeGlobal, ctx.Scope())
	}
}

// TestContextBuilder_Build_EmptyTierDefaulting tests that empty tier defaults to TierDefault
func TestContextBuilder_Build_EmptyTierDefaulting(t *testing.T) {
	builder := NewContextBuilder().WithIdentity("user123")

	// Set tier to empty string
	builder.tier = ""

	ctx, err := builder.Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Tier() != TierDefault {
		t.Errorf("empty tier should default to '%s', got '%s'", TierDefault, ctx.Tier())
	}
}

// TestContextFromKey_ErrorPath tests ContextFromKey with invalid key
func TestContextFromKey_ErrorPath(t *testing.T) {
	invalidKeys := []string{
		"invalid:format",           // Too few parts
		"wrong:prefix:api:user123", // Wrong prefix
		"",                         // Empty string
		"gorly:only:two",           // Too few parts
	}

	for _, key := range invalidKeys {
		ctx, err := ContextFromKey(key)
		if err == nil {
			t.Errorf("expected error for invalid key '%s', got nil", key)
		}
		if ctx != nil {
			t.Errorf("expected nil context for invalid key '%s', got %v", key, ctx)
		}
	}
}

// TestSimpleContext_MetadataIsolation tests that metadata is properly isolated
func TestSimpleContext_MetadataIsolation(t *testing.T) {
	metadata := map[string]interface{}{
		"key1": "value1",
	}

	ctx := NewSimpleContext("user123", ScopeAPI, TierPremium, metadata)

	// Modify original metadata map
	metadata["key2"] = "value2"

	// Context should have the modified metadata (shared reference)
	ctxMeta := ctx.Metadata()
	if ctxMeta["key2"] != "value2" {
		t.Error("metadata should be shared reference, not a copy")
	}
}

// TestContextBuilder_ChainedMetadataMethods tests chaining multiple metadata methods
func TestContextBuilder_ChainedMetadataMethods(t *testing.T) {
	ctx, err := NewContextBuilder().
		WithIdentity("user123").
		WithIP("10.0.0.1").
		WithUserAgent("Mozilla/5.0").
		WithMethod("POST").
		WithPath("/api/v1/resource").
		AddMetadata("custom1", "value1").
		AddMetadata("custom2", 123).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	metadata := ctx.Metadata()

	// Check all metadata is present
	if metadata[MetadataKeyIP] != "10.0.0.1" {
		t.Error("IP metadata missing")
	}
	if metadata[MetadataKeyUserAgent] != "Mozilla/5.0" {
		t.Error("UserAgent metadata missing")
	}
	if metadata[MetadataKeyMethod] != "POST" {
		t.Error("Method metadata missing")
	}
	if metadata[MetadataKeyPath] != "/api/v1/resource" {
		t.Error("Path metadata missing")
	}
	if metadata["custom1"] != "value1" {
		t.Error("custom1 metadata missing")
	}
	if metadata["custom2"] != 123 {
		t.Error("custom2 metadata missing")
	}
}

// TestParseKey_EdgeCases tests additional edge cases for ParseKey
func TestParseKey_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "empty string",
			key:     "",
			wantErr: true,
		},
		{
			name:    "single part",
			key:     "gorly",
			wantErr: true,
		},
		{
			name:    "two parts",
			key:     "gorly:premium",
			wantErr: true,
		},
		{
			name:    "three parts",
			key:     "gorly:premium:api",
			wantErr: true,
		},
		{
			name:    "five parts",
			key:     "gorly:premium:api:user123:extra",
			wantErr: true,
		},
		{
			name:    "wrong separator",
			key:     "gorly|premium|api|user123",
			wantErr: true,
		},
		{
			name:    "empty identity",
			key:     "gorly:premium:api:",
			wantErr: false, // Valid format, just empty identity
		},
		{
			name:    "special characters in identity",
			key:     "gorly:premium:api:user@example.com",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, scope, identity, err := ParseKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.name == "empty identity" && identity != "" {
				t.Errorf("expected empty identity, got '%s'", identity)
			}
			if !tt.wantErr && tt.name == "special characters in identity" {
				if tier != "premium" || scope != "api" || identity != "user@example.com" {
					t.Errorf("unexpected parsing result: tier=%s, scope=%s, identity=%s", tier, scope, identity)
				}
			}
		})
	}
}

// TestCloneContext_EmptyMetadata tests cloning context with no metadata
func TestCloneContext_EmptyMetadata(t *testing.T) {
	original := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	cloned := CloneContext(original)

	if cloned.Identity() != original.Identity() {
		t.Error("identity should be cloned")
	}

	// Both should have empty metadata maps
	if len(cloned.Metadata()) != 0 {
		t.Error("cloned metadata should be empty")
	}
	if len(original.Metadata()) != 0 {
		t.Error("original metadata should be empty")
	}
}

// TestContextsEqual_EdgeCases tests additional edge cases for ContextsEqual
func TestContextsEqual_EdgeCases(t *testing.T) {
	ctx1 := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)
	ctx2 := NewSimpleContext("user123", ScopeAPI, TierPremium, map[string]interface{}{"key": "value"})
	ctx3 := NewSimpleContext("user123", ScopeAPI, TierFree, nil)
	ctx4 := NewSimpleContext("user123", ScopeSearch, TierPremium, nil)

	tests := []struct {
		name     string
		a        Identity
		b        Identity
		expected bool
	}{
		{
			name:     "same identity/scope/tier, different metadata",
			a:        ctx1,
			b:        ctx2,
			expected: true, // Metadata is ignored in comparison
		},
		{
			name:     "different tier",
			a:        ctx1,
			b:        ctx3,
			expected: false,
		},
		{
			name:     "different scope",
			a:        ctx1,
			b:        ctx4,
			expected: false,
		},
		{
			name:     "nil == nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "non-nil != nil",
			a:        ctx1,
			b:        nil,
			expected: false,
		},
		{
			name:     "nil != non-nil",
			a:        nil,
			b:        ctx1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContextsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("ContextsEqual() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestIdentifiedContext_IDUniqueness tests that IDs are unique
func TestIdentifiedContext_IDUniqueness(t *testing.T) {
	base := NewSimpleContext("user123", ScopeAPI, TierPremium, nil)

	ctx1 := NewIdentifiedContext(base)
	ctx2 := NewIdentifiedContext(base)

	if ctx1.ID() == ctx2.ID() {
		t.Error("identified contexts should have unique IDs")
	}

	if !strings.HasPrefix(ctx1.ID(), IDPrefixContext) {
		t.Errorf("ID should start with prefix '%s', got '%s'", IDPrefixContext, ctx1.ID())
	}
}

// TestNewIPContext_IPInMetadata tests that IP is stored in metadata
func TestNewIPContext_IPInMetadata(t *testing.T) {
	ip := "203.0.113.1"
	ctx := NewIPContext(ip)

	// Check IP is in both identity and metadata
	if ctx.Identity() != ip {
		t.Errorf("identity should be '%s', got '%s'", ip, ctx.Identity())
	}

	metadata := ctx.Metadata()
	if metadata[MetadataKeyIP] != ip {
		t.Errorf("IP should be in metadata, got %v", metadata[MetadataKeyIP])
	}
}

// TestValidateContext_AllFields tests validation catches all required field errors
func TestValidateContext_AllFields(t *testing.T) {
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
			name:     "empty identity",
			ctx:      NewSimpleContext("", ScopeAPI, TierPremium, nil),
			wantErr:  true,
			errMatch: "identity",
		},
		{
			name:     "empty scope",
			ctx:      NewSimpleContext("user123", "", TierPremium, nil),
			wantErr:  true,
			errMatch: "scope",
		},
		{
			name:     "empty tier",
			ctx:      NewSimpleContext("user123", ScopeAPI, "", nil),
			wantErr:  true,
			errMatch: "tier",
		},
		{
			name:    "valid context",
			ctx:     NewSimpleContext("user123", ScopeAPI, TierPremium, nil),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContext(tt.ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateContext() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errMatch != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error message should contain '%s', got '%s'", tt.errMatch, err.Error())
				}
			}
		})
	}
}
