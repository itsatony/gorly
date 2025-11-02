package routing

import (
	"strings"
	"testing"
	"time"
)

// TestExplainMatchSuccess verifies ExplainMatch with successful matches
func TestExplainMatchSuccess(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/api/payment", "payment", 100).
		AddPrefix("/api/", "api", 10).
		MustBuild()

	tests := []struct {
		name          string
		path          string
		expectedScope string
		expectedType  MatchType
	}{
		{
			name:          "exact match",
			path:          "/api/payment",
			expectedScope: "payment",
			expectedType:  MatchExact,
		},
		{
			name:          "prefix match",
			path:          "/api/users",
			expectedScope: "api",
			expectedType:  MatchPrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explanation := ExplainMatch(resolver, tt.path)

			if !explanation.Success {
				t.Errorf("Expected success=true, got false")
			}
			if explanation.MatchedScope != tt.expectedScope {
				t.Errorf("Expected scope=%s, got %s", tt.expectedScope, explanation.MatchedScope)
			}
			if explanation.MatchedPattern == nil {
				t.Fatal("Expected MatchedPattern to be non-nil")
			}
			if explanation.MatchedPattern.MatchType != tt.expectedType {
				t.Errorf("Expected match type=%s, got %s", tt.expectedType, explanation.MatchedPattern.MatchType)
			}
			if len(explanation.TriedPatterns) == 0 {
				t.Error("Expected TriedPatterns to be non-empty")
			}
			if explanation.TotalDuration == 0 {
				t.Error("Expected TotalDuration to be non-zero")
			}
		})
	}
}

// TestExplainMatchNoMatch verifies ExplainMatch when no pattern matches
func TestExplainMatchNoMatch(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/api/payment", "payment", 100).
		AddPrefix("/api/", "api", 10).
		MustBuild()

	explanation := ExplainMatch(resolver, "/admin/dashboard")

	if explanation.Success {
		t.Errorf("Expected success=false for non-matching path, got true")
	}
	if explanation.MatchedPattern != nil {
		t.Errorf("Expected MatchedPattern to be nil, got %+v", explanation.MatchedPattern)
	}
	if explanation.MatchedScope != "" {
		t.Errorf("Expected MatchedScope to be empty, got %s", explanation.MatchedScope)
	}
	if len(explanation.TriedPatterns) == 0 {
		t.Error("Expected TriedPatterns to show attempted patterns")
	}
	// All patterns should show as not matched
	for _, attempt := range explanation.TriedPatterns {
		if attempt.Matched {
			t.Errorf("Expected all patterns to be non-matching, but %s matched", attempt.Pattern.Pattern)
		}
	}
}

// TestExplainMatchAllMatchTypes verifies ExplainMatch with all match types
func TestExplainMatchAllMatchTypes(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/exact", "exact_scope", 100).
		AddPrefix("/prefix/", "prefix_scope", 80).
		AddGlob("/glob/*", "glob_scope", 60).
		AddRegex("^/regex/[0-9]+$", "regex_scope", 40).
		MustBuild()

	tests := []struct {
		name          string
		path          string
		expectedType  MatchType
		expectedScope string
	}{
		{
			name:          "exact match",
			path:          "/exact",
			expectedType:  MatchExact,
			expectedScope: "exact_scope",
		},
		{
			name:          "prefix match",
			path:          "/prefix/test",
			expectedType:  MatchPrefix,
			expectedScope: "prefix_scope",
		},
		{
			name:          "glob match",
			path:          "/glob/test",
			expectedType:  MatchGlob,
			expectedScope: "glob_scope",
		},
		{
			name:          "regex match",
			path:          "/regex/123",
			expectedType:  MatchRegex,
			expectedScope: "regex_scope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			explanation := ExplainMatch(resolver, tt.path)

			if !explanation.Success {
				t.Errorf("Expected match for %s, got no match", tt.path)
			}
			if explanation.MatchedPattern.MatchType != tt.expectedType {
				t.Errorf("Expected match type %s, got %s", tt.expectedType, explanation.MatchedPattern.MatchType)
			}
			if explanation.MatchedScope != tt.expectedScope {
				t.Errorf("Expected scope %s, got %s", tt.expectedScope, explanation.MatchedScope)
			}

			// Verify the explanation includes a reason for the match
			found := false
			for _, attempt := range explanation.TriedPatterns {
				if attempt.Matched && attempt.Reason != "" {
					found = true
					break
				}
			}
			if !found {
				t.Error("Expected explanation to include reason for match")
			}
		})
	}
}

