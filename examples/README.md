# Gorly Examples

This directory contains practical examples demonstrating gorly's rate limiting capabilities.

## Running Examples

All examples are self-contained and can be run directly:

```bash
# Basic example - Simple rate limiting
go run examples/basic/main.go

# Middleware example - HTTP integration
go run examples/middleware/main.go

# Builder example - Configuration patterns
go run examples/builder/main.go

# Tiers example - Multi-tier and scopes
go run examples/tiers/main.go
```

## Examples Overview

### 1. Basic (`basic/`)

Demonstrates the simplest possible usage:
- Creating a rate limiter with `NewSimple()`
- Checking rate limits with `Allow()`
- Viewing statistics with `Stats()`
- Resetting limits with `Reset()`

**Best for**: Getting started, understanding the core concepts

### 2. Middleware (`middleware/`)

Shows HTTP middleware integration:
- Creating HTTP middleware
- IP-based rate limiting
- Custom rate limit responses
- Rate limit headers (X-RateLimit-*)

**Best for**: Web applications, REST APIs

### 3. Builder (`builder/`)

Demonstrates the builder pattern:
- Fluent API configuration
- Rate string format (`"100/1h"`)
- Preset configurations (API, Web, SaaS)
- Token bucket vs sliding window

**Best for**: Complex configurations, production setups

### 4. Tiers (`tiers/`)

Shows multi-tier rate limiting:
- Different limits per user tier (Free, Premium, Enterprise)
- Scope-based limits (global, API, search, upload)
- Quick helper functions
- Batch operations

**Best for**: SaaS applications, API services with subscription tiers

## Key Concepts

### Rate Limit Context

All rate limit operations require a context that identifies what is being rate limited:

```go
// IP-based
rlCtx := ratelimit.NewIPContext("192.168.1.100")

// User-based with tier
rlCtx := ratelimit.NewUserContext("user123", ratelimit.TierPremium)

// Custom with scope and metadata
rlCtx := ratelimit.NewSimpleContext("user123", ratelimit.ScopeAPI, ratelimit.TierFree, nil)
```

### Stores

Gorly supports different storage backends:

```go
// In-memory (for development/testing)
store := stores.NewMemoryStore(nil)

// Redis (for production/distributed systems)
store, _ := stores.NewRedisStore(&stores.RedisConfig{
    Addrs: []string{"localhost:6379"},
})
```

### Algorithms

Choose between different rate limiting algorithms:

- **Token Bucket** (default): Allows bursts, smooth refilling
- **Sliding Window**: More precise, no burst allowance

```go
limiter, _ := ratelimit.NewBuilder().
    WithStore(store).
    WithTokenBucket().  // or WithSlidingWindow()
    Build()
```

## Next Steps

1. Read the main [README](../README.md) for complete documentation
2. Check [CLAUDE.md](../CLAUDE.md) for development guidelines
3. Review the [API documentation](https://pkg.go.dev/github.com/itsatony/gorly)
4. Explore the test files for more advanced usage patterns
