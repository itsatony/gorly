package routing

import (
	"strings"
	"testing"
	"time"
)

// ============================================================================
// EXACT MATCH TESTS
// ============================================================================

func TestExactMatch(t *testing.T) {
	resolver := NewRouteResolver()

	// Add exact match patterns
	err := resolver.AddPattern(RoutePattern{
		Pattern:   "/api/payment",
		Scope:     "payment",
		MatchType: MatchExact,
		Priority:  100,
	})
	if err != nil {
		t.Fatalf("Failed to add exact pattern: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		shouldMatch   bool
	}{
		{"exact match", "/api/payment", "payment", true},
		{"no match - prefix", "/api/payment/process", "", false},
		{"no match - different path", "/api/users", "", false},
		{"no match - trailing slash", "/api/payment/", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v", tt.shouldMatch, matched)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q", tt.expectedScope, scope)
			}
		})
	}
}

// ============================================================================
// PREFIX MATCH TESTS
// ============================================================================

func TestPrefixMatch(t *testing.T) {
	resolver := NewRouteResolver()

	// Add prefix match patterns
	err := resolver.AddPattern(RoutePattern{
		Pattern:   "/api/admin",
		Scope:     "admin",
		MatchType: MatchPrefix,
		Priority:  50,
	})
	if err != nil {
		t.Fatalf("Failed to add prefix pattern: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		shouldMatch   bool
	}{
		{"exact match", "/api/admin", "admin", true},
		{"prefix match", "/api/admin/users", "admin", true},
		{"prefix match - deep", "/api/admin/users/123", "admin", true},
		{"no match", "/api/user", "", false},
		{"no match - substring but not prefix", "/other/api/admin", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v", tt.shouldMatch, matched)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q", tt.expectedScope, scope)
			}
		})
	}
}

// ============================================================================
// GLOB MATCH TESTS
// ============================================================================

func TestGlobMatchSingleWildcard(t *testing.T) {
	resolver := NewRouteResolver()

	// Add glob patterns with single wildcard
	patterns := []RoutePattern{
		{Pattern: "/api/users/*", Scope: "users", MatchType: MatchGlob, Priority: 50},
		{Pattern: "/api/*/profile", Scope: "profile", MatchType: MatchGlob, Priority: 50},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add glob pattern %q: %v", p.Pattern, err)
		}
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		shouldMatch   bool
	}{
		{"single wildcard match", "/api/users/123", "users", true},
		{"single wildcard match - string", "/api/users/john", "users", true},
		{"wildcard in middle", "/api/customers/profile", "profile", true},
		{"no match - extra segments", "/api/users/123/edit", "", false},
		{"no match - missing segment", "/api/users", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v", tt.shouldMatch, matched)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q", tt.expectedScope, scope)
			}
		})
	}
}

func TestGlobMatchDoubleWildcard(t *testing.T) {
	resolver := NewRouteResolver()

	// Add glob patterns with double wildcard
	patterns := []RoutePattern{
		{Pattern: "/api/**", Scope: "api_all", MatchType: MatchGlob, Priority: 10},
		{Pattern: "/admin/**/edit", Scope: "admin_edit", MatchType: MatchGlob, Priority: 20},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add glob pattern %q: %v", p.Pattern, err)
		}
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		shouldMatch   bool
	}{
		{"double wildcard - one level", "/api/users", "api_all", true},
		{"double wildcard - multiple levels", "/api/v1/users/123", "api_all", true},
		{"double wildcard - empty", "/api/", "api_all", true},
		{"double wildcard with suffix", "/admin/users/edit", "admin_edit", true},
		{"double wildcard with suffix - deep", "/admin/users/123/roles/edit", "admin_edit", true},
		{"no match - wrong prefix", "/public/users", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v for path %q", tt.shouldMatch, matched, tt.path)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q for path %q", tt.expectedScope, scope, tt.path)
			}
		})
	}
}

