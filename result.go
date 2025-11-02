package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

// ============================================================================
// RESULT TYPE - Rate limit decision outcome
// ============================================================================

// Result represents the outcome of a rate limit check
// All fields are concrete types - no interface{}
//
// ============================================================================
// P0-4 THREAD SAFETY DOCUMENTATION
// ============================================================================
//
// SAFE OPERATIONS (can be called concurrently):
//   - Reading immutable fields after construction: Allowed, Limit, Remaining, etc.
//   - GetMetadata(), SetMetadata(), GetAllMetadata() - all metadata operations
//   - Clone() - creates a thread-safe copy
//   - String(), UsagePercentage(), helper methods - all read-only operations
//
// UNSAFE OPERATIONS (NOT thread-safe, must NOT be called concurrently):
//   - WithContext() - modifies Scope, Entity, Tier without synchronization
//   - WithStrategy() - modifies Strategy without synchronization
//   - Direct field writes after construction
//
// RECOMMENDED USAGE PATTERN:
//  1. Create Result with constructor (NewAllowedResult, NewDeniedResult)
//  2. Call WithContext() and WithStrategy() to configure (BEFORE sharing)
//  3. Return/share Result (now treat as read-only except for metadata)
//  4. Use only GetMetadata/SetMetadata for concurrent metadata access
//  5. If multiple goroutines need different fields, use Clone()
//
// ANTI-PATTERNS (will cause race conditions):
//
//	❌ Sharing Result and calling WithContext/WithStrategy concurrently
//	❌ Modifying non-metadata fields after sharing across goroutines
//	❌ Direct writes to fields after construction: result.Scope = "new"
//
// Example - SAFE:
//
//	result := NewAllowedResult(100, 90, 10, resetTime, window)
//	result.WithContext("api", "user123", "premium")  // OK - before sharing
//	result.WithStrategy("token_bucket")              // OK - before sharing
//	go processResult(result)                         // OK - Result now read-only
//
// Example - UNSAFE (RACE CONDITION):
//
//	result := NewAllowedResult(...)
//	go func() { result.WithContext(...) }()  // ❌ RACE
//	go func() { fmt.Println(result.Scope) }() // ❌ RACE
//
// IMPORTANT: While individual Result instances are thread-safe for metadata
// operations, Result objects should generally not be shared across goroutines.
// Each rate limit check returns a new Result that should be consumed by a
// single goroutine or properly synchronized if sharing is necessary.
//
// If you need to safely share a Result across goroutines:
//   - Option 1: Use Clone() to create independent copies
//   - Option 2: Only read immutable fields and use Get/SetMetadata()
//   - Option 3: Add external synchronization (sync.Mutex)
type Result struct {
	// Allowed indicates if the request is permitted
	Allowed bool

	// Limit is the maximum requests allowed in the window
	Limit int64

	// Remaining is how many requests are left in current window
	Remaining int64

	// Used is how many requests have been consumed
	Used int64

	// RetryAfter indicates when the client can retry (if not allowed)
	// This is the duration to wait before the next request
	RetryAfter time.Duration

	// ResetAt indicates when the rate limit window resets
	ResetAt time.Time

	// Window is the rate limit time window duration
	Window time.Duration

	// Scope is the scope that was evaluated
	Scope string

	// Entity is the entity that was evaluated
	Entity string

	// Tier is the tier that was evaluated
	Tier string

	// Strategy is the name of the strategy that was used
	Strategy string

	// metadata contains strategy-specific or custom information
	// BREAKING CHANGE: This field is now private. Use GetMetadata/SetMetadata
	// for thread-safe access. Direct field access is no longer supported.
	metadata map[string]interface{}

	// metadataMu protects concurrent access to the metadata map
	metadataMu sync.RWMutex
}

// ============================================================================
// RESULT CONSTRUCTORS
// ============================================================================

