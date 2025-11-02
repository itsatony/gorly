// Package routing provides debugging and inspection tools for pattern-based routing.
package routing

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// MatchExplanation provides detailed information about why a pattern matched or didn't match.
type MatchExplanation struct {
	Path           string
	MatchedPattern *RoutePattern
	MatchedScope   string
	TriedPatterns  []PatternAttempt
	TotalDuration  time.Duration
	Success        bool
}

// PatternAttempt records an attempt to match a pattern.
type PatternAttempt struct {
	Pattern  RoutePattern
	Matched  bool
	Duration time.Duration
	Reason   string // Why it matched or didn't match
}

// String returns a human-readable explanation of the match attempt.
func (e *MatchExplanation) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== Pattern Match Explanation for: %s ===\n", e.Path))
	sb.WriteString(fmt.Sprintf("Result: %v\n", map[bool]string{true: "MATCHED", false: "NO MATCH"}[e.Success]))

	if e.Success {
		sb.WriteString(fmt.Sprintf("Matched Pattern: %s (%s)\n", e.MatchedPattern.Pattern, e.MatchedPattern.MatchType))
		sb.WriteString(fmt.Sprintf("Resolved Scope: %s\n", e.MatchedScope))
		sb.WriteString(fmt.Sprintf("Priority: %d\n", e.MatchedPattern.Priority))
	}

	sb.WriteString(fmt.Sprintf("Total Duration: %v\n", e.TotalDuration))
	sb.WriteString(fmt.Sprintf("\nPatterns Tried: %d\n", len(e.TriedPatterns)))

	for i, attempt := range e.TriedPatterns {
		status := "❌ NO MATCH"
		if attempt.Matched {
			status = "✅ MATCHED"
		}

		sb.WriteString(fmt.Sprintf("\n%d. %s [%s] (priority=%d)\n",
			i+1, attempt.Pattern.Pattern, attempt.Pattern.MatchType, attempt.Pattern.Priority))
		sb.WriteString(fmt.Sprintf("   Status: %s\n", status))
		sb.WriteString(fmt.Sprintf("   Duration: %v\n", attempt.Duration))
		sb.WriteString(fmt.Sprintf("   Reason: %s\n", attempt.Reason))

		if attempt.Matched {
			sb.WriteString(fmt.Sprintf("   → Resolved to scope: %s\n", attempt.Pattern.Scope))
			break // Stop after first match
		}
	}

	return sb.String()
}

// ExplainMatch explains why a path matched (or didn't match) any pattern.
// This is useful for debugging routing configuration issues.
func ExplainMatch(resolver RouteResolver, path string) *MatchExplanation {
	start := time.Now()

	explanation := &MatchExplanation{
		Path:          path,
		TriedPatterns: make([]PatternAttempt, 0),
		Success:       false,
	}

	// Get patterns from resolver
	patterns := resolver.GetPatterns()

	// Try each pattern in priority order
	for _, pattern := range patterns {
		attemptStart := time.Now()

		matched := false
		reason := ""

		switch pattern.MatchType {
		case MatchExact:
			matched = (path == pattern.Pattern)
			if matched {
				reason = fmt.Sprintf("Exact match: '%s' == '%s'", path, pattern.Pattern)
			} else {
				reason = fmt.Sprintf("Not an exact match: '%s' != '%s'", path, pattern.Pattern)
			}

		case MatchPrefix:
			matched = strings.HasPrefix(path, pattern.Pattern)
			if matched {
				reason = fmt.Sprintf("Path starts with prefix '%s'", pattern.Pattern)
			} else {
				reason = fmt.Sprintf("Path does not start with prefix '%s'", pattern.Pattern)
			}

		case MatchGlob:
			matched = matchesGlobPattern(path, pattern.Pattern)
			if matched {
				reason = fmt.Sprintf("Path matches glob pattern '%s'", pattern.Pattern)
			} else {
				reason = fmt.Sprintf("Path does not match glob pattern '%s'", pattern.Pattern)
			}

		case MatchRegex:
			if pattern.compiled != nil {
				matched = pattern.compiled.MatchString(path)
				if matched {
					reason = fmt.Sprintf("Path matches regex '%s'", pattern.Pattern)
				} else {
					reason = fmt.Sprintf("Path does not match regex '%s'", pattern.Pattern)
				}
			} else {
				reason = "Regex pattern not compiled (error during configuration)"
			}
		}

		attempt := PatternAttempt{
			Pattern:  pattern,
			Matched:  matched,
			Duration: time.Since(attemptStart),
			Reason:   reason,
		}

		explanation.TriedPatterns = append(explanation.TriedPatterns, attempt)

		if matched {
			explanation.Success = true
			explanation.MatchedPattern = &pattern
			explanation.MatchedScope = pattern.Scope
			break // Stop on first match
		}
	}

	explanation.TotalDuration = time.Since(start)
	return explanation
}