// ============================================================================
// REGEX MATCH TESTS
// ============================================================================

func TestRegexMatch(t *testing.T) {
	resolver := NewRouteResolver()

	// Add regex patterns
	patterns := []RoutePattern{
		{Pattern: "^/api/v[0-9]+/.*$", Scope: "api_versioned", MatchType: MatchRegex, Priority: 30},
		{Pattern: "^/users/[0-9]+$", Scope: "user_by_id", MatchType: MatchRegex, Priority: 40},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add regex pattern %q: %v", p.Pattern, err)
		}
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		shouldMatch   bool
	}{
		{"regex version match v1", "/api/v1/users", "api_versioned", true},
		{"regex version match v2", "/api/v2/config", "api_versioned", true},
		{"regex version match v100", "/api/v100/data", "api_versioned", true},
		{"regex user ID", "/users/123", "user_by_id", true},
		{"regex user ID - large number", "/users/999999", "user_by_id", true},
		{"no match - letters in version", "/api/vX/users", "", false},
		{"no match - letters in user ID", "/users/abc", "", false},
		{"no match - extra path in user ID", "/users/123/profile", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if matched != tt.shouldMatch {
				t.Errorf("Expected match=%v, got %v for path %q", tt.shouldMatch, matched, tt.path)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q for path %q", tt.expectedScope, scope, tt.path)
			}
		})
	}
}

// ============================================================================
// PRIORITY RESOLUTION TESTS
// ============================================================================

func TestPriorityResolution(t *testing.T) {
	resolver := NewRouteResolver()

	// Add patterns with different priorities
	// Higher priority should win when multiple patterns match
	patterns := []RoutePattern{
		{Pattern: "/api/", Scope: "api_default", MatchType: MatchPrefix, Priority: 10},
		{Pattern: "/api/payment/*", Scope: "payment_glob", MatchType: MatchGlob, Priority: 50},
		{Pattern: "/api/payment/process", Scope: "payment_exact", MatchType: MatchExact, Priority: 100},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add pattern %q: %v", p.Pattern, err)
		}
	}

	tests := []struct {
		name          string
		path          string
		expectedScope string
		reason        string
	}{
		{
			name:          "exact wins over glob and prefix",
			path:          "/api/payment/process",
			expectedScope: "payment_exact",
			reason:        "Exact match (priority 100) should win",
		},
		{
			name:          "glob wins over prefix",
			path:          "/api/payment/cancel",
			expectedScope: "payment_glob",
			reason:        "Glob match (priority 50) should win over prefix (priority 10)",
		},
		{
			name:          "prefix as fallback",
			path:          "/api/users",
			expectedScope: "api_default",
			reason:        "Only prefix matches (priority 10)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, matched := resolver.ResolveScope(tt.path)
			if !matched {
				t.Fatalf("Expected a match but got none for path %q", tt.path)
			}
			if scope != tt.expectedScope {
				t.Errorf("Expected scope=%q, got %q. Reason: %s", tt.expectedScope, scope, tt.reason)
			}
		})
	}
}

// ============================================================================
// PATTERN VALIDATION TESTS
// ============================================================================

