package ratelimit

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// CONSTRUCTOR TESTS
// ============================================================================

func TestNewAllowedResult(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	result := NewAllowedResult(100, 75, 25, resetAt, window)

	if !result.Allowed {
		t.Error("Result should be allowed")
	}

	if result.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", result.Limit)
	}

	if result.Remaining != 75 {
		t.Errorf("Expected remaining 75, got %d", result.Remaining)
	}

	if result.Used != 25 {
		t.Errorf("Expected used 25, got %d", result.Used)
	}

	if result.RetryAfter != 0 {
		t.Errorf("Expected RetryAfter 0, got %v", result.RetryAfter)
	}

	if !result.ResetAt.Equal(resetAt) {
		t.Errorf("Expected ResetAt %v, got %v", resetAt, result.ResetAt)
	}

	if result.Window != window {
		t.Errorf("Expected window %v, got %v", window, result.Window)
	}

	// Metadata is now private and always initialized - just verify we can access it
	metadata := result.GetAllMetadata()
	if metadata == nil {
		t.Error("GetAllMetadata should never return nil")
	}
}

func TestNewDeniedResult(t *testing.T) {
	resetAt := time.Now().Add(30 * time.Second)
	window := time.Minute

	result := NewDeniedResult(100, 100, resetAt, window)

	if result.Allowed {
		t.Error("Result should be denied")
	}

	if result.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", result.Limit)
	}

	if result.Remaining != 0 {
		t.Errorf("Expected remaining 0, got %d", result.Remaining)
	}

	if result.Used != 100 {
		t.Errorf("Expected used 100, got %d", result.Used)
	}

	if result.RetryAfter < 0 {
		t.Errorf("RetryAfter should not be negative, got %v", result.RetryAfter)
	}

	if result.RetryAfter > 31*time.Second {
		t.Errorf("RetryAfter should be ~30 seconds, got %v", result.RetryAfter)
	}

	if result.Window != window {
		t.Errorf("Expected window %v, got %v", window, result.Window)
	}

	// Metadata is now private and always initialized - just verify we can access it
	metadata := result.GetAllMetadata()
	if metadata == nil {
		t.Error("GetAllMetadata should never return nil")
	}
}

func TestNewDeniedResult_PastResetTime(t *testing.T) {
	// Reset time in the past
	resetAt := time.Now().Add(-10 * time.Second)
	window := time.Minute

	result := NewDeniedResult(100, 100, resetAt, window)

	if result.RetryAfter != 0 {
		t.Errorf("RetryAfter should be 0 for past reset time, got %v", result.RetryAfter)
	}
}

// ============================================================================
// RESULT METHOD TESTS
// ============================================================================

func TestWithMetadata(t *testing.T) {
	result := NewAllowedResult(100, 99, 1, time.Now().Add(time.Hour), time.Hour)

	result.SetMetadata("key1", "value1")
	result.SetMetadata("key2", 42)
	result.SetMetadata("key3", true)

	if val, _ := result.GetMetadata("key1"); val != "value1" {
		t.Errorf("Expected key1='value1', got %v", val)
	}

	if val, _ := result.GetMetadata("key2"); val != 42 {
		t.Errorf("Expected key2=42, got %v", val)
	}

	if val, _ := result.GetMetadata("key3"); val != true {
		t.Errorf("Expected key3=true, got %v", val)
	}
}

func TestWithMetadata_NilMetadata(t *testing.T) {
	result := &Result{}

	// Should initialize metadata if nil
	result.SetMetadata("test", "value")

	if len(result.GetAllMetadata()) == 0 {
		t.Error("Metadata should be initialized")
	}

	if val, _ := result.GetMetadata("test"); val != "value" {
		t.Errorf("Expected test='value', got %v", val)
	}
}

func TestWithMetadataMap(t *testing.T) {
	result := NewAllowedResult(100, 99, 1, time.Now().Add(time.Hour), time.Hour)

	metadata := map[string]interface{}{
		"ip":         "192.168.1.1",
		"user_agent": "Mozilla/5.0",
		"method":     "POST",
		"count":      5,
	}

	result.SetMetadataMap(metadata)

	if val, _ := result.GetMetadata("ip"); val != "192.168.1.1" {
		t.Errorf("Expected ip='192.168.1.1', got %v", val)
	}

	if val, _ := result.GetMetadata("user_agent"); val != "Mozilla/5.0" {
		t.Errorf("Expected user_agent='Mozilla/5.0', got %v", val)
	}

	if val, _ := result.GetMetadata("method"); val != "POST" {
		t.Errorf("Expected method='POST', got %v", val)
	}

	if val, _ := result.GetMetadata("count"); val != 5 {
		t.Errorf("Expected count=5, got %v", val)
	}
}

func TestWithMetadataMap_NilMetadata(t *testing.T) {
	result := &Result{}

	metadata := map[string]interface{}{
		"key": "value",
	}

	result.SetMetadataMap(metadata)

	if len(result.GetAllMetadata()) == 0 {
		t.Error("Metadata should be initialized")
	}

	if val, _ := result.GetMetadata("key"); val != "value" {
		t.Errorf("Expected key='value', got %v", val)
	}
}

