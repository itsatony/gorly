// Package routing provides metrics and observability for pattern-based routing.
package routing

import (
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics provides observability into pattern matching behavior.
// This enables performance tuning, debugging, and operational monitoring.
type Metrics interface {
	// RecordMatch records a successful pattern match
	RecordMatch(matchType MatchType, pattern string, duration time.Duration)

	// RecordNoMatch records when no pattern matched the path
	RecordNoMatch(duration time.Duration)

	// GetStats returns current statistics (optional, for non-Prometheus use)
	GetStats() MetricsSnapshot
}

// MetricsSnapshot provides a point-in-time view of routing metrics.
type MetricsSnapshot struct {
	TotalResolves   uint64
	ExactMatches    uint64
	PrefixMatches   uint64
	GlobMatches     uint64
	RegexMatches    uint64
	NoMatches       uint64
	AverageDuration time.Duration
}

// PrometheusMetrics implements Metrics using Prometheus instrumentation.
type PrometheusMetrics struct {
	// Prometheus metrics
	matchesTotal   *prometheus.CounterVec
	matchDuration  *prometheus.HistogramVec
	patternsTotal  *prometheus.GaugeVec
	noMatchesTotal prometheus.Counter

	// In-memory counters for GetStats()
	totalResolves atomic.Uint64
	exactMatches  atomic.Uint64
	prefixMatches atomic.Uint64
	globMatches   atomic.Uint64
	regexMatches  atomic.Uint64
	noMatches     atomic.Uint64
	totalDuration atomic.Uint64 // nanoseconds
}

// NewPrometheusMetrics creates a new Prometheus-backed metrics collector.
// Metrics are automatically registered with the default Prometheus registry.
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		matchesTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "gorly_routing_matches_total",
				Help: "Total number of pattern matches by type",
			},
			[]string{"match_type"},
		),
		matchDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "gorly_routing_match_duration_seconds",
				Help: "Pattern matching duration in seconds",
				Buckets: []float64{
					0.000001, // 1μs
					0.000005, // 5μs
					0.00001,  // 10μs
					0.00005,  // 50μs
					0.0001,   // 100μs
					0.0005,   // 500μs
					0.001,    // 1ms
					0.005,    // 5ms
					0.01,     // 10ms
				},
			},
			[]string{"match_type"},
		),
		patternsTotal: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "gorly_routing_patterns_total",
				Help: "Total number of configured patterns by type",
			},
			[]string{"match_type"},
		),
		noMatchesTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "gorly_routing_no_matches_total",
				Help: "Total number of requests that matched no pattern",
			},
		),
	}
}

// NewPrometheusMetricsWithRegistry creates metrics with a custom registry.
// Use this when you need to use a non-default Prometheus registry.
func NewPrometheusMetricsWithRegistry(registry prometheus.Registerer) *PrometheusMetrics {
	matchesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gorly_routing_matches_total",
			Help: "Total number of pattern matches by type",
		},
		[]string{"match_type"},
	)

	matchDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "gorly_routing_match_duration_seconds",
			Help: "Pattern matching duration in seconds",
			Buckets: []float64{
				0.000001, 0.000005, 0.00001, 0.00005, 0.0001,
				0.0005, 0.001, 0.005, 0.01,
			},
		},
		[]string{"match_type"},
	)

	patternsTotal := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gorly_routing_patterns_total",
			Help: "Total number of configured patterns by type",
		},
		[]string{"match_type"},
	)

	noMatchesTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gorly_routing_no_matches_total",
			Help: "Total number of requests that matched no pattern",
		},
	)

	registry.MustRegister(matchesTotal, matchDuration, patternsTotal, noMatchesTotal)

	return &PrometheusMetrics{
		matchesTotal:   matchesTotal,
		matchDuration:  matchDuration,
		patternsTotal:  patternsTotal,
		noMatchesTotal: noMatchesTotal,
	}
}

