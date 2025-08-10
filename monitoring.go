// Package ratelimit provides monitoring and HTTP handlers for observability
package ratelimit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MonitoringServer provides HTTP endpoints for metrics and health checks
type MonitoringServer struct {
	limiter *ObservableLimiter
	mux     *http.ServeMux
}

// NewMonitoringServer creates a new monitoring server
func NewMonitoringServer(limiter *ObservableLimiter) *MonitoringServer {
	ms := &MonitoringServer{
		limiter: limiter,
		mux:     http.NewServeMux(),
	}

	ms.setupRoutes()
	return ms
}

// ServeHTTP implements http.Handler
func (ms *MonitoringServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ms.mux.ServeHTTP(w, r)
}

// GetHandler returns the HTTP handler
func (ms *MonitoringServer) GetHandler() http.Handler {
	return ms.mux
}

func (ms *MonitoringServer) setupRoutes() {
	ms.mux.HandleFunc("/health", ms.handleHealth)
	ms.mux.HandleFunc("/healthz", ms.handleHealth) // Kubernetes standard
	ms.mux.HandleFunc("/ready", ms.handleReady)
	ms.mux.HandleFunc("/metrics", ms.handleMetrics)
	ms.mux.HandleFunc("/metrics/prometheus", ms.handlePrometheusMetrics)
	ms.mux.HandleFunc("/stats", ms.handleStats)
	ms.mux.HandleFunc("/debug", ms.handleDebug)
	ms.mux.HandleFunc("/", ms.handleIndex)
}

// handleHealth returns health check status
func (ms *MonitoringServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := ms.limiter.GetHealthStatus(r.Context())

	w.Header().Set("Content-Type", "application/json")

	if status.Healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// handleReady returns readiness status (similar to health for now)
func (ms *MonitoringServer) handleReady(w http.ResponseWriter, r *http.Request) {
	// For rate limiters, ready is essentially the same as healthy
	ms.handleHealth(w, r)
}

// handleMetrics returns JSON metrics
func (ms *MonitoringServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ms.limiter.GetMetrics()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"metrics":   metrics,
	})
}

// handlePrometheusMetrics returns Prometheus-formatted metrics
func (ms *MonitoringServer) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := ms.limiter.GetMetrics()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Convert metrics to Prometheus format
	prometheus := ms.convertToPrometheusFormat(metrics)
	w.Write([]byte(prometheus))
}

// handleStats returns comprehensive statistics
func (ms *MonitoringServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := ms.limiter.Stats(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"stats":     stats,
	})
}

