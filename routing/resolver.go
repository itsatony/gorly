// Package routing provides flexible pattern-based routing for rate limiting.
// It supports exact matching, prefix matching, glob patterns, and regular expressions
// to map requests/operations to appropriate rate limiting scopes.
package routing

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// MatchType defines how a pattern should be matched against a request path.
type MatchType int

const (
	// MatchExact requires an exact string match.
	// Example: "/api/payment" matches "/api/payment" only
	MatchExact MatchType = iota

	// MatchPrefix matches if the path starts with the pattern.
	// Example: "/api/admin" matches "/api/admin", "/api/admin/users", etc.
	MatchPrefix

	// MatchGlob uses glob-style wildcards (* and **).
	// * matches any sequence within a path segment
	// ** matches any sequence across multiple path segments
	// Example: "/api/users/*" matches "/api/users/123", "/api/users/profile"
	// Example: "/api/**" matches "/api/v1/users", "/api/v2/admin/config"
	MatchGlob

	// MatchRegex uses regular expression matching.
	// Example: "^/api/v[0-9]+/.*$" matches "/api/v1/users", "/api/v2/config"
	// Note: Regex patterns must be carefully written to avoid ReDoS attacks
	MatchRegex
)

// String returns the string representation of the MatchType.
func (mt MatchType) String() string {
	switch mt {
	case MatchExact:
		return "exact"
	case MatchPrefix:
		return "prefix"
	case MatchGlob:
		return "glob"
	case MatchRegex:
		return "regex"
	default:
		return fmt.Sprintf("unknown(%d)", mt)
	}
}

// RoutePattern defines a pattern to match against request paths/identifiers
// and the scope to use when the pattern matches.
type RoutePattern struct {
	// Pattern is the string pattern to match against.
	// The format depends on MatchType:
	// - Exact: literal string
	// - Prefix: prefix string
	// - Glob: glob pattern with * and **
	// - Regex: regular expression
	Pattern string

	// Scope is the rate limiting scope to use when this pattern matches.
	// This scope is used to look up the appropriate rate limit configuration.
	Scope string

	// MatchType determines how the pattern is interpreted.
	MatchType MatchType

	// Priority determines which pattern wins when multiple patterns match.
	// Higher priority patterns are checked first.
	// Recommended priorities: Exact=100, Glob=50, Regex=30, Prefix=10
	Priority int

	// compiled is the pre-compiled regex (only used for MatchRegex).
	// This is set during pattern validation.
	compiled *regexp.Regexp
}

// RouteResolver maps request paths/identifiers to rate limiting scopes
// based on configured patterns.
type RouteResolver interface {
	// ResolveScope returns the scope for the given path and whether a match was found.
	// If no pattern matches, returns ("", false).
	// Patterns are evaluated in priority order (highest first).
	ResolveScope(path string) (scope string, matched bool)

	// AddPattern adds a new pattern to the resolver.
	// Returns an error if the pattern is invalid or already exists.
	AddPattern(pattern RoutePattern) error

	// RemovePattern removes a pattern by its pattern string.
	// Returns true if a pattern was removed, false if not found.
	RemovePattern(patternStr string) bool

	// GetPatterns returns all configured patterns in priority order.
	GetPatterns() []RoutePattern

	// Clear removes all patterns from the resolver.
	Clear()
}

// Security limits for pattern matching to prevent DoS attacks
const (
	// MaxGlobWildcards limits the number of wildcards in a glob pattern
	MaxGlobWildcards = 10

	// MaxRegexComplexity limits regex complexity (based on pattern length)
	MaxRegexComplexity = 500

	// DefaultRegexTimeout is the maximum time allowed for regex matching
	DefaultRegexTimeout = 100 * time.Microsecond
)

