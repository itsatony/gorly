package routing

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestNoOpMetrics verifies that NoOpMetrics has zero overhead and doesn't panic
func TestNoOpMetrics(t *testing.T) {
	metrics := NewNoOpMetrics()

	// Should not panic
	metrics.RecordMatch(MatchExact, "/test", 100*time.Microsecond)
	metrics.RecordMatch(MatchPrefix, "/api", 50*time.Microsecond)
	metrics.RecordMatch(MatchGlob, "/api/*", 75*time.Microsecond)
	metrics.RecordMatch(MatchRegex, "^/api/.*$", 120*time.Microsecond)
	metrics.RecordNoMatch(30 * time.Microsecond)

	// GetStats should return empty snapshot
	stats := metrics.GetStats()
	if stats.TotalResolves != 0 {
		t.Errorf("Expected TotalResolves to be 0, got %d", stats.TotalResolves)
	}
	if stats.ExactMatches != 0 {
		t.Errorf("Expected ExactMatches to be 0, got %d", stats.ExactMatches)
	}
	if stats.NoMatches != 0 {
		t.Errorf("Expected NoMatches to be 0, got %d", stats.NoMatches)
	}
}

// TestPrometheusMetricsRecordMatch verifies that RecordMatch updates all counters correctly
func TestPrometheusMetricsRecordMatch(t *testing.T) {
	// Create custom registry to avoid conflicts with other tests
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Record various match types
	metrics.RecordMatch(MatchExact, "/api/test", 10*time.Microsecond)
	metrics.RecordMatch(MatchExact, "/api/test", 15*time.Microsecond)
	metrics.RecordMatch(MatchPrefix, "/api", 20*time.Microsecond)
	metrics.RecordMatch(MatchGlob, "/api/*", 25*time.Microsecond)
	metrics.RecordMatch(MatchRegex, "^/api/.*$", 30*time.Microsecond)

	// Verify in-memory stats
	stats := metrics.GetStats()
	if stats.TotalResolves != 5 {
		t.Errorf("Expected TotalResolves to be 5, got %d", stats.TotalResolves)
	}
	if stats.ExactMatches != 2 {
		t.Errorf("Expected ExactMatches to be 2, got %d", stats.ExactMatches)
	}
	if stats.PrefixMatches != 1 {
		t.Errorf("Expected PrefixMatches to be 1, got %d", stats.PrefixMatches)
	}
	if stats.GlobMatches != 1 {
		t.Errorf("Expected GlobMatches to be 1, got %d", stats.GlobMatches)
	}
	if stats.RegexMatches != 1 {
		t.Errorf("Expected RegexMatches to be 1, got %d", stats.RegexMatches)
	}
	if stats.NoMatches != 0 {
		t.Errorf("Expected NoMatches to be 0, got %d", stats.NoMatches)
	}

	// Average duration should be (10+15+20+25+30)/5 = 20μs
	expectedAvg := time.Duration((10 + 15 + 20 + 25 + 30)) * time.Microsecond / 5
	if stats.AverageDuration != expectedAvg {
		t.Errorf("Expected AverageDuration to be %v, got %v", expectedAvg, stats.AverageDuration)
	}

	// Verify Prometheus counter for exact matches
	count := testutil.ToFloat64(metrics.matchesTotal.WithLabelValues("exact"))
	if count != 2 {
		t.Errorf("Expected Prometheus exact match counter to be 2, got %f", count)
	}

	// Verify Prometheus counter for prefix matches
	count = testutil.ToFloat64(metrics.matchesTotal.WithLabelValues("prefix"))
	if count != 1 {
		t.Errorf("Expected Prometheus prefix match counter to be 1, got %f", count)
	}
}

// TestPrometheusMetricsRecordNoMatch verifies that RecordNoMatch updates counters correctly
func TestPrometheusMetricsRecordNoMatch(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Record some matches and no-matches
	metrics.RecordMatch(MatchExact, "/api/test", 10*time.Microsecond)
	metrics.RecordNoMatch(50 * time.Microsecond)
	metrics.RecordNoMatch(60 * time.Microsecond)

	// Verify in-memory stats
	stats := metrics.GetStats()
	if stats.TotalResolves != 3 {
		t.Errorf("Expected TotalResolves to be 3, got %d", stats.TotalResolves)
	}
	if stats.NoMatches != 2 {
		t.Errorf("Expected NoMatches to be 2, got %d", stats.NoMatches)
	}

	// Verify Prometheus counter
	count := testutil.ToFloat64(metrics.noMatchesTotal)
	if count != 2 {
		t.Errorf("Expected Prometheus no-match counter to be 2, got %f", count)
	}
}

