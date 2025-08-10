// examples/config/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/itsatony/gorly"
)

func main() {
	fmt.Println("Gorly Configuration Examples")
	fmt.Println("============================")

	// Example 1: Load from YAML file
	fmt.Println("\n1. Loading configuration from YAML file...")
	yamlExample()

	// Example 2: Load from JSON file
	fmt.Println("\n2. Loading configuration from JSON file...")
	jsonExample()

	// Example 3: Load from environment variables
	fmt.Println("\n3. Loading configuration from environment variables...")
	envExample()

	// Example 4: Load from multiple sources (layered configuration)
	fmt.Println("\n4. Loading configuration from multiple sources...")
	multiSourceExample()

	// Example 5: Using default configuration
	fmt.Println("\n5. Using default configuration...")
	defaultExample()
}

func yamlExample() {
	loader := ratelimit.NewConfigLoader()
	config, err := loader.LoadFromFile("gorly.yaml")
	if err != nil {
		log.Printf("Failed to load YAML config: %v", err)
		return
	}

	printConfigSummary("YAML", config)
	testRateLimiter(config)
}

func jsonExample() {
	loader := ratelimit.NewConfigLoader()
	config, err := loader.LoadFromFile("gorly.json")
	if err != nil {
		log.Printf("Failed to load JSON config: %v", err)
		return
	}

	printConfigSummary("JSON", config)
	testRateLimiter(config)
}

func envExample() {
	// Set some environment variables for the example
	os.Setenv("GORLY_ENABLED", "true")
	os.Setenv("GORLY_ALGORITHM", "sliding_window")
	os.Setenv("GORLY_STORE", "memory")
	os.Setenv("GORLY_DEFAULT_LIMIT", "100/1m")
	defer func() {
		os.Unsetenv("GORLY_ENABLED")
		os.Unsetenv("GORLY_ALGORITHM")
		os.Unsetenv("GORLY_STORE")
		os.Unsetenv("GORLY_DEFAULT_LIMIT")
	}()

	loader := ratelimit.NewConfigLoader()
	config, err := loader.LoadFromEnv()
	if err != nil {
		log.Printf("Failed to load env config: %v", err)
		return
	}

	printConfigSummary("Environment", config)
	testRateLimiter(config)
}

func multiSourceExample() {
	// Set environment override
	os.Setenv("GORLY_ALGORITHM", "sliding_window")
	defer os.Unsetenv("GORLY_ALGORITHM")

	loader := ratelimit.NewConfigLoader()

	// Define sources in priority order (later sources override earlier ones)
	sources := []ratelimit.ConfigSource{
		// 1. Start with JSON base configuration
		&ratelimit.FileConfigSource{
			Filename: "gorly.json",
			Required: false, // Don't fail if file doesn't exist
		},
		// 2. Override with environment variables
		&ratelimit.EnvConfigSource{
			Required: false,
		},
	}

	config, err := loader.LoadFromMultipleSources(sources...)
	if err != nil {
		log.Printf("Failed to load multi-source config: %v", err)
		return
	}

	printConfigSummary("Multi-source (JSON + ENV)", config)
	testRateLimiter(config)
}

func defaultExample() {
	config := ratelimit.DefaultConfig()
	printConfigSummary("Default", config)
	testRateLimiter(config)
}

func printConfigSummary(source string, config *ratelimit.Config) {
	fmt.Printf("Config loaded from: %s\n", source)
	fmt.Printf("  Enabled: %t\n", config.Enabled)
	fmt.Printf("  Algorithm: %s\n", config.Algorithm)
	fmt.Printf("  Store: %s\n", config.Store)
	fmt.Printf("  Key Prefix: %s\n", config.KeyPrefix)
	fmt.Printf("  Metrics Enabled: %t\n", config.EnableMetrics)
	fmt.Printf("  Operation Timeout: %v\n", config.OperationTimeout)

	if config.Store == "redis" {
		fmt.Printf("  Redis Address: %s\n", config.Redis.Address)
		fmt.Printf("  Redis Database: %d\n", config.Redis.Database)
	}

	fmt.Printf("  Default Limits: %d scope(s)\n", len(config.DefaultLimits))
	for scope, limit := range config.DefaultLimits {
		fmt.Printf("    %s: %d requests per %v\n", scope, limit.Requests, limit.Window)
	}

	fmt.Printf("  Tier Limits: %d tier(s)\n", len(config.TierLimits))
	for tier := range config.TierLimits {
		fmt.Printf("    %s\n", tier)
	}

	fmt.Printf("  Entity Overrides: %d override(s)\n", len(config.EntityOverrides))
	for entity := range config.EntityOverrides {
		fmt.Printf("    %s\n", entity)
	}
}

func testRateLimiter(config *ratelimit.Config) {
	if !config.Enabled {
		fmt.Println("  Rate limiting is disabled, skipping test")
		return
	}

	limiter, err := ratelimit.NewRateLimiter(config)
	if err != nil {
		log.Printf("  Failed to create rate limiter: %v", err)
		return
	}
	defer limiter.Close()

	ctx := context.Background()
	entity := ratelimit.NewDefaultAuthEntity("test_user", ratelimit.EntityTypeUser, ratelimit.TierFree)

	// Test a few requests
	for i := 0; i < 3; i++ {
		result, err := limiter.Allow(ctx, entity, ratelimit.ScopeGlobal)
		if err != nil {
			log.Printf("  Request %d failed: %v", i+1, err)
			continue
		}

		fmt.Printf("  Request %d: Allowed=%t, Remaining=%d, Algorithm=%s\n",
			i+1, result.Allowed, result.Remaining, result.Algorithm)

		if !result.Allowed {
			fmt.Printf("    Rate limited! Retry after: %v\n", result.RetryAfter)
			break
		}
	}

	// Test health check
	if err := limiter.Health(ctx); err != nil {
		fmt.Printf("  Health check failed: %v\n", err)
	} else {
		fmt.Println("  Health check passed ✓")
	}
}
