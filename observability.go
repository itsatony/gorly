// Package ratelimit provides comprehensive observability features
package ratelimit

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/itsatony/gorly/internal/middleware"
)

// Logger interface for custom logging implementations
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
}

// Field represents a structured logging field
type Field struct {
	Key   string
	Value interface{}
}

// DefaultLogger provides a simple console logger
type DefaultLogger struct {
	level LogLevel
	mu    sync.RWMutex
}

// LogLevel represents logging levels
type LogLevel int

const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarn
	LogLevelError
)

// NewDefaultLogger creates a new default console logger
func NewDefaultLogger(level LogLevel) *DefaultLogger {
	return &DefaultLogger{level: level}
}

func (dl *DefaultLogger) log(level LogLevel, msg string, fields ...Field) {
	dl.mu.RLock()
	defer dl.mu.RUnlock()

	if level < dl.level {
		return
	}

	levelStr := ""
	switch level {
	case LogLevelDebug:
		levelStr = "DEBUG"
	case LogLevelInfo:
		levelStr = "INFO"
	case LogLevelWarn:
		levelStr = "WARN"
	case LogLevelError:
		levelStr = "ERROR"
	}

	log.Printf("[%s] %s %v", levelStr, msg, fields)
}

func (dl *DefaultLogger) Debug(msg string, fields ...Field) { dl.log(LogLevelDebug, msg, fields...) }
func (dl *DefaultLogger) Info(msg string, fields ...Field)  { dl.log(LogLevelInfo, msg, fields...) }
func (dl *DefaultLogger) Warn(msg string, fields ...Field)  { dl.log(LogLevelWarn, msg, fields...) }
func (dl *DefaultLogger) Error(msg string, fields ...Field) { dl.log(LogLevelError, msg, fields...) }

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	// Counter metrics
	IncrementRequestTotal(entity, scope string)
	IncrementRequestDenied(entity, scope string)
	IncrementRequestAllowed(entity, scope string)

	// Gauge metrics
	SetRateLimitRemaining(entity, scope string, remaining int64)
	SetRateLimitUsed(entity, scope string, used int64)

	// Histogram metrics
	RecordRequestDuration(entity, scope string, duration time.Duration)
	RecordQueueSize(size int)

	// Health metrics
	SetHealthy(healthy bool)
	IncrementHealthCheck()
}

// PrometheusMetrics implements MetricsCollector for Prometheus
type PrometheusMetrics struct {
	requestTotal       map[string]int64
	requestDenied      map[string]int64
	requestAllowed     map[string]int64
	rateLimitRemaining map[string]int64
	rateLimitUsed      map[string]int64
	requestDurations   []time.Duration
	queueSize          int64
	healthy            int64
	healthChecks       int64
	mu                 sync.RWMutex
}

// NewPrometheusMetrics creates a new Prometheus metrics collector
func NewPrometheusMetrics() *PrometheusMetrics {
	return &PrometheusMetrics{
		requestTotal:       make(map[string]int64),
		requestDenied:      make(map[string]int64),
		requestAllowed:     make(map[string]int64),
		rateLimitRemaining: make(map[string]int64),
		rateLimitUsed:      make(map[string]int64),
		requestDurations:   make([]time.Duration, 0),
		healthy:            1,
	}
}

func (pm *PrometheusMetrics) makeKey(entity, scope string) string {
	return fmt.Sprintf("%s:%s", entity, scope)
}

func (pm *PrometheusMetrics) IncrementRequestTotal(entity, scope string) {
	key := pm.makeKey(entity, scope)
	pm.mu.Lock()
	pm.requestTotal[key]++
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) IncrementRequestDenied(entity, scope string) {
	key := pm.makeKey(entity, scope)
	pm.mu.Lock()
	pm.requestDenied[key]++
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) IncrementRequestAllowed(entity, scope string) {
	key := pm.makeKey(entity, scope)
	pm.mu.Lock()
	pm.requestAllowed[key]++
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) SetRateLimitRemaining(entity, scope string, remaining int64) {
	key := pm.makeKey(entity, scope)
	pm.mu.Lock()
	pm.rateLimitRemaining[key] = remaining
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) SetRateLimitUsed(entity, scope string, used int64) {
	key := pm.makeKey(entity, scope)
	pm.mu.Lock()
	pm.rateLimitUsed[key] = used
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) RecordRequestDuration(entity, scope string, duration time.Duration) {
	pm.mu.Lock()
	pm.requestDurations = append(pm.requestDurations, duration)
	// Keep only last 1000 durations to prevent memory growth
	if len(pm.requestDurations) > 1000 {
		pm.requestDurations = pm.requestDurations[len(pm.requestDurations)-1000:]
	}
	pm.mu.Unlock()
}

func (pm *PrometheusMetrics) RecordQueueSize(size int) {
	atomic.StoreInt64(&pm.queueSize, int64(size))
}

func (pm *PrometheusMetrics) SetHealthy(healthy bool) {
	if healthy {
		atomic.StoreInt64(&pm.healthy, 1)
	} else {
		atomic.StoreInt64(&pm.healthy, 0)
	}
}

