# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**gorly** is a world-class, reusable Go rate limiting library designed for production use. It was extracted from a larger REST API project and transformed into a standalone package that can be integrated anywhere.

### Core Architecture

The library follows a clean, modular architecture with clear separation of concerns:

- **Interfaces**: Core abstractions in `interfaces.go` and `ratelimit.go`
- **Algorithms**: Pluggable rate limiting algorithms in `algorithms/`
- **Stores**: Backend storage implementations in `stores/`
- **Middleware**: HTTP integration helpers in `middleware/`
- **Configuration**: Comprehensive config system in `config.go`

### Key Components

1. **RateLimiter Interface** (`ratelimit.go:115-137`): Main entry point with `Allow()`, `AllowN()`, `Reset()`, `Stats()` methods
2. **AuthEntity System** (`ratelimit.go:9-51`): Flexible entity abstraction supporting API keys, users, tenants, IPs
3. **Algorithm Interface** (`interfaces.go:9-19`): Pluggable rate limiting algorithms
4. **Store Interface** (`interfaces.go:22-46`): Storage backend abstraction
5. **Configuration** (`config.go:11-44`): Tier-based, scope-aware configuration system

### Implemented Features

- **Token Bucket Algorithm** (`algorithms/token_bucket.go`): Production-ready with burst support
- **Redis Store** (`stores/redis.go`): Full Redis backend with connection pooling
- **HTTP Middleware** (`middleware/http.go`): Ready-to-use HTTP integration
- **Prometheus Metrics** (`prometheus.go`, `metrics.go`): Built-in monitoring
- **Multi-tier Support**: Free, Premium, Enterprise tiers with different limits
- **Multi-scope Support**: Global, memory, search, metadata, analytics, admin scopes

## Development Commands

### Testing

```bash
# Run all tests
go test ./... -v

# Run tests with race detection
go test ./... -v -race

# Run integration tests
go test ./... -v -tags=integration

# Run benchmarks
go test ./... -bench=. -benchmem

# Test specific packages
go test ./algorithms -v
go test ./stores -v
go test ./middleware -v
```

### Code Quality

```bash
# Format code
go fmt ./...

# Lint code (requires golangci-lint)
golangci-lint run

# Vet code
go vet ./...

# Security scan (requires govulncheck)
govulncheck ./...
```

### Building & Validation

```bash
# Build the package (validation only - no binary output for libraries)
go build ./...

# Verify dependencies
go mod verify
go mod tidy

# Check for unused dependencies
go mod tidy && git diff --exit-code go.mod go.sum
```

## Architecture Guidelines

### Configuration Hierarchy

The library uses a sophisticated configuration hierarchy:

1. **Entity-specific overrides** (`config.go:387-392`)
2. **Tier-specific limits** (`config.go:395-411`)
3. **Scope-specific limits** (`config.go:414-416`)  
4. **Default global limits** (`config.go:418-420`)

### Rate Limit Resolution

When checking rate limits, the system:

1. Extracts entity (AuthEntity) and scope from request
2. Builds storage key using KeyBuilder (`ratelimit.go:208-231`)
3. Resolves appropriate rate limit using config hierarchy
4. Delegates to algorithm implementation
5. Records metrics and returns Result

### Error Handling

The library uses structured error types:

- **RateLimitError** (`ratelimit.go:141-178`): Main error type with categorization
- **ErrorType constants** (`ratelimit.go:144-149`): Store, algorithm, config, network, timeout
- Error wrapping follows Go 1.13+ patterns with `Unwrap()` support

### Thread Safety

All components are designed to be thread-safe:

- **rateLimiter struct** (`limiter.go:14-23`): Protected by RWMutex
- **Token bucket algorithm**: Uses atomic operations via storage backend
- **Redis store**: Inherently thread-safe with connection pooling
- **Mock stores**: Use sync.RWMutex for testing

## Extension Points

### Adding New Algorithms