func TestPatternValidation(t *testing.T) {
	resolver := NewRouteResolver()

	tests := []struct {
		name        string
		pattern     RoutePattern
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid exact pattern",
			pattern: RoutePattern{
				Pattern:   "/api/users",
				Scope:     "users",
				MatchType: MatchExact,
				Priority:  100,
			},
			expectError: false,
		},
		{
			name: "empty pattern",
			pattern: RoutePattern{
				Pattern:   "",
				Scope:     "test",
				MatchType: MatchExact,
				Priority:  100,
			},
			expectError: true,
			errorMsg:    "pattern cannot be empty",
		},
		{
			name: "empty scope",
			pattern: RoutePattern{
				Pattern:   "/test",
				Scope:     "",
				MatchType: MatchExact,
				Priority:  100,
			},
			expectError: true,
			errorMsg:    "scope cannot be empty",
		},
		{
			name: "too many glob wildcards",
			pattern: RoutePattern{
				Pattern:   "/*/*/*/*/*/*/*/*/*/*/*/*/*", // 12 wildcards
				Scope:     "test",
				MatchType: MatchGlob,
				Priority:  100,
			},
			expectError: true,
			errorMsg:    "too many wildcards",
		},
		{
			name: "invalid regex syntax",
			pattern: RoutePattern{
				Pattern:   "[invalid(regex",
				Scope:     "test",
				MatchType: MatchRegex,
				Priority:  100,
			},
			expectError: true,
			errorMsg:    "invalid regex",
		},
		{
			name: "regex too long",
			pattern: RoutePattern{
				Pattern:   strings.Repeat("a", MaxRegexComplexity+1),
				Scope:     "test",
				MatchType: MatchRegex,
				Priority:  100,
			},
			expectError: true,
			errorMsg:    "regex too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := resolver.AddPattern(tt.pattern)

			if tt.expectError && err == nil {
				t.Errorf("Expected error containing %q, but got nil", tt.errorMsg)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}

			if tt.expectError && err != nil {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, but got: %v", tt.errorMsg, err)
				}
			}
		})
	}
}

// ============================================================================
// BUILDER API TESTS
// ============================================================================

func TestBuilder(t *testing.T) {
	t.Run("successful build", func(t *testing.T) {
		resolver, err := NewBuilder().
			AddExact("/api/payment", "payment", 100).
			AddPrefix("/api/", "api", 10).
			AddGlob("/users/*", "users", 50).
			AddRegex("^/v[0-9]+/.*$", "versioned", 30).
			Build()

		if err != nil {
			t.Fatalf("Expected successful build, got error: %v", err)
		}

		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}

		// Verify patterns were added
		patterns := resolver.GetPatterns()
		if len(patterns) != 4 {
			t.Errorf("Expected 4 patterns, got %d", len(patterns))
		}
	})

	t.Run("build with error", func(t *testing.T) {
		_, err := NewBuilder().
			AddExact("/api/payment", "payment", 100).
			AddRegex("[invalid", "test", 30). // Invalid regex
			Build()

		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if !strings.Contains(err.Error(), "invalid regex") {
			t.Errorf("Expected error about invalid regex, got: %v", err)
		}
	})

	t.Run("must build success", func(t *testing.T) {
		resolver := NewBuilder().
			AddExact("/api/payment", "payment", 100).
			MustBuild()

		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}
	})

	t.Run("must build panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic, but didn't panic")
			}
		}()

		NewBuilder().
			AddRegex("[invalid", "test", 30). // Invalid regex
			MustBuild()
	})

	t.Run("custom timeout", func(t *testing.T) {
		customTimeout := 200 * time.Microsecond
		resolver, err := NewBuilder().
			WithTimeout(customTimeout).
			AddExact("/test", "test", 100).
			Build()

		if err != nil {
			t.Fatalf("Expected successful build, got error: %v", err)
		}

		// Verify resolver was created (timeout is internal, can't directly test)
		if resolver == nil {
			t.Fatal("Expected non-nil resolver")
		}
	})
}

// ============================================================================
// REMOVE PATTERN TESTS
// ============================================================================

