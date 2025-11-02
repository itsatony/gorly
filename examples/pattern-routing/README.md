# Pattern-Based Routing Example

This example demonstrates Gorly's advanced pattern-based routing system for per-endpoint rate limiting, introduced in v1.2.0.

## What This Demonstrates

- **Multiple Pattern Types**: Exact matches, prefix matches, glob patterns, and regex patterns
- **Priority-Based Resolution**: Higher priority patterns take precedence when multiple patterns match
- **Per-Endpoint Rate Limits**: Different rate limits for payment, admin, API versioned, and general endpoints
- **Tier-Based Access**: Free, premium, and enterprise tiers with different limits
- **Prometheus Metrics Integration**: Real-time observability for pattern matching and rate limiting
- **Debug & Inspection Tools**: Endpoint for explaining pattern matches and validating configuration
- **Production-Ready Pattern**: Demonstrates secure, performant routing suitable for production use

## Quick Start

### 1. Run the Server

```bash
go run main.go
```

The server starts on `http://localhost:8080` with the following endpoints:
- `/api/payment/process` - Critical payment endpoint (100 req/min, priority 100)
- `/api/payment/*` - General payment operations (500 req/hour, priority 50)
- `/api/admin/**` - Admin operations (1000 req/hour, priority 80)
- `/api/v1/*`, `/api/v2/*` - Versioned API (5000 req/hour, priority 30)
- `/api/*` - General API endpoints (10000 req/hour, priority 10)
- `/metrics` - Prometheus metrics (no rate limiting)
- `/debug/routing/*` - Debug and inspection tools (no rate limiting)

### 2. Test Different Endpoints

**Test critical payment endpoint (strictest limit):**
```bash
# Premium tier - allowed higher burst
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/payment/process

# Free tier - limited burst
curl -H "X-API-Key: free-key-456" http://localhost:8080/api/payment/process
```

**Test admin endpoints:**
```bash
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/admin/users
curl -H "X-API-Key: premium-key-123" http://localhost:8080/api/admin/settings/security
```

**Test API versioning:**
```bash
curl http://localhost:8080/api/v1/products
curl http://localhost:8080/api/v2/products
```

**Test general API:**
```bash
curl http://localhost:8080/api/products
curl http://localhost:8080/api/search?q=test
```

### 3. View Metrics

```bash
# Prometheus metrics (includes routing metrics if enabled)
curl http://localhost:8080/metrics

# Look for these metrics:
# - gorly_routing_matches_total{match_type="exact|glob|regex|prefix"}
# - gorly_routing_match_duration_seconds{match_type="..."}
# - gorly_routing_patterns_total{match_type="..."}
# - gorly_routing_no_matches_total
```

### 4. Use Debug Tools