1. Implement the `Algorithm` interface (`interfaces.go:9-19`)
2. Add algorithm creation in `createAlgorithm()` (`limiter.go:286-302`)
3. Update configuration validation (`config.go:287-294`)

Example algorithm structure:
```go
type MyAlgorithm struct {
    name string
}

func (ma *MyAlgorithm) Name() string { return ma.name }
func (ma *MyAlgorithm) Allow(ctx context.Context, store Store, key string, limit int64, window time.Duration, n int64) (*Result, error)
func (ma *MyAlgorithm) Reset(ctx context.Context, store Store, key string) error
```

### Adding New Stores

1. Implement the `Store` interface (`interfaces.go:22-46`)
2. Add store creation in `createStore()` (`limiter.go:262-283`)
3. Update configuration validation (`config.go:298-316`)

### Adding Middleware

Follow the pattern in `middleware/http.go`:

1. Create middleware struct with configuration
2. Implement entity extraction logic
3. Provide scope extraction if needed
4. Handle rate limit responses appropriately

## Testing Strategy

### Test Structure

- **Unit tests**: `*_test.go` files alongside source code
- **Integration tests**: `integration_test.go` with mock stores
- **Benchmarks**: Performance testing with `Benchmark*` functions
- **Mock implementations**: For testing without external dependencies

### Test Data Patterns

The codebase uses table-driven tests extensively:

```go
tests := []struct {
    name        string
    input       InputType
    expected    ExpectedType
    expectError bool
}{
    // test cases
}
```

### Mock Store Usage

Integration tests use `mockRedisStore` (`integration_test.go:14-89`) to simulate Redis behavior without requiring actual Redis instances.

## Code Conventions

### Naming Patterns

- **Constants**: ALL_CAPS with descriptive prefixes (`EntityTypeAPIKey`, `TierPremium`)
- **Interfaces**: Clear, focused contracts (`RateLimiter`, `AuthEntity`)
- **Implementations**: Descriptive names (`rateLimiter`, `tokenBucketWrapper`)
- **Errors**: Structured with type classification (`RateLimitError`)

### Package Structure

The package uses a flat structure with sub-packages for logical grouping:

```
gorly/
├── *.go           # Core interfaces and types
├── algorithms/    # Rate limiting algorithms
├── stores/        # Storage backends
└── middleware/    # HTTP/framework integration
```

### Error Patterns

All public methods return `error` as the last return value. Internal errors are wrapped with context using the `RateLimitError` type.

## Integration Examples

### Basic Usage

```go
config := ratelimit.DefaultConfig()
limiter, err := ratelimit.NewRateLimiter(config)
entity := ratelimit.NewDefaultAuthEntity("user123", ratelimit.EntityTypeUser, ratelimit.TierFree)
result, err := limiter.Allow(ctx, entity, ratelimit.ScopeGlobal)
```

### HTTP Middleware

```go
middleware := middleware.NewHTTPMiddleware(&middleware.HTTPMiddlewareConfig{
    Limiter: limiter,
    EntityExtractor: middleware.APIKeyEntityExtractor("X-API-Key", getUserTier),
})
router.Use(middleware.Middleware)
```

### Custom Configuration

Rate limits are configured using human-readable strings:

```go
config.TierLimits[ratelimit.TierPremium] = ratelimit.TierConfig{
    DefaultLimits: map[string]ratelimit.RateLimit{
        ratelimit.ScopeGlobal: {
            RateString: "1000/1h",  // 1000 requests per hour
            BurstSize:  100,        // Allow bursts of 100
        },
    },
}
```

## Dependencies

The library is designed to be lightweight with minimal dependencies:

- **Required**: `github.com/redis/go-redis/v9` for Redis store
- **Development**: Standard Go testing tools
- **Optional**: Prometheus client for metrics (if metrics enabled)

No external dependencies are required for the core interfaces and token bucket algorithm.

## Performance Characteristics

- **Memory**: Stateless design with all state in external stores
- **Latency**: Single Redis roundtrip per rate limit check
- **Throughput**: Supports high concurrency with connection pooling
- **Scaling**: Horizontal scaling through shared Redis backend

