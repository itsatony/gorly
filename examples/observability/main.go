// examples/observability/main.go - Comprehensive observability example
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("📊 Gorly Observability Features Demonstration")
	fmt.Println("============================================")

	// Create rate limiter with full observability
	baseLimiter, err := ratelimit.APIGateway().
		Limits(map[string]string{
			"global": "100/minute",
			"auth":   "20/minute",
			"upload": "10/minute",
			"search": "50/minute",
		}).
		TierLimits(map[string]string{
			"free":    "50/minute",
			"premium": "500/minute",
		}).
		Build()
	if err != nil {
		log.Fatalf("Failed to build limiter: %v", err)
	}

	// Configure observability
	observabilityConfig := ratelimit.DefaultObservabilityConfig()
	observabilityConfig.LogLevel = ratelimit.LogLevelInfo

	// Wrap with observability features
	limiter := ratelimit.NewObservableLimiter(baseLimiter, observabilityConfig)

	// Create monitoring server
	monitoringServer := ratelimit.NewMonitoringServer(limiter)

	// Create alert manager
	alertManager := ratelimit.NewAlertManager()
	alertManager.SetThreshold("error_rate", 50.0) // Alert if error rate > 50%
	alertManager.SetThreshold("health", 1.0)      // Alert if unhealthy
	alertManager.AddHandler(ratelimit.ConsoleAlertHandler)

	// Setup HTTP server with monitoring endpoints
	setupHTTPServer(limiter, monitoringServer, alertManager)

	// Run simulation to generate metrics
	runSimulation(limiter)

	// Display results
	displayMetrics(limiter, alertManager)
}

