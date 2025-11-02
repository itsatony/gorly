package routing

import (
	"fmt"
	"testing"
)

// ============================================================================
// BENCHMARK: Individual Match Types
// ============================================================================

func BenchmarkExactMatch(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 10 exact match patterns
	for i := 0; i < 10; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/endpoint%d", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchExact,
			Priority:  100,
		})
	}

	testPath := "/api/endpoint5" // Will match middle pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkPrefixMatch(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 10 prefix patterns
	for i := 0; i < 10; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/section%d", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchPrefix,
			Priority:  50,
		})
	}

	testPath := "/api/section5/users/123" // Will match middle pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkGlobMatchSingleWildcard(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 10 glob patterns with single wildcard
	for i := 0; i < 10; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/resource%d/*", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchGlob,
			Priority:  50,
		})
	}

	testPath := "/api/resource5/123" // Will match middle pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkGlobMatchDoubleWildcard(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 10 glob patterns with double wildcard
	for i := 0; i < 10; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/section%d/**", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchGlob,
			Priority:  50,
		})
	}

	testPath := "/api/section5/users/123/profile" // Will match middle pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkRegexMatch(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 10 regex patterns
	for i := 0; i < 10; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("^/api/v%d/.*$", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchRegex,
			Priority:  30,
		})
	}

	testPath := "/api/v5/users" // Will match middle pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

// ============================================================================
// BENCHMARK: Scalability Tests (Different Pattern Counts)
// ============================================================================

func benchmarkResolverScale(b *testing.B, patternCount int) {
	resolver := NewRouteResolver()

	// Mix of different pattern types
	for i := 0; i < patternCount; i++ {
		switch i % 4 {
		case 0: // Exact
			resolver.AddPattern(RoutePattern{
				Pattern:   fmt.Sprintf("/api/exact/%d", i),
				Scope:     fmt.Sprintf("exact%d", i),
				MatchType: MatchExact,
				Priority:  100,
			})
		case 1: // Prefix
			resolver.AddPattern(RoutePattern{
				Pattern:   fmt.Sprintf("/api/prefix/%d", i),
				Scope:     fmt.Sprintf("prefix%d", i),
				MatchType: MatchPrefix,
				Priority:  50,
			})
		case 2: // Glob
			resolver.AddPattern(RoutePattern{
				Pattern:   fmt.Sprintf("/api/glob/%d/*", i),
				Scope:     fmt.Sprintf("glob%d", i),
				MatchType: MatchGlob,
				Priority:  50,
			})
		case 3: // Regex
			resolver.AddPattern(RoutePattern{
				Pattern:   fmt.Sprintf("^/api/regex/%d/.*$", i),
				Scope:     fmt.Sprintf("regex%d", i),
				MatchType: MatchRegex,
				Priority:  30,
			})
		}
	}

	testPath := fmt.Sprintf("/api/glob/%d/123", patternCount/2) // Match pattern in middle

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkResolverScale10(b *testing.B) {
	benchmarkResolverScale(b, 10)
}

func BenchmarkResolverScale50(b *testing.B) {
	benchmarkResolverScale(b, 50)
}

func BenchmarkResolverScale100(b *testing.B) {
	benchmarkResolverScale(b, 100)
}

func BenchmarkResolverScale500(b *testing.B) {
	benchmarkResolverScale(b, 500)
}

// ============================================================================
// BENCHMARK: Priority Resolution Overhead
// ============================================================================

func BenchmarkPriorityResolution(b *testing.B) {
	resolver := NewRouteResolver()

	// Add overlapping patterns with different priorities
	resolver.AddPattern(RoutePattern{
		Pattern:   "/api/",
		Scope:     "api_default",
		MatchType: MatchPrefix,
		Priority:  10,
	})
	resolver.AddPattern(RoutePattern{
		Pattern:   "/api/users/*",
		Scope:     "users_glob",
		MatchType: MatchGlob,
		Priority:  50,
	})
	resolver.AddPattern(RoutePattern{
		Pattern:   "/api/users/profile",
		Scope:     "users_profile",
		MatchType: MatchExact,
		Priority:  100,
	})

	testPath := "/api/users/profile" // Matches all 3, should pick highest priority

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

// ============================================================================
// BENCHMARK: Pattern Operations
// ============================================================================

func BenchmarkAddPattern(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/api/users",
			Scope:     "users",
			MatchType: MatchExact,
			Priority:  100,
		})
	}
}

func BenchmarkRemovePattern(b *testing.B) {
	// Pre-create resolver with patterns
	resolvers := make([]RouteResolver, b.N)
	for i := 0; i < b.N; i++ {
		resolver := NewRouteResolver()
		resolver.AddPattern(RoutePattern{
			Pattern:   "/api/users",
			Scope:     "users",
			MatchType: MatchExact,
			Priority:  100,
		})
		resolvers[i] = resolver
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolvers[i].RemovePattern("/api/users")
	}
}

// ============================================================================
// BENCHMARK: Builder API
// ============================================================================

func BenchmarkBuilder(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewBuilder().
			AddExact("/api/payment", "payment", 100).
			AddPrefix("/api/", "api", 10).
			AddGlob("/users/*", "users", 50).
			AddRegex("^/v[0-9]+/.*$", "versioned", 30).
			Build()
	}
}

// ============================================================================
// BENCHMARK: Worst Case Scenarios
// ============================================================================

func BenchmarkWorstCaseNoMatch(b *testing.B) {
	resolver := NewRouteResolver()

	// Add 100 patterns
	for i := 0; i < 100; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/endpoint%d", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchExact,
			Priority:  100,
		})
	}

	testPath := "/api/nonexistent" // Will not match any pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

func BenchmarkWorstCaseComplexRegex(b *testing.B) {
	resolver := NewRouteResolver()

	// Add complex regex patterns
	patterns := []string{
		"^/api/v[0-9]+/users/[a-zA-Z0-9_-]+$",
		"^/api/v[0-9]+/posts/[0-9]+/comments$",
		"^/admin/reports/[0-9]{4}/[0-9]{2}/[0-9]{2}$",
	}

	for i, pattern := range patterns {
		resolver.AddPattern(RoutePattern{
			Pattern:   pattern,
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchRegex,
			Priority:  30,
		})
	}

	testPath := "/admin/reports/2024/01/15" // Matches last pattern

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}

// ============================================================================
// BENCHMARK: Concurrent Access
// ============================================================================

func BenchmarkConcurrentReads(b *testing.B) {
	resolver := NewRouteResolver()

	// Add patterns
	for i := 0; i < 20; i++ {
		resolver.AddPattern(RoutePattern{
			Pattern:   fmt.Sprintf("/api/endpoint%d", i),
			Scope:     fmt.Sprintf("scope%d", i),
			MatchType: MatchExact,
			Priority:  100,
		})
	}

	testPath := "/api/endpoint10"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			resolver.ResolveScope(testPath)
		}
	})
}

// ============================================================================
// BENCHMARK: Memory Allocations
// ============================================================================

func BenchmarkResolveScopeAllocs(b *testing.B) {
	resolver := NewRouteResolver()

	resolver.AddPattern(RoutePattern{
		Pattern:   "/api/users/*",
		Scope:     "users",
		MatchType: MatchGlob,
		Priority:  50,
	})

	testPath := "/api/users/123"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ResolveScope(testPath)
	}
}
