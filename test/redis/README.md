# Redis Integration Tests

This directory contains integration tests that require a running Redis instance.

## Setup

### Quick Start with Podman
```bash
# Setup Redis with Podman
./scripts/setup-redis.sh

# Run Redis tests
make test-redis

# Cleanup when done
./scripts/cleanup-redis.sh
```

### Manual Redis Setup
If you prefer to manage Redis manually:

```bash
# Start Redis with podman
podman run -d --name redis-test -p 6379:6379 docker.io/redis:7-alpine

# Run tests
go test -tags=redis ./test/redis/...

# Stop Redis
podman stop redis-test
podman rm redis-test
```

## Test Structure

### integration_test.go
Contains comprehensive tests for:

- **Basic Redis Integration**: Verifies rate limiter works with Redis backend
- **Middleware Integration**: Tests middleware plugins with Redis persistence
- **Plugin Registry**: Ensures all framework plugins work with Redis
- **Health Checks**: Validates Redis connectivity
- **Statistics**: Tests stats collection with Redis backend

## Test Scenarios

### 1. Basic Rate Limiting
Tests that basic rate limiting works with Redis:
```go
limiter, err := ratelimit.NewRateLimiter(config)
result, err := limiter.Allow(ctx, entity, scope)
```

### 2. Middleware Integration
Tests that middleware plugins persist data to Redis:
```go
middlewareConfig := middleware.DefaultConfig()
middlewareConfig.Limiter = limiter
result, err := middleware.ProcessRequest(req, middlewareConfig)
```

### 3. Multi-Request Sequences
Tests rate limiting behavior over multiple requests:
- First 5 requests should be allowed
- 6th request should be denied
- Data persists between requests via Redis

### 4. Plugin Compatibility
Verifies all framework plugins work with Redis:
- Gin plugin + Redis
- Echo plugin + Redis  
- Fiber plugin + Redis
- Chi plugin + Redis

### 5. Health and Stats
Tests monitoring capabilities:
- Redis health checks
- Statistics collection
- Scope-specific metrics

## Configuration

Tests use different Redis databases to avoid conflicts:
- Database 0: Basic integration tests
- Database 1: Middleware tests
- Database 2: Plugin registry tests  
- Database 3: Statistics tests

## Running Tests

### All Redis Tests
```bash
make test-redis
```

### Specific Test
```bash
go test -tags=redis -v ./test/redis/... -run TestRedisIntegration
```

### With Race Detection
```bash
go test -tags=redis -race ./test/redis/...
```

### Verbose Output
```bash
go test -tags=redis -v ./test/redis/...
```

## Troubleshooting

### Redis Not Available
```
Error: dial tcp 127.0.0.1:6379: connect: connection refused
```

**Solution**: Ensure Redis is running
```bash
./scripts/setup-redis.sh
```

### Permission Denied
```
Error: unable to start container process
```

**Solution**: Check podman permissions
```bash
podman system info
```

### Tests Timeout
```
Error: context deadline exceeded
```

**Solution**: Redis might be slow to start
```bash
# Check Redis status
podman logs gorly-redis

# Restart if needed
./scripts/cleanup-redis.sh
./scripts/setup-redis.sh
```

### Database Conflicts
Tests use separate databases, but you can manually clear:
```bash
# Connect to Redis
podman exec -it gorly-redis redis-cli

# Clear all test databases
> SELECT 0
> FLUSHDB
> SELECT 1
> FLUSHDB
> SELECT 2
> FLUSHDB
> SELECT 3
> FLUSHDB
```

## Redis Configuration

The tests use optimized Redis settings:
- **Memory limit**: 256MB with LRU eviction
- **Persistence**: AOF enabled for data durability
- **Health checks**: Built-in monitoring
- **Network**: Isolated network for testing

## Monitoring

### Redis Commander Web UI
Access at: http://localhost:8081
- View keys and data
- Monitor memory usage
- Execute Redis commands

### CLI Monitoring
```bash
# Connect to Redis CLI
podman exec -it gorly-redis redis-cli

# Monitor commands
> MONITOR

# Get info
> INFO

# List keys
> KEYS gorly:*
```

## Performance Testing

For performance testing with Redis:

```bash
# Run benchmarks
go test -tags=redis -bench=. -benchmem ./test/redis/...

# High concurrency test
go test -tags=redis -v ./test/redis/... -run TestRedisMiddleware -count=10
```

## Best Practices

1. **Always cleanup**: Use cleanup script after testing
2. **Separate databases**: Use different DBs for different test suites  
3. **Health checks**: Verify Redis is ready before running tests
4. **Data isolation**: Clear test data between runs
5. **Monitor resources**: Check memory usage during long tests

## Production Considerations

When moving to production Redis:

1. **Use Redis Cluster** for high availability
2. **Configure persistence** (RDB + AOF)
3. **Set up monitoring** (Redis metrics)
4. **Use connection pooling** in your application
5. **Configure memory limits** and eviction policies
6. **Set up backups** for critical data