// handleDebug returns debug information
func (ms *MonitoringServer) handleDebug(w http.ResponseWriter, r *http.Request) {
	health := ms.limiter.GetHealthStatus(r.Context())
	metrics := ms.limiter.GetMetrics()

	debug := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"health":    health,
		"metrics":   metrics,
		"config": map[string]interface{}{
			"metrics_enabled":       ms.limiter.config.EnableMetrics,
			"logging_enabled":       ms.limiter.config.EnableLogging,
			"health_checks_enabled": ms.limiter.config.EnableHealthCheck,
			"log_level":             ms.limiter.config.LogLevel,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(debug)
}

// handleIndex returns available endpoints
func (ms *MonitoringServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	endpoints := map[string]interface{}{
		"service": "Gorly Rate Limiter Monitoring",
		"version": "1.0.0",
		"endpoints": map[string]string{
			"/health":             "Health check status (JSON)",
			"/healthz":            "Health check status (Kubernetes standard)",
			"/ready":              "Readiness check status",
			"/metrics":            "Metrics in JSON format",
			"/metrics/prometheus": "Metrics in Prometheus format",
			"/stats":              "Rate limiting statistics",
			"/debug":              "Debug information",
		},
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(endpoints)
}

// convertToPrometheusFormat converts metrics to Prometheus text format
func (ms *MonitoringServer) convertToPrometheusFormat(metrics map[string]interface{}) string {
	var lines []string

	// Add metadata
	lines = append(lines, "# HELP gorly_info Information about Gorly rate limiter")
	lines = append(lines, "# TYPE gorly_info gauge")
	lines = append(lines, fmt.Sprintf("gorly_info{version=\"1.0.0\"} 1"))
	lines = append(lines, "")

	// Process request counters
	if requestTotal, ok := metrics["request_total"].(map[string]int64); ok {
		lines = append(lines, "# HELP gorly_requests_total Total number of rate limit checks")
		lines = append(lines, "# TYPE gorly_requests_total counter")
		for key, value := range requestTotal {
			entity, scope := parseKey(key)
			lines = append(lines, fmt.Sprintf("gorly_requests_total{entity=\"%s\",scope=\"%s\"} %d", entity, scope, value))
		}
		lines = append(lines, "")
	}

	if requestDenied, ok := metrics["request_denied"].(map[string]int64); ok {
		lines = append(lines, "# HELP gorly_requests_denied_total Total number of denied requests")
		lines = append(lines, "# TYPE gorly_requests_denied_total counter")
		for key, value := range requestDenied {
			entity, scope := parseKey(key)
			lines = append(lines, fmt.Sprintf("gorly_requests_denied_total{entity=\"%s\",scope=\"%s\"} %d", entity, scope, value))
		}
		lines = append(lines, "")
	}

	if requestAllowed, ok := metrics["request_allowed"].(map[string]int64); ok {
		lines = append(lines, "# HELP gorly_requests_allowed_total Total number of allowed requests")
		lines = append(lines, "# TYPE gorly_requests_allowed_total counter")
		for key, value := range requestAllowed {
			entity, scope := parseKey(key)
			lines = append(lines, fmt.Sprintf("gorly_requests_allowed_total{entity=\"%s\",scope=\"%s\"} %d", entity, scope, value))
		}
		lines = append(lines, "")
	}

	// Process gauge metrics
	if rateLimitRemaining, ok := metrics["rate_limit_remaining"].(map[string]int64); ok {
		lines = append(lines, "# HELP gorly_rate_limit_remaining Current remaining requests in rate limit window")
		lines = append(lines, "# TYPE gorly_rate_limit_remaining gauge")
		for key, value := range rateLimitRemaining {
			entity, scope := parseKey(key)
			lines = append(lines, fmt.Sprintf("gorly_rate_limit_remaining{entity=\"%s\",scope=\"%s\"} %d", entity, scope, value))
		}
		lines = append(lines, "")
	}

	if rateLimitUsed, ok := metrics["rate_limit_used"].(map[string]int64); ok {
		lines = append(lines, "# HELP gorly_rate_limit_used Current used requests in rate limit window")
		lines = append(lines, "# TYPE gorly_rate_limit_used gauge")
		for key, value := range rateLimitUsed {
			entity, scope := parseKey(key)
			lines = append(lines, fmt.Sprintf("gorly_rate_limit_used{entity=\"%s\",scope=\"%s\"} %d", entity, scope, value))
		}
		lines = append(lines, "")
	}

	// Process duration metrics
	if avgDuration, ok := metrics["avg_request_duration"].(time.Duration); ok {
		lines = append(lines, "# HELP gorly_request_duration_seconds Average request processing duration")
		lines = append(lines, "# TYPE gorly_request_duration_seconds gauge")
		lines = append(lines, fmt.Sprintf("gorly_request_duration_seconds %f", avgDuration.Seconds()))
		lines = append(lines, "")
	}

	// Process health metrics
	if healthy, ok := metrics["healthy"].(bool); ok {
		lines = append(lines, "# HELP gorly_healthy Whether the rate limiter is healthy")
		lines = append(lines, "# TYPE gorly_healthy gauge")
		healthValue := "0"
		if healthy {
			healthValue = "1"
		}
		lines = append(lines, fmt.Sprintf("gorly_healthy %s", healthValue))
		lines = append(lines, "")
	}

	if healthChecks, ok := metrics["health_checks"].(int64); ok {
		lines = append(lines, "# HELP gorly_health_checks_total Total number of health checks performed")
		lines = append(lines, "# TYPE gorly_health_checks_total counter")
		lines = append(lines, fmt.Sprintf("gorly_health_checks_total %d", healthChecks))
		lines = append(lines, "")
	}

	// Process queue size
	if queueSize, ok := metrics["queue_size"].(int64); ok {
		lines = append(lines, "# HELP gorly_queue_size Current queue size")
		lines = append(lines, "# TYPE gorly_queue_size gauge")
		lines = append(lines, fmt.Sprintf("gorly_queue_size %d", queueSize))
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// parseKey splits "entity:scope" back into entity and scope
func parseKey(key string) (string, string) {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return key, "unknown"
}

// HealthCheckHandler creates a simple health check handler
func HealthCheckHandler(limiter Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := limiter.Health(r.Context())

		w.Header().Set("Content-Type", "application/json")

		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"healthy":   false,
				"error":     err.Error(),
				"timestamp": time.Now().Unix(),
			})
		} else {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"healthy":   true,
				"timestamp": time.Now().Unix(),
			})
		}
	}
}

// MetricsHandler creates a simple metrics handler
func MetricsHandler(limiter *ObservableLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics := limiter.GetMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"metrics":   metrics,
		})
	}
}