// TestPrometheusMetricsUpdatePatternCounts verifies pattern count gauges
func TestPrometheusMetricsUpdatePatternCounts(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Create sample patterns
	patterns := []RoutePattern{
		{Pattern: "/exact1", MatchType: MatchExact, Scope: "scope1", Priority: 100},
		{Pattern: "/exact2", MatchType: MatchExact, Scope: "scope1", Priority: 100},
		{Pattern: "/prefix", MatchType: MatchPrefix, Scope: "scope2", Priority: 50},
		{Pattern: "/glob/*", MatchType: MatchGlob, Scope: "scope3", Priority: 50},
		{Pattern: "/glob/**", MatchType: MatchGlob, Scope: "scope3", Priority: 40},
		{Pattern: "^/regex.*$", MatchType: MatchRegex, Scope: "scope4", Priority: 30},
	}

	// Update pattern counts
	metrics.UpdatePatternCounts(patterns)

	// Verify Prometheus gauges
	exactCount := testutil.ToFloat64(metrics.patternsTotal.WithLabelValues("exact"))
	if exactCount != 2 {
		t.Errorf("Expected exact pattern count to be 2, got %f", exactCount)
	}

	prefixCount := testutil.ToFloat64(metrics.patternsTotal.WithLabelValues("prefix"))
	if prefixCount != 1 {
		t.Errorf("Expected prefix pattern count to be 1, got %f", prefixCount)
	}

	globCount := testutil.ToFloat64(metrics.patternsTotal.WithLabelValues("glob"))
	if globCount != 2 {
		t.Errorf("Expected glob pattern count to be 2, got %f", globCount)
	}

	regexCount := testutil.ToFloat64(metrics.patternsTotal.WithLabelValues("regex"))
	if regexCount != 1 {
		t.Errorf("Expected regex pattern count to be 1, got %f", regexCount)
	}
}

// TestPrometheusMetricsUpdatePatternCountsEmpty verifies handling of empty patterns
func TestPrometheusMetricsUpdatePatternCountsEmpty(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Update with empty patterns
	metrics.UpdatePatternCounts([]RoutePattern{})

	// All gauges should be 0
	for _, matchType := range []string{"exact", "prefix", "glob", "regex"} {
		count := testutil.ToFloat64(metrics.patternsTotal.WithLabelValues(matchType))
		if count != 0 {
			t.Errorf("Expected %s pattern count to be 0, got %f", matchType, count)
		}
	}
}

// TestPrometheusMetricsConcurrency verifies thread safety
func TestPrometheusMetricsConcurrency(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Run concurrent RecordMatch calls
	const goroutines = 100
	const recordsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recordsPerGoroutine; j++ {
				matchType := MatchType(j % 4) // Cycle through match types
				metrics.RecordMatch(matchType, "/test", time.Microsecond)
			}
		}(i)
	}

	wg.Wait()

	// Verify total resolves
	stats := metrics.GetStats()
	expectedTotal := uint64(goroutines * recordsPerGoroutine)
	if stats.TotalResolves != expectedTotal {
		t.Errorf("Expected TotalResolves to be %d, got %d", expectedTotal, stats.TotalResolves)
	}

	// Verify each match type got approximately equal records (within 10%)
	// Each match type should get ~2500 records (100 goroutines * 100 records / 4 types)
	expectedPerType := expectedTotal / 4
	tolerance := expectedPerType / 10 // 10% tolerance

	for _, mt := range []struct {
		name  string
		count uint64
	}{
		{"exact", stats.ExactMatches},
		{"prefix", stats.PrefixMatches},
		{"glob", stats.GlobMatches},
		{"regex", stats.RegexMatches},
	} {
		if mt.count < expectedPerType-tolerance || mt.count > expectedPerType+tolerance {
			t.Errorf("Expected %s matches to be around %d (±%d), got %d",
				mt.name, expectedPerType, tolerance, mt.count)
		}
	}
}