// NewAllowedResult creates a result indicating the request is allowed
func NewAllowedResult(limit, remaining, used int64, resetAt time.Time, window time.Duration) *Result {
	return &Result{
		Allowed:    true,
		Limit:      limit,
		Remaining:  remaining,
		Used:       used,
		RetryAfter: 0,
		ResetAt:    resetAt,
		Window:     window,
		metadata:   make(map[string]interface{}),
	}
}

// NewDeniedResult creates a result indicating the request is denied
func NewDeniedResult(limit, used int64, resetAt time.Time, window time.Duration) *Result {
	retryAfter := time.Until(resetAt)
	if retryAfter < 0 {
		retryAfter = 0
	}

	return &Result{
		Allowed:    false,
		Limit:      limit,
		Remaining:  0,
		Used:       used,
		RetryAfter: retryAfter,
		ResetAt:    resetAt,
		Window:     window,
		metadata:   make(map[string]interface{}),
	}
}

// NewEmptyResult creates an empty result (for stats queries when no data exists)
func NewEmptyResult(limit int64, window time.Duration) *Result {
	return &Result{
		Allowed:    true,
		Limit:      limit,
		Remaining:  limit,
		Used:       0,
		RetryAfter: 0,
		ResetAt:    time.Now().Add(window),
		Window:     window,
		metadata:   make(map[string]interface{}),
	}
}

// ============================================================================
// RESULT METHODS
// ============================================================================

// WithContext adds context information to the result
//
// P0-4 THREAD SAFETY WARNING: This method modifies the Result and is NOT thread-safe.
// It should only be called during Result construction before the Result is shared
// across goroutines. Do NOT call this method on a Result that may be accessed
// concurrently by multiple goroutines.
//
// Safe pattern:
//
//	result := NewAllowedResult(...)
//	result.WithContext(scope, entity, tier)  // OK - before sharing
//	return result
//
// Unsafe pattern:
//
//	go func() { result.WithContext(...) }()  // RACE CONDITION
//	go func() { fmt.Println(result.Scope) }()  // RACE CONDITION
func (r *Result) WithContext(scope, entity, tier string) *Result {
	r.Scope = scope
	r.Entity = entity
	r.Tier = tier
	return r
}

// WithStrategy adds strategy information to the result
//
// P0-4 THREAD SAFETY WARNING: This method modifies the Result and is NOT thread-safe.
// It should only be called during Result construction before the Result is shared
// across goroutines. Do NOT call this method on a Result that may be accessed
// concurrently by multiple goroutines.
//
// Safe pattern:
//
//	result := NewAllowedResult(...)
//	result.WithStrategy("token_bucket")  // OK - before sharing
//	return result
//
// Unsafe pattern:
//
//	go func() { result.WithStrategy(...) }()  // RACE CONDITION
//	go func() { fmt.Println(result.Strategy) }()  // RACE CONDITION
func (r *Result) WithStrategy(strategy string) *Result {
	r.Strategy = strategy
	return r
}

// ============================================================================
// METADATA ACCESS - Thread-safe methods for concurrent access
// ============================================================================

// GetMetadata retrieves a metadata value by key (thread-safe)
// Returns the value and a boolean indicating if the key exists
func (r *Result) GetMetadata(key string) (interface{}, bool) {
	r.metadataMu.RLock()
	defer r.metadataMu.RUnlock()

	if r.metadata == nil {
		return nil, false
	}

	value, exists := r.metadata[key]
	return value, exists
}

// SetMetadata sets a metadata key-value pair (thread-safe)
func (r *Result) SetMetadata(key string, value interface{}) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()

	if r.metadata == nil {
		r.metadata = make(map[string]interface{})
	}
	r.metadata[key] = value
}

// SetMetadataMap sets multiple metadata entries (thread-safe)
func (r *Result) SetMetadataMap(metadata map[string]interface{}) {
	r.metadataMu.Lock()
	defer r.metadataMu.Unlock()

	if r.metadata == nil {
		r.metadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		r.metadata[k] = v
	}
}

