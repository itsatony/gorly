# Production Safety Guidelines - Storage Backend Selection

## Overview

Gorly supports two storage backends: **Memory Store** and **Redis Store**. This document provides critical guidance on when each should be used, with a focus on production safety.

## TL;DR - Quick Decision Guide

| Scenario | Use Memory Store | Use Redis Store |
|----------|-----------------|-----------------|
| Development | ✅ Yes | Optional |
| Testing | ✅ Yes | Optional |
| Single Instance (Low Traffic) | ⚠️ Maybe | ✅ Recommended |
| Multiple Instances | ❌ No | ✅ Required |
| High Traffic (>1000 req/s) | ❌ No | ✅ Required |
| Mission Critical | ❌ No | ✅ Required |
| Distributed System | ❌ No | ✅ Required |
| Requires Persistence | ❌ No | ✅ Required |

## Memory Store

### What It Is

The memory store (`stores/memory.go`) maintains rate limit state in local process memory using Go maps protected by mutexes.

### ✅ Appropriate Use Cases

**1. Development & Testing**
```go
// Perfect for development
store, _ := ratelimit.NewMemoryStore(&ratelimit.MemoryStoreConfig{
    CleanupInterval: 5 * time.Minute,
})
```
- Fast iteration
- No external dependencies
- Easy debugging
- Simplified setup

**2. Single-Instance Applications (Low Traffic)**
```go
// Acceptable for simple CLI tools or low-traffic services
// Example: Personal blog, internal admin tool, development server
```
- Traffic < 100 req/s
- Single server deployment
- Non-critical functionality
- Short rate limit windows (< 1 minute)

**3. In-Process Rate Limiting (Emergency Fallback)**
```go
// As a circuit breaker when Redis is unavailable
if redisDown {
    fallbackToMemoryStore() // Temporary protection
}
```
- Circuit breaker pattern
- Graceful degradation
- Emergency rate limiting

### ❌ CRITICAL LIMITATIONS - DO NOT USE IN PRODUCTION FOR:

#### 1. Race Conditions ⚠️ **P0 SECURITY ISSUE**

**Problem**: The non-atomic fallback methods have race conditions in high-concurrency scenarios.

**Impact**:
```go
// Two requests at exactly the same time:
// Request A: Read state (50 tokens)
// Request B: Read state (50 tokens)  ← Same state!
// Request A: Write state (49 tokens)
// Request B: Write state (49 tokens)  ← Overwrites A's write!
// Result: Only 1 token consumed instead of 2
```

**Exploit Scenario**:
- Attacker sends 1000 concurrent requests
- Race conditions allow 200 requests through
- Expected: 100 requests allowed
- Actual: 200 requests allowed (100% bypass)

**Recommendation**:
- ❌ **DO NOT use memory store for API rate limiting in production**
- ✅ Use Redis with atomic Lua scripts

#### 2. No Shared State ⚠️ **P0 AVAILABILITY ISSUE**

**Problem**: Each server instance has its own memory store with independent state.

**Impact**:
```go
// 3-server deployment with 100 req/min limit
// Server 1: 100 requests allowed
// Server 2: 100 requests allowed
// Server 3: 100 requests allowed
// Total: 300 requests allowed (3x the limit!)
```

**Real-World Example**:
```
Expected Rate Limit: 1000 requests/hour per user
Actual with 5 servers: 5000 requests/hour per user
Attack Surface: 5x larger than intended
```

**Recommendation**:
- ❌ **DO NOT use memory store in multi-instance deployments**
- ✅ Use Redis for shared state across instances

#### 3. No Persistence ⚠️ **P0 AVAILABILITY ISSUE**

**Problem**: All rate limit state is lost on restart.

**Impact**:
```go
// Server restart scenario:
// Before restart: User consumed 90/100 daily quota
// After restart: User has fresh 100/100 quota
// Result: User gets 10 + 100 = 110 requests
```

**Exploit Scenario**:
- Attacker detects server restart pattern
- Waits for restart to reset quotas
- Bypasses rate limits by timing requests after restarts

**Recommendation**:
- ❌ **DO NOT use memory store for long-window limits (hour/day)**
- ✅ Use Redis for persistent state

#### 4. Memory Leaks & Unbounded Growth ⚠️ **P1 STABILITY ISSUE**

**Problem**: Improper cleanup can cause memory leaks.

**Impact**:
```go
// 10,000 unique IPs × 1KB state = 10MB
// 100,000 unique IPs × 1KB state = 100MB
// 1,000,000 unique IPs × 1KB state = 1GB
// 10,000,000 unique IPs × 1KB state = 10GB → OOM
```

**Mitigation** (Partial):
```go
store, _ := ratelimit.NewMemoryStore(&ratelimit.MemoryStoreConfig{
    CleanupInterval: 5 * time.Minute,  // Regular cleanup
    MaxKeys:         100000,            // Hard limit
})
```

**Limitation**: Even with cleanup, memory store is vulnerable to:
- High cardinality keys (many unique users/IPs)
- Long rate limit windows
- Slow memory growth over days

