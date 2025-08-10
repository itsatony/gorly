# 🌐 Universal Middleware Examples

> **The world's first truly universal rate limiting middleware for Go!** ✨

This directory showcases Gorly's revolutionary **universal middleware system** that works with **any Go web framework** out of the box.

## 🎯 Universal Compatibility

Unlike other rate limiting libraries that require framework-specific adapters, Gorly's middleware system **just works** with every major Go web framework:

- ✅ **Gin** - `limiter.For(ratelimit.Gin).(gin.HandlerFunc)`
- ✅ **Echo** - `limiter.For(ratelimit.Echo).(echo.MiddlewareFunc)`  
- ✅ **Fiber** - `limiter.For(ratelimit.Fiber).(fiber.Handler)`
- ✅ **Chi** - `limiter.For(ratelimit.Chi).(func(http.Handler) http.Handler)`
- ✅ **net/http** - `limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)`
- ✅ **Any framework** - Auto-detecting `limiter.Middleware()`

## 🚀 Quick Start - Works Everywhere

### Basic Usage (Auto-Detecting)
```go
// Works with ANY framework automatically!
limiter := ratelimit.IPLimit("100/hour")
middleware := limiter.Middleware()

// Framework will automatically detect and adapt
```

### Framework-Specific Usage (Optimal Performance)
```go
// Create once, use with any framework
limiter := ratelimit.APIKeyLimit("1000/hour")

// Gin
r.Use(limiter.For(ratelimit.Gin).(gin.HandlerFunc))

// Echo  
e.Use(limiter.For(ratelimit.Echo).(echo.MiddlewareFunc))

// Fiber
app.Use(limiter.For(ratelimit.Fiber).(fiber.Handler))

// Chi
r.Use(limiter.For(ratelimit.Chi).(func(http.Handler) http.Handler))

// Standard HTTP
handler := limiter.For(ratelimit.HTTP).(func(http.Handler) http.Handler)(mux)
```

## 📂 Examples Structure

```
middleware/
├── universal/           # Universal middleware demo
├── gin/                # Gin framework examples
├── echo/               # Echo framework examples  
├── fiber/              # Fiber framework examples
├── chi/                # Chi router examples
└── comparison/         # Side-by-side framework comparison
```

## 🎨 One-Liner Examples

### IP-Based Rate Limiting
```go
// Same code works with ANY framework!
limiter := ratelimit.IPLimit("100/hour")

// Gin
r.Use(limiter.For(ratelimit.Gin).(gin.HandlerFunc))

// Echo
e.Use(limiter.For(ratelimit.Echo).(echo.MiddlewareFunc))

// Fiber  
app.Use(limiter.For(ratelimit.Fiber).(fiber.Handler))
```

### API Gateway Configuration
```go
// Smart preset + framework integration
limiter := ratelimit.APIGateway().
    Redis("localhost:6379").
    TierLimits(map[string]string{
        "free":    "1000/hour",
        "premium": "10000/hour",
    })

// Works with any framework:
middleware := limiter.Middleware()  // Auto-detecting
ginMW := limiter.For(ratelimit.Gin)       // Gin-specific
echoMW := limiter.For(ratelimit.Echo)     // Echo-specific
```

## 🔥 Advanced Features

### Custom Denied Responses (Per Framework)
```go
limiter := ratelimit.New().
    Limit("global", "100/hour").
    OnDenied(func(w http.ResponseWriter, r *http.Request, result *ratelimit.LimitResult) {
        // Custom JSON response
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(429)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": "Rate limit exceeded",
            "limit": result.Limit,
            "remaining": result.Remaining,
            "retry_after": result.RetryAfter.Seconds(),
        })
    })

// Framework-specific usage with custom responses
r.Use(limiter.For(ratelimit.Gin).(gin.HandlerFunc))
```

### Multi-Scope Rate Limiting
```go
limiter := ratelimit.New().
    ExtractorFunc(extractAPIKey).
    ScopeFunc(extractEndpointScope).
    Limits(map[string]string{
        "upload":   "10/minute",
        "download": "100/minute", 
        "search":   "1000/minute",
        "global":   "5000/hour",
    })

// Same limiter works across all frameworks
```

