// Package main demonstrates the most basic usage of gorly
package main

import (
	"context"
	"fmt"
	"time"

	ratelimit "github.com/itsatony/gorly"
	"github.com/itsatony/gorly/stores"
)

func main() {
	fmt.Println("=== Basic Rate Limiting Example ===")

	// Create an in-memory store
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	// Create a simple rate limiter: 5 requests per second
	limiter, err := ratelimit.NewSimple(store, 5, time.Second)
	if err != nil {
		panic(err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Create a rate limit context for a user
	rlCtx := ratelimit.NewIPContext("192.168.1.100")

	// Check rate limits
	fmt.Println("Making 7 requests (limit is 5/second)...")
	for i := 1; i <= 7; i++ {
		result, err := limiter.Allow(ctx, rlCtx)
		if err != nil {
			fmt.Printf("Request %d: Error - %v\n", i, err)
			continue
		}

		if result.Allowed {
			fmt.Printf("Request %d: ✅ ALLOWED (remaining: %d/%d)\n",
				i, result.Remaining, result.Limit)
		} else {
			fmt.Printf("Request %d: ❌ DENIED (retry after: %.1fs)\n",
				i, result.RetryAfter.Seconds())
		}
	}

	// Check stats
	fmt.Println("\n--- Current Stats ---")
	stats, _ := limiter.Stats(ctx, rlCtx)
	fmt.Printf("Limit: %d\n", stats.Limit)
	fmt.Printf("Used: %d\n", stats.Used)
	fmt.Printf("Remaining: %d\n", stats.Remaining)
	fmt.Printf("Window: %v\n", stats.Window)

	// Reset the limit
	fmt.Println("\nResetting rate limit...")
	limiter.Reset(ctx, rlCtx)

	// Try again
	result, _ := limiter.Allow(ctx, rlCtx)
	fmt.Printf("After reset: %s (remaining: %d/%d)\n",
		map[bool]string{true: "✅ ALLOWED", false: "❌ DENIED"}[result.Allowed],
		result.Remaining, result.Limit)
}