// matchesGlobPattern is a helper for ExplainMatch to check glob patterns
func matchesGlobPattern(path, pattern string) bool {
	// This mirrors the logic in resolver.go but simplified for explanation
	// Handle ** wildcards
	if strings.Contains(pattern, "**") {
		return matchesGlobRecursive(path, pattern)
	}

	// Use filepath.Match for single-segment wildcards (mirrors resolver behavior)
	matched, err := filepath.Match(pattern, path)
	if err != nil {
		return false
	}
	return matched
}

// matchesGlobRecursive handles ** wildcards
func matchesGlobRecursive(path, pattern string) bool {
	parts := strings.Split(pattern, "**")

	// Path must start with first part (if non-empty)
	if len(parts) > 0 && parts[0] != "" {
		if !strings.HasPrefix(path, parts[0]) {
			return false
		}
		path = path[len(parts[0]):]
	}

	// Path must end with last part (if non-empty)
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		if !strings.HasSuffix(path, parts[len(parts)-1]) {
			return false
		}
		path = path[:len(path)-len(parts[len(parts)-1])]
	}

	// Check middle parts in order
	for i := 1; i < len(parts)-1; i++ {
		part := parts[i]
		if part == "" {
			continue
		}

		idx := strings.Index(path, part)
		if idx < 0 {
			return false
		}
		path = path[idx+len(part):]
	}

	return true
}

// InspectResolver provides detailed information about a resolver's configuration.
type InspectResolver struct {
	TotalPatterns  int
	ExactPatterns  int
	PrefixPatterns int
	GlobPatterns   int
	RegexPatterns  int
	UniqueScopes   []string
	PriorityRange  [2]int // [min, max]
	Patterns       []RoutePattern
}

// String returns a human-readable summary of the resolver configuration.
func (i *InspectResolver) String() string {
	var sb strings.Builder

	sb.WriteString("=== Route Resolver Configuration ===\n")
	sb.WriteString(fmt.Sprintf("Total Patterns: %d\n", i.TotalPatterns))
	sb.WriteString(fmt.Sprintf("  Exact:  %d\n", i.ExactPatterns))
	sb.WriteString(fmt.Sprintf("  Prefix: %d\n", i.PrefixPatterns))
	sb.WriteString(fmt.Sprintf("  Glob:   %d\n", i.GlobPatterns))
	sb.WriteString(fmt.Sprintf("  Regex:  %d\n", i.RegexPatterns))
	sb.WriteString(fmt.Sprintf("\nUnique Scopes: %d\n", len(i.UniqueScopes)))
	for _, scope := range i.UniqueScopes {
		count := 0
		for _, p := range i.Patterns {
			if p.Scope == scope {
				count++
			}
		}
		sb.WriteString(fmt.Sprintf("  - %s (%d patterns)\n", scope, count))
	}
	sb.WriteString(fmt.Sprintf("\nPriority Range: %d - %d\n", i.PriorityRange[0], i.PriorityRange[1]))

	return sb.String()
}