func TestRemovePattern(t *testing.T) {
	resolver := NewRouteResolver()

	// Add patterns
	patterns := []RoutePattern{
		{Pattern: "/api/users", Scope: "users", MatchType: MatchExact, Priority: 100},
		{Pattern: "/api/admin", Scope: "admin", MatchType: MatchPrefix, Priority: 50},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add pattern: %v", err)
		}
	}

	// Verify both patterns exist
	if scope, matched := resolver.ResolveScope("/api/users"); !matched || scope != "users" {
		t.Error("Pattern should exist before removal")
	}

	// Remove first pattern
	removed := resolver.RemovePattern("/api/users")
	if !removed {
		t.Error("Expected RemovePattern to return true")
	}

	// Verify pattern was removed
	if _, matched := resolver.ResolveScope("/api/users"); matched {
		t.Error("Pattern should not match after removal")
	}

	// Verify second pattern still exists
	if scope, matched := resolver.ResolveScope("/api/admin/test"); !matched || scope != "admin" {
		t.Error("Other pattern should still exist")
	}

	// Try removing non-existent pattern
	removed = resolver.RemovePattern("/nonexistent")
	if removed {
		t.Error("Expected RemovePattern to return false for non-existent pattern")
	}
}

// ============================================================================
// CLEAR PATTERNS TESTS
// ============================================================================

func TestClear(t *testing.T) {
	resolver := NewRouteResolver()

	// Add patterns
	patterns := []RoutePattern{
		{Pattern: "/api/users", Scope: "users", MatchType: MatchExact, Priority: 100},
		{Pattern: "/api/admin", Scope: "admin", MatchType: MatchPrefix, Priority: 50},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add pattern: %v", err)
		}
	}

	// Verify patterns exist
	if len(resolver.GetPatterns()) != 2 {
		t.Error("Expected 2 patterns before clear")
	}

	// Clear all patterns
	resolver.Clear()

	// Verify all patterns were removed
	if len(resolver.GetPatterns()) != 0 {
		t.Error("Expected 0 patterns after clear")
	}

	if _, matched := resolver.ResolveScope("/api/users"); matched {
		t.Error("No patterns should match after clear")
	}
}

// ============================================================================
// GET PATTERNS TESTS
// ============================================================================

func TestGetPatterns(t *testing.T) {
	resolver := NewRouteResolver()

	// Add patterns with different priorities
	patterns := []RoutePattern{
		{Pattern: "/api/", Scope: "api_default", MatchType: MatchPrefix, Priority: 10},
		{Pattern: "/api/payment", Scope: "payment", MatchType: MatchExact, Priority: 100},
		{Pattern: "/api/*", Scope: "api_glob", MatchType: MatchGlob, Priority: 50},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add pattern: %v", err)
		}
	}

	// Get patterns
	retrieved := resolver.GetPatterns()

	// Should be sorted by priority (highest first)
	if len(retrieved) != 3 {
		t.Fatalf("Expected 3 patterns, got %d", len(retrieved))
	}

	// Verify priority order
	if retrieved[0].Priority < retrieved[1].Priority || retrieved[1].Priority < retrieved[2].Priority {
		t.Error("Patterns should be sorted by priority (highest first)")
	}

	// Verify exact values
	if retrieved[0].Pattern != "/api/payment" || retrieved[0].Priority != 100 {
		t.Errorf("First pattern should be exact match with priority 100, got %v", retrieved[0])
	}

	// Verify returned slice is a copy (modifying it shouldn't affect resolver)
	retrieved[0].Scope = "modified"
	retrievedAgain := resolver.GetPatterns()
	if retrievedAgain[0].Scope == "modified" {
		t.Error("GetPatterns should return a copy, not the original slice")
	}
}

// ============================================================================
// DUPLICATE PATTERN TESTS
// ============================================================================

func TestDuplicatePattern(t *testing.T) {
	resolver := NewRouteResolver()

	pattern := RoutePattern{
		Pattern:   "/api/users",
		Scope:     "users",
		MatchType: MatchExact,
		Priority:  100,
	}

	// Add pattern first time - should succeed
	err := resolver.AddPattern(pattern)
	if err != nil {
		t.Fatalf("First add should succeed, got error: %v", err)
	}

	// Add same pattern again - should fail
	err = resolver.AddPattern(pattern)
	if err == nil {
		t.Fatal("Expected error for duplicate pattern")
	}

	if err != ErrDuplicatePattern {
		t.Errorf("Expected ErrDuplicatePattern, got: %v", err)
	}
}

