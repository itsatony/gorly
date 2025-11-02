// Package main demonstrates comprehensive version information features using go-version
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	version "github.com/itsatony/go-version"
	ratelimit "github.com/itsatony/gorly"
)

func main() {
	fmt.Println("=== Gorly Version Information Example ===")

	// Initialize version information
	// This loads the versions.yaml manifest and enriches with git/build info
	if err := ratelimit.InitializeVersion(); err != nil {
		log.Printf("Warning: Failed to initialize version: %v\n", err)
	}

	// Example 1: Basic version information
	fmt.Println("--- Basic Version Info ---")
	fmt.Printf("Project Name: %s\n", ratelimit.GetProjectName())
	fmt.Printf("Version: %s\n", ratelimit.GetVersion())
	fmt.Printf("Git Commit: %s\n", ratelimit.GetGitCommit())
	fmt.Println()

	// Example 2: Formatted version string
	fmt.Println("--- Formatted Version ---")
	fmt.Println(ratelimit.FormatVersionString())
	fmt.Println()

	// Example 3: Full version information
	fmt.Println("--- Detailed Version Info ---")
	info := ratelimit.GetVersionInfo()
	if info != nil {
		fmt.Printf("Project: %s v%s\n", info.Project.Name, info.Project.Version)
		if info.Git.Commit != "" {
			fmt.Printf("Git Commit: %s\n", info.Git.Commit)
			fmt.Printf("Git Tree State: %s\n", info.Git.TreeState)
			if info.Git.Tag != "" {
				fmt.Printf("Git Tag: %s\n", info.Git.Tag)
			}
		}
		if info.Build.Time != "" {
			fmt.Printf("Build Time: %s\n", info.Build.Time)
			fmt.Printf("Go Version: %s\n", info.Build.GoVersion)
		}
	}
	fmt.Println()

	// Example 4: Component versions
	fmt.Println("--- Component Versions ---")
	components := []string{"algorithms", "stores", "middleware"}
	for _, comp := range components {
		if v, err := ratelimit.GetComponentVersion(comp); err == nil {
			fmt.Printf("  %s: v%s\n", comp, v)
		}
	}
	fmt.Println()

	// Example 5: API versions
	fmt.Println("--- API Versions ---")
	if apiVer, err := ratelimit.GetAPIVersion("http_middleware"); err == nil {
		fmt.Printf("  HTTP Middleware API: v%s\n", apiVer)
	}
	fmt.Println()

	// Example 6: Schema versions
	fmt.Println("--- Schema Versions ---")
	if schemaVer, err := ratelimit.GetSchemaVersion("rate_limit_config"); err == nil {
		fmt.Printf("  Rate Limit Config Schema: v%s\n", schemaVer)
	}
	fmt.Println()

	// Example 7: Version validation
	fmt.Println("--- Version Validation ---")
	ctx := context.Background()

	// Validate that algorithms component meets minimum version
	algValidator := version.NewComponentValidator("algorithms", "0.9.0")
	if err := ratelimit.ValidateVersions(ctx, algValidator); err != nil {
		fmt.Printf("❌ Algorithms validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Algorithms component meets version requirements")
	}

	// Validate stores component
	storeValidator := version.NewComponentValidator("stores", "0.5.0")
	if err := ratelimit.ValidateVersions(ctx, storeValidator); err != nil {
		fmt.Printf("❌ Stores validation failed: %v\n", err)
	} else {
		fmt.Println("✅ Stores component meets version requirements")
	}
	fmt.Println()

	// Example 8: HTTP version endpoint
	fmt.Println("--- HTTP Version Endpoint ---")
	fmt.Println("Starting HTTP server with version endpoint on :8080...")
	fmt.Println("Try these endpoints:")
	fmt.Println("  - http://localhost:8080/version     (JSON version info)")
	fmt.Println("  - http://localhost:8080/health      (Health check with version)")
	fmt.Println("  - http://localhost:8080/api/test    (Example API with version middleware)")
	fmt.Println("\nPress Ctrl+C to stop")
	fmt.Println()

	// Set up HTTP routes
	mux := http.NewServeMux()

	// Version endpoint using go-version's built-in handler
	mux.Handle("/version", ratelimit.VersionHandler())

	// Health endpoint with version info
	mux.Handle("/health", ratelimit.HealthHandler())

	// Example API endpoint with version middleware
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "Hello from Gorly!", "version": "%s"}`, ratelimit.GetVersion())
	})
	mux.Handle("/api/test", ratelimit.VersionMiddleware(apiHandler))

	// Start server with timeout
	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v\n", err)
		}
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make example requests to show the endpoints work
	fmt.Println("--- Example HTTP Requests ---")

	// Request version endpoint
	resp, err := http.Get("http://localhost:8080/version")
	if err != nil {
		fmt.Printf("Error requesting /version: %v\n", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("GET /version - Status: %d\n", resp.StatusCode)
	}

	// Request health endpoint
	resp, err = http.Get("http://localhost:8080/health")
	if err != nil {
		fmt.Printf("Error requesting /health: %v\n", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("GET /health - Status: %d\n", resp.StatusCode)
	}

	// Request test API endpoint
	resp, err = http.Get("http://localhost:8080/api/test")
	if err != nil {
		fmt.Printf("Error requesting /api/test: %v\n", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("GET /api/test - Status: %d\n", resp.StatusCode)
	}

	fmt.Println("\nServer is running. Press Ctrl+C to exit or wait 5 seconds...")

	// Wait a bit then shutdown
	time.Sleep(5 * time.Second)

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Server stopped successfully")
}
