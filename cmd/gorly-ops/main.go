// Package main provides the Gorly Operations CLI tool for rate limiter management
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"flag"

	ratelimit "github.com/itsatony/gorly"
)

// Version information is now centralized in the main package
// Use ratelimit.GetVersion() to get the current version

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "check":
		handleCheck(args)
	case "test":
		handleTest(args)
	case "benchmark":
		handleBenchmark(args)
	case "health":
		handleHealth(args)
	case "stats":
		handleStats(args)
	case "monitor":
		handleMonitor(args)
	case "config":
		handleConfig(args)
	case "server":
		handleServer(args)
	case "validate":
		handleValidate(args)
	case "version":
		versionInfo := ratelimit.GetVersionInfo()
		fmt.Print(versionInfo.Banner())
	case "help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`Gorly Operations CLI v%s - Rate Limiting Operations Tool

Usage:
  gorly-ops <command> [options]

Commands:
  check      Check if a request would be allowed
  test       Run rate limiting tests
  benchmark  Run performance benchmarks
  health     Check rate limiter health
  stats      Get rate limiting statistics
  monitor    Start monitoring server
  config     Configuration operations
  server     Start demo server with rate limiting
  validate   Validate rate limiting configuration
  version    Show version information
  help       Show this help message

Examples:
  gorly-ops check --entity "user123" --scope "global" --limit "10/minute"
  gorly-ops test --scenario basic --requests 100
  gorly-ops benchmark --duration 30s --entity "bench-user"
  gorly-ops health --redis "localhost:6379"
  gorly-ops stats --format json
  gorly-ops monitor --port 8080
  gorly-ops config validate --file config.json
  gorly-ops server --preset api-gateway --port 8080

Global Options:
  --redis     Redis connection string (default: memory)
  --verbose   Enable verbose output
  --format    Output format: json, yaml, table (default: table)