The token bucket algorithm performs all calculations client-side, minimizing Redis CPU usage and enabling predictable performance characteristics.

## Release Procedure

This comprehensive release procedure can be adapted for any software project, not just Gorly. It follows industry best practices for reliable, professional software releases.

### Pre-Release Checklist

#### 1. Code Quality & Testing
```bash
# Ensure all tests pass
go test ./... -v -race -timeout 30s

# Run security scanning
govulncheck ./...

# Code quality checks
golangci-lint run
go vet ./...
go fmt ./...

# Verify dependencies
go mod tidy
go mod verify

# Check for unused dependencies
git diff --exit-code go.mod go.sum
```

#### 2. Documentation Updates
- [ ] Update README.md with new features and examples
- [ ] Update CHANGELOG.md with all changes since last release
- [ ] Update API documentation (godoc comments)
- [ ] Verify all examples work with new version
- [ ] Update version references in documentation

#### 3. Version Management
```bash
# Determine version number (semantic versioning)
# MAJOR.MINOR.PATCH (e.g., v1.2.3)
# MAJOR: Breaking changes
# MINOR: New features, backward compatible
# PATCH: Bug fixes, backward compatible

export VERSION="v1.2.3"

# Update version constants in code if applicable
# Update go.mod version if needed
```

#### 4. Backward Compatibility Check
- [ ] Review API changes for breaking changes
- [ ] Document migration steps if breaking changes exist
- [ ] Ensure deprecated features are properly marked
- [ ] Test against previous version's examples

### Release Process

#### Phase 1: Preparation
```bash
# 1. Create release branch
git checkout -b release/${VERSION}
git push -u origin release/${VERSION}

# 2. Final testing on release branch
go test ./... -v -race -count=3

# 3. Build verification
go build ./...

# 4. Integration testing (if applicable)
go test ./... -tags=integration

# 5. Performance benchmarks (record for regression tracking)
go test ./... -bench=. -benchmem > benchmarks-${VERSION}.txt
```

#### Phase 2: Documentation & Communication
```bash
# 1. Update CHANGELOG.md
cat >> CHANGELOG.md << EOF

## [${VERSION}] - $(date +%Y-%m-%d)

### Added
- New feature descriptions

### Changed
- Modified behavior descriptions

### Deprecated
- Features marked for future removal

### Removed
- Deleted features

### Fixed
- Bug fix descriptions

### Security
- Security-related changes
EOF

# 2. Update README.md version references
sed -i "s/v[0-9]\+\.[0-9]\+\.[0-9]\+/${VERSION}/g" README.md

# 3. Commit documentation updates
git add .
git commit -m "docs: prepare for ${VERSION} release"
git push origin release/${VERSION}
```

#### Phase 3: Tag & Release Creation
```bash
# 1. Create annotated tag
git tag -a ${VERSION} -m "Release ${VERSION}

$(grep -A 20 "## \[${VERSION}\]" CHANGELOG.md | tail -n +2)"

# 2. Push tag
git push origin ${VERSION}

# 3. Merge release branch to main
git checkout main
git merge --no-ff release/${VERSION} -m "Release ${VERSION}"
git push origin main

# 4. Clean up release branch
git branch -d release/${VERSION}
git push origin --delete release/${VERSION}
```

#### Phase 4: Artifact Creation & Distribution

For Go libraries (like Gorly):
```bash
# 1. Verify module proxy can access the release
curl "https://proxy.golang.org/github.com/itsatony/gorly/@v/${VERSION}.info"

# 2. Test installation from public registry
cd /tmp
go mod init test-install
go get github.com/itsatony/gorly@${VERSION}
```