func TestString_Allowed(t *testing.T) {
	resetAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	result := NewAllowedResult(100, 75, 25, resetAt, time.Hour)

	str := result.String()

	if !strings.Contains(str, "ALLOWED") {
		t.Errorf("String should contain 'ALLOWED', got: %s", str)
	}

	if !strings.Contains(str, "25/100") {
		t.Errorf("String should contain usage '25/100', got: %s", str)
	}

	if !strings.Contains(str, "75 remaining") {
		t.Errorf("String should contain 'remaining', got: %s", str)
	}

	if !strings.Contains(str, "resets at") {
		t.Errorf("String should contain 'resets at', got: %s", str)
	}
}

func TestString_Denied(t *testing.T) {
	resetAt := time.Now().Add(30 * time.Second)
	result := NewDeniedResult(100, 100, resetAt, time.Minute)

	str := result.String()

	if !strings.Contains(str, "DENIED") {
		t.Errorf("String should contain 'DENIED', got: %s", str)
	}

	if !strings.Contains(str, "100/100") {
		t.Errorf("String should contain usage '100/100', got: %s", str)
	}

	if !strings.Contains(str, "limit exceeded") {
		t.Errorf("String should contain 'limit exceeded', got: %s", str)
	}

	if !strings.Contains(str, "retry after") {
		t.Errorf("String should contain 'retry after', got: %s", str)
	}
}

func TestResetAtUnix(t *testing.T) {
	resetAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	result := NewAllowedResult(100, 99, 1, resetAt, time.Hour)

	unixTime := result.ResetAtUnix()

	if unixTime != resetAt.Unix() {
		t.Errorf("Expected Unix time %d, got %d", resetAt.Unix(), unixTime)
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	resetAt := time.Now().Add(45 * time.Second)
	result := NewDeniedResult(100, 100, resetAt, time.Minute)

	seconds := result.RetryAfterSeconds()

	// Should be approximately 45 seconds (allow small variation for test execution time)
	if seconds < 44 || seconds > 46 {
		t.Errorf("Expected ~45 seconds, got %d", seconds)
	}
}

func TestRetryAfterSeconds_Zero(t *testing.T) {
	resetAt := time.Now().Add(-time.Second)
	result := NewDeniedResult(100, 100, resetAt, time.Minute)

	seconds := result.RetryAfterSeconds()

	if seconds != 0 {
		t.Errorf("Expected 0 seconds for past reset time, got %d", seconds)
	}
}

// ============================================================================
// RESULT UTILITY TESTS
// ============================================================================

func TestClone(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	original := NewAllowedResult(100, 75, 25, resetAt, time.Hour)
	original.WithContext(ScopeGlobal, "user123", TierPremium)
	original.WithStrategy(StrategyTokenBucket)
	original.SetMetadata("ip", "192.168.1.1")
	original.SetMetadata("count", 5)

	cloned := original.Clone()

	// Verify all fields are copied
	if cloned.Allowed != original.Allowed {
		t.Error("Allowed not cloned correctly")
	}

	if cloned.Limit != original.Limit {
		t.Error("Limit not cloned correctly")
	}

	if cloned.Remaining != original.Remaining {
		t.Error("Remaining not cloned correctly")
	}

	if cloned.Used != original.Used {
		t.Error("Used not cloned correctly")
	}

	if cloned.RetryAfter != original.RetryAfter {
		t.Error("RetryAfter not cloned correctly")
	}

	if !cloned.ResetAt.Equal(original.ResetAt) {
		t.Error("ResetAt not cloned correctly")
	}

	if cloned.Window != original.Window {
		t.Error("Window not cloned correctly")
	}

	if cloned.Scope != original.Scope {
		t.Error("Scope not cloned correctly")
	}

	if cloned.Entity != original.Entity {
		t.Error("Entity not cloned correctly")
	}

	if cloned.Tier != original.Tier {
		t.Error("Tier not cloned correctly")
	}

	if cloned.Strategy != original.Strategy {
		t.Error("Strategy not cloned correctly")
	}

	// Verify metadata is deep copied
	if val, _ := cloned.GetMetadata("ip"); val != "192.168.1.1" {
		t.Error("Metadata not cloned correctly")
	}

	if val, _ := cloned.GetMetadata("count"); val != 5 {
		t.Error("Metadata count not cloned correctly")
	}

	// Modify cloned metadata and verify original is unchanged
	cloned.SetMetadata("ip", "10.0.0.1")
	if val, _ := original.GetMetadata("ip"); val != "192.168.1.1" {
		t.Error("Modifying cloned metadata affected original")
	}
}

func TestUsagePercentage(t *testing.T) {
	tests := []struct {
		name     string
		limit    int64
		used     int64
		expected float64
	}{
		{"Empty", 100, 0, 0.0},
		{"Half", 100, 50, 50.0},
		{"Full", 100, 100, 100.0},
		{"Over limit", 100, 150, 150.0},
		{"Precise", 100, 33, 33.0},
		{"Zero limit", 0, 10, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAllowedResult(tt.limit, tt.limit-tt.used, tt.used, time.Now().Add(time.Hour), time.Hour)

			percentage := result.UsagePercentage()

			if percentage != tt.expected {
				t.Errorf("Expected %.2f%%, got %.2f%%", tt.expected, percentage)
			}
		})
	}
}

func TestResult_IsNearLimit(t *testing.T) {
	tests := []struct {
		name      string
		used      int64
		threshold float64
		expected  bool
	}{
		{"Well below", 10, 80.0, false},
		{"Just below", 79, 80.0, false},
		{"At threshold", 80, 80.0, true},
		{"Above threshold", 90, 80.0, true},
		{"At limit", 100, 80.0, true},
		{"High threshold", 50, 90.0, false},
		{"Zero threshold", 1, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAllowedResult(100, 100-tt.used, tt.used, time.Now().Add(time.Hour), time.Hour)

			near := result.IsNearLimit(tt.threshold)

			if near != tt.expected {
				t.Errorf("Expected IsNearLimit(%.1f) = %v for %d%% usage, got %v",
					tt.threshold, tt.expected, tt.used, near)
			}
		})
	}
}

