// Package main demonstrates HTTP middleware integration
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	ratelimit "github.com/itsatony/gorly"
	"github.com/itsatony/gorly/middleware"
	"github.com/itsatony/gorly/stores"
)

func main() {
	fmt.Println("=== HTTP Middleware Example ===")

	// Create store and limiter
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	limiter, err := ratelimit.NewSimple(store, 10, time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	defer limiter.Close()

	// Create HTTP middleware
	mw, err := middleware.NewHTTPMiddleware(&middleware.HTTPMiddlewareConfig{
		Limiter:          limiter,
		ContextExtractor: middleware.DefaultIPContextExtractor,
		AddHeaders:       true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create HTTP handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"message": "Request successful!", "timestamp": "%s"}`,
			time.Now().Format(time.RFC3339))
	})

	// Wrap with rate limiting middleware
	http.Handle("/api/data", mw.Middleware(handler))

	// Health check endpoint (no rate limiting)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status": "healthy"}`)
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("Try: curl http://localhost:8080/api/data")
	fmt.Println("Rate limit: 10 requests per minute")
	fmt.Println("\nPress Ctrl+C to stop")

	// Start server with timeout for testing
	server := &http.Server{
		Addr:    ":8080",
		Handler: http.DefaultServeMux,
	}

	// Graceful shutdown after 30 seconds (for testing)
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Println("\n\nStopping server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
