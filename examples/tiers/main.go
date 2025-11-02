// Package main demonstrates multi-tier rate limiting with scopes
package main

import (
	"context"
	"fmt"

	ratelimit "github.com/itsatony/gorly"
	"github.com/itsatony/gorly/stores"
)

func main() {
	fmt.Println("=== Multi-Tier Rate Limiting Example ===")

	// Create store
	store, err := stores.NewMemoryStore(nil)
	if err != nil {
		panic(err)
	}
	defer store.Close()

	// Create limiter with tier support
	limiter, err := ratelimit.NewForSaaS(store)
	if err != nil {
		panic(err)
	}
	defer limiter.Close()

	ctx := context.Background()

	// Test different tiers
	fmt.Println("Testing Different User Tiers:")

	users := []struct {
		id   string
		tier string
		name string
	}{
		{"user_free_1", ratelimit.TierFree, "Free User"},
		{"user_premium_1", ratelimit.TierPremium, "Premium User"},
		{"user_enterprise_1", ratelimit.TierEnterprise, "Enterprise User"},
	}

	for _, user := range users {
		fmt.Printf("%s (%s):\n", user.name, user.tier)
		testUserScopes(ctx, limiter, user.id, user.tier)
		fmt.Println()
	}

	// Test quick helpers
	fmt.Println("Using Quick Helper Functions:")

	allowed, err := ratelimit.QuickCheck(ctx, limiter, "test_user", ratelimit.ScopeGlobal, ratelimit.TierFree)
	if err != nil {
		fmt.Printf("QuickCheck error: %v\n", err)
	} else {
		fmt.Printf("QuickCheck: %v\n", allowed)
	}

	limit, used, remaining, err := ratelimit.QuickStats(ctx, limiter, "test_user", ratelimit.ScopeGlobal, ratelimit.TierFree)
	if err != nil {
		fmt.Printf("QuickStats error: %v\n", err)
	} else {
		fmt.Printf("QuickStats: Limit=%d, Used=%d, Remaining=%d\n", limit, used, remaining)
	}

	// Test batch operations
	fmt.Println("\nBatch Operations:")

	identities := []string{"batch_user1", "batch_user2", "batch_user3"}
	results := ratelimit.CheckMultiple(ctx, limiter, identities, ratelimit.ScopeGlobal, ratelimit.TierFree)

	for id, result := range results {
		fmt.Printf("  %s: Allowed=%v, Remaining=%d/%d\n",
			id, result.Allowed, result.Remaining, result.Limit)
	}
}

func testUserScopes(ctx context.Context, limiter ratelimit.RateLimiter, userID, tier string) {
	scopes := []string{
		ratelimit.ScopeGlobal,
		ratelimit.ScopeAPI,
		ratelimit.ScopeSearch,
		ratelimit.ScopeUpload,
	}

	for _, scope := range scopes {
		rlCtx := ratelimit.NewSimpleContext(userID, scope, tier, nil)
		result, err := limiter.Allow(ctx, rlCtx)
		if err != nil {
			fmt.Printf("  %s: Error - %v\n", scope, err)
			continue
		}

		fmt.Printf("  %s scope: %d/%s (remaining: %d)\n",
			scope, result.Limit, result.Window, result.Remaining)
	}
}