func TestTimeUntilReset(t *testing.T) {
	resetAt := time.Now().Add(5 * time.Minute)
	result := NewAllowedResult(100, 99, 1, resetAt, time.Hour)

	timeUntil := result.TimeUntilReset()

	// Should be approximately 5 minutes (allow small variation)
	expected := 5 * time.Minute
	tolerance := 2 * time.Second

	if timeUntil < expected-tolerance || timeUntil > expected+tolerance {
		t.Errorf("Expected ~5 minutes, got %v", timeUntil)
	}
}

func TestTimeUntilReset_Past(t *testing.T) {
	resetAt := time.Now().Add(-5 * time.Minute)
	result := NewAllowedResult(100, 99, 1, resetAt, time.Hour)

	timeUntil := result.TimeUntilReset()

	if timeUntil > 0 {
		t.Errorf("Expected negative or zero duration for past reset, got %v", timeUntil)
	}
}

// ============================================================================
// RESULT BUILDER TESTS
// ============================================================================

func TestNewResultBuilder(t *testing.T) {
	builder := NewResultBuilder()

	if builder == nil {
		t.Fatal("Builder should not be nil")
	}

	if builder.result == nil {
		t.Fatal("Builder result should not be nil")
	}

	// Metadata is now private and always initialized via NewResultBuilder
	metadata := builder.result.GetAllMetadata()
	if metadata == nil {
		t.Error("GetAllMetadata should never return nil")
	}
}

func TestResultBuilder_FluentAPI(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	result := NewResultBuilder().
		Allowed(true).
		Limit(100).
		Remaining(75).
		Used(25).
		RetryAfter(0).
		ResetAt(resetAt).
		Window(window).
		Scope(ScopeGlobal).
		Entity("user123").
		Tier(TierPremium).
		Strategy(StrategyTokenBucket).
		Metadata("ip", "192.168.1.1").
		Metadata("method", "POST").
		Build()

	if !result.Allowed {
		t.Error("Allowed not set correctly")
	}

	if result.Limit != 100 {
		t.Errorf("Expected limit 100, got %d", result.Limit)
	}

	if result.Remaining != 75 {
		t.Errorf("Expected remaining 75, got %d", result.Remaining)
	}

	if result.Used != 25 {
		t.Errorf("Expected used 25, got %d", result.Used)
	}

	if result.RetryAfter != 0 {
		t.Errorf("Expected RetryAfter 0, got %v", result.RetryAfter)
	}

	if !result.ResetAt.Equal(resetAt) {
		t.Errorf("Expected ResetAt %v, got %v", resetAt, result.ResetAt)
	}

	if result.Window != window {
		t.Errorf("Expected window %v, got %v", window, result.Window)
	}

	if result.Scope != ScopeGlobal {
		t.Errorf("Expected scope %s, got %s", ScopeGlobal, result.Scope)
	}

	if result.Entity != "user123" {
		t.Errorf("Expected entity 'user123', got %s", result.Entity)
	}

	if result.Tier != TierPremium {
		t.Errorf("Expected tier %s, got %s", TierPremium, result.Tier)
	}

	if result.Strategy != StrategyTokenBucket {
		t.Errorf("Expected strategy %s, got %s", StrategyTokenBucket, result.Strategy)
	}

	if val, _ := result.GetMetadata("ip"); val != "192.168.1.1" {
		t.Errorf("Expected ip='192.168.1.1', got %v", val)
	}

	if val, _ := result.GetMetadata("method"); val != "POST" {
		t.Errorf("Expected method='POST', got %v", val)
	}
}

func TestResultBuilder_DeniedResult(t *testing.T) {
	result := NewResultBuilder().
		Allowed(false).
		Limit(100).
		Remaining(0).
		Used(100).
		RetryAfter(30 * time.Second).
		Build()

	if result.Allowed {
		t.Error("Result should be denied")
	}

	if result.Remaining != 0 {
		t.Error("Remaining should be 0 for denied result")
	}

	if result.RetryAfter != 30*time.Second {
		t.Errorf("Expected RetryAfter 30s, got %v", result.RetryAfter)
	}
}

// ============================================================================
// RESULT COMPARISON TESTS
// ============================================================================

func TestResultsEqual_Identical(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	a.WithContext(ScopeGlobal, "user123", TierPremium)
	a.WithStrategy(StrategyTokenBucket)

	b := NewAllowedResult(100, 75, 25, resetAt, window)
	b.WithContext(ScopeGlobal, "user123", TierPremium)
	b.WithStrategy(StrategyTokenBucket)

	if !ResultsEqual(a, b) {
		t.Error("Identical results should be equal")
	}
}

func TestResultsEqual_DifferentAllowed(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	b := NewDeniedResult(100, 100, resetAt, window)

	if ResultsEqual(a, b) {
		t.Error("Results with different Allowed should not be equal")
	}
}

func TestResultsEqual_DifferentLimit(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	b := NewAllowedResult(200, 75, 25, resetAt, window)

	if ResultsEqual(a, b) {
		t.Error("Results with different Limit should not be equal")
	}
}