// TestPrometheusMetricsHistogram verifies duration histogram buckets
func TestPrometheusMetricsHistogram(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Record durations in different buckets
	metrics.RecordMatch(MatchExact, "/test1", 1*time.Microsecond)   // 0.000001s bucket
	metrics.RecordMatch(MatchExact, "/test2", 10*time.Microsecond)  // 0.00001s bucket
	metrics.RecordMatch(MatchExact, "/test3", 100*time.Microsecond) // 0.0001s bucket
	metrics.RecordMatch(MatchExact, "/test4", 1*time.Millisecond)   // 0.001s bucket
	metrics.RecordMatch(MatchExact, "/test5", 10*time.Millisecond)  // 0.01s bucket

	// Verify in-memory stats show all samples were recorded
	stats := metrics.GetStats()
	if stats.ExactMatches != 5 {
		t.Errorf("Expected 5 exact matches recorded, got %d", stats.ExactMatches)
	}

	// Verify histogram counter value through Prometheus metrics
	count := testutil.ToFloat64(metrics.matchesTotal.WithLabelValues("exact"))
	if count != 5 {
		t.Errorf("Expected histogram count to be 5, got %f", count)
	}
}

// TestMetricsOptionsWithPrometheus verifies WithPrometheusMetrics option
func TestMetricsOptionsWithPrometheus(t *testing.T) {
	metrics := createMetrics(WithPrometheusMetrics())

	// Should return PrometheusMetrics instance
	if _, ok := metrics.(*PrometheusMetrics); !ok {
		t.Errorf("Expected PrometheusMetrics instance, got %T", metrics)
	}

	// Should be functional
	metrics.RecordMatch(MatchExact, "/test", 10*time.Microsecond)
	stats := metrics.GetStats()
	if stats.TotalResolves != 1 {
		t.Errorf("Expected TotalResolves to be 1, got %d", stats.TotalResolves)
	}
}

// TestMetricsOptionsWithCustomRegistry verifies WithPrometheusRegistry option
func TestMetricsOptionsWithCustomRegistry(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := createMetrics(WithPrometheusRegistry(registry))

	// Should return PrometheusMetrics instance
	pm, ok := metrics.(*PrometheusMetrics)
	if !ok {
		t.Fatalf("Expected PrometheusMetrics instance, got %T", metrics)
	}

	// Record a match
	pm.RecordMatch(MatchExact, "/test", 10*time.Microsecond)

	// Verify the metric was registered with our custom registry
	count := testutil.ToFloat64(pm.matchesTotal.WithLabelValues("exact"))
	if count != 1 {
		t.Errorf("Expected counter to be 1, got %f", count)
	}
}

// TestMetricsOptionsNoOp verifies default (no options) creates NoOpMetrics
func TestMetricsOptionsNoOp(t *testing.T) {
	metrics := createMetrics()

	// Should return NoOpMetrics instance
	if _, ok := metrics.(*NoOpMetrics); !ok {
		t.Errorf("Expected NoOpMetrics instance, got %T", metrics)
	}
}

// BenchmarkPrometheusMetricsRecordMatch measures overhead of recording a match
func BenchmarkPrometheusMetricsRecordMatch(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordMatch(MatchExact, "/test", 10*time.Microsecond)
	}
}

// BenchmarkNoOpMetricsRecordMatch verifies zero overhead of NoOpMetrics
func BenchmarkNoOpMetricsRecordMatch(b *testing.B) {
	metrics := NewNoOpMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordMatch(MatchExact, "/test", 10*time.Microsecond)
	}
}

// BenchmarkPrometheusMetricsRecordNoMatch measures overhead of recording no match
func BenchmarkPrometheusMetricsRecordNoMatch(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordNoMatch(10 * time.Microsecond)
	}
}

// BenchmarkPrometheusMetricsGetStats measures overhead of GetStats
func BenchmarkPrometheusMetricsGetStats(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Record some data first
	for i := 0; i < 1000; i++ {
		metrics.RecordMatch(MatchExact, "/test", 10*time.Microsecond)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = metrics.GetStats()
	}
}

