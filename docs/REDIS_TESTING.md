# Redis Testing Setup

This guide explains how to set up and run Redis integration tests for the Gorly rate limiting library using Podman.

## Quick Start

```bash
# 1. Setup Redis with Podman
make redis-setup

# 2. Run Redis integration tests
make test-redis

# 3. Cleanup when done
make redis-cleanup
```

## Prerequisites

### Required Tools
- **Podman** - Container runtime (preferred over Docker)
- **netcat (nc)** - For port checking
- **Go 1.21+** - For running tests

### Installation

#### Podman Installation
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install podman

# RHEL/CentOS/Fedora
sudo dnf install podman

# macOS
brew install podman

# Arch Linux  
sudo pacman -S podman
```

#### Verify Installation
```bash
make tools-check
```

## Available Make Targets

### Redis Management
```bash
make redis-setup        # Setup Redis container with Podman
make redis-cleanup      # Clean up Redis testing environment
make redis-logs         # Show Redis container logs
make redis-cli          # Connect to Redis CLI
```

### Testing
```bash
make test-redis         # Run Redis integration tests (requires Redis)
make test-redis-setup   # Setup Redis and run tests in one command
make test-redis-verbose # Run Redis tests with verbose output
```

### Development
```bash
make redis-logs         # Monitor Redis logs
make redis-cli          # Interactive Redis CLI access
```

## Manual Setup

If you prefer manual control:

### Start Redis
```bash
# Using the setup script
./scripts/setup-redis.sh

# Or manually with podman
podman run -d \
  --name gorly-redis \
  -p 6379:6379 \
  -v gorly-redis-data:/data \
  docker.io/redis:7-alpine \
  redis-server --appendonly yes
```

### Run Tests
```bash
# All Redis tests
go test -tags=redis ./test/redis/...

# Specific test
go test -tags=redis -v ./test/redis/... -run TestRedisIntegration

# With race detection
go test -tags=redis -race ./test/redis/...
```

### Stop Redis
```bash
# Using cleanup script
./scripts/cleanup-redis.sh

# Or manually
podman stop gorly-redis
podman rm gorly-redis
```

## Redis Configuration

The test setup uses optimized Redis configuration:

```yaml
# docker-compose.yml
services:
  redis:
    image: docker.io/redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
```

### Key Settings
- **Persistence**: AOF (Append Only File) enabled for durability
- **Memory Limit**: 256MB with LRU eviction policy  
- **Health Checks**: Built-in monitoring with retries
- **Data Volume**: Persistent storage for test data

## Test Structure

### Test Databases
Tests use separate Redis databases to avoid conflicts:
- **Database 0**: Basic integration tests
- **Database 1**: Middleware integration tests
- **Database 2**: Plugin registry tests
- **Database 3**: Statistics and monitoring tests

### Test Coverage
1. **Basic Rate Limiting**: Verify core functionality with Redis backend
2. **Middleware Integration**: Test all framework plugins with Redis persistence
3. **Multi-Request Sequences**: Validate rate limit enforcement over time
4. **Plugin Compatibility**: Ensure all plugins (Gin, Echo, Fiber, Chi) work with Redis
5. **Health Monitoring**: Test Redis connectivity and health checks
6. **Statistics Collection**: Verify metrics and stats persistence

## Monitoring and Debugging

### Redis Commander Web UI
Access at: http://localhost:8081

Features:
- Browse keys and data structures
- Execute Redis commands
- Monitor memory usage
- View database statistics

### CLI Monitoring
```bash
# Connect to Redis CLI
make redis-cli

# Monitor all commands in real-time
> MONITOR

# Get Redis info
> INFO

# List all keys
> KEYS *

# List Gorly-specific keys
> KEYS gorly:*

# Check database sizes
> DBSIZE

# Switch databases
> SELECT 1
> DBSIZE
```

### Container Logs
```bash
# View current logs
make redis-logs

# Follow logs in real-time
podman logs -f gorly-redis

# View logs from specific time
podman logs --since="2024-01-01T00:00:00" gorly-redis
```

## Troubleshooting

### Redis Not Starting

**Error**: `Failed to create rate limiter: dial tcp 127.0.0.1:6379: connect: connection refused`

**Solutions**:
```bash
# Check if Redis is running
podman ps | grep redis

# Check Redis logs
make redis-logs

# Restart Redis
make redis-cleanup
make redis-setup