func TestResultsEqual_DifferentRemaining(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	b := NewAllowedResult(100, 50, 25, resetAt, window)

	if ResultsEqual(a, b) {
		t.Error("Results with different Remaining should not be equal")
	}
}

func TestResultsEqual_DifferentUsed(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	b := NewAllowedResult(100, 75, 50, resetAt, window)

	if ResultsEqual(a, b) {
		t.Error("Results with different Used should not be equal")
	}
}

func TestResultsEqual_DifferentRetryAfter(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	a.RetryAfter = 10 * time.Second

	b := NewAllowedResult(100, 75, 25, resetAt, window)
	b.RetryAfter = 20 * time.Second

	if ResultsEqual(a, b) {
		t.Error("Results with different RetryAfter should not be equal")
	}
}

func TestResultsEqual_DifferentResetAt(t *testing.T) {
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, time.Now().Add(time.Hour), window)
	b := NewAllowedResult(100, 75, 25, time.Now().Add(2*time.Hour), window)

	if ResultsEqual(a, b) {
		t.Error("Results with different ResetAt should not be equal")
	}
}

func TestResultsEqual_DifferentWindow(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	a := NewAllowedResult(100, 75, 25, resetAt, time.Hour)
	b := NewAllowedResult(100, 75, 25, resetAt, 2*time.Hour)

	if ResultsEqual(a, b) {
		t.Error("Results with different Window should not be equal")
	}
}

func TestResultsEqual_DifferentContext(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	a.WithContext(ScopeGlobal, "user123", TierPremium)

	b := NewAllowedResult(100, 75, 25, resetAt, window)
	b.WithContext(ScopeAPI, "user456", TierFree)

	if ResultsEqual(a, b) {
		t.Error("Results with different context should not be equal")
	}
}

func TestResultsEqual_DifferentStrategy(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)
	a.WithStrategy(StrategyTokenBucket)

	b := NewAllowedResult(100, 75, 25, resetAt, window)
	b.WithStrategy(StrategySlidingWindow)

	if ResultsEqual(a, b) {
		t.Error("Results with different Strategy should not be equal")
	}
}

func TestResultsEqual_NilResults(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	window := time.Hour

	a := NewAllowedResult(100, 75, 25, resetAt, window)

	if !ResultsEqual(nil, nil) {
		t.Error("Two nil results should be equal")
	}

	if ResultsEqual(a, nil) {
		t.Error("Non-nil and nil results should not be equal")
	}

	if ResultsEqual(nil, a) {
		t.Error("Nil and non-nil results should not be equal")
	}
}

// ============================================================================
// AGGREGATED RESULT TESTS
// ============================================================================

func TestNewAggregatedResult_AllAllowed(t *testing.T) {
	resetAt1 := time.Now().Add(time.Hour)
	resetAt2 := time.Now().Add(2 * time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, resetAt1, time.Hour),
		ScopeAPI:    NewAllowedResult(200, 150, 50, resetAt2, time.Hour),
	}

	agg := NewAggregatedResult(results)

	if !agg.Overall {
		t.Error("Overall should be true when all results are allowed")
	}

	if len(agg.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(agg.Results))
	}

	// LowestRemaining should be ScopeGlobal (75 < 150)
	if agg.LowestRemaining != ScopeGlobal {
		t.Errorf("Expected LowestRemaining to be %s, got %s", ScopeGlobal, agg.LowestRemaining)
	}

	// NextReset should be resetAt1 (earlier)
	if !agg.NextReset.Equal(resetAt1) {
		t.Errorf("Expected NextReset %v, got %v", resetAt1, agg.NextReset)
	}
}

func TestNewAggregatedResult_SomeDenied(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, resetAt, time.Hour),
		ScopeAPI:    NewDeniedResult(200, 200, resetAt, time.Hour),
	}

	agg := NewAggregatedResult(results)

	if agg.Overall {
		t.Error("Overall should be false when any result is denied")
	}
}

func TestNewAggregatedResult_AllDenied(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewDeniedResult(100, 100, resetAt, time.Hour),
		ScopeAPI:    NewDeniedResult(200, 200, resetAt, time.Hour),
	}

	agg := NewAggregatedResult(results)

	if agg.Overall {
		t.Error("Overall should be false when all results are denied")
	}
}

func TestNewAggregatedResult_SingleResult(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, resetAt, time.Hour),
	}

	agg := NewAggregatedResult(results)

	if !agg.Overall {
		t.Error("Overall should be true for single allowed result")
	}

	if agg.LowestRemaining != ScopeGlobal {
		t.Errorf("Expected LowestRemaining to be %s, got %s", ScopeGlobal, agg.LowestRemaining)
	}
}

func TestNewAggregatedResult_EarliestReset(t *testing.T) {
	earliest := time.Now().Add(30 * time.Minute)
	later := time.Now().Add(2 * time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, later, time.Hour),
		ScopeAPI:    NewAllowedResult(200, 150, 50, earliest, time.Hour),
	}

	agg := NewAggregatedResult(results)

	// NextReset should be the earliest reset time
	if !agg.NextReset.Equal(earliest) {
		t.Errorf("Expected NextReset to be earliest (%v), got %v", earliest, agg.NextReset)
	}
}