// PrometheusHandler creates a Prometheus metrics handler
func PrometheusHandler(limiter *ObservableLimiter) http.HandlerFunc {
	ms := &MonitoringServer{limiter: limiter}

	return func(w http.ResponseWriter, r *http.Request) {
		ms.handlePrometheusMetrics(w, r)
	}
}

// StatsHandler creates a statistics handler
func StatsHandler(limiter Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats, err := limiter.Stats(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting stats: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"timestamp": time.Now().Unix(),
			"stats":     stats,
		})
	}
}

// MonitoringMiddleware adds basic monitoring to any HTTP handler
func MonitoringMiddleware(limiter *ObservableLimiter, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer that captures status code
		recorder := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Serve the request
		handler.ServeHTTP(recorder, r)

		// Record metrics
		duration := time.Since(start)

		if limiter.config.EnableMetrics {
			// Record request duration
			limiter.config.Metrics.RecordRequestDuration("http", "request", duration)

			// You could also record status code metrics here
			// limiter.config.Metrics.IncrementStatusCode(recorder.statusCode)
		}

		if limiter.config.EnableLogging {
			limiter.config.Logger.Debug("HTTP request processed",
				Field{"method", r.Method},
				Field{"path", r.URL.Path},
				Field{"status", recorder.statusCode},
				Field{"duration", duration})
		}
	})
}

// statusRecorder captures HTTP response status codes
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

// AlertManager provides basic alerting functionality
type AlertManager struct {
	alerts    []Alert
	handlers  []AlertHandler
	threshold map[string]float64
}

// Alert represents an alert condition
type Alert struct {
	Name      string                 `json:"name"`
	Message   string                 `json:"message"`
	Severity  string                 `json:"severity"`
	Timestamp time.Time              `json:"timestamp"`
	Resolved  bool                   `json:"resolved"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// AlertHandler defines how alerts are handled
type AlertHandler func(Alert)

// NewAlertManager creates a new alert manager
func NewAlertManager() *AlertManager {
	return &AlertManager{
		alerts:    make([]Alert, 0),
		handlers:  make([]AlertHandler, 0),
		threshold: make(map[string]float64),
	}
}

// AddHandler adds an alert handler
func (am *AlertManager) AddHandler(handler AlertHandler) {
	am.handlers = append(am.handlers, handler)
}

// SetThreshold sets an alert threshold
func (am *AlertManager) SetThreshold(name string, threshold float64) {
	am.threshold[name] = threshold
}

// CheckMetrics checks metrics against thresholds and triggers alerts
func (am *AlertManager) CheckMetrics(metrics map[string]interface{}) {
	// Check error rate
	if requestTotal, ok := metrics["request_total"].(map[string]int64); ok {
		if requestDenied, ok := metrics["request_denied"].(map[string]int64); ok {
			for key := range requestTotal {
				total := requestTotal[key]
				denied := requestDenied[key]

				if total > 0 {
					errorRate := float64(denied) / float64(total) * 100
					if threshold, exists := am.threshold["error_rate"]; exists && errorRate > threshold {
						am.triggerAlert(Alert{
							Name:      "High Error Rate",
							Message:   fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%% for %s", errorRate, threshold, key),
							Severity:  "warning",
							Timestamp: time.Now(),
							Metadata: map[string]interface{}{
								"key":        key,
								"error_rate": errorRate,
								"threshold":  threshold,
								"total":      total,
								"denied":     denied,
							},
						})
					}
				}
			}
		}
	}

	// Check if service is unhealthy
	if healthy, ok := metrics["healthy"].(bool); ok && !healthy {
		if threshold, exists := am.threshold["health"]; exists && threshold > 0 {
			am.triggerAlert(Alert{
				Name:      "Service Unhealthy",
				Message:   "Rate limiter health check failed",
				Severity:  "critical",
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"healthy": healthy,
				},
			})
		}
	}
}

func (am *AlertManager) triggerAlert(alert Alert) {
	am.alerts = append(am.alerts, alert)

	// Trigger all handlers
	for _, handler := range am.handlers {
		handler(alert)
	}
}

// GetAlerts returns current alerts
func (am *AlertManager) GetAlerts() []Alert {
	return am.alerts
}

// ConsoleAlertHandler logs alerts to console
func ConsoleAlertHandler(alert Alert) {
	fmt.Printf("[ALERT] %s - %s: %s\n", alert.Severity, alert.Name, alert.Message)
}

// HTTPAlertHandler sends alerts to an HTTP endpoint
func HTTPAlertHandler(endpoint string) AlertHandler {
	return func(alert Alert) {
		// In a real implementation, you would send the alert to an HTTP endpoint
		// For now, just log it
		fmt.Printf("[HTTP ALERT to %s] %s\n", endpoint, alert.Message)
	}
}