// ============================================================================
// EDGE CASES AND SPECIAL SCENARIOS
// ============================================================================

func TestEdgeCases(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/",
			Scope:     "root",
			MatchType: MatchExact,
			Priority:  100,
		})

		scope, matched := resolver.ResolveScope("")
		if matched && scope == "root" {
			t.Error("Empty path should not match /")
		}
	})

	t.Run("root path", func(t *testing.T) {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/",
			Scope:     "root",
			MatchType: MatchExact,
			Priority:  100,
		})

		scope, matched := resolver.ResolveScope("/")
		if !matched || scope != "root" {
			t.Error("Root path should match exactly")
		}
	})

	t.Run("path with query parameters", func(t *testing.T) {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/api/users",
			Scope:     "users",
			MatchType: MatchExact,
			Priority:  100,
		})

		// Query parameters should not be stripped by resolver
		_, matched := resolver.ResolveScope("/api/users?id=123")
		if matched {
			t.Error("Path with query params should not match (middleware should strip them)")
		}
	})

	t.Run("case sensitivity", func(t *testing.T) {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/api/users",
			Scope:     "users",
			MatchType: MatchExact,
			Priority:  100,
		})

		// Matching is case-sensitive
		_, matched := resolver.ResolveScope("/API/USERS")
		if matched {
			t.Error("Matching should be case-sensitive")
		}
	})

	t.Run("unicode paths", func(t *testing.T) {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/api/用户",
			Scope:     "users_cn",
			MatchType: MatchExact,
			Priority:  100,
		})

		scope, matched := resolver.ResolveScope("/api/用户")
		if !matched || scope != "users_cn" {
			t.Error("Should support unicode paths")
		}
	})
}

// ============================================================================
// CONCURRENT ACCESS TESTS
// ============================================================================

func TestConcurrentAccess(t *testing.T) {
	resolver := NewRouteResolver()

	// Add initial patterns
	patterns := []RoutePattern{
		{Pattern: "/api/users", Scope: "users", MatchType: MatchExact, Priority: 100},
		{Pattern: "/api/admin", Scope: "admin", MatchType: MatchPrefix, Priority: 50},
	}

	for _, p := range patterns {
		if err := resolver.AddPattern(p); err != nil {
			t.Fatalf("Failed to add pattern: %v", err)
		}
	}

	// Test concurrent reads
	t.Run("concurrent reads", func(t *testing.T) {
		done := make(chan bool)

		for i := 0; i < 100; i++ {
			go func() {
				scope, matched := resolver.ResolveScope("/api/users")
				if !matched || scope != "users" {
					t.Error("Concurrent read failed")
				}
				done <- true
			}()
		}

		for i := 0; i < 100; i++ {
			<-done
		}
	})

	// Test concurrent writes
	t.Run("concurrent writes", func(t *testing.T) {
		done := make(chan bool)

		for i := 0; i < 10; i++ {
			go func(idx int) {
				pattern := RoutePattern{
					Pattern:   "/concurrent/" + string(rune('a'+idx)),
					Scope:     "concurrent",
					MatchType: MatchExact,
					Priority:  100,
				}
				resolver.AddPattern(pattern) // Ignore errors (duplicates expected)
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})

	// Test mixed reads and writes
	t.Run("mixed reads and writes", func(t *testing.T) {
		done := make(chan bool)

		// Readers
		for i := 0; i < 50; i++ {
			go func() {
				resolver.ResolveScope("/api/users")
				done <- true
			}()
		}

		// Writers
		for i := 0; i < 10; i++ {
			go func(idx int) {
				pattern := RoutePattern{
					Pattern:   "/mixed/" + string(rune('a'+idx)),
					Scope:     "mixed",
					MatchType: MatchExact,
					Priority:  100,
				}
				resolver.AddPattern(pattern)
				done <- true
			}(i)
		}

		for i := 0; i < 60; i++ {
			<-done
		}
	})
}