func TestAggregatedResult_String_Allowed(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, resetAt, time.Hour),
		ScopeAPI:    NewAllowedResult(200, 150, 50, resetAt, time.Hour),
	}

	agg := NewAggregatedResult(results)
	str := agg.String()

	if !strings.Contains(str, "ALLOWED") {
		t.Errorf("String should contain 'ALLOWED', got: %s", str)
	}

	if !strings.Contains(str, "2 scopes") {
		t.Errorf("String should contain '2 scopes', got: %s", str)
	}

	if !strings.Contains(str, "lowest remaining") {
		t.Errorf("String should contain 'lowest remaining', got: %s", str)
	}

	if !strings.Contains(str, "next reset") {
		t.Errorf("String should contain 'next reset', got: %s", str)
	}
}

func TestAggregatedResult_String_Denied(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)

	results := map[string]*Result{
		ScopeGlobal: NewAllowedResult(100, 75, 25, resetAt, time.Hour),
		ScopeAPI:    NewDeniedResult(200, 200, resetAt, time.Hour),
	}

	agg := NewAggregatedResult(results)
	str := agg.String()

	if !strings.Contains(str, "DENIED") {
		t.Errorf("String should contain 'DENIED', got: %s", str)
	}

	if !strings.Contains(str, "2 scopes") {
		t.Errorf("String should contain '2 scopes', got: %s", str)
	}

	if !strings.Contains(str, "one or more scopes exceeded") {
		t.Errorf("String should contain 'one or more scopes exceeded', got: %s", str)
	}
}

// ============================================================================
// EDGE CASE TESTS
// ============================================================================

func TestResult_ChainedWithMethods(t *testing.T) {
	resetAt := time.Now().Add(time.Hour)
	result := NewAllowedResult(100, 99, 1, resetAt, time.Hour)

	// Chain all With* methods
	result.
		WithContext(ScopeGlobal, "user123", TierPremium).
		WithStrategy(StrategyTokenBucket)

	// Metadata methods don't support chaining (SetMetadata/SetMetadataMap don't return *Result)
	result.SetMetadata("key1", "value1")
	result.SetMetadataMap(map[string]interface{}{
		"key2": "value2",
		"key3": 42,
	})

	// Verify all values were set
	if result.Scope != ScopeGlobal {
		t.Error("Scope not set in chain")
	}

	if result.Entity != "user123" {
		t.Error("Entity not set in chain")
	}

	if result.Tier != TierPremium {
		t.Error("Tier not set in chain")
	}

	if result.Strategy != StrategyTokenBucket {
		t.Error("Strategy not set in chain")
	}

	if val, _ := result.GetMetadata("key1"); val != "value1" {
		t.Error("Metadata key1 not set in chain")
	}

	if val, _ := result.GetMetadata("key2"); val != "value2" {
		t.Error("Metadata key2 not set in chain")
	}

	if val, _ := result.GetMetadata("key3"); val != 42 {
		t.Error("Metadata key3 not set in chain")
	}
}

func TestResult_ZeroValues(t *testing.T) {
	result := &Result{}

	// Test methods with zero values
	percentage := result.UsagePercentage()
	if percentage != 0 {
		t.Errorf("Expected 0%% for zero limit, got %.2f%%", percentage)
	}

	near := result.IsNearLimit(50.0)
	if near {
		t.Error("Zero usage should not be near limit")
	}

	unixTime := result.ResetAtUnix()
	// Zero time gives Unix() of large negative number, not 0
	// Just verify it's callable with zero values
	_ = unixTime

	seconds := result.RetryAfterSeconds()
	if seconds != 0 {
		t.Errorf("Expected 0 for zero RetryAfter, got %d", seconds)
	}
}

// ============================================================================
// THREAD SAFETY TESTS - Concurrent metadata access
// ============================================================================

// TestResult_MetadataConcurrentWrites tests concurrent writes to metadata
func TestResult_MetadataConcurrentWrites(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	const (
		numGoroutines      = 100
		writesPerGoroutine = 1000
	)

	var wg sync.WaitGroup

	// Launch many goroutines that all write to metadata
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				key := fmt.Sprintf("goroutine_%d_key_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				result.SetMetadata(key, value)
			}
		}(i)
	}

	wg.Wait()

	// Verify all metadata was written correctly
	expectedKeys := numGoroutines * writesPerGoroutine
	allMetadata := result.GetAllMetadata()
	actualKeys := len(allMetadata)

	if actualKeys != expectedKeys {
		t.Errorf("Race condition detected! Expected %d keys, got %d", expectedKeys, actualKeys)
	}

	// Verify we can read all keys without panic
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < writesPerGoroutine; j++ {
			key := fmt.Sprintf("goroutine_%d_key_%d", i, j)
			if !result.HasMetadata(key) {
				t.Errorf("Missing key: %s", key)
				break
			}
		}
	}
}

// TestResult_MetadataConcurrentReads tests concurrent reads from metadata
func TestResult_MetadataConcurrentReads(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// Pre-populate metadata
	const numKeys = 100
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := fmt.Sprintf("value_%d", i)
		result.SetMetadata(key, value)
	}

	const (
		numGoroutines     = 100
		readsPerGoroutine = 1000
	)

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Launch many goroutines that all read metadata
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < readsPerGoroutine; j++ {
				key := fmt.Sprintf("key_%d", j%numKeys)
				expectedValue := fmt.Sprintf("value_%d", j%numKeys)

				value, exists := result.GetMetadata(key)
				if !exists {
					errors <- fmt.Errorf("goroutine %d: key %s not found", id, key)
					return
				}

				if value != expectedValue {
					errors <- fmt.Errorf("goroutine %d: expected %s, got %v", id, expectedValue, value)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}
}