**Recommendation**:
- ⚠️ **Set MaxKeys if you must use memory store**
- ✅ Use Redis with TTL-based expiration

#### 5. No Atomic Operations ⚠️ **P0 SECURITY ISSUE**

**Problem**: Read-modify-write operations are not atomic.

**Code**:
```go
// Memory store operations are NOT atomic:
state := store.Get(key)      // Read
state.tokens -= n            // Modify
store.Set(key, state)        // Write
// ← Race condition window between Get and Set
```

**Comparison**:
```lua
-- Redis Lua script IS atomic:
local tokens = redis.call('GET', key)
tokens = tokens - n
redis.call('SET', key, tokens)
-- Executed atomically, no race conditions
```

**Recommendation**:
- ❌ **DO NOT use memory store for high-concurrency rate limiting**
- ✅ Use Redis Lua scripts for atomic operations

### 📊 Memory Store Performance Characteristics

**Pros**:
- ✅ Fast: ~100ns per operation (in-memory)
- ✅ Simple: No external dependencies
- ✅ Zero network latency
- ✅ Easy to debug

**Cons**:
- ❌ Not thread-safe for atomic operations
- ❌ Memory grows with unique keys
- ❌ No persistence
- ❌ No shared state
- ❌ Vulnerable to race conditions

### 🔧 Safe Memory Store Configuration

If you must use memory store (development only):

```go
store, err := ratelimit.NewMemoryStore(&ratelimit.MemoryStoreConfig{
    // REQUIRED: Prevent unbounded memory growth
    MaxKeys: 100000,

    // REQUIRED: Regular cleanup
    CleanupInterval: 5 * time.Minute,

    // OPTIONAL: Sharding for better concurrency
    ShardCount: 32,
})

if err != nil {
    log.Fatal("Failed to create memory store:", err)
}
defer store.Close()
```

## Redis Store

### What It Is

The Redis store (`stores/redis.go`) maintains rate limit state in Redis using atomic Lua scripts for all operations.

### ✅ Production-Ready Features

**1. Atomic Operations** ✅
```lua
-- All operations execute atomically in Redis
-- No race conditions, no concurrent write conflicts
```

**2. Shared State** ✅
```go
// All server instances share the same Redis state
// True distributed rate limiting
```

**3. Persistence** ✅
```go
// Redis persists state to disk (if configured)
// Survives server restarts
```

**4. Scalability** ✅
```go
// Redis handles millions of keys
// Automatic TTL-based expiration
// No memory leaks
```

**5. High Performance** ✅
```go
// ~1-2ms latency per operation
// Connection pooling
// Pipeline support
```

### 🔧 Production Redis Configuration

```go
store, err := ratelimit.NewRedisStore(&ratelimit.RedisStoreConfig{
    // REQUIRED: Redis connection
    Address: "redis:6379",

    // RECOMMENDED: Authentication
    Password: os.Getenv("REDIS_PASSWORD"),

    // RECOMMENDED: Connection pool
    PoolSize: 10,

    // RECOMMENDED: Timeouts
    DialTimeout:  5 * time.Second,
    ReadTimeout:  3 * time.Second,
    WriteTimeout: 3 * time.Second,

    // RECOMMENDED: Retries
    MaxRetries: 3,

    // OPTIONAL: Key prefix for namespacing
    KeyPrefix: "gorly",

    // OPTIONAL: TLS for production
    TLS: &tls.Config{
        MinVersion: tls.VersionTLS12,
    },
})

if err != nil {
    log.Fatal("Failed to create Redis store:", err)
}
defer store.Close()
```

### 🚀 Redis High Availability

**1. Redis Sentinel** (Recommended)
```go
store, err := ratelimit.NewRedisStore(&ratelimit.RedisStoreConfig{
    MasterName: "mymaster",
    SentinelAddrs: []string{
        "sentinel1:26379",
        "sentinel2:26379",
        "sentinel3:26379",
    },
})
```

**2. Redis Cluster**
```go
// Use redis-go-cluster or redis cluster client
// Automatic sharding across multiple Redis nodes
```

**3. Redis Cloud** (Managed Service)
```go
// AWS ElastiCache, Redis Cloud, Azure Cache for Redis
// Fully managed, automatic failover, backups
```

### 📊 Redis Performance Benchmarks

| Operation | Latency | Throughput |
|-----------|---------|------------|
| Single Allow() | 1-2ms | 1000 req/s per connection |
| Batch Allow() | 2-5ms | 5000 req/s per connection |
| With Connection Pool (10) | 1-2ms | 10,000 req/s |
| With Connection Pool (100) | 1-3ms | 50,000 req/s |

## Migration Guide

### From Memory Store to Redis

```go
// Step 1: Update configuration
- store, _ := ratelimit.NewMemoryStore(...)
+ store, _ := ratelimit.NewRedisStore(&ratelimit.RedisStoreConfig{
+     Address: "redis:6379",
+ })

// Step 2: No code changes needed!
// The Store interface is the same for both implementations

// Step 3: Deploy with Redis available
// The limiter will automatically use Redis atomic operations
```