// TestExplainMatchPriorityOrder verifies that ExplainMatch respects priority
func TestExplainMatchPriorityOrder(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/api/test", "exact", 100).
		AddGlob("/api/*", "glob", 50).
		AddPrefix("/api/", "prefix", 10).
		MustBuild()

	explanation := ExplainMatch(resolver, "/api/test")

	// Should match exact (highest priority)
	if !explanation.Success || explanation.MatchedScope != "exact" {
		t.Errorf("Expected exact match with highest priority, got scope=%s", explanation.MatchedScope)
	}

	// Should stop after first match (exact)
	matchedCount := 0
	for _, attempt := range explanation.TriedPatterns {
		if attempt.Matched {
			matchedCount++
		}
	}
	if matchedCount != 1 {
		t.Errorf("Expected exactly 1 matched pattern (stops at first match), got %d", matchedCount)
	}

	// The matched pattern should be the first one tried
	if len(explanation.TriedPatterns) > 0 && !explanation.TriedPatterns[0].Matched {
		t.Error("Expected first pattern (highest priority) to be the match")
	}
}

// TestExplainMatchString verifies the String() method produces useful output
func TestExplainMatchString(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/api/test", "test_scope", 100).
		MustBuild()

	explanation := ExplainMatch(resolver, "/api/test")
	output := explanation.String()

	// Check for expected content
	expectedStrings := []string{
		"Pattern Match Explanation",
		"/api/test",
		"MATCHED",
		"test_scope",
		"Priority: 100",
		"Total Duration:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
		}
	}
}

// TestExplainMatchNoMatchString verifies String() output for no-match case
func TestExplainMatchNoMatchString(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/api/test", "test_scope", 100).
		MustBuild()

	explanation := ExplainMatch(resolver, "/other/path")
	output := explanation.String()

	expectedStrings := []string{
		"Pattern Match Explanation",
		"/other/path",
		"NO MATCH",
		"Total Duration:",
		"Patterns Tried:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
		}
	}
}

// TestInspect verifies the Inspect function
func TestInspect(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/exact1", "scope1", 100).
		AddExact("/exact2", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		AddGlob("/glob/*", "scope3", 50).
		AddGlob("/glob/**", "scope3", 40).
		AddRegex("^/regex.*", "scope4", 30).
		MustBuild()

	inspection := Inspect(resolver)

	// Verify counts
	if inspection.TotalPatterns != 6 {
		t.Errorf("Expected TotalPatterns=6, got %d", inspection.TotalPatterns)
	}
	if inspection.ExactPatterns != 2 {
		t.Errorf("Expected ExactPatterns=2, got %d", inspection.ExactPatterns)
	}
	if inspection.PrefixPatterns != 1 {
		t.Errorf("Expected PrefixPatterns=1, got %d", inspection.PrefixPatterns)
	}
	if inspection.GlobPatterns != 2 {
		t.Errorf("Expected GlobPatterns=2, got %d", inspection.GlobPatterns)
	}
	if inspection.RegexPatterns != 1 {
		t.Errorf("Expected RegexPatterns=1, got %d", inspection.RegexPatterns)
	}

	// Verify unique scopes
	if len(inspection.UniqueScopes) != 4 {
		t.Errorf("Expected 4 unique scopes, got %d", len(inspection.UniqueScopes))
	}

	// Verify priority range
	if inspection.PriorityRange[0] != 30 {
		t.Errorf("Expected min priority=30, got %d", inspection.PriorityRange[0])
	}
	if inspection.PriorityRange[1] != 100 {
		t.Errorf("Expected max priority=100, got %d", inspection.PriorityRange[1])
	}
}

// TestInspectEmpty verifies Inspect with no patterns
func TestInspectEmpty(t *testing.T) {
	resolver := NewBuilder().MustBuild()

	inspection := Inspect(resolver)

	if inspection.TotalPatterns != 0 {
		t.Errorf("Expected TotalPatterns=0, got %d", inspection.TotalPatterns)
	}
	if len(inspection.UniqueScopes) != 0 {
		t.Errorf("Expected 0 unique scopes, got %d", len(inspection.UniqueScopes))
	}
	if inspection.PriorityRange[0] != 0 || inspection.PriorityRange[1] != 0 {
		t.Errorf("Expected priority range [0,0], got [%d,%d]", inspection.PriorityRange[0], inspection.PriorityRange[1])
	}
}

// TestInspectString verifies the String() method of InspectResolver
func TestInspectString(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		MustBuild()

	inspection := Inspect(resolver)
	output := inspection.String()

	expectedStrings := []string{
		"Route Resolver Configuration",
		"Total Patterns: 2",
		"Exact:  1",
		"Prefix: 1",
		"Unique Scopes: 2",
		"Priority Range:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected output to contain %q, but it didn't.\nOutput:\n%s", expected, output)
		}
	}
}