var (
	// ErrInvalidPattern indicates the pattern format is invalid
	ErrInvalidPattern = errors.New("invalid pattern")

	// ErrDuplicatePattern indicates a pattern already exists
	ErrDuplicatePattern = errors.New("duplicate pattern")

	// ErrPatternTooComplex indicates the pattern is too complex (security limit)
	ErrPatternTooComplex = errors.New("pattern too complex (security limit)")

	// ErrRegexTimeout indicates regex matching exceeded the timeout
	ErrRegexTimeout = errors.New("regex matching timeout")
)

// defaultRouteResolver is the default implementation of RouteResolver.
type defaultRouteResolver struct {
	mu sync.RWMutex

	// patterns stores all patterns in priority order (highest first)
	patterns []RoutePattern

	// exactMatches provides O(1) lookup for exact matches
	exactMatches map[string]string

	// regexTimeout is the maximum time allowed for regex matching
	regexTimeout time.Duration

	// metrics tracks pattern matching performance and behavior
	metrics Metrics
}

// NewRouteResolver creates a new RouteResolver with default settings.
// Metrics are disabled by default. Use NewRouteResolverWithMetrics to enable.
func NewRouteResolver() RouteResolver {
	return &defaultRouteResolver{
		patterns:     make([]RoutePattern, 0),
		exactMatches: make(map[string]string),
		regexTimeout: DefaultRegexTimeout,
		metrics:      NewNoOpMetrics(),
	}
}

// NewRouteResolverWithTimeout creates a new RouteResolver with custom regex timeout.
func NewRouteResolverWithTimeout(timeout time.Duration) RouteResolver {
	return &defaultRouteResolver{
		patterns:     make([]RoutePattern, 0),
		exactMatches: make(map[string]string),
		regexTimeout: timeout,
		metrics:      NewNoOpMetrics(),
	}
}

// NewRouteResolverWithMetrics creates a new RouteResolver with metrics enabled.
func NewRouteResolverWithMetrics(metricsOpts ...MetricsOption) RouteResolver {
	return &defaultRouteResolver{
		patterns:     make([]RoutePattern, 0),
		exactMatches: make(map[string]string),
		regexTimeout: DefaultRegexTimeout,
		metrics:      createMetrics(metricsOpts...),
	}
}

// ResolveScope implements RouteResolver.ResolveScope
func (r *defaultRouteResolver) ResolveScope(path string) (scope string, matched bool) {
	start := time.Now()
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Fast path: check exact matches first (O(1))
	if scope, ok := r.exactMatches[path]; ok {
		duration := time.Since(start)
		r.metrics.RecordMatch(MatchExact, path, duration)
		return scope, true
	}

	// Check patterns in priority order
	for _, pattern := range r.patterns {
		// Skip exact matches (already checked above)
		if pattern.MatchType == MatchExact {
			continue
		}

		matched := false
		switch pattern.MatchType {
		case MatchPrefix:
			matched = r.matchPrefix(path, pattern.Pattern)
		case MatchGlob:
			matched = r.matchGlob(path, pattern.Pattern)
		case MatchRegex:
			matched = r.matchRegexWithTimeout(path, pattern.compiled)
		}

		if matched {
			duration := time.Since(start)
			r.metrics.RecordMatch(pattern.MatchType, pattern.Pattern, duration)
			return pattern.Scope, true
		}
	}

	// No match found
	duration := time.Since(start)
	r.metrics.RecordNoMatch(duration)
	return "", false
}