// Inspect returns detailed information about the resolver's configuration.
func Inspect(resolver RouteResolver) *InspectResolver {
	patterns := resolver.GetPatterns()

	inspection := &InspectResolver{
		TotalPatterns: len(patterns),
		Patterns:      patterns,
		PriorityRange: [2]int{999999, -999999},
	}

	scopeSet := make(map[string]bool)

	for _, p := range patterns {
		// Count by type
		switch p.MatchType {
		case MatchExact:
			inspection.ExactPatterns++
		case MatchPrefix:
			inspection.PrefixPatterns++
		case MatchGlob:
			inspection.GlobPatterns++
		case MatchRegex:
			inspection.RegexPatterns++
		}

		// Track unique scopes
		scopeSet[p.Scope] = true

		// Track priority range
		if p.Priority < inspection.PriorityRange[0] {
			inspection.PriorityRange[0] = p.Priority
		}
		if p.Priority > inspection.PriorityRange[1] {
			inspection.PriorityRange[1] = p.Priority
		}
	}

	// Convert scope set to sorted slice
	inspection.UniqueScopes = make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		inspection.UniqueScopes = append(inspection.UniqueScopes, scope)
	}

	// Reset priority range if no patterns
	if len(patterns) == 0 {
		inspection.PriorityRange = [2]int{0, 0}
	}

	return inspection
}

// ValidateConfiguration checks for common routing configuration issues.
type ValidationIssue struct {
	Severity string // "warning" or "error"
	Pattern  string
	Message  string
}

// ValidateConfiguration returns potential issues with the routing configuration.
func ValidateConfiguration(resolver RouteResolver) []ValidationIssue {
	patterns := resolver.GetPatterns()
	issues := make([]ValidationIssue, 0)

	// Check for overlapping patterns with same priority
	priorityGroups := make(map[int][]RoutePattern)
	for _, p := range patterns {
		priorityGroups[p.Priority] = append(priorityGroups[p.Priority], p)
	}

	for priority, group := range priorityGroups {
		if len(group) > 1 {
			// Check if patterns might overlap
			for i := 0; i < len(group); i++ {
				for j := i + 1; j < len(group); j++ {
					if mightOverlap(group[i], group[j]) {
						issues = append(issues, ValidationIssue{
							Severity: "warning",
							Pattern:  fmt.Sprintf("%s and %s", group[i].Pattern, group[j].Pattern),
							Message:  fmt.Sprintf("Patterns with same priority (%d) might overlap - match order is undefined", priority),
						})
					}
				}
			}
		}
	}

	// Check for unreachable patterns (lower priority fully covered by higher priority)
	for i := 0; i < len(patterns); i++ {
		for j := i + 1; j < len(patterns); j++ {
			if patterns[i].Priority > patterns[j].Priority {
				if fullyCovered(patterns[j], patterns[i]) {
					issues = append(issues, ValidationIssue{
						Severity: "warning",
						Pattern:  patterns[j].Pattern,
						Message:  fmt.Sprintf("Pattern is unreachable - fully covered by higher priority pattern %s", patterns[i].Pattern),
					})
				}
			}
		}
	}

	// Check for patterns with no corresponding scope configuration
	// (This would require access to rate limit config, so just note the scopes)

	return issues
}

// mightOverlap checks if two patterns might match the same paths
func mightOverlap(p1, p2 RoutePattern) bool {
	// Exact matches never overlap unless identical
	if p1.MatchType == MatchExact && p2.MatchType == MatchExact {
		return p1.Pattern == p2.Pattern
	}

	// Prefix patterns can overlap
	if p1.MatchType == MatchPrefix && p2.MatchType == MatchPrefix {
		return strings.HasPrefix(p1.Pattern, p2.Pattern) || strings.HasPrefix(p2.Pattern, p1.Pattern)
	}

	// Glob and regex are complex - assume they might overlap
	return true
}

// fullyCovered checks if pattern p1 is fully covered by pattern p2
func fullyCovered(p1, p2 RoutePattern) bool {
	// If p2 is a prefix and p1's pattern starts with it, p1 is covered
	if p2.MatchType == MatchPrefix {
		return strings.HasPrefix(p1.Pattern, p2.Pattern)
	}

	// If both are exact matches and identical, p1 is covered
	if p1.MatchType == MatchExact && p2.MatchType == MatchExact {
		return p1.Pattern == p2.Pattern
	}

	// Glob/regex coverage is complex - be conservative
	return false
}