# Check port availability
netstat -tulpn | grep 6379
```

### Permission Issues

**Error**: `Error: cannot clone: Operation not permitted`

**Solutions**:
```bash
# Check Podman setup
podman system info

# Reset Podman if needed
podman system reset

# Check user namespace
echo $XDG_RUNTIME_DIR

# Try rootful mode (if needed)
sudo podman run ...
```

### Memory Issues

**Error**: `OOM: cannot allocate memory`

**Solutions**:
```bash
# Check available memory
free -h

# Reduce Redis memory limit in docker-compose.yml
maxmemory 128mb

# Clear Redis data
make redis-cli
> FLUSHALL

# Restart with clean state
make redis-cleanup
make redis-setup
```

### Network Conflicts

**Error**: `Port already in use`

**Solutions**:
```bash
# Check what's using port 6379
lsof -i :6379

# Stop conflicting services
sudo systemctl stop redis-server

# Use different port (edit docker-compose.yml)
ports:
  - "6380:6379"

# Update test configuration
export REDIS_ADDRESS=localhost:6380
```

### Test Failures

**Error**: `Test timed out`

**Solutions**:
```bash
# Increase test timeout
go test -timeout 5m -tags=redis ./test/redis/...

# Run tests with more verbose output
go test -v -tags=redis ./test/redis/...

# Run single test for debugging
go test -v -tags=redis ./test/redis/... -run TestRedisIntegration

# Check Redis connectivity
make redis-cli
> PING
```

### Data Persistence Issues

**Error**: `Keys disappearing between tests`

**Solutions**:
```bash
# Check Redis persistence settings
make redis-cli
> CONFIG GET save
> CONFIG GET appendonly

# Verify volume mounting
podman volume inspect gorly-redis-data

# Force save data
> BGSAVE
> BGREWRITEAOF
```

## Performance Testing

### Load Testing
```bash
# Run benchmarks with Redis
go test -bench=. -benchmem -tags=redis ./test/redis/...

# Concurrent test
go test -v -tags=redis ./test/redis/... -run TestRedisMiddleware -count=10

# High-load simulation
for i in {1..100}; do
  go test -tags=redis ./test/redis/... &
done
wait
```

### Monitoring During Tests
```bash
# Terminal 1: Run tests
make test-redis-verbose

# Terminal 2: Monitor Redis
make redis-cli
> INFO stats

# Terminal 3: Monitor container resources  
podman stats gorly-redis
```

## Production Considerations

When moving from testing to production:

### Redis Deployment
1. **High Availability**: Use Redis Cluster or Sentinel
2. **Persistence**: Configure both RDB and AOF
3. **Memory Management**: Set appropriate maxmemory and eviction policies
4. **Security**: Enable authentication and TLS
5. **Monitoring**: Set up Redis monitoring (Prometheus, Grafana)
6. **Backup Strategy**: Regular backups and point-in-time recovery

### Configuration Changes
```yaml
# Production Redis config
redis:
  address: "redis-cluster.example.com:6379"
  password: "${REDIS_PASSWORD}"
  tls_enabled: true
  pool_size: 20
  min_idle_conns: 5
  max_retries: 3
  retry_delay: "100ms"
```

### Application Settings
```yaml
# Production rate limiter config
store: redis
enable_metrics: true
operation_timeout: "5s"
max_concurrent_requests: 1000
cleanup_interval: "1h"
stats_retention: "24h"
```

## Best Practices

1. **Always use cleanup**: Run cleanup script after testing to free resources
2. **Separate test databases**: Use different DBs for different test suites
3. **Monitor resources**: Check memory and CPU usage during long test runs
4. **Data isolation**: Clear test data between test suites when needed
5. **Health checks**: Verify Redis is healthy before running tests
6. **Version pinning**: Use specific Redis versions for consistent testing
7. **Container naming**: Use descriptive names for easy identification
8. **Log retention**: Configure appropriate log retention policies

## Integration with CI/CD

### GitHub Actions Example
```yaml
# .github/workflows/redis-tests.yml
name: Redis Integration Tests

on: [push, pull_request]

jobs:
  redis-tests:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v3
      with:
        go-version: '1.21'
    
    - name: Run Redis tests
      run: make test-redis
```

### Local Development Workflow
```bash
# Daily development cycle
make redis-setup        # Start Redis (once per day)
make test-redis         # Run tests (multiple times)
make redis-cleanup      # Cleanup (end of day)

# Quick test cycle during development
make test-redis-verbose  # Run with detailed output
make redis-logs         # Check for any Redis issues
```