// AddPattern implements RouteResolver.AddPattern
func (r *defaultRouteResolver) AddPattern(pattern RoutePattern) error {
	// Validate pattern
	if err := r.validatePattern(&pattern); err != nil {
		return fmt.Errorf("pattern validation failed: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	for _, p := range r.patterns {
		if p.Pattern == pattern.Pattern && p.MatchType == pattern.MatchType {
			return ErrDuplicatePattern
		}
	}

	// Add to patterns list
	r.patterns = append(r.patterns, pattern)

	// Sort by priority (highest first)
	sort.Slice(r.patterns, func(i, j int) bool {
		return r.patterns[i].Priority > r.patterns[j].Priority
	})

	// Add to exact matches map for fast lookup
	if pattern.MatchType == MatchExact {
		r.exactMatches[pattern.Pattern] = pattern.Scope
	}

	// Update pattern count metrics
	if pm, ok := r.metrics.(*PrometheusMetrics); ok {
		pm.UpdatePatternCounts(r.patterns)
	}

	return nil
}

// RemovePattern implements RouteResolver.RemovePattern
func (r *defaultRouteResolver) RemovePattern(patternStr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, p := range r.patterns {
		if p.Pattern == patternStr {
			// Remove from patterns slice
			r.patterns = append(r.patterns[:i], r.patterns[i+1:]...)

			// Remove from exact matches map
			if p.MatchType == MatchExact {
				delete(r.exactMatches, patternStr)
			}

			// Update pattern count metrics
			if pm, ok := r.metrics.(*PrometheusMetrics); ok {
				pm.UpdatePatternCounts(r.patterns)
			}

			return true
		}
	}

	return false
}

// GetPatterns implements RouteResolver.GetPatterns
func (r *defaultRouteResolver) GetPatterns() []RoutePattern {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	patterns := make([]RoutePattern, len(r.patterns))
	copy(patterns, r.patterns)
	return patterns
}

// Clear implements RouteResolver.Clear
func (r *defaultRouteResolver) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.patterns = make([]RoutePattern, 0)
	r.exactMatches = make(map[string]string)
}

// validatePattern validates a pattern and prepares it for matching
func (r *defaultRouteResolver) validatePattern(pattern *RoutePattern) error {
	if pattern.Pattern == "" {
		return fmt.Errorf("%w: pattern cannot be empty", ErrInvalidPattern)
	}

	if pattern.Scope == "" {
		return fmt.Errorf("%w: scope cannot be empty", ErrInvalidPattern)
	}

	switch pattern.MatchType {
	case MatchExact:
		// No special validation needed
		return nil

	case MatchPrefix:
		// No special validation needed
		return nil

	case MatchGlob:
		// Check wildcard count to prevent DoS
		wildcardCount := strings.Count(pattern.Pattern, "*")
		if wildcardCount > MaxGlobWildcards {
			return fmt.Errorf("%w: too many wildcards (%d > %d)", ErrPatternTooComplex, wildcardCount, MaxGlobWildcards)
		}
		return nil

	case MatchRegex:
		// Check pattern length to prevent ReDoS
		if len(pattern.Pattern) > MaxRegexComplexity {
			return fmt.Errorf("%w: regex too long (%d > %d)", ErrPatternTooComplex, len(pattern.Pattern), MaxRegexComplexity)
		}

		// Compile regex to catch syntax errors
		compiled, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return fmt.Errorf("%w: invalid regex: %v", ErrInvalidPattern, err)
		}

		// Store compiled regex for reuse
		pattern.compiled = compiled

		return nil

	default:
		return fmt.Errorf("%w: unknown match type: %v", ErrInvalidPattern, pattern.MatchType)
	}
}

// matchPrefix checks if path starts with the prefix
func (r *defaultRouteResolver) matchPrefix(path, prefix string) bool {
	return strings.HasPrefix(path, prefix)
}

// matchGlob performs safe glob matching without exponential backtracking.
// Supports * (single segment wildcard) and ** (multi-segment wildcard).
func (r *defaultRouteResolver) matchGlob(path, pattern string) bool {
	// Handle ** wildcard specially (matches across path segments)
	if strings.Contains(pattern, "**") {
		return r.matchGlobRecursive(path, pattern)
	}

	// Use filepath.Match for single-segment wildcards
	// filepath.Match is safe and doesn't have backtracking issues
	matched, err := filepath.Match(pattern, path)
	if err != nil {
		// Pattern is malformed, shouldn't happen after validation
		return false
	}

	return matched
}