// RecordMatch implements Metrics.RecordMatch
func (m *PrometheusMetrics) RecordMatch(matchType MatchType, pattern string, duration time.Duration) {
	matchTypeStr := matchType.String()

	// Update Prometheus metrics
	m.matchesTotal.WithLabelValues(matchTypeStr).Inc()
	m.matchDuration.WithLabelValues(matchTypeStr).Observe(duration.Seconds())

	// Update in-memory counters for GetStats()
	m.totalResolves.Add(1)
	m.totalDuration.Add(uint64(duration.Nanoseconds()))

	switch matchType {
	case MatchExact:
		m.exactMatches.Add(1)
	case MatchPrefix:
		m.prefixMatches.Add(1)
	case MatchGlob:
		m.globMatches.Add(1)
	case MatchRegex:
		m.regexMatches.Add(1)
	}
}

// RecordNoMatch implements Metrics.RecordNoMatch
func (m *PrometheusMetrics) RecordNoMatch(duration time.Duration) {
	m.noMatchesTotal.Inc()
	m.matchDuration.WithLabelValues("none").Observe(duration.Seconds())

	// Update in-memory counters
	m.totalResolves.Add(1)
	m.noMatches.Add(1)
	m.totalDuration.Add(uint64(duration.Nanoseconds()))
}

// GetStats implements Metrics.GetStats
func (m *PrometheusMetrics) GetStats() MetricsSnapshot {
	totalResolves := m.totalResolves.Load()
	totalDurationNs := m.totalDuration.Load()

	avgDuration := time.Duration(0)
	if totalResolves > 0 {
		avgDuration = time.Duration(totalDurationNs / totalResolves)
	}

	return MetricsSnapshot{
		TotalResolves:   totalResolves,
		ExactMatches:    m.exactMatches.Load(),
		PrefixMatches:   m.prefixMatches.Load(),
		GlobMatches:     m.globMatches.Load(),
		RegexMatches:    m.regexMatches.Load(),
		NoMatches:       m.noMatches.Load(),
		AverageDuration: avgDuration,
	}
}

// UpdatePatternCounts updates the pattern count gauge.
// This should be called after AddPattern() or RemovePattern().
func (m *PrometheusMetrics) UpdatePatternCounts(patterns []RoutePattern) {
	// Count patterns by type
	counts := make(map[MatchType]int)
	for _, p := range patterns {
		counts[p.MatchType]++
	}

	// Update gauges
	for matchType, count := range counts {
		m.patternsTotal.WithLabelValues(matchType.String()).Set(float64(count))
	}

	// Ensure all match types are represented (even if zero)
	for _, mt := range []MatchType{MatchExact, MatchPrefix, MatchGlob, MatchRegex} {
		if _, exists := counts[mt]; !exists {
			m.patternsTotal.WithLabelValues(mt.String()).Set(0)
		}
	}
}

// NoOpMetrics is a metrics implementation that does nothing.
// Use this when you don't want metrics overhead (e.g., testing, minimal deployments).
type NoOpMetrics struct{}

// NewNoOpMetrics creates a no-op metrics collector.
func NewNoOpMetrics() *NoOpMetrics {
	return &NoOpMetrics{}
}

// RecordMatch does nothing (no-op)
func (m *NoOpMetrics) RecordMatch(matchType MatchType, pattern string, duration time.Duration) {}

// RecordNoMatch does nothing (no-op)
func (m *NoOpMetrics) RecordNoMatch(duration time.Duration) {}

// GetStats returns an empty snapshot
func (m *NoOpMetrics) GetStats() MetricsSnapshot {
	return MetricsSnapshot{}
}

// MetricsOption configures metrics behavior
type MetricsOption func(*metricsConfig)

type metricsConfig struct {
	enabled  bool
	registry prometheus.Registerer
}

// WithPrometheusMetrics enables Prometheus metrics with default registry
func WithPrometheusMetrics() MetricsOption {
	return func(c *metricsConfig) {
		c.enabled = true
	}
}

// WithPrometheusRegistry enables Prometheus metrics with custom registry
func WithPrometheusRegistry(registry prometheus.Registerer) MetricsOption {
	return func(c *metricsConfig) {
		c.enabled = true
		c.registry = registry
	}
}

// createMetrics creates the appropriate metrics implementation based on options
func createMetrics(opts ...MetricsOption) Metrics {
	config := &metricsConfig{enabled: false}
	for _, opt := range opts {
		opt(config)
	}

	if !config.enabled {
		return NewNoOpMetrics()
	}

	if config.registry != nil {
		return NewPrometheusMetricsWithRegistry(config.registry)
	}

	return NewPrometheusMetrics()
}
