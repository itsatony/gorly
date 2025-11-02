// Package main demonstrates the builder pattern for configuring rate limiters
package main

import (
	"context"
	"fmt"
	"time"

	ratelimit "github.com/itsatony/gorly"
	"github.com/itsatony/gorly/stores"
)

func main() {
	fmt.Println("=== Builder Pattern Example ===")

	// Create store
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	// Example 1: Simple rate limiter with builder
	fmt.Println("1. Simple Configuration")
	limiter1, err := ratelimit.NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithLimit(100, time.Hour).
		WithBurst(10).
		Build()
	if err != nil {
		panic(err)
	}
	defer limiter1.Close()

	testLimiter(limiter1, "user1", "Simple (100/hour, burst 10)")

	// Example 2: Using rate string format
	fmt.Println("\n2. Rate String Format")
	limiter2, err := ratelimit.NewBuilder().
		WithStore(store).
		WithSlidingWindow().
		WithLimitString("50/5m"). // 50 requests per 5 minutes
		Build()
	if err != nil {
		panic(err)
	}
	defer limiter2.Close()

	testLimiter(limiter2, "user2", "Rate String (50/5m)")

	// Example 3: With tier support
	fmt.Println("\n3. Multi-Tier Support")
	limiter3, err := ratelimit.NewBuilder().
		WithStore(store).
		WithTokenBucket().
		WithDefaultTiers().
		Build()
	if err != nil {
		panic(err)
	}
	defer limiter3.Close()

	testTieredLimiter(limiter3)

	// Example 4: Using preset configurations
	fmt.Println("\n4. Preset Configurations")

	apiLimiter, _ := ratelimit.NewForAPI(store)
	defer apiLimiter.Close()
	testLimiter(apiLimiter, "api_user", "API Preset")

	webLimiter, _ := ratelimit.NewForWebApp(store)
	defer webLimiter.Close()
	testLimiter(webLimiter, "web_user", "Web App Preset")
}

func testLimiter(limiter ratelimit.RateLimiter, userID, description string) {
	ctx := context.Background()
	rlCtx := ratelimit.NewUserContext(userID, ratelimit.TierFree)

	result, _ := limiter.Allow(ctx, rlCtx)
	fmt.Printf("   %s: Limit=%d, Window=%v, Allowed=%v\n",
		description, result.Limit, result.Window, result.Allowed)
}

func testTieredLimiter(limiter ratelimit.RateLimiter) {
	ctx := context.Background()

	tiers := []struct {
		tier string
		desc string
	}{
		{ratelimit.TierFree, "Free"},
		{ratelimit.TierPremium, "Premium"},
		{ratelimit.TierEnterprise, "Enterprise"},
	}

	for _, t := range tiers {
		rlCtx := ratelimit.NewUserContext("user_"+t.tier, t.tier)
		result, _ := limiter.Allow(ctx, rlCtx)
		fmt.Printf("   %s Tier: Limit=%d/hour\n", t.desc, result.Limit)
	}
}