func setupHTTPServer(limiter *ratelimit.ObservableLimiter, monitoring *ratelimit.MonitoringServer, alertManager *ratelimit.AlertManager) {
	// Create main application handler
	appMux := http.NewServeMux()

	// API endpoints with different scopes
	appMux.HandleFunc("/api/auth", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Authentication endpoint"))
	})

	appMux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Upload endpoint"))
	})

	appMux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Search endpoint"))
	})

	appMux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("General data endpoint"))
	})

	// Apply rate limiting middleware with custom entity/scope extraction
	rateLimitedApp := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(appMux)

	// Main server with monitoring
	mainMux := http.NewServeMux()

	// Application routes
	mainMux.Handle("/api/", rateLimitedApp)

	// Monitoring routes
	mainMux.Handle("/monitoring/", http.StripPrefix("/monitoring", monitoring))

	// Individual monitoring endpoints for convenience
	mainMux.HandleFunc("/health", ratelimit.HealthCheckHandler(limiter))
	mainMux.HandleFunc("/metrics", ratelimit.MetricsHandler(limiter))
	mainMux.HandleFunc("/metrics/prometheus", ratelimit.PrometheusHandler(limiter))
	mainMux.HandleFunc("/stats", ratelimit.StatsHandler(limiter))

	// Alert endpoint
	mainMux.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		alerts := alertManager.GetAlerts()
		fmt.Fprintf(w, `{"alerts": %v}`, alerts)
	})

	// Info endpoint
	mainMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"service": "Gorly Observability Demo",
			"endpoints": {
				"application": {
					"/api/auth": "Authentication endpoint (20/min limit)",
					"/api/upload": "Upload endpoint (10/min limit)",
					"/api/search": "Search endpoint (50/min limit)",
					"/api/data": "General data endpoint (100/min global limit)"
				},
				"monitoring": {
					"/health": "Health check",
					"/healthz": "Kubernetes health check",
					"/ready": "Readiness check",
					"/metrics": "JSON metrics",
					"/metrics/prometheus": "Prometheus metrics",
					"/stats": "Rate limiting statistics",
					"/alerts": "Current alerts"
				},
				"full_monitoring": {
					"/monitoring/": "Full monitoring server endpoints"
				}
			},
			"instructions": [
				"Try the API endpoints to generate metrics",
				"curl http://localhost:8080/api/data",
				"curl -H 'X-User-Tier: premium' http://localhost:8080/api/search",
				"curl http://localhost:8080/metrics to see metrics",
				"curl http://localhost:8080/health for health status"
			]
		}`))
	})

	fmt.Println("🌐 Starting HTTP server with full observability...")
	fmt.Println("   Main app: http://localhost:8080/")
	fmt.Println("   Health: http://localhost:8080/health")
	fmt.Println("   Metrics: http://localhost:8080/metrics")
	fmt.Println("   Prometheus: http://localhost:8080/metrics/prometheus")
	fmt.Println("   Stats: http://localhost:8080/stats")
	fmt.Println("   Alerts: http://localhost:8080/alerts")

	// Start server in background
	go func() {
		if err := http.ListenAndServe(":8080", mainMux); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	// Wait for server to start
	time.Sleep(time.Millisecond * 100)
}

func runSimulation(limiter *ratelimit.ObservableLimiter) {
	fmt.Println("\n🎮 Running Traffic Simulation...")
	fmt.Println("--------------------------------")

	ctx := context.Background()

	scenarios := []struct {
		name     string
		entity   string
		scope    string
		requests int
		delay    time.Duration
	}{
		{"Free user browsing", "free:user1", "global", 20, time.Millisecond * 100},
		{"Premium user searching", "premium:user2", "search", 15, time.Millisecond * 80},
		{"Anonymous authentication", "ip:192.168.1.100", "auth", 25, time.Millisecond * 50},
		{"Free user uploading", "free:user3", "upload", 15, time.Millisecond * 200},
		{"Premium bulk operations", "premium:user4", "global", 30, time.Millisecond * 60},
		{"Rate limit testing", "ip:192.168.1.200", "global", 50, time.Millisecond * 20},
	}

	for _, scenario := range scenarios {
		fmt.Printf("   🔄 %s (%d requests)...\n", scenario.name, scenario.requests)

		allowed, denied := 0, 0

		for i := 0; i < scenario.requests; i++ {
			result, err := limiter.Check(ctx, scenario.entity, scenario.scope)
			if err != nil {
				fmt.Printf("      ❌ Error: %v\n", err)
				continue
			}

			if result.Allowed {
				allowed++
			} else {
				denied++
			}

			time.Sleep(scenario.delay)
		}

		fmt.Printf("      ✅ Allowed: %d, ❌ Denied: %d\n", allowed, denied)
	}

	fmt.Println("   🎯 Simulation complete!")
}

func displayMetrics(limiter *ratelimit.ObservableLimiter, alertManager *ratelimit.AlertManager) {
	fmt.Println("\n📈 Final Metrics & Health Report")
	fmt.Println("================================")

	// Health Status
	health := limiter.GetHealthStatus(context.Background())
	fmt.Printf("🏥 Health Status: %s\n", health.Status)
	fmt.Printf("   Overall healthy: %t\n", health.Healthy)
	fmt.Printf("   Check duration: %v\n", health.Duration)

	if len(health.Checks) > 0 {
		fmt.Println("   Individual checks:")
		for name, check := range health.Checks {
			status := "✅"
			if !check.Healthy {
				status = "❌"
			}
			fmt.Printf("     %s %s: %v (%v)\n", status, name, check.Healthy, check.Duration)
			if check.Error != "" {
				fmt.Printf("         Error: %s\n", check.Error)
			}
		}
	}

	// Metrics Summary
	metrics := limiter.GetMetrics()
	fmt.Println("\n📊 Metrics Summary:")

	if requestTotal, ok := metrics["request_total"].(map[string]int64); ok {
		fmt.Println("   Request totals by entity:scope:")
		for key, count := range requestTotal {
			fmt.Printf("     %s: %d requests\n", key, count)
		}
	}

	if requestDenied, ok := metrics["request_denied"].(map[string]int64); ok {
		fmt.Println("   Denied requests:")
		for key, count := range requestDenied {
			fmt.Printf("     %s: %d denied\n", key, count)
		}
	}

	if avgDuration, ok := metrics["avg_request_duration"].(time.Duration); ok {
		fmt.Printf("   Average processing time: %v\n", avgDuration)
	}

	if healthy, ok := metrics["healthy"].(bool); ok {
		fmt.Printf("   Service health: %t\n", healthy)
	}

	if healthChecks, ok := metrics["health_checks"].(int64); ok {
		fmt.Printf("   Health checks performed: %d\n", healthChecks)
	}

	// Check for alerts
	alertManager.CheckMetrics(metrics)
	alerts := alertManager.GetAlerts()

	fmt.Printf("\n🚨 Alerts: %d total\n", len(alerts))
	for _, alert := range alerts {
		severity := "ℹ️"
		if alert.Severity == "warning" {
			severity = "⚠️"
		} else if alert.Severity == "critical" {
			severity = "🚨"
		}

		fmt.Printf("   %s [%s] %s: %s\n", severity, alert.Severity, alert.Name, alert.Message)
	}

	// Performance Analysis
	fmt.Println("\n⚡ Performance Analysis:")
	if requestTotal, ok := metrics["request_total"].(map[string]int64); ok {
		if requestDenied, ok := metrics["request_denied"].(map[string]int64); ok {
			var totalRequests, totalDenied int64

			for key, total := range requestTotal {
				totalRequests += total
				if denied, exists := requestDenied[key]; exists {
					totalDenied += denied
				}
			}

			if totalRequests > 0 {
				allowRate := float64(totalRequests-totalDenied) / float64(totalRequests) * 100
				denyRate := float64(totalDenied) / float64(totalRequests) * 100

				fmt.Printf("   Total requests: %d\n", totalRequests)
				fmt.Printf("   Allow rate: %.2f%%\n", allowRate)
				fmt.Printf("   Deny rate: %.2f%%\n", denyRate)

				if denyRate > 30 {
					fmt.Printf("   ⚠️  High deny rate detected - consider adjusting limits\n")
				} else if denyRate < 5 {
					fmt.Printf("   ✅ Rate limiting working effectively\n")
				} else {
					fmt.Printf("   ✅ Reasonable deny rate\n")
				}
			}
		}
	}

	fmt.Println("\n🎯 Observability Demo Complete!")
	fmt.Println("   Server running at http://localhost:8080")
	fmt.Println("   Try the endpoints to see live metrics!")
	fmt.Println("   Press Ctrl+C to stop")

	// Keep server running
	select {}
}
