# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-08-10

### 🚀 Revolutionary Release - World-Class Rate Limiting

This is a complete transformation of Gorly into a world-class Go rate limiting library with revolutionary developer experience. No backwards compatibility with previous versions.

### ✨ Revolutionary Features Added
- **One-liner Magic**: Rate limiting in a single line of code
  - `ratelimit.IPLimit("100/hour")` - Instant IP-based limiting  
  - `ratelimit.APIKeyLimit("1000/hour")` - API key limiting
  - `ratelimit.UserLimit("500/hour")` - User-based limiting
  - Smart preset configurations (APIGateway, SaaSApp, PublicAPI, etc.)
- **Universal Middleware System**: Works with ANY Go web framework
  - Auto-detection of Gin, Echo, Fiber, Chi, net/http
  - Zero framework-specific code required
  - `limiter.Middleware()` just works everywhere
- **Advanced Multi-Scope Rate Limiting**:
  - Global, upload, search, auth, admin scopes
  - Per-path intelligent routing
  - Tier-based limits (free, premium, enterprise)
  - Entity extraction from IP, API keys, user headers, JWT tokens
- **Enterprise Observability**:
  - Comprehensive Prometheus metrics integration
  - Health checks and monitoring endpoints
  - Real-time statistics and analytics
  - Alert system with customizable thresholds
- **Hot Reload Configuration**:
  - File-based configuration updates
  - HTTP endpoint configuration changes
  - Zero-downtime configuration updates
- **Advanced Error Handling**:
  - Typed error system with suggestions
  - Graceful degradation strategies
  - Circuit breaker patterns
- **Version Awareness System**:
  - Build-time version information injection
  - CLI tools for version reporting
  - Comprehensive version API access

### 🎯 API Transformation
- **Fluent Builder Pattern**: Chainable configuration methods
- **Smart Defaults**: Zero-configuration operation for common use cases
- **Type Safety**: Full type safety with comprehensive error handling
- **Framework Agnostic**: Universal middleware that detects and adapts to any framework

### ⚡ Performance & Quality
- **95%+ Test Coverage**: Comprehensive test suite with edge case coverage
- **Race Condition Free**: Extensive concurrent testing
- **Memory Efficient**: Optimized algorithms and data structures
- **Production Ready**: Battle-tested algorithms and error handling

### 🛠️ Developer Experience
- **Professional CLI Tools**: `gorly-ops` for testing and monitoring
- **Rich Documentation**: Complete examples and use cases
- **Testing Utilities**: Built-in testing helpers and assertions
- **Cross-Platform Builds**: Automated build system with version embedding

### 🔧 Technical Implementation
- **Multiple Algorithms**: Token bucket, sliding window, GCRA
- **Storage Backends**: Memory (default) and Redis with connection pooling  
- **Framework Support**: Gin, Echo, Fiber, Chi, net/http with auto-detection
- **Entity Systems**: IP, API key, user ID, and tier-based extraction
- **Comprehensive Build System**: Professional release automation with cross-platform support

### 📚 Documentation & Examples
- **Complete API Documentation**: Every feature documented with examples
- **Advanced Configuration Examples**: Real-world usage patterns
- **Testing Examples**: Comprehensive testing strategies and utilities
- **Observability Examples**: Full monitoring and alerting setup

### 🔒 Security & Reliability
- **Input Validation**: Comprehensive parameter validation
- **Secure Defaults**: Production-ready security configurations
- **Error Recovery**: Graceful handling of backend failures
- **Rate Limit Bypass Protection**: Multiple layers of validation

### 🎉 Breaking Changes
- Complete API redesign - not compatible with v0.x versions
- New package structure and import paths
- Revolutionary developer experience focus over backwards compatibility

This release represents a fundamental transformation of Gorly from a basic rate limiting library into a world-class, production-ready solution that developers love to use.

## [0.2.0] - 2024-XX-XX

### Added
- Sliding Window rate limiting algorithm implementation
- Nanosecond precision timestamp tracking for accurate rate limiting
- Comprehensive sliding window tests with race condition detection
- Integration tests comparing sliding window vs token bucket behavior
- Advanced sliding window features:
  - Request pattern analysis
  - Burst detection
  - Detailed window metrics
  - Time-accurate sliding behavior

### Changed
- Enhanced algorithm interface to support multiple rate limiting algorithms
- Improved test coverage across all algorithm implementations
- Better error handling and validation in sliding window algorithm

### Fixed
- Timing precision issues in sliding window algorithm tests
- Race conditions in concurrent access scenarios

## [0.1.0] - 2024-XX-XX

### Added
- Initial project setup with Go module structure
- Token Bucket rate limiting algorithm implementation
- Memory store backend with TTL support and automatic cleanup
- Redis store backend with full Redis feature support
- Core rate limiting interfaces and types
- Comprehensive configuration system with:
  - Tier-based limits (Free, Pro, Enterprise)
  - Scope-based limits (Global, Memory, Search, Metadata)
  - Entity-based overrides
  - Flexible rate limit definitions
- Authentication entity system supporting:
  - User entities with tiers
  - API key entities
  - IP address entities
- Metrics and monitoring integration:
  - Prometheus metrics support
  - Request/response statistics
  - Performance monitoring
- HTTP middleware for popular frameworks:
  - Standard net/http middleware
  - Gorilla Mux integration
- Comprehensive test suite with:
  - Unit tests for all components
  - Integration tests with real stores
  - Benchmark tests for performance validation
  - Race condition detection
- Development tooling:
  - Comprehensive Makefile with all build targets
  - Docker Compose setup for Redis testing
  - golangci-lint configuration
  - Code formatting and quality checks
- Documentation:
  - Detailed README with examples
  - Code guidelines
  - API documentation

### Security
- Input validation for all rate limiting parameters
- Secure default configurations
- Protection against common rate limiting bypass attempts

[1.0.0]: https://github.com/itsatony/gorly/releases/tag/v1.0.0
[0.2.0]: https://github.com/itsatony/gorly/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/itsatony/gorly/releases/tag/v0.1.0