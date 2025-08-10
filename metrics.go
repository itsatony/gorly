// metrics.go
package ratelimit

import (
	"sync"
	"time"
)

// Metrics provides metrics collection for rate limiting operations
// This is a placeholder implementation until Prometheus metrics are integrated
type Metrics struct {
	prefix string
	mu     sync.RWMutex

	// Request metrics
	requestsTotal   map[string]int64
	requestsAllowed map[string]int64
	requestsDenied  map[string]int64

	// Latency metrics
	latencySum   map[string]time.Duration
	latencyCount map[string]int64

	// Rate limit metrics
	rateLimitHits map[string]int64

	// Error metrics
	errors map[string]int64
}

// NewMetrics creates a new metrics collector
func NewMetrics(prefix string) *Metrics {
	return &Metrics{
		prefix:          prefix,
		requestsTotal:   make(map[string]int64),
		requestsAllowed: make(map[string]int64),
		requestsDenied:  make(map[string]int64),
		latencySum:      make(map[string]time.Duration),
		latencyCount:    make(map[string]int64),
		rateLimitHits:   make(map[string]int64),
		errors:          make(map[string]int64),
	}
}

// RecordRequest records a request with its outcome and latency
func (m *Metrics) RecordRequest(entityType, tier, scope string, allowed bool, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(entityType, tier, scope)

	m.requestsTotal[key]++
	m.latencySum[key] += duration
	m.latencyCount[key]++

	if allowed {
		m.requestsAllowed[key]++
	} else {
		m.requestsDenied[key]++
	}
}

// RecordRateLimit records a rate limit hit
func (m *Metrics) RecordRateLimit(entityType, tier, scope string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.buildKey(entityType, tier, scope)
	m.rateLimitHits[key]++
}

// RecordError records an error
func (m *Metrics) RecordError(entityType, scope string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errorType string
	if rlErr, ok := err.(*RateLimitError); ok {
		errorType = string(rlErr.Type)
	} else {
		errorType = "unknown"
	}

	key := m.buildErrorKey(entityType, scope, errorType)
	m.errors[key]++
}

// GetMetrics returns current metrics snapshot
func (m *Metrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := make(map[string]interface{})

	// Copy all metrics
	metrics["requests_total"] = copyMap(m.requestsTotal)
	metrics["requests_allowed"] = copyMap(m.requestsAllowed)
	metrics["requests_denied"] = copyMap(m.requestsDenied)
	metrics["rate_limit_hits"] = copyMap(m.rateLimitHits)
	metrics["errors"] = copyMap(m.errors)

	// Calculate average latencies
	avgLatencies := make(map[string]float64)
	for key, sum := range m.latencySum {
		if count := m.latencyCount[key]; count > 0 {
			avgLatencies[key] = float64(sum.Nanoseconds()) / float64(count) / 1000000 // Convert to milliseconds
		}
	}
	metrics["average_latency_ms"] = avgLatencies

	return metrics
}

// Reset resets all metrics
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestsTotal = make(map[string]int64)
	m.requestsAllowed = make(map[string]int64)
	m.requestsDenied = make(map[string]int64)
	m.latencySum = make(map[string]time.Duration)
	m.latencyCount = make(map[string]int64)
	m.rateLimitHits = make(map[string]int64)
	m.errors = make(map[string]int64)
}

// buildKey builds a metrics key from entity type, tier, and scope
func (m *Metrics) buildKey(entityType, tier, scope string) string {
	return entityType + "_" + tier + "_" + scope
}

// buildErrorKey builds an error metrics key
func (m *Metrics) buildErrorKey(entityType, scope, errorType string) string {
	return entityType + "_" + scope + "_" + errorType
}

// copyMap creates a copy of a map[string]int64
func copyMap(original map[string]int64) map[string]int64 {
	copy := make(map[string]int64)
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// MetricsSnapshot represents a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	Timestamp        time.Time          `json:"timestamp"`
	RequestsTotal    map[string]int64   `json:"requests_total"`
	RequestsAllowed  map[string]int64   `json:"requests_allowed"`
	RequestsDenied   map[string]int64   `json:"requests_denied"`
	RateLimitHits    map[string]int64   `json:"rate_limit_hits"`
	Errors           map[string]int64   `json:"errors"`
	AverageLatencyMs map[string]float64 `json:"average_latency_ms"`
}

// Snapshot returns a metrics snapshot
func (m *Metrics) Snapshot() *MetricsSnapshot {
	metrics := m.GetMetrics()

	return &MetricsSnapshot{
		Timestamp:        time.Now(),
		RequestsTotal:    metrics["requests_total"].(map[string]int64),
		RequestsAllowed:  metrics["requests_allowed"].(map[string]int64),
		RequestsDenied:   metrics["requests_denied"].(map[string]int64),
		RateLimitHits:    metrics["rate_limit_hits"].(map[string]int64),
		Errors:           metrics["errors"].(map[string]int64),
		AverageLatencyMs: metrics["average_latency_ms"].(map[string]float64),
	}
}