func (pm *PrometheusMetrics) IncrementHealthCheck() {
	atomic.AddInt64(&pm.healthChecks, 1)
}

// GetMetrics returns current metrics snapshot
func (pm *PrometheusMetrics) GetMetrics() map[string]interface{} {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	metrics := make(map[string]interface{})

	// Copy counters
	metrics["request_total"] = copyInt64Map(pm.requestTotal)
	metrics["request_denied"] = copyInt64Map(pm.requestDenied)
	metrics["request_allowed"] = copyInt64Map(pm.requestAllowed)
	metrics["rate_limit_remaining"] = copyInt64Map(pm.rateLimitRemaining)
	metrics["rate_limit_used"] = copyInt64Map(pm.rateLimitUsed)

	// Calculate duration statistics
	if len(pm.requestDurations) > 0 {
		var total time.Duration
		for _, d := range pm.requestDurations {
			total += d
		}
		metrics["avg_request_duration"] = total / time.Duration(len(pm.requestDurations))
		metrics["request_duration_samples"] = len(pm.requestDurations)
	}

	metrics["queue_size"] = atomic.LoadInt64(&pm.queueSize)
	metrics["healthy"] = atomic.LoadInt64(&pm.healthy) == 1
	metrics["health_checks"] = atomic.LoadInt64(&pm.healthChecks)

	return metrics
}

func copyInt64Map(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// HealthChecker provides health check functionality
type HealthChecker struct {
	checks []HealthCheck
	mu     sync.RWMutex
}

// HealthCheck represents a single health check
type HealthCheck struct {
	Name     string
	Check    func(context.Context) error
	Timeout  time.Duration
	Critical bool
}

// HealthStatus represents overall health status
type HealthStatus struct {
	Healthy   bool                   `json:"healthy"`
	Status    string                 `json:"status"`
	Timestamp time.Time              `json:"timestamp"`
	Duration  time.Duration          `json:"duration"`
	Checks    map[string]CheckResult `json:"checks"`
}

// CheckResult represents individual check result
type CheckResult struct {
	Healthy  bool          `json:"healthy"`
	Message  string        `json:"message,omitempty"`
	Duration time.Duration `json:"duration"`
	Critical bool          `json:"critical"`
	Error    string        `json:"error,omitempty"`
}

// NewHealthChecker creates a new health checker
func NewHealthChecker() *HealthChecker {
	return &HealthChecker{
		checks: make([]HealthCheck, 0),
	}
}

// AddCheck adds a health check
func (hc *HealthChecker) AddCheck(name string, check func(context.Context) error, timeout time.Duration, critical bool) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.checks = append(hc.checks, HealthCheck{
		Name:     name,
		Check:    check,
		Timeout:  timeout,
		Critical: critical,
	})
}

// CheckHealth performs all health checks
func (hc *HealthChecker) CheckHealth(ctx context.Context) *HealthStatus {
	start := time.Now()

	hc.mu.RLock()
	checks := make([]HealthCheck, len(hc.checks))
	copy(checks, hc.checks)
	hc.mu.RUnlock()

	results := make(map[string]CheckResult)
	allHealthy := true

	for _, check := range checks {
		checkStart := time.Now()

		// Create context with timeout
		checkCtx, cancel := context.WithTimeout(ctx, check.Timeout)

		var err error
		var healthy bool

		// Run check
		err = check.Check(checkCtx)
		healthy = err == nil

		cancel()

		result := CheckResult{
			Healthy:  healthy,
			Duration: time.Since(checkStart),
			Critical: check.Critical,
		}

		if !healthy {
			result.Error = err.Error()
			if check.Critical {
				allHealthy = false
			}
		}

		results[check.Name] = result
	}

	status := "healthy"
	if !allHealthy {
		status = "unhealthy"
	}

	return &HealthStatus{
		Healthy:   allHealthy,
		Status:    status,
		Timestamp: time.Now(),
		Duration:  time.Since(start),
		Checks:    results,
	}
}

// ObservabilityConfig configures observability features
type ObservabilityConfig struct {
	EnableMetrics     bool
	EnableLogging     bool
	EnableHealthCheck bool
	Logger            Logger
	Metrics           MetricsCollector
	HealthChecker     *HealthChecker
	LogLevel          LogLevel
}

// DefaultObservabilityConfig returns a default observability configuration
func DefaultObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		EnableMetrics:     true,
		EnableLogging:     true,
		EnableHealthCheck: true,
		Logger:            NewDefaultLogger(LogLevelInfo),
		Metrics:           NewPrometheusMetrics(),
		HealthChecker:     NewHealthChecker(),
		LogLevel:          LogLevelInfo,
	}
}

// ObservableLimiter wraps a limiter with observability features
type ObservableLimiter struct {
	limiter   Limiter
	config    *ObservabilityConfig
	startTime time.Time
}

// NewObservableLimiter creates a limiter with observability features
func NewObservableLimiter(limiter Limiter, config *ObservabilityConfig) *ObservableLimiter {
	ol := &ObservableLimiter{
		limiter:   limiter,
		config:    config,
		startTime: time.Now(),
	}

	// Add default health checks
	if config.EnableHealthCheck && config.HealthChecker != nil {
		config.HealthChecker.AddCheck("limiter_health", ol.checkLimiterHealth, time.Second*5, true)
		config.HealthChecker.AddCheck("uptime", ol.checkUptime, time.Millisecond*100, false)
	}

	return ol
}