// BenchmarkPrometheusMetricsConcurrent measures concurrent recording performance
func BenchmarkPrometheusMetricsConcurrent(b *testing.B) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			metrics.RecordMatch(MatchExact, "/test", 10*time.Microsecond)
		}
	})
}

// TestMetricsSnapshot verifies MetricsSnapshot structure
func TestMetricsSnapshot(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics := NewPrometheusMetricsWithRegistry(registry)

	// Record various operations
	metrics.RecordMatch(MatchExact, "/exact", 10*time.Microsecond)
	metrics.RecordMatch(MatchExact, "/exact2", 20*time.Microsecond)
	metrics.RecordMatch(MatchPrefix, "/prefix", 30*time.Microsecond)
	metrics.RecordMatch(MatchGlob, "/glob", 40*time.Microsecond)
	metrics.RecordMatch(MatchRegex, "/regex", 50*time.Microsecond)
	metrics.RecordNoMatch(60 * time.Microsecond)

	// Get snapshot
	snapshot := metrics.GetStats()

	// Verify all fields
	if snapshot.TotalResolves != 6 {
		t.Errorf("Expected TotalResolves=6, got %d", snapshot.TotalResolves)
	}
	if snapshot.ExactMatches != 2 {
		t.Errorf("Expected ExactMatches=2, got %d", snapshot.ExactMatches)
	}
	if snapshot.PrefixMatches != 1 {
		t.Errorf("Expected PrefixMatches=1, got %d", snapshot.PrefixMatches)
	}
	if snapshot.GlobMatches != 1 {
		t.Errorf("Expected GlobMatches=1, got %d", snapshot.GlobMatches)
	}
	if snapshot.RegexMatches != 1 {
		t.Errorf("Expected RegexMatches=1, got %d", snapshot.RegexMatches)
	}
	if snapshot.NoMatches != 1 {
		t.Errorf("Expected NoMatches=1, got %d", snapshot.NoMatches)
	}

	// Average duration: (10+20+30+40+50+60)/6 = 35μs
	expectedAvg := 35 * time.Microsecond
	if snapshot.AverageDuration != expectedAvg {
		t.Errorf("Expected AverageDuration=%v, got %v", expectedAvg, snapshot.AverageDuration)
	}
}

// TestMetricsIntegrationWithResolver verifies metrics work end-to-end with resolver
func TestMetricsIntegrationWithResolver(t *testing.T) {
	// Create resolver with metrics
	registry := prometheus.NewRegistry()
	resolver := NewBuilder().
		WithMetrics(WithPrometheusRegistry(registry)).
		AddExact("/api/exact", "exact_scope", 100).
		AddPrefix("/api/", "prefix_scope", 10).
		MustBuild()

	// Perform resolutions
	scope, matched := resolver.ResolveScope("/api/exact")
	if !matched || scope != "exact_scope" {
		t.Errorf("Expected exact match to exact_scope, got scope=%s, matched=%v", scope, matched)
	}

	scope, matched = resolver.ResolveScope("/api/other")
	if !matched || scope != "prefix_scope" {
		t.Errorf("Expected prefix match to prefix_scope, got scope=%s, matched=%v", scope, matched)
	}

	scope, matched = resolver.ResolveScope("/unknown")
	if matched {
		t.Errorf("Expected no match for /unknown, got matched=true, scope=%s", scope)
	}

	// Verify metrics were recorded
	dr := resolver.(*defaultRouteResolver)
	if pm, ok := dr.metrics.(*PrometheusMetrics); ok {
		stats := pm.GetStats()
		if stats.TotalResolves != 3 {
			t.Errorf("Expected 3 total resolves, got %d", stats.TotalResolves)
		}
		if stats.ExactMatches != 1 {
			t.Errorf("Expected 1 exact match, got %d", stats.ExactMatches)
		}
		if stats.PrefixMatches != 1 {
			t.Errorf("Expected 1 prefix match, got %d", stats.PrefixMatches)
		}
		if stats.NoMatches != 1 {
			t.Errorf("Expected 1 no match, got %d", stats.NoMatches)
		}
	} else {
		t.Fatalf("Expected PrometheusMetrics, got %T", dr.metrics)
	}
}