// matchGlobRecursive handles ** wildcards by splitting pattern and matching parts
func (r *defaultRouteResolver) matchGlobRecursive(path, pattern string) bool {
	// Split pattern on **
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

// matchRegexWithTimeout performs regex matching with timeout protection
func (r *defaultRouteResolver) matchRegexWithTimeout(path string, compiled *regexp.Regexp) bool {
	if compiled == nil {
		return false
	}

	// Create a channel to receive the result
	done := make(chan bool, 1)

	// Run regex matching in a goroutine
	go func() {
		done <- compiled.MatchString(path)
	}()

	// Wait for result or timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.regexTimeout)
	defer cancel()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		// Timeout occurred - this is a security measure
		// In practice, well-formed regexes should never timeout
		return false
	}
}

// Builder provides a fluent API for constructing a RouteResolver
type Builder struct {
	resolver *defaultRouteResolver
	err      error
}

// NewBuilder creates a new Builder for constructing a RouteResolver
func NewBuilder() *Builder {
	return &Builder{
		resolver: &defaultRouteResolver{
			patterns:     make([]RoutePattern, 0),
			exactMatches: make(map[string]string),
			regexTimeout: DefaultRegexTimeout,
			metrics:      NewNoOpMetrics(),
		},
	}
}

// WithTimeout sets a custom regex timeout for the resolver
func (b *Builder) WithTimeout(timeout time.Duration) *Builder {
	if b.err != nil {
		return b
	}
	b.resolver.regexTimeout = timeout
	return b
}

// WithMetrics enables metrics collection for the resolver
func (b *Builder) WithMetrics(metricsOpts ...MetricsOption) *Builder {
	if b.err != nil {
		return b
	}
	b.resolver.metrics = createMetrics(metricsOpts...)
	return b
}

// AddExact adds an exact match pattern
func (b *Builder) AddExact(pattern, scope string, priority int) *Builder {
	if b.err != nil {
		return b
	}

	err := b.resolver.AddPattern(RoutePattern{
		Pattern:   pattern,
		Scope:     scope,
		MatchType: MatchExact,
		Priority:  priority,
	})

	if err != nil {
		b.err = fmt.Errorf("failed to add exact pattern %q: %w", pattern, err)
	}

	return b
}

// AddPrefix adds a prefix match pattern
func (b *Builder) AddPrefix(pattern, scope string, priority int) *Builder {
	if b.err != nil {
		return b
	}

	err := b.resolver.AddPattern(RoutePattern{
		Pattern:   pattern,
		Scope:     scope,
		MatchType: MatchPrefix,
		Priority:  priority,
	})

	if err != nil {
		b.err = fmt.Errorf("failed to add prefix pattern %q: %w", pattern, err)
	}

	return b
}

// AddGlob adds a glob match pattern
func (b *Builder) AddGlob(pattern, scope string, priority int) *Builder {
	if b.err != nil {
		return b
	}

	err := b.resolver.AddPattern(RoutePattern{
		Pattern:   pattern,
		Scope:     scope,
		MatchType: MatchGlob,
		Priority:  priority,
	})

	if err != nil {
		b.err = fmt.Errorf("failed to add glob pattern %q: %w", pattern, err)
	}

	return b
}

// AddRegex adds a regex match pattern
func (b *Builder) AddRegex(pattern, scope string, priority int) *Builder {
	if b.err != nil {
		return b
	}

	err := b.resolver.AddPattern(RoutePattern{
		Pattern:   pattern,
		Scope:     scope,
		MatchType: MatchRegex,
		Priority:  priority,
	})

	if err != nil {
		b.err = fmt.Errorf("failed to add regex pattern %q: %w", pattern, err)
	}

	return b
}

// Build returns the constructed RouteResolver or an error if any operations failed
func (b *Builder) Build() (RouteResolver, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.resolver, nil
}

// MustBuild is like Build but panics on error. Useful for static configurations.
func (b *Builder) MustBuild() RouteResolver {
	resolver, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build RouteResolver: %v", err))
	}
	return resolver
}