// TestResult_MetadataMixedReadWrite tests concurrent reads and writes
func TestResult_MetadataMixedReadWrite(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// Pre-populate with some keys
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("initial_key_%d", i)
		result.SetMetadata(key, i)
	}

	const (
		numReaders = 50
		numWriters = 50
		operations = 500
	)

	var wg sync.WaitGroup

	// Launch readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Read initial keys
				key := fmt.Sprintf("initial_key_%d", j%50)
				result.GetMetadata(key)

				// Read potentially new keys
				newKey := fmt.Sprintf("writer_key_%d", j%numWriters)
				result.GetMetadata(newKey)

				// Check existence
				result.HasMetadata(key)

				// Get all metadata
				_ = result.GetAllMetadata()
			}
		}(i)
	}

	// Launch writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := fmt.Sprintf("writer_key_%d", id)
				value := fmt.Sprintf("value_%d_%d", id, j)
				result.SetMetadata(key, value)
			}
		}(i)
	}

	wg.Wait()

	// Verify no panics and data integrity
	allMetadata := result.GetAllMetadata()
	if len(allMetadata) < 50 {
		t.Errorf("Expected at least 50 keys, got %d", len(allMetadata))
	}

	// Verify initial keys still exist
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("initial_key_%d", i)
		if !result.HasMetadata(key) {
			t.Errorf("Initial key lost: %s", key)
		}
	}
}

// TestResult_CloneConcurrent tests concurrent Clone operations
func TestResult_CloneConcurrent(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// Pre-populate metadata
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key_%d", i)
		result.SetMetadata(key, i)
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	clones := make([]*Result, numGoroutines)

	// Launch many goroutines that clone the result
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			clones[id] = result.Clone()
		}(i)
	}

	wg.Wait()

	// Verify all clones have correct metadata count
	for i, clone := range clones {
		metadata := clone.GetAllMetadata()
		if len(metadata) != 100 {
			t.Errorf("Clone %d has %d metadata entries, expected 100", i, len(metadata))
		}
	}
}

// TestResult_WithMetadataConcurrent tests the fluent API under concurrency
func TestResult_WithMetadataConcurrent(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	const numGoroutines = 100
	var wg sync.WaitGroup

	// Launch many goroutines using SetMetadata (thread-safe)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("fluent_%d_%d", id, j)
				result.SetMetadata(key, id*1000+j)
			}
		}(i)
	}

	wg.Wait()

	// Verify data integrity
	expectedKeys := numGoroutines * 100
	allMetadata := result.GetAllMetadata()
	if len(allMetadata) != expectedKeys {
		t.Errorf("Expected %d keys, got %d", expectedKeys, len(allMetadata))
	}
}

// TestResult_SetMetadataMapConcurrent tests SetMetadataMap under concurrency
func TestResult_SetMetadataMapConcurrent(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	const numGoroutines = 50
	var wg sync.WaitGroup

	// Launch many goroutines setting metadata maps
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			batch := make(map[string]interface{})
			for j := 0; j < 20; j++ {
				key := fmt.Sprintf("batch_%d_key_%d", id, j)
				batch[key] = fmt.Sprintf("value_%d_%d", id, j)
			}

			result.SetMetadataMap(batch)
		}(i)
	}

	wg.Wait()

	// Verify all batch keys were written
	allMetadata := result.GetAllMetadata()
	expectedKeys := numGoroutines * 20
	if len(allMetadata) != expectedKeys {
		t.Errorf("Expected %d keys from batches, got %d", expectedKeys, len(allMetadata))
	}
}

// TestResult_StressTest is an aggressive stress test
func TestResult_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// Reduce numbers when race detector is enabled (causes massive overhead)
	// Race detector increases contention significantly
	numGoroutines := 50 // Reasonable for race detector
	operations := 100   // Reasonable for race detector

	var wg sync.WaitGroup
	startTime := time.Now()

	// Mix of all operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				switch j % 5 {
				case 0:
					// Write
					key := fmt.Sprintf("stress_%d_%d", id, j)
					result.SetMetadata(key, j)
				case 1:
					// Read
					key := fmt.Sprintf("stress_%d_%d", id, j-1)
					result.GetMetadata(key)
				case 2:
					// Check existence
					key := fmt.Sprintf("stress_%d_%d", id, j-2)
					result.HasMetadata(key)
				case 3:
					// Get all
					_ = result.GetAllMetadata()
				case 4:
					// Clone
					_ = result.Clone()
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// Performance metric
	totalOps := numGoroutines * operations
	opsPerSecond := float64(totalOps) / duration.Seconds()
	t.Logf("Stress test completed: %d goroutines, %d ops each, %.0f ops/sec",
		numGoroutines, operations, opsPerSecond)

	// Verify no data corruption
	allMetadata := result.GetAllMetadata()
	if len(allMetadata) == 0 {
		t.Error("Stress test resulted in empty metadata")
	}
}

// ============================================================================
// NEW API FUNCTIONALITY TESTS
// ============================================================================

// TestResult_GetMetadata tests basic GetMetadata functionality
func TestResult_GetMetadata(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// Test non-existent key
	_, exists := result.GetMetadata("nonexistent")
	if exists {
		t.Error("Expected key to not exist")
	}

	// Set and get
	result.SetMetadata("test_key", "test_value")
	value, exists := result.GetMetadata("test_key")
	if !exists {
		t.Error("Expected key to exist")
	}
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %v", value)
	}
}