// GetAllMetadata returns a copy of all metadata (thread-safe)
// Returns a new map so modifications won't affect the original
func (r *Result) GetAllMetadata() map[string]interface{} {
	r.metadataMu.RLock()
	defer r.metadataMu.RUnlock()

	if r.metadata == nil {
		return make(map[string]interface{})
	}

	// Create a shallow copy
	copy := make(map[string]interface{}, len(r.metadata))
	for k, v := range r.metadata {
		copy[k] = v
	}
	return copy
}

// HasMetadata checks if a metadata key exists (thread-safe)
func (r *Result) HasMetadata(key string) bool {
	r.metadataMu.RLock()
	defer r.metadataMu.RUnlock()

	if r.metadata == nil {
		return false
	}

	_, exists := r.metadata[key]
	return exists
}

// String returns a human-readable representation of the result
func (r *Result) String() string {
	if r.Allowed {
		return fmt.Sprintf("ALLOWED: %d/%d used, %d remaining, resets at %s",
			r.Used, r.Limit, r.Remaining, r.ResetAt.Format(time.RFC3339))
	}
	return fmt.Sprintf("DENIED: %d/%d used (limit exceeded), retry after %s",
		r.Used, r.Limit, r.RetryAfter)
}

// ResetAtUnix returns the ResetAt timestamp as Unix seconds
// Useful for HTTP headers
func (r *Result) ResetAtUnix() int64 {
	return r.ResetAt.Unix()
}

// RetryAfterSeconds returns the RetryAfter duration as seconds
// Useful for HTTP headers
func (r *Result) RetryAfterSeconds() int64 {
	return int64(r.RetryAfter.Seconds())
}

// ============================================================================
// RESULT UTILITIES
// ============================================================================

// Clone creates a deep copy of the result (thread-safe)
func (r *Result) Clone() *Result {
	cloned := &Result{
		Allowed:    r.Allowed,
		Limit:      r.Limit,
		Remaining:  r.Remaining,
		Used:       r.Used,
		RetryAfter: r.RetryAfter,
		ResetAt:    r.ResetAt,
		Window:     r.Window,
		Scope:      r.Scope,
		Entity:     r.Entity,
		Tier:       r.Tier,
		Strategy:   r.Strategy,
		metadata:   make(map[string]interface{}),
	}

	// Deep copy metadata with lock protection
	r.metadataMu.RLock()
	for k, v := range r.metadata {
		cloned.metadata[k] = v
	}
	r.metadataMu.RUnlock()

	return cloned
}

// UsagePercentage returns the percentage of limit used (0-100)
func (r *Result) UsagePercentage() float64 {
	if r.Limit == 0 {
		return 0
	}
	return (float64(r.Used) / float64(r.Limit)) * 100
}

// IsNearLimit checks if usage is near the limit (>= threshold percentage)
// threshold should be between 0 and 100
func (r *Result) IsNearLimit(threshold float64) bool {
	return r.UsagePercentage() >= threshold
}

// TimeUntilReset returns the duration until the window resets
func (r *Result) TimeUntilReset() time.Duration {
	return time.Until(r.ResetAt)
}

// ============================================================================
// RESULT BUILDER - Fluent API for result construction
// ============================================================================

// ResultBuilder provides a fluent API for building results
type ResultBuilder struct {
	result *Result
}

// NewResultBuilder creates a new result builder
func NewResultBuilder() *ResultBuilder {
	return &ResultBuilder{
		result: &Result{
			metadata: make(map[string]interface{}),
		},
	}
}

// Allowed sets whether the request is allowed
func (rb *ResultBuilder) Allowed(allowed bool) *ResultBuilder {
	rb.result.Allowed = allowed
	return rb
}

// Limit sets the limit
func (rb *ResultBuilder) Limit(limit int64) *ResultBuilder {
	rb.result.Limit = limit
	return rb
}

// Remaining sets the remaining count
func (rb *ResultBuilder) Remaining(remaining int64) *ResultBuilder {
	rb.result.Remaining = remaining
	return rb
}