For applications with binaries:
```bash
# 1. Build release binaries for multiple platforms
PLATFORMS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

for platform in $PLATFORMS; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    
    output="gorly-${VERSION}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        output="${output}.exe"
    fi
    
    GOOS=$GOOS GOARCH=$GOARCH go build -o "dist/${output}" ./cmd/gorly
    
    # Create checksums
    shasum -a 256 "dist/${output}" >> "dist/checksums-${VERSION}.txt"
done

# 2. Create release archives
cd dist
for file in gorly-${VERSION}-*; do
    if [[ "$file" == *.exe ]]; then
        zip "${file%.*}.zip" "$file"
    else
        tar -czf "${file}.tar.gz" "$file"
    fi
done
```

### Post-Release Activities

#### 1. GitHub Release (if using GitHub)
```bash
# Using GitHub CLI
gh release create ${VERSION} \
    --title "Release ${VERSION}" \
    --notes-from-tag \
    dist/gorly-${VERSION}-*.tar.gz \
    dist/gorly-${VERSION}-*.zip \
    dist/checksums-${VERSION}.txt
```

#### 2. Communication & Announcements
- [ ] Update project website (if applicable)
- [ ] Announce on relevant community channels
- [ ] Update package registries (if applicable)
- [ ] Notify dependent projects (if breaking changes)
- [ ] Post release notes to social media/blog

#### 3. Docker Images (if applicable)
```bash
# Build and push Docker images
docker build -t myorg/gorly:${VERSION} .
docker build -t myorg/gorly:latest .

docker push myorg/gorly:${VERSION}
docker push myorg/gorly:latest
```

#### 4. Monitoring & Rollback Preparation
```bash
# 1. Monitor for issues
# - Check download statistics
# - Monitor issue tracker for new bugs
# - Watch community channels for feedback

# 2. Prepare rollback if needed
# Keep previous version artifacts available
# Document rollback procedure
```

### Release Types

#### Major Release (v2.0.0)
- Contains breaking changes
- Requires migration guide
- Extended testing period
- Clear communication about changes
- Consider compatibility layers

#### Minor Release (v1.2.0)
- New features, backward compatible
- Standard testing and documentation
- Regular release cycle

#### Patch Release (v1.1.1)
- Bug fixes only
- Expedited process for critical fixes
- Minimal risk, focused testing

#### Pre-Release (v1.2.0-beta.1)
```bash
# Use pre-release tags for testing
git tag -a v1.2.0-beta.1 -m "Beta release for testing"

# Go modules handle pre-releases automatically
go get github.com/itsatony/gorly@v1.2.0-beta.1
```

### Hotfix Process

For critical bug fixes that can't wait for the regular release cycle:

```bash
# 1. Create hotfix branch from main
git checkout main
git pull origin main
git checkout -b hotfix/${VERSION}

# 2. Make minimal fix
# 3. Test thoroughly
# 4. Update PATCH version
# 5. Follow abbreviated release process
# 6. Merge to both main and develop branches (if using GitFlow)
```

### Automation Recommendations

Consider automating parts of this process:

1. **CI/CD Integration**: Automate testing, building, and deployment
2. **Semantic Release**: Automatically determine version numbers
3. **Changelog Generation**: Generate changelogs from commit messages
4. **Artifact Building**: Build binaries and Docker images automatically
5. **Security Scanning**: Automated vulnerability checks

### Version Strategy

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR**: Incompatible API changes
- **MINOR**: Backward-compatible functionality additions
- **PATCH**: Backward-compatible bug fixes

### Quality Gates

Before any release:
- [ ] All tests pass (unit, integration, E2E)
- [ ] Code coverage meets minimum threshold
- [ ] Security scans pass
- [ ] Performance benchmarks within acceptable range
- [ ] Documentation is up-to-date
- [ ] Breaking changes are documented
- [ ] Migration guides are provided (if needed)

### Rollback Procedures

If issues are discovered post-release:

1. **Immediate**: Communicate the issue to users
2. **Short-term**: Provide workarounds if possible
3. **Fix**: Prepare and release a hotfix
4. **Learn**: Document lessons learned for future releases

This release procedure ensures reliable, professional software delivery while maintaining code quality and user trust.