// TestResult_HasMetadata tests HasMetadata functionality
func TestResult_HasMetadata(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	if result.HasMetadata("test") {
		t.Error("Expected HasMetadata to return false for non-existent key")
	}

	result.SetMetadata("test", "value")
	if !result.HasMetadata("test") {
		t.Error("Expected HasMetadata to return true for existing key")
	}
}

// TestResult_GetAllMetadata tests GetAllMetadata returns a copy
func TestResult_GetAllMetadata(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	result.SetMetadata("key1", "value1")
	result.SetMetadata("key2", "value2")

	// Get metadata copy
	metadata := result.GetAllMetadata()

	// Modify the copy
	metadata["key3"] = "value3"
	delete(metadata, "key1")

	// Verify original is unchanged
	if !result.HasMetadata("key1") {
		t.Error("Original metadata was modified when copy was modified")
	}
	if result.HasMetadata("key3") {
		t.Error("Original metadata was modified when copy was modified")
	}
}

// TestResult_CloneIndependence tests Clone creates independent copy
func TestResult_CloneIndependence(t *testing.T) {
	original := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)
	original.SetMetadata("test", "value")

	clone := original.Clone()

	// Modify clone metadata
	clone.SetMetadata("clone_key", "clone_value")
	clone.SetMetadata("test", "modified")

	// Verify original is unchanged
	if original.HasMetadata("clone_key") {
		t.Error("Original was modified when clone was modified")
	}

	value, _ := original.GetMetadata("test")
	if value != "value" {
		t.Error("Original metadata value was modified when clone was modified")
	}
}

// ============================================================================
// P0-4 CONCURRENT ACCESS TESTS - Thread safety verification
// Run with: go test -race to detect race conditions
// ============================================================================

// TestResult_ConcurrentMetadataAccess verifies P0-4 fix:
// Metadata operations must be thread-safe for concurrent access
func TestResult_ConcurrentMetadataAccess(t *testing.T) {
	result := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)
	result.WithContext("api", "user123", "premium")
	result.WithStrategy("token_bucket")

	// P0-4: Concurrent metadata operations should be safe
	const goroutines = 100
	const operations = 1000

	done := make(chan bool, goroutines*2)

	// Launch writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < operations; j++ {
				result.SetMetadata(fmt.Sprintf("key_%d", id), j)
			}
			done <- true
		}(i)
	}

	// Launch readers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < operations; j++ {
				result.GetMetadata(fmt.Sprintf("key_%d", id))
				result.GetAllMetadata() // Also test full map reads
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < goroutines*2; i++ {
		<-done
	}

	// Verify no corruption - all metadata should be accessible
	allMetadata := result.GetAllMetadata()
	if len(allMetadata) == 0 {
		t.Error("P0-4: Metadata was not written correctly")
	}
}

// TestResult_ConcurrentMetadataAndRead verifies concurrent metadata writes
// don't interfere with reading immutable fields
func TestResult_ConcurrentMetadataAndRead(t *testing.T) {
	result := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)
	result.WithContext("api", "user123", "premium")

	const goroutines = 50
	const operations = 1000

	done := make(chan bool, goroutines*2)

	// Launch metadata writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < operations; j++ {
				result.SetMetadata(fmt.Sprintf("writer_%d", id), j)
			}
			done <- true
		}(i)
	}

	// Launch immutable field readers
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < operations; j++ {
				// P0-4: Reading immutable fields should be safe concurrently
				_ = result.Allowed
				_ = result.Limit
				_ = result.Remaining
				_ = result.Used
				_ = result.Scope
				_ = result.Entity
				_ = result.Tier
				_ = result.Strategy
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines*2; i++ {
		<-done
	}

	// Verify immutable fields weren't corrupted
	if result.Limit != 100 {
		t.Errorf("P0-4 FAILURE: Immutable field corrupted: Limit = %d, expected 100", result.Limit)
	}
	if result.Remaining != 90 {
		t.Errorf("P0-4 FAILURE: Immutable field corrupted: Remaining = %d, expected 90", result.Remaining)
	}
}

// TestResult_Clone_ThreadSafety verifies Clone() is thread-safe
func TestResult_Clone_ThreadSafety(t *testing.T) {
	original := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)
	original.WithContext("api", "user123", "premium")
	original.SetMetadata("original", true)

	const goroutines = 50
	done := make(chan *Result, goroutines)

	// Clone concurrently from multiple goroutines
	for i := 0; i < goroutines; i++ {
		go func() {
			cloned := original.Clone()
			done <- cloned
		}()
	}

	// Collect and verify all clones
	for i := 0; i < goroutines; i++ {
		cloned := <-done

		// Verify clone has correct values
		if cloned.Limit != 100 {
			t.Errorf("Clone %d: Limit incorrect, got %d", i, cloned.Limit)
		}
		if cloned.Scope != "api" {
			t.Errorf("Clone %d: Scope incorrect, got %s", i, cloned.Scope)
		}

		// Verify metadata was copied
		val, exists := cloned.GetMetadata("original")
		if !exists {
			t.Errorf("Clone %d: Metadata not copied", i)
		}
		if val != true {
			t.Errorf("Clone %d: Metadata value incorrect, got %v", i, val)
		}
	}
}

