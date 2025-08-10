# Gin Middleware Examples

This directory contains examples of using Gorly rate limiting with the Gin web framework.

## Examples

### basic.go
Simple setup with default configuration:
- IP-based rate limiting
- Default limits (100 requests/hour)
- Standard rate limit headers
- Memory store (for development)

### advanced.go  
Advanced configuration demonstrating:
- API key-based authentication
- Tier-based rate limits (free vs premium)
- Scope-based limits (read/write/upload/admin)
- Custom skip functions
- Enhanced logging and metrics
- Custom response messages

## Running the Examples

### Basic Example
```bash
cd examples/middleware/gin
go run basic.go
```

Test with:
```bash
# Normal request
curl -i http://localhost:8080/api/users

# Check rate limit status
curl http://localhost:8080/api/status
```

### Advanced Example
```bash
go run advanced.go
```

Test with different scenarios:

```bash
# IP-based rate limiting (free tier)
curl -i http://localhost:8080/api/users

# API key-based (premium tier)
curl -i -H "X-API-Key: premium-key-123" \
     -H "X-User-Tier: premium" \
     http://localhost:8080/api/users

# Different scopes
curl -i http://localhost:8080/api/search?q=test     # search scope
curl -i -X POST http://localhost:8080/api/upload   # upload scope  
curl -i http://localhost:8080/api/admin/stats      # admin scope

# Check detailed status
curl http://localhost:8080/api/rate-limit-status
```

## Gin-Specific Features

### Middleware Integration
```go
// Simple setup
r.Use(middleware.GinMiddleware(limiter))

// Custom configuration
plugin := &middleware.GinPlugin{}
ginMiddleware := plugin.CreateMiddleware(limiter, config)
r.Use(ginMiddleware.(gin.HandlerFunc))

// With custom config helper
r.Use(middleware.GinMiddlewareWithConfig(limiter, config))
```

### Context Integration
Rate limit information is automatically added to the Gin context:

```go
r.GET("/status", func(c *gin.Context) {
    result, _ := c.Get("ratelimit_result")
    entityID, _ := c.Get("ratelimit_entity_id") 
    scope, _ := c.Get("ratelimit_scope")
    
    rlResult := result.(*ratelimit.Result)
    // Use rate limit information
})
```

### Skip Functions
Gin provides several built-in skip functions:

```go
config.SkipFunc = middleware.CombineSkipFuncs(
    middleware.GinSkipHealthChecks(),  // Skip /health, /metrics, etc.
    middleware.GinSkipOptions(),       // Skip OPTIONS requests
    middleware.GinSkipStatic(),        // Skip static files
)
```

### Custom Skip Function
```go
config.SkipFunc = func(req *middleware.RequestInfo) bool {
    // Skip requests to /internal paths
    return strings.HasPrefix(req.Path, "/internal")
}
```

### Entity Extraction
The Gin plugin automatically extracts entities from:

1. **API Keys**: `Authorization`, `X-API-Key` headers
2. **User IDs**: `X-User-ID`, `X-User-Id` headers  
3. **IP Address**: Fallback using `c.ClientIP()` (handles proxies)

### Scope Extraction
Scopes are determined by:

1. **Route patterns**: `/api/admin/*` → `admin` scope
2. **HTTP methods**: `GET` → `read`, `POST` → `write`
3. **Custom logic**: Based on your configuration

### Configuration Options

```go
middlewareConfig := &middleware.Config{
    Limiter: limiter,
    
    EntityExtractor: middleware.GinEntityExtractor(),
    ScopeExtractor:  middleware.GinScopeExtractor(), 
    TierExtractor:   &middleware.DefaultTierExtractor{},
    
    SkipFunc: middleware.GinSkipHealthChecks(),
    
    ResponseConfig: middleware.ResponseConfig{
        RateLimitedStatusCode: 429,
        ErrorStatusCode:       500,
        IncludeHeaders:        true,
        HeaderPrefix:          "X-RateLimit-",
        ContentType:           "application/json",
        RateLimitedResponse:   []byte(`{"error":"Too many requests"}`),
    },
    
    Logger:         &ConsoleLogger{},
    MetricsEnabled: true,
}
```

## Production Considerations

### Redis Backend
For production, use Redis instead of memory store:

```go
config := ratelimit.DefaultConfig()
config.Store = "redis"
config.Redis = ratelimit.RedisConfig{
    Address:  "localhost:6379",
    Password: "",
    Database: 0,
}
```

### Rate Limit Headers
The middleware automatically adds standard headers:
- `X-RateLimit-Limit`: Total limit for the time window
- `X-RateLimit-Remaining`: Remaining requests in current window
- `X-RateLimit-Reset`: When the window resets (Unix timestamp)
- `Retry-After`: Seconds to wait when rate limited

### Error Handling
```go
config.ErrorHandler = func(req *middleware.RequestInfo, err error) bool {
    log.Printf("Rate limit error: %v", err)
    // Return false to reject the request
    return false
}
```

### Monitoring
Enable metrics collection:

```go
config.MetricsEnabled = true
config.MetricsPrefix = "gin_api_"
```

This exposes Prometheus metrics like:
- `gin_api_requests_total`
- `gin_api_requests_denied_total`  
- `gin_api_request_duration_seconds`

## Best Practices

1. **Use different scopes** for different types of operations
2. **Skip health checks** and static files from rate limiting  
3. **Configure appropriate limits** based on your API capacity
4. **Use tier-based limits** to offer different service levels
5. **Monitor rate limit hits** to understand usage patterns
6. **Test limits** in staging environment before production
7. **Handle rate limit errors gracefully** in your client code

## Troubleshooting

### Rate Limits Not Applied
- Check that middleware is registered before routes
- Verify entity extraction (check headers)
- Confirm rate limiter configuration

### High Memory Usage
- Switch from memory to Redis store
- Reduce stats retention period
- Check for memory leaks in custom extractors

### Performance Issues  
- Use Redis connection pooling
- Optimize custom extractor logic
- Consider using GCRA algorithm for high throughput

### Headers Not Showing
- Ensure `IncludeHeaders: true` in ResponseConfig
- Check that requests are not being skipped
- Verify middleware is processing requests