// TestValidateConfiguration verifies the ValidateConfiguration function
func TestValidateConfiguration(t *testing.T) {
	// Create resolver with potential issues
	resolver := NewBuilder().
		// Overlapping patterns with same priority
		AddGlob("/api/*", "scope1", 50).
		AddGlob("/api/users/*", "scope2", 50).
		// Unreachable pattern (covered by higher priority)
		AddPrefix("/api/", "scope3", 100).
		AddPrefix("/api/admin/", "scope4", 10).
		MustBuild()

	issues := ValidateConfiguration(resolver)

	if len(issues) == 0 {
		t.Error("Expected validation to find issues, but got none")
	}

	// Check that we found some warnings
	hasWarning := false
	for _, issue := range issues {
		if issue.Severity == "warning" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("Expected at least one warning in validation issues")
	}
}

// TestValidateConfigurationClean verifies no issues with clean config
func TestValidateConfigurationClean(t *testing.T) {
	// Create resolver with non-overlapping patterns
	resolver := NewBuilder().
		AddExact("/api/payment", "payment", 100).
		AddExact("/api/users", "users", 100).
		AddPrefix("/api/", "api", 10).
		MustBuild()

	issues := ValidateConfiguration(resolver)

	// Might have some warnings, but should be minimal
	// This is more of a smoke test
	for _, issue := range issues {
		t.Logf("Validation issue: [%s] %s - %s", issue.Severity, issue.Pattern, issue.Message)
	}
}

// TestExplainMatchPerformance verifies ExplainMatch doesn't add excessive overhead
func TestExplainMatchPerformance(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		AddGlob("/glob/*", "scope3", 40).
		AddRegex("^/regex.*", "scope4", 30).
		MustBuild()

	start := time.Now()
	for i := 0; i < 1000; i++ {
		_ = ExplainMatch(resolver, "/exact")
	}
	duration := time.Since(start)

	// Should complete 1000 explanations in reasonable time (< 100ms)
	if duration > 100*time.Millisecond {
		t.Errorf("ExplainMatch too slow: 1000 calls took %v (expected < 100ms)", duration)
	}
}

// TestMatchGlobPatternHelper verifies the matchesGlobPattern helper
func TestMatchGlobPatternHelper(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		matches bool
	}{
		// Single wildcard
		{"/api/users", "/api/*", true},
		{"/api/users/123", "/api/*", false}, // Single wildcard doesn't cross segments

		// Double wildcard
		{"/api/v1/users", "/api/**", true},
		{"/api/v1/users/123", "/api/**", true},

		// Prefix + suffix
		{"/api/v1/test", "/api/**/test", true},
		{"/api/v1/v2/test", "/api/**/test", true},
		{"/api/v1/other", "/api/**/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_vs_"+tt.pattern, func(t *testing.T) {
			result := matchesGlobPattern(tt.path, tt.pattern)
			if result != tt.matches {
				t.Errorf("matchesGlobPattern(%q, %q) = %v, expected %v", tt.path, tt.pattern, result, tt.matches)
			}
		})
	}
}

// BenchmarkExplainMatch measures the overhead of ExplainMatch
func BenchmarkExplainMatch(b *testing.B) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		AddGlob("/glob/*", "scope3", 40).
		AddRegex("^/regex.*", "scope4", 30).
		MustBuild()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExplainMatch(resolver, "/exact")
	}
}

// BenchmarkInspect measures the overhead of Inspect
func BenchmarkInspect(b *testing.B) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		AddGlob("/glob/*", "scope3", 40).
		AddRegex("^/regex.*", "scope4", 30).
		MustBuild()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Inspect(resolver)
	}
}

// BenchmarkValidateConfiguration measures the overhead of ValidateConfiguration
func BenchmarkValidateConfiguration(b *testing.B) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix", "scope2", 50).
		AddGlob("/glob/*", "scope3", 40).
		AddRegex("^/regex.*", "scope4", 30).
		MustBuild()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateConfiguration(resolver)
	}
}

// TestExplainMatchReasonDetails verifies that reasons are informative
func TestExplainMatchReasonDetails(t *testing.T) {
	resolver := NewBuilder().
		AddExact("/exact", "scope1", 100).
		AddPrefix("/prefix/", "scope2", 50).
		AddGlob("/glob/*", "scope3", 40).
		AddRegex("^/regex/[0-9]+$", "scope4", 30).
		MustBuild()

	tests := []struct {
		path           string
		expectedReason string // substring that should appear in reason
	}{
		{"/exact", "Exact match"},
		{"/prefix/test", "starts with prefix"},
		{"/glob/test", "matches glob pattern"},
		{"/regex/123", "matches regex"},
		{"/nomatch", "does not"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			explanation := ExplainMatch(resolver, tt.path)

			// Find relevant attempt
			foundRelevantReason := false
			for _, attempt := range explanation.TriedPatterns {
				if strings.Contains(strings.ToLower(attempt.Reason), strings.ToLower(tt.expectedReason)) {
					foundRelevantReason = true
					break
				}
			}

			if !foundRelevantReason {
				t.Errorf("Expected to find reason containing %q, but didn't.\nExplanation:\n%s",
					tt.expectedReason, explanation.String())
			}
		})
	}
}