// TestResult_SafeUsagePattern demonstrates the P0-4 safe usage pattern
func TestResult_SafeUsagePattern(t *testing.T) {
	// P0-4 SAFE PATTERN DEMONSTRATION
	//
	// Step 1: Create Result
	result := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)

	// Step 2: Configure with WithContext/WithStrategy (BEFORE sharing)
	result.WithContext("api", "user123", "premium")
	result.WithStrategy("token_bucket")

	// Step 3: Share Result across goroutines (now treat as read-only except metadata)
	const readers = 10
	const operations = 1000
	done := make(chan bool, readers)

	for i := 0; i < readers; i++ {
		go func() {
			for j := 0; j < operations; j++ {
				// SAFE: Reading immutable fields
				_ = result.Allowed
				_ = result.Limit
				_ = result.Scope

				// SAFE: Thread-safe metadata operations
				result.SetMetadata("goroutine_id", i)
				_, _ = result.GetMetadata("goroutine_id")

				// SAFE: Read-only helper methods
				_ = result.String()
				_ = result.UsagePercentage()
			}
			done <- true
		}()
	}

	// Wait for all readers
	for i := 0; i < readers; i++ {
		<-done
	}

	// Verify Result still has correct values
	if result.Limit != 100 || result.Scope != "api" {
		t.Error("P0-4: Safe pattern resulted in data corruption")
	}
}

// TestResult_ReadOnlyFields_ConcurrentRead verifies concurrent reads of
// all immutable fields are safe (no race detector warnings)
func TestResult_ReadOnlyFields_ConcurrentRead(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Minute)
	result.WithContext("api", "user123", "premium")
	result.WithStrategy("token_bucket")

	const readers = 100
	const reads = 10000
	done := make(chan bool, readers)

	for i := 0; i < readers; i++ {
		go func() {
			for j := 0; j < reads; j++ {
				// Read all fields concurrently - should be safe
				_ = result.Allowed
				_ = result.Limit
				_ = result.Remaining
				_ = result.Used
				_ = result.RetryAfter
				_ = result.ResetAt
				_ = result.Window
				_ = result.Scope
				_ = result.Entity
				_ = result.Tier
				_ = result.Strategy

				// Call read-only methods
				_ = result.String()
				_ = result.UsagePercentage()
				_ = result.ResetAtUnix()
				_ = result.RetryAfterSeconds()
				_ = result.TimeUntilReset()
			}
			done <- true
		}()
	}

	// Wait for all readers
	for i := 0; i < readers; i++ {
		<-done
	}
}

// TestResult_MetadataIsolation verifies that GetAllMetadata returns a copy
// so external modifications don't affect the Result
func TestResult_MetadataIsolation(t *testing.T) {
	result := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)
	result.SetMetadata("key1", "value1")
	result.SetMetadata("key2", "value2")

	// Get metadata copy
	metadata := result.GetAllMetadata()

	// Modify the returned map (should NOT affect Result)
	metadata["key1"] = "MODIFIED"
	metadata["key3"] = "NEW_KEY"
	delete(metadata, "key2")

	// Verify Result metadata wasn't affected
	val1, exists1 := result.GetMetadata("key1")
	if !exists1 || val1 != "value1" {
		t.Errorf("P0-4 FAILURE: GetAllMetadata didn't return a copy, external modifications affected Result")
	}

	val2, exists2 := result.GetMetadata("key2")
	if !exists2 || val2 != "value2" {
		t.Error("P0-4 FAILURE: Metadata was corrupted by external map modifications")
	}

	_, exists3 := result.GetMetadata("key3")
	if exists3 {
		t.Error("P0-4 FAILURE: External map modifications affected Result")
	}
}

// Benchmark metadata operations under concurrent access
func BenchmarkResult_ConcurrentMetadataAccess(b *testing.B) {
	result := NewAllowedResult(100, 90, 10, time.Now().Add(time.Hour), time.Minute)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			result.SetMetadata(fmt.Sprintf("key_%d", i%10), i)
			result.GetMetadata(fmt.Sprintf("key_%d", i%10))
			i++
		}
	})
}

// TestResult_CloneDeadlock tests for deadlock between Clone and SetMetadata
// This test was added after discovering a P0 deadlock in production code review.
// The deadlock occurred when Clone() read fields without lock protection while
// SetMetadata() held the metadataMu lock, creating contention under high concurrency.
//
// REGRESSION TEST: DO NOT REMOVE - This validates the fix in result.go:317-319
func TestResult_CloneDeadlock(t *testing.T) {
	result := NewAllowedResult(100, 50, 50, time.Now().Add(time.Hour), time.Hour)

	// 5 second timeout - if test doesn't complete, it's deadlocked
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan bool)

	go func() {
		var wg sync.WaitGroup

		// Heavy Clone usage - 100 goroutines, 100 clones each = 10,000 clones
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					cloned := result.Clone()
					// Verify clone is valid
					if cloned == nil {
						t.Error("Clone returned nil")
						return
					}
					if cloned.Limit != 100 {
						t.Errorf("Clone has invalid Limit: %d", cloned.Limit)
					}
				}
			}()
		}

		// Heavy SetMetadata usage - 100 goroutines, 100 sets each = 10,000 sets
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					result.SetMetadata(fmt.Sprintf("key_%d_%d", id, j), j)
				}
			}(i)
		}

		wg.Wait()
		done <- true
	}()

	select {
	case <-done:
		t.Log("✅ No deadlock detected - Clone() and SetMetadata() work correctly under high concurrency")
	case <-ctx.Done():
		t.Fatal("🚨 DEADLOCK DETECTED: Test timed out after 5 seconds. " +
			"This indicates Clone() is deadlocking with SetMetadata(). " +
			"Check result.go:317-319 for proper lock acquisition.")
	}
}