**Explain why a path matched (or didn't):**
```bash
curl http://localhost:8080/debug/routing/explain?path=/api/payment/process
```

**Inspect routing configuration:**
```bash
curl http://localhost:8080/debug/routing/inspect
```

**Validate configuration for issues:**
```bash
curl http://localhost:8080/debug/routing/validate
```

## Key Concepts Demonstrated

### 1. Pattern Priority System

When multiple patterns match, the highest priority wins:

```
Request: /api/payment/process

Matching patterns (in priority order):
1. Exact: /api/payment/process (priority 100) ← WINNER
2. Glob:  /api/payment/* (priority 50)
3. Prefix: /api/ (priority 10)

Result: Uses "payment_critical" scope with 100 req/min limit
```

### 2. Glob Pattern Syntax

- `*` - Matches single path segment (no slashes)
  - `/api/users/*` matches `/api/users/123` but not `/api/users/123/posts`
- `**` - Matches multiple segments (including slashes)
  - `/api/admin/**` matches `/api/admin/users/123/settings`

### 3. Tier-Based Multipliers

Rate limits are adjusted based on user tier:

```go
Free tier:      1.0x (base limit)
Premium tier:   2.0x (double the limit)
Enterprise tier: 5.0x (5x the limit)
```

Example: 100 req/min base limit
- Free: 100 req/min
- Premium: 200 req/min
- Enterprise: 500 req/min

### 4. Security Features

- **ReDoS Protection**: Regex matching with 100μs timeout
- **Complexity Limits**: Max 10 wildcards in globs, max 500 chars in regex
- **Pattern Validation**: All patterns validated at startup, not request time
- **Thread-Safe**: Full concurrency protection with RWMutex

### 5. Performance Characteristics

- **Exact matches**: ~13.5 ns/op (O(1) hash map lookup)
- **Glob patterns**: ~100-500 ns/op (optimized glob matching)
- **Regex patterns**: ~500-2000 ns/op (compiled regex with timeout)
- **Metrics overhead**: ~70 ns/op when enabled (negligible vs network latency)

## Code Structure

```
main.go (376 lines)
├── Pattern configuration (lines 45-73)
│   ├── Exact match for /api/payment/process
│   ├── Glob patterns for /api/payment/* and /api/admin/**
│   ├── Regex for versioned APIs
│   └── Prefix catch-all for /api/
├── Rate limiter configuration (lines 75-105)
│   └── Per-scope rate limits
├── Middleware setup (lines 107-154)
│   ├── Route-aware context extractor
│   └── Tier-based identity extraction
├── HTTP handlers (lines 156-250)
│   ├── Main API handler
│   ├── Debug endpoints
│   └── Metrics endpoint
└── Server initialization (lines 252-376)
    └── Graceful shutdown
```

## Testing Rate Limits

**Exceed the limit to see rate limiting in action:**

```bash
# Test with free-key-456 (payment_critical: 100/min base, free tier 1.0x = 100/min)
for i in {1..5}; do
  echo "Request $i:"
  curl -H "X-API-Key: free-key-456" http://localhost:8080/api/payment/process
  echo ""
done

# Expected results:
# Request 1:  HTTP 200 ✅ (within limit, ~99 remaining)
# Request 2:  HTTP 200 ✅ (within limit, ~98 remaining)
# ...
# Request 100: HTTP 200 ✅ (last allowed request, 0 remaining)
# Request 101: HTTP 429 ❌ (rate limited)
#
# Response when rate limited:
# HTTP 429 Too Many Requests
# X-RateLimit-Limit: 100
# X-RateLimit-Remaining: 0
# X-RateLimit-Reset: <unix-timestamp>
# Retry-After: <seconds-until-reset>
# {"error": "rate limit exceeded", "retry_after": "59s"}
```

**Test burst limit with rapid requests:**

```bash
# Rapidly hit endpoint to test burst handling
for i in {1..150}; do
  curl -s -o /dev/null -w "%{http_code} " \
    -H "X-API-Key: free-key-456" \
    http://localhost:8080/api/payment/process
done
echo ""

# You'll see: 200 200 200 ... (100 times) ... 429 429 429 (50 times)
```

## Customization Ideas

1. **Add Your Own Patterns**: Modify the routing.NewBuilder() configuration
2. **Adjust Rate Limits**: Change the ScopeLimits map for different thresholds
3. **Custom Tiers**: Add new tiers beyond free/premium/enterprise
4. **Database Integration**: Replace hardcoded API keys with database lookups
5. **Redis Backend**: Switch from memory store to Redis for distributed rate limiting
6. **Custom Metrics**: Add application-specific metrics alongside routing metrics

## Related Documentation

- [Main README](../../README.md) - Full Gorly documentation
- [Use Case 9](../../README.md#use-case-9-pattern-based-per-endpoint-rate-limiting-v120) - Pattern-based routing explanation
- [CHANGELOG v1.2.0](../../CHANGELOG.md#120---2025-11-02) - Complete v1.2.0 feature list
- [routing package godoc](https://pkg.go.dev/github.com/itsatony/gorly/routing) - API reference

## Production Considerations

1. **Metrics**: Enable Prometheus metrics in production for observability
2. **Pattern Complexity**: Keep patterns simple for best performance
3. **Priority Ranges**: Use priority ranges (e.g., 100s for critical, 50s for important, 10s for general)
4. **Testing**: Use `ExplainMatch()` and `ValidateConfiguration()` during development
5. **Monitoring**: Alert on `gorly_routing_no_matches_total` to catch unexpected paths

## Troubleshooting

**Pattern not matching?**
- Use the `/debug/routing/explain?path=<path>` endpoint to see why
- Check pattern syntax (glob `*` vs `**`, regex anchors `^$`)
- Verify priority order (higher priority patterns win)

**Rate limits not applying correctly?**
- Check that the scope returned by the pattern matches your ScopeLimits configuration
- Use `/debug/routing/inspect` to see all configured patterns and scopes
- Verify tier extraction logic is returning correct tier names

**Performance issues?**
- Profile with `go test -bench .` in routing package
- Consider reducing regex complexity or using glob patterns instead
- Check metrics for slow pattern matching (>1ms)

## License

This example is part of the Gorly project and is available under the same MIT license.