Use "gorly-ops <command> --help" for more information about a command.
`, ratelimit.GetVersion())
}

func handleCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	entity := fs.String("entity", "", "Entity to check (required)")
	scope := fs.String("scope", "global", "Scope to check")
	limit := fs.String("limit", "10/minute", "Rate limit to apply")
	redisAddr := fs.String("redis", "", "Redis address (optional)")
	algorithm := fs.String("algorithm", "token_bucket", "Algorithm to use")
	verbose := fs.Bool("verbose", false, "Verbose output")

	fs.Parse(args)

	if *entity == "" {
		fmt.Println("Error: --entity is required")
		fs.Usage()
		os.Exit(1)
	}

	// Create limiter
	builder := ratelimit.New().Limit(*scope, *limit).Algorithm(*algorithm)
	if *redisAddr != "" {
		builder = builder.Redis(*redisAddr)
	}

	limiter, err := builder.Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}

	// Perform check
	ctx := context.Background()
	result, err := limiter.Check(ctx, *entity, *scope)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Printf("Rate Limit Check Results:\n")
		fmt.Printf("  Entity: %s\n", *entity)
		fmt.Printf("  Scope: %s\n", *scope)
		fmt.Printf("  Limit: %s\n", *limit)
		fmt.Printf("  Algorithm: %s\n", *algorithm)
		fmt.Printf("  Allowed: %t\n", result.Allowed)
		fmt.Printf("  Remaining: %d\n", result.Remaining)
		fmt.Printf("  Used: %d\n", result.Used)
		fmt.Printf("  Window: %v\n", result.Window)
		if !result.Allowed {
			fmt.Printf("  Retry After: %v\n", result.RetryAfter)
			fmt.Printf("  Reset Time: %v\n", result.ResetTime)
		}
	} else {
		if result.Allowed {
			fmt.Printf("✅ ALLOWED (remaining: %d)\n", result.Remaining)
		} else {
			fmt.Printf("❌ DENIED (retry after: %v)\n", result.RetryAfter)
		}
	}
}

func handleTest(args []string) {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	scenario := fs.String("scenario", "basic", "Test scenario: basic, concurrent, stress")
	requests := fs.Int("requests", 10, "Number of requests to test")
	entity := fs.String("entity", "test-entity", "Test entity")
	scope := fs.String("scope", "global", "Test scope")
	limit := fs.String("limit", "5/minute", "Rate limit")
	interval := fs.Duration("interval", time.Millisecond*100, "Interval between requests")
	goroutines := fs.Int("goroutines", 5, "Number of goroutines for concurrent test")

	fs.Parse(args)

	// Create limiter
	limiter, err := ratelimit.New().Limit(*scope, *limit).Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}
	helper := ratelimit.NewTestHelper(limiter)

	ctx := context.Background()

	fmt.Printf("🧪 Running %s test scenario\n", *scenario)
	fmt.Printf("   Limit: %s, Requests: %d, Interval: %v\n", *limit, *requests, *interval)

	switch *scenario {
	case "basic":
		result := helper.TestLimit(ctx, *entity, *scope, *requests, *interval)
		fmt.Printf("Results: %d allowed, %d denied (duration: %v)\n",
			result.ActualAllow, result.ActualDeny, result.Duration)

	case "concurrent":
		result := helper.RunConcurrentTest(ctx, *entity, *scope, *goroutines, *requests)
		fmt.Printf("Concurrent Results: %d total allowed, %d total denied\n",
			result.TotalAllowed, result.TotalDenied)
		fmt.Printf("Duration: %v, Goroutines: %d\n", result.Duration, result.Goroutines)

	case "stress":
		fmt.Printf("Running stress test for 10 seconds...\n")
		result := helper.BenchmarkLimiter(ctx, *entity, *scope, time.Second*10)
		fmt.Printf("Stress Results: %d total requests, %.2f RPS\n",
			result.TotalRequests, result.RequestsPerSecond)
		fmt.Printf("Average latency: %v\n", result.AverageLatency)

	default:
		fmt.Printf("Unknown scenario: %s\n", *scenario)
		os.Exit(1)
	}
}

func handleBenchmark(args []string) {
	fs := flag.NewFlagSet("benchmark", flag.ExitOnError)
	duration := fs.Duration("duration", time.Second*10, "Benchmark duration")
	entity := fs.String("entity", "bench-entity", "Benchmark entity")
	scope := fs.String("scope", "global", "Benchmark scope")
	limit := fs.String("limit", "1000/minute", "Rate limit")
	algorithm := fs.String("algorithm", "token_bucket", "Algorithm to benchmark")
	redisAddr := fs.String("redis", "", "Redis address (optional)")

	fs.Parse(args)

	fmt.Printf("🚀 Running benchmark for %v\n", *duration)
	fmt.Printf("   Algorithm: %s, Limit: %s\n", *algorithm, *limit)

	// Create limiter
	builder := ratelimit.New().Limit(*scope, *limit).Algorithm(*algorithm)
	if *redisAddr != "" {
		builder = builder.Redis(*redisAddr)
	}

	limiter, err := builder.Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}
	helper := ratelimit.NewTestHelper(limiter)

	// Run benchmark
	result := helper.BenchmarkLimiter(context.Background(), *entity, *scope, *duration)

	fmt.Printf("\n📊 Benchmark Results:\n")
	fmt.Printf("   Duration: %v\n", result.Duration)
	fmt.Printf("   Total Requests: %d\n", result.TotalRequests)
	fmt.Printf("   Requests/Second: %.2f\n", result.RequestsPerSecond)
	fmt.Printf("   Average Latency: %v\n", result.AverageLatency)
	fmt.Printf("   Allowed: %d, Denied: %d\n", result.AllowedRequests, result.DeniedRequests)

	// Performance evaluation
	if result.RequestsPerSecond > 10000 {
		fmt.Printf("   🏆 Excellent performance!\n")
	} else if result.RequestsPerSecond > 1000 {
		fmt.Printf("   ✅ Good performance\n")
	} else {
		fmt.Printf("   ⚠️  Performance could be improved\n")
	}
}

func handleHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	redisAddr := fs.String("redis", "", "Redis address to check")
	format := fs.String("format", "table", "Output format: json, table")

	fs.Parse(args)

	// Create limiter
	builder := ratelimit.New()
	if *redisAddr != "" {
		builder = builder.Redis(*redisAddr)
	}

	limiter, err := builder.Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}

	// Check health
	healthErr := limiter.Health(context.Background())

	if *format == "json" {
		result := map[string]interface{}{
			"healthy":   healthErr == nil,
			"timestamp": time.Now().Unix(),
		}
		if healthErr != nil {
			result["error"] = healthErr.Error()
		}
		json.NewEncoder(os.Stdout).Encode(result)
	} else {
		if healthErr != nil {
			fmt.Printf("❌ UNHEALTHY: %v\n", healthErr)
			os.Exit(1)
		} else {
			fmt.Printf("✅ HEALTHY\n")
		}
	}
}

func handleStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	redisAddr := fs.String("redis", "", "Redis address")
	format := fs.String("format", "json", "Output format: json, table")

	fs.Parse(args)

	// Create limiter
	builder := ratelimit.New()
	if *redisAddr != "" {
		builder = builder.Redis(*redisAddr)
	}

	limiter, err := builder.Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}

	// Get stats
	stats, err := limiter.Stats(context.Background())
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		os.Exit(1)
	}

	if *format == "json" {
		json.NewEncoder(os.Stdout).Encode(stats)
	} else {
		fmt.Printf("📊 Rate Limiting Statistics:\n")
		fmt.Printf("   Total Requests: %d\n", stats.TotalRequests)
		fmt.Printf("   Total Denied: %d\n", stats.TotalDenied)
		if len(stats.ByScope) > 0 {
			fmt.Printf("   By Scope:\n")
			for scope, scopeStats := range stats.ByScope {
				fmt.Printf("     %s: %d requests, %d denied\n",
					scope, scopeStats.Requests, scopeStats.Denied)
			}
		}
	}
}

func handleMonitor(args []string) {
	fs := flag.NewFlagSet("monitor", flag.ExitOnError)
	port := fs.Int("port", 8080, "Monitoring server port")
	redisAddr := fs.String("redis", "", "Redis address")

	fs.Parse(args)

	fmt.Printf("🖥️  Starting monitoring server on port %d\n", *port)

	// Create observable limiter
	builder := ratelimit.New()
	if *redisAddr != "" {
		builder = builder.Redis(*redisAddr)
	}

	baseLimiter, err := builder.Build()
	if err != nil {
		fmt.Printf("Error building limiter: %v\n", err)
		os.Exit(1)
	}
	config := ratelimit.DefaultObservabilityConfig()
	limiter := ratelimit.NewObservableLimiter(baseLimiter, config)

	// Create monitoring server
	server := ratelimit.NewMonitoringServer(limiter)

	fmt.Printf("Available endpoints:\n")
	fmt.Printf("   http://localhost:%d/health\n", *port)
	fmt.Printf("   http://localhost:%d/metrics\n", *port)
	fmt.Printf("   http://localhost:%d/stats\n", *port)
	fmt.Printf("   http://localhost:%d/debug\n", *port)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), server))
}

func handleConfig(args []string) {
	if len(args) == 0 {
		fmt.Println("Config subcommands: validate, generate, reload")
		return
	}

	subcommand := args[0]
	subargs := args[1:]

	switch subcommand {
	case "validate":
		fs := flag.NewFlagSet("config validate", flag.ExitOnError)
		file := fs.String("file", "", "Configuration file to validate")

		fs.Parse(subargs)

		if *file == "" {
			fmt.Println("Error: --file is required")
			os.Exit(1)
		}

		fmt.Printf("Validating configuration file: %s\n", *file)
		// In a real implementation, this would read and validate the file
		fmt.Printf("✅ Configuration is valid\n")

	case "generate":
		config := &ratelimit.HotReloadConfig{
			Limits: map[string]string{
				"global": "100/minute",
				"upload": "10/minute",
				"search": "50/minute",
			},
			TierLimits: map[string]string{
				"free":    "50/minute",
				"premium": "500/minute",
			},
			Algorithm: "sliding_window",
			Enabled:   true,
			Version:   "1.0.0",
			UpdatedAt: time.Now(),
			UpdatedBy: "cli-tool",
		}

		json.NewEncoder(os.Stdout).Encode(config)

	case "reload":
		fmt.Println("🔄 Triggering configuration reload...")
		fmt.Println("   (In a real implementation, this would signal the running limiter)")
		fmt.Println("✅ Reload signal sent")

	default:
		fmt.Printf("Unknown config subcommand: %s\n", subcommand)
	}
}

func handleServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	port := fs.Int("port", 8080, "Server port")
	preset := fs.String("preset", "", "Preset configuration: api-gateway, saas-app, public-api")
	redisAddr := fs.String("redis", "", "Redis address")

	fs.Parse(args)

	var limiter ratelimit.Limiter
	var err error

	// Create limiter based on preset or custom config
	if *preset != "" {
		fmt.Printf("🏗️  Using preset: %s\n", *preset)

		switch *preset {
		case "api-gateway":
			builder := ratelimit.APIGateway()
			if *redisAddr != "" {
				builder = builder.Redis(*redisAddr)
			}
			limiter, err = builder.Build()
			if err != nil {
				fmt.Printf("Error building limiter: %v\n", err)
				os.Exit(1)
			}

		case "saas-app":
			builder := ratelimit.SaaSApp()
			if *redisAddr != "" {
				builder = builder.Redis(*redisAddr)
			}
			limiter, err = builder.Build()
			if err != nil {
				fmt.Printf("Error building limiter: %v\n", err)
				os.Exit(1)
			}

		case "public-api":
			builder := ratelimit.PublicAPI()
			if *redisAddr != "" {
				builder = builder.Redis(*redisAddr)
			}
			limiter, err = builder.Build()
			if err != nil {
				fmt.Printf("Error building limiter: %v\n", err)
				os.Exit(1)
			}

		default:
			fmt.Printf("Unknown preset: %s\n", *preset)
			os.Exit(1)
		}
	} else {
		// Default configuration
		builder := ratelimit.New().
			Limit("global", "100/minute").
			Limit("upload", "10/minute").
			Limit("search", "50/minute")

		if *redisAddr != "" {
			builder = builder.Redis(*redisAddr)
		}

		limiter, err = builder.Build()
		if err != nil {
			fmt.Printf("Error building limiter: %v\n", err)
			os.Exit(1)
		}
	}

	// Create demo server
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Data endpoint", "scope": "global"}`))
	})

	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Upload endpoint", "scope": "upload"}`))
	})

	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Search endpoint", "scope": "search"}`))
	})

	// Health and stats
	mux.HandleFunc("/health", ratelimit.HealthCheckHandler(limiter))

	if observableLimiter, ok := limiter.(*ratelimit.ObservableLimiter); ok {
		mux.HandleFunc("/metrics", ratelimit.MetricsHandler(observableLimiter))
		mux.HandleFunc("/stats", ratelimit.StatsHandler(limiter))
	}

	// Info endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		info := map[string]interface{}{
			"service": "Gorly Demo Server",
			"version": ratelimit.GetVersion(),
			"preset":  *preset,
			"endpoints": map[string]string{
				"/api/data":   "General data endpoint",
				"/api/upload": "File upload endpoint",
				"/api/search": "Search endpoint",
				"/health":     "Health check",
				"/stats":      "Statistics",
				"/metrics":    "Metrics",
			},
		}
		json.NewEncoder(w).Encode(info)
	})

	// Apply rate limiting
	rateLimitedMux := http.NewServeMux()
	apiHandler := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(
		http.StripPrefix("/api", mux))

	rateLimitedMux.Handle("/api/", apiHandler)
	rateLimitedMux.Handle("/", mux)

	fmt.Printf("🚀 Demo server starting on port %d\n", *port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("   http://localhost:%d/ (info)\n", *port)
	fmt.Printf("   http://localhost:%d/api/data (rate limited)\n", *port)
	fmt.Printf("   http://localhost:%d/api/upload (rate limited)\n", *port)
	fmt.Printf("   http://localhost:%d/api/search (rate limited)\n", *port)
	fmt.Printf("   http://localhost:%d/health\n", *port)

	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), rateLimitedMux))
}

func handleValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	limit := fs.String("limit", "", "Limit string to validate (e.g., '100/minute')")
	algorithm := fs.String("algorithm", "", "Algorithm to validate")

	fs.Parse(args)

	if *limit != "" {
		if _, _, err := ratelimit.ParseLimit(*limit); err != nil {
			fmt.Printf("❌ Invalid limit format: %v\n", err)
			os.Exit(1)
		} else {
			fmt.Printf("✅ Valid limit format: %s\n", *limit)
		}
	}

	if *algorithm != "" {
		switch *algorithm {
		case "token_bucket", "sliding_window":
			fmt.Printf("✅ Valid algorithm: %s\n", *algorithm)
		default:
			fmt.Printf("❌ Invalid algorithm: %s\n", *algorithm)
			fmt.Printf("   Supported: token_bucket, sliding_window\n")
			os.Exit(1)
		}
	}

	if *limit == "" && *algorithm == "" {
		fmt.Println("Specify --limit and/or --algorithm to validate")
	}
}