// Check implements the Limiter interface with observability
func (ol *ObservableLimiter) Check(ctx context.Context, entity string, scope ...string) (*LimitResult, error) {
	start := time.Now()

	scopeStr := "global"
	if len(scope) > 0 {
		scopeStr = scope[0]
	}

	// Log request
	if ol.config.EnableLogging {
		ol.config.Logger.Debug("Rate limit check",
			Field{"entity", entity},
			Field{"scope", scopeStr})
	}

	// Record metrics
	if ol.config.EnableMetrics {
		ol.config.Metrics.IncrementRequestTotal(entity, scopeStr)
	}

	// Perform the actual check
	result, err := ol.limiter.Check(ctx, entity, scope...)

	duration := time.Since(start)

	// Record metrics based on result
	if ol.config.EnableMetrics && err == nil {
		if result.Allowed {
			ol.config.Metrics.IncrementRequestAllowed(entity, scopeStr)
		} else {
			ol.config.Metrics.IncrementRequestDenied(entity, scopeStr)
		}

		ol.config.Metrics.SetRateLimitRemaining(entity, scopeStr, result.Remaining)
		ol.config.Metrics.SetRateLimitUsed(entity, scopeStr, result.Used)
		ol.config.Metrics.RecordRequestDuration(entity, scopeStr, duration)
	}

	// Log result
	if ol.config.EnableLogging {
		if err != nil {
			ol.config.Logger.Error("Rate limit check error",
				Field{"entity", entity},
				Field{"scope", scopeStr},
				Field{"error", err.Error()},
				Field{"duration", duration})
		} else if !result.Allowed {
			ol.config.Logger.Warn("Rate limit exceeded",
				Field{"entity", entity},
				Field{"scope", scopeStr},
				Field{"remaining", result.Remaining},
				Field{"retry_after", result.RetryAfter},
				Field{"duration", duration})
		} else {
			ol.config.Logger.Debug("Rate limit check passed",
				Field{"entity", entity},
				Field{"scope", scopeStr},
				Field{"remaining", result.Remaining},
				Field{"duration", duration})
		}
	}

	return result, err
}

// Allow implements the Limiter interface with observability
func (ol *ObservableLimiter) Allow(ctx context.Context, entity string, scope ...string) (bool, error) {
	result, err := ol.Check(ctx, entity, scope...)
	if err != nil {
		return false, err
	}
	return result.Allowed, nil
}

// Stats implements the Limiter interface with observability
func (ol *ObservableLimiter) Stats(ctx context.Context) (*LimitStats, error) {
	stats, err := ol.limiter.Stats(ctx)
	if err != nil && ol.config.EnableLogging {
		ol.config.Logger.Error("Failed to get stats", Field{"error", err.Error()})
	}
	return stats, err
}

// Health implements the Limiter interface with observability
func (ol *ObservableLimiter) Health(ctx context.Context) error {
	if ol.config.EnableHealthCheck {
		ol.config.Metrics.IncrementHealthCheck()
	}

	err := ol.limiter.Health(ctx)

	if ol.config.EnableMetrics {
		ol.config.Metrics.SetHealthy(err == nil)
	}

	return err
}

// GetHealthStatus returns comprehensive health status
func (ol *ObservableLimiter) GetHealthStatus(ctx context.Context) *HealthStatus {
	if !ol.config.EnableHealthCheck || ol.config.HealthChecker == nil {
		return &HealthStatus{
			Healthy:   true,
			Status:    "health_checks_disabled",
			Timestamp: time.Now(),
		}
	}

	return ol.config.HealthChecker.CheckHealth(ctx)
}

// GetMetrics returns current metrics
func (ol *ObservableLimiter) GetMetrics() map[string]interface{} {
	if !ol.config.EnableMetrics {
		return map[string]interface{}{
			"metrics_disabled": true,
		}
	}

	if pm, ok := ol.config.Metrics.(*PrometheusMetrics); ok {
		return pm.GetMetrics()
	}

	return map[string]interface{}{
		"metrics_available": false,
	}
}

// Middleware implements the Limiter interface
func (ol *ObservableLimiter) Middleware() interface{} {
	return ol.limiter.Middleware()
}

// For implements the Limiter interface
func (ol *ObservableLimiter) For(framework middleware.FrameworkType) interface{} {
	return ol.limiter.For(framework)
}

// Close implements the Limiter interface
func (ol *ObservableLimiter) Close() error {
	return ol.limiter.Close()
}

// Private health check methods

func (ol *ObservableLimiter) checkLimiterHealth(ctx context.Context) error {
	return ol.limiter.Health(ctx)
}

func (ol *ObservableLimiter) checkUptime(ctx context.Context) error {
	uptime := time.Since(ol.startTime)
	if uptime < time.Second {
		return fmt.Errorf("service just started")
	}
	return nil
}