// Used sets the used count
func (rb *ResultBuilder) Used(used int64) *ResultBuilder {
	rb.result.Used = used
	return rb
}

// RetryAfter sets the retry after duration
func (rb *ResultBuilder) RetryAfter(retryAfter time.Duration) *ResultBuilder {
	rb.result.RetryAfter = retryAfter
	return rb
}

// ResetAt sets the reset time
func (rb *ResultBuilder) ResetAt(resetAt time.Time) *ResultBuilder {
	rb.result.ResetAt = resetAt
	return rb
}

// Window sets the window duration
func (rb *ResultBuilder) Window(window time.Duration) *ResultBuilder {
	rb.result.Window = window
	return rb
}

// Scope sets the scope
func (rb *ResultBuilder) Scope(scope string) *ResultBuilder {
	rb.result.Scope = scope
	return rb
}

// Entity sets the entity
func (rb *ResultBuilder) Entity(entity string) *ResultBuilder {
	rb.result.Entity = entity
	return rb
}

// Tier sets the tier
func (rb *ResultBuilder) Tier(tier string) *ResultBuilder {
	rb.result.Tier = tier
	return rb
}

// Strategy sets the strategy
func (rb *ResultBuilder) Strategy(strategy string) *ResultBuilder {
	rb.result.Strategy = strategy
	return rb
}

// Metadata adds metadata (thread-safe)
func (rb *ResultBuilder) Metadata(key string, value interface{}) *ResultBuilder {
	rb.result.SetMetadata(key, value)
	return rb
}

// Build returns the constructed result
func (rb *ResultBuilder) Build() *Result {
	return rb.result
}

// ============================================================================
// RESULT COMPARISON
// ============================================================================

// ResultsEqual checks if two results are equivalent
func ResultsEqual(a, b *Result) bool {
	if a == nil || b == nil {
		return a == b
	}

	return a.Allowed == b.Allowed &&
		a.Limit == b.Limit &&
		a.Remaining == b.Remaining &&
		a.Used == b.Used &&
		a.RetryAfter == b.RetryAfter &&
		a.ResetAt.Equal(b.ResetAt) &&
		a.Window == b.Window &&
		a.Scope == b.Scope &&
		a.Entity == b.Entity &&
		a.Tier == b.Tier &&
		a.Strategy == b.Strategy
}

// ============================================================================
// RESULT AGGREGATION
// ============================================================================

// AggregatedResult combines multiple results (e.g., from multi-scope checks)
type AggregatedResult struct {
	// Overall indicates if all checks passed
	Overall bool

	// Results contains individual results by scope
	Results map[string]*Result

	// LowestRemaining is the scope with the least remaining capacity
	LowestRemaining string

	// NextReset is the earliest reset time across all scopes
	NextReset time.Time
}

// NewAggregatedResult creates an aggregated result from multiple results
func NewAggregatedResult(results map[string]*Result) *AggregatedResult {
	ar := &AggregatedResult{
		Overall: true,
		Results: results,
	}

	var lowestRemaining int64 = -1
	var earliestReset time.Time

	for scope, result := range results {
		// If any result is denied, overall is denied
		if !result.Allowed {
			ar.Overall = false
		}

		// Track lowest remaining
		if lowestRemaining == -1 || result.Remaining < lowestRemaining {
			lowestRemaining = result.Remaining
			ar.LowestRemaining = scope
		}

		// Track earliest reset
		if earliestReset.IsZero() || result.ResetAt.Before(earliestReset) {
			earliestReset = result.ResetAt
		}
	}

	ar.NextReset = earliestReset
	return ar
}

// String returns a human-readable representation
func (ar *AggregatedResult) String() string {
	if ar.Overall {
		return fmt.Sprintf("ALLOWED across %d scopes, lowest remaining: %s, next reset: %s",
			len(ar.Results), ar.LowestRemaining, ar.NextReset.Format(time.RFC3339))
	}
	return fmt.Sprintf("DENIED (one or more scopes exceeded), %d scopes checked",
		len(ar.Results))
}