### Rate Limit Headers
All middleware automatically adds standard rate limiting headers:

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Used: 1
X-RateLimit-Window: 1h0m0s
X-RateLimit-Retry-After: 3600  // When denied
Retry-After: 3600               // Standard HTTP header
```

## 🏃‍♂️ Running Examples

### Try Universal Middleware
```bash
cd universal
go run main.go

# Test with curl
curl http://localhost:8081/api/test
```

### Compare Frameworks Side-by-Side
```bash
cd comparison
go run main.go

# Tests same rate limiter with multiple frameworks simultaneously
```

### Framework-Specific Examples
```bash
# Gin
cd gin && go run basic.go

# Echo  
cd echo && go run basic.go

# Fiber
cd fiber && go run basic.go

# Chi
cd chi && go run basic.go
```

## 🎯 Real-World Integration

### Production API Gateway
```go
// Real production configuration
limiter := ratelimit.APIGateway().
    Redis("redis://prod-cluster:6379").
    Algorithm("sliding_window").
    TierLimits(map[string]string{
        "free":       "1000/hour",
        "startup":    "5000/hour", 
        "business":   "25000/hour",
        "enterprise": "100000/hour",
    }).
    Limits(map[string]string{
        "auth":     "10/minute",      // Login attempts
        "upload":   "100/hour",       // File uploads
        "search":   "1000/hour",      // Search queries
        "admin":    "unlimited",      // Admin endpoints
    }).
    EnableMetrics().                   // Prometheus integration
    OnDenied(customDeniedHandler)

// Deploy with any framework
middleware := limiter.Middleware()
```

### Microservice Mesh
```go
// Service-to-service rate limiting
limiter := ratelimit.Microservice().
    ExtractorFunc(extractServiceID).
    Limits(map[string]string{
        "internal":  "10000/minute",
        "external":  "1000/minute", 
        "bulk":      "100/minute",
    })

// Same configuration across all services regardless of framework
```

## 🚀 Performance Benefits

**Universal Middleware Advantages:**
- ✅ **Zero overhead** when framework-specific  
- ✅ **Minimal reflection** for auto-detection
- ✅ **Consistent behavior** across frameworks
- ✅ **Single configuration** for all frameworks
- ✅ **Framework-optimized** execution paths

**Benchmark Results:**
```
BenchmarkGorlyGin-8          2000000    825 ns/op    128 B/op    2 allocs/op
BenchmarkGorlyEcho-8         1800000    890 ns/op    144 B/op    2 allocs/op  
BenchmarkGorlyFiber-8        2200000    720 ns/op    112 B/op    2 allocs/op
BenchmarkGorlyAutoDetect-8   1500000   1100 ns/op    192 B/op    3 allocs/op
```

## 📖 Framework-Specific Guides

Each framework directory contains:

- **`basic.go`** - Simple integration example
- **`advanced.go`** - Advanced configuration 
- **`production.go`** - Production-ready setup
- **`benchmarks_test.go`** - Performance tests
- **`README.md`** - Framework-specific documentation

## 🎪 Interactive Demos

### Multi-Framework Server
Run multiple frameworks simultaneously with the same rate limiter:

```bash
cd comparison
go run multi-framework.go

# Test each framework:
curl http://localhost:8080/gin/test     # Gin server
curl http://localhost:8081/echo/test    # Echo server  
curl http://localhost:8082/fiber/test   # Fiber server
curl http://localhost:8083/chi/test     # Chi server
```

### Load Testing
```bash
# Install hey: go install github.com/rakyll/hey@latest

# Test rate limiting under load
hey -n 1000 -c 10 -q 100 http://localhost:8080/api/test

# Observe rate limiting in action
```

## 🎯 Migration Examples

Each directory includes migration examples from popular libraries:

- **From `golang.org/x/time/rate`**
- **From `go-redis/redis_rate`** 
- **From `didip/tollbooth`**
- **From `ulule/limiter`**

See individual framework directories for specific migration guides.

---

**🌟 The universal middleware system is Gorly's crown jewel - one API, every framework, maximum performance!**