### Gradual Migration (Feature Flag)

```go
func createStore() (ratelimit.Store, error) {
    if useRedis := os.Getenv("USE_REDIS"); useRedis == "true" {
        return ratelimit.NewRedisStore(&ratelimit.RedisStoreConfig{
            Address: os.Getenv("REDIS_ADDR"),
        })
    }

    // Fallback to memory store (development only)
    log.Warn("Using memory store - NOT FOR PRODUCTION")
    return ratelimit.NewMemoryStore(&ratelimit.MemoryStoreConfig{
        MaxKeys: 100000,
    })
}
```

## Security Checklist

Before deploying to production, verify:

### Memory Store (Development Only)
- [ ] Not used for API rate limiting
- [ ] Not used in multi-instance deployments
- [ ] MaxKeys configured to prevent DoS
- [ ] CleanupInterval configured
- [ ] Only used for development/testing

### Redis Store (Production)
- [ ] Authentication enabled (`requirepass` in redis.conf)
- [ ] Network security (firewall, VPC, security groups)
- [ ] TLS enabled for connections
- [ ] Connection timeouts configured
- [ ] Connection pooling enabled
- [ ] Retry logic configured
- [ ] Monitoring enabled (Redis INFO, logs)
- [ ] Backup strategy in place
- [ ] High availability configured (Sentinel/Cluster)
- [ ] Key expiration verified (TTL set correctly)

## Disaster Recovery

### Memory Store Failure
```
Issue: Process restart → All state lost
Impact: Rate limits reset, potential abuse window
Recovery: None (by design)
Prevention: Use Redis
```

### Redis Store Failure
```
Issue: Redis unavailable
Impact: Rate limiting fails (returns errors)
Recovery Options:
  1. Redis automatic failover (Sentinel)
  2. Fallback to memory store (degraded mode)
  3. Fail open (allow all requests - dangerous!)
  4. Fail closed (deny all requests - service outage)
```

**Recommended Approach**:
```go
// Circuit breaker pattern
limiter, err := ratelimit.NewRateLimiter(&ratelimit.Config{
    Store: redisStore,
    // ... other config
})

// In middleware
result, err := limiter.Allow(ctx, context)
if err != nil {
    // Redis is down - decide policy:
    // Option 1: Fail open (allow request, log error)
    log.Error("Rate limiter failed, allowing request:", err)
    return next(ctx)

    // Option 2: Fail closed (deny request)
    return http.StatusServiceUnavailable

    // Option 3: Fallback to in-memory (best effort)
    result, err = fallbackLimiter.Allow(ctx, context)
}
```

## Compliance & Audit

### Data Residency
- **Memory Store**: Data stays in process memory (same server)
- **Redis Store**: Data stored in Redis (may be different server/region)
- **Consideration**: Check data residency requirements (GDPR, etc.)

### Data Retention
- **Memory Store**: Automatic expiration via cleanup
- **Redis Store**: Automatic TTL-based expiration
- **Consideration**: Rate limit data is temporary (seconds to hours)

### Logging & Monitoring
- **Memory Store**: Limited observability (in-process only)
- **Redis Store**: Full Redis monitoring (INFO, MONITOR, slow log)
- **Recommendation**: Export rate limit metrics to Prometheus/Datadog

## Summary

| Feature | Memory Store | Redis Store |
|---------|-------------|-------------|
| **Production Ready** | ❌ No | ✅ Yes |
| **Atomic Operations** | ❌ No | ✅ Yes |
| **Shared State** | ❌ No | ✅ Yes |
| **Persistence** | ❌ No | ✅ Yes |
| **Race Condition Free** | ❌ No | ✅ Yes |
| **Scalability** | ⚠️ Limited | ✅ Excellent |
| **High Availability** | ❌ No | ✅ Yes |
| **Performance** | ✅ Excellent | ✅ Very Good |
| **Setup Complexity** | ✅ Simple | ⚠️ Moderate |

### Final Recommendation

**Use Redis Store for production**. The memory store is **only suitable for development and testing**.

The marginal performance benefit of memory store (50-100ns faster) is **completely outweighed** by the critical security and reliability issues it has in production environments.

## References

- Redis Lua Scripting: https://redis.io/docs/manual/programmability/eval-intro/
- Redis Persistence: https://redis.io/docs/manual/persistence/
- Redis Sentinel: https://redis.io/docs/manual/sentinel/
- Redis Cluster: https://redis.io/docs/manual/scaling/

## Questions?

If you're unsure whether to use memory store or Redis store:

**Ask yourself:**
1. Is this production? → Use Redis
2. Multiple instances? → Use Redis
3. Need shared state? → Use Redis
4. High traffic (>100 req/s)? → Use Redis
5. Mission critical? → Use Redis

If you answered "yes" to any of these: **Use Redis Store**.
