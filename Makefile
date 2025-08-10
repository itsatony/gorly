# Makefile for gorly - World-class Go Rate Limiting Library
#
# Available targets:
#   make help         - Show this help message
#   make build        - Build all packages
#   make test         - Run all tests
#   make test-redis   - Run Redis integration tests
#   make bench        - Run benchmarks
#   make coverage     - Generate test coverage report
#   make lint         - Run linters
#   make fmt          - Format code
#   make vet          - Run go vet
#   make clean        - Clean build artifacts
#   make deps         - Download dependencies
#   make check        - Run all checks (fmt, vet, lint, test)
#   make redis-setup  - Setup Redis for testing with Podman
#   make examples     - Build example applications

.PHONY: help build test test-redis test-redis-setup test-redis-verbose bench coverage lint fmt vet clean deps check redis-setup redis-cleanup redis-logs redis-cli docker docker-test docker-clean examples all

# Variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Paths
PKG_LIST := $(shell go list ./...)
TEST_COVERAGE_PROFILE := coverage.out
TEST_COVERAGE_HTML := coverage.html

# Build flags
BUILD_FLAGS := -v
TEST_FLAGS := -v -race -timeout=30s
BENCH_FLAGS := -benchmem -run=^$$ -bench .

# Default target
all: check build

# Help target
help: ## Show this help message
	@echo "gorly - World-class Go Rate Limiting Library"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
build: ## Build all packages
	@echo "Building all packages..."
	$(GOBUILD) $(BUILD_FLAGS) ./...

build-cli: ## Build CLI binary
	@echo "Building CLI binary..."
	@mkdir -p bin
	$(GOBUILD) $(BUILD_FLAGS) -o bin/gorly ./cmd/gorly

# Test targets  
test: ## Run all tests
	@echo "Running all tests..."
	$(GOTEST) $(TEST_FLAGS) ./...

test-short: ## Run short tests only
	@echo "Running short tests..."
	$(GOTEST) $(TEST_FLAGS) -short ./...

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	$(GOTEST) $(TEST_FLAGS) ./...

integration-test: ## Run integration tests
	@echo "Running integration tests..."
	$(GOTEST) $(TEST_FLAGS) -run "TestRateLimiter.*Integration|TestRateLimiter.*Memory|TestRateLimiter.*SlidingWindow" ./...

test-redis: ## Run Redis integration tests (requires Redis)
	@echo "Running Redis integration tests..."
	@if ! command -v podman >/dev/null 2>&1; then \
		echo "❌ Podman not found. Please install podman or start Redis manually on localhost:6379"; \
		exit 1; \
	fi
	@echo "Checking if Redis is available on localhost:6379..."
	@if ! nc -z localhost 6379 2>/dev/null; then \
		echo "❌ Redis not found on localhost:6379"; \
		echo "💡 Run './scripts/setup-redis.sh' to start Redis with Podman"; \
		exit 1; \
	fi
	@echo "✅ Redis is available, running tests..."
	$(GOTEST) $(TEST_FLAGS) -tags=redis ./test/redis/...

test-redis-setup: ## Setup Redis and run integration tests
	@echo "Setting up Redis and running integration tests..."
	./scripts/setup-redis.sh
	$(GOTEST) $(TEST_FLAGS) -tags=redis ./test/redis/...

test-redis-verbose: ## Run Redis tests with verbose output
	@echo "Running Redis integration tests (verbose)..."
	$(GOTEST) $(TEST_FLAGS) -tags=redis -v ./test/redis/...

bench: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) $(BENCH_FLAGS) ./...

bench-compare: ## Run benchmarks with comparison
	@echo "Running benchmarks with memory allocation info..."
	$(GOTEST) $(BENCH_FLAGS) -memprofile=mem.prof -cpuprofile=cpu.prof ./...

# Coverage targets
coverage: ## Generate test coverage report
	@echo "Generating test coverage report..."
	$(GOTEST) $(TEST_FLAGS) -coverprofile=$(TEST_COVERAGE_PROFILE) ./...
	$(GOCMD) tool cover -html=$(TEST_COVERAGE_PROFILE) -o $(TEST_COVERAGE_HTML)
	@echo "Coverage report generated: $(TEST_COVERAGE_HTML)"

coverage-func: ## Show test coverage by function
	@echo "Showing test coverage by function..."
	$(GOTEST) $(TEST_FLAGS) -coverprofile=$(TEST_COVERAGE_PROFILE) ./...
	$(GOCMD) tool cover -func=$(TEST_COVERAGE_PROFILE)

# Code quality targets
fmt: ## Format code with gofmt
	@echo "Formatting code..."
	@files=$$(find . -name '*.go' -not -path './vendor/*' -not -path './.git/*'); \
	if [ -n "$$files" ]; then \
		$(GOFMT) -w $$files; \
		echo "Formatted $$(echo $$files | wc -w) files"; \
	fi

fmt-check: ## Check if code is formatted
	@echo "Checking code formatting..."
	@files=$$(find . -name '*.go' -not -path './vendor/*' -not -path './.git/*'); \
	if [ -n "$$files" ]; then \
		unformatted=$$($(GOFMT) -l $$files); \
		if [ -n "$$unformatted" ]; then \
			echo "The following files are not formatted:"; \
			echo "$$unformatted"; \
			exit 1; \
		fi; \
	fi

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

lint: ## Run golangci-lint (requires golangci-lint to be installed)
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --config .golangci.yml; \
	else \
		echo "golangci-lint is not installed. Please install it: https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

lint-install: ## Install golangci-lint
	@echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.54.2

# Security targets
security: ## Run security checks with govulncheck
	@echo "Running security checks..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck is not installed. Installing..."; \
		go install golang.org/x/vuln/cmd/govulncheck@latest; \
		govulncheck ./...; \
	fi

# Dependency targets
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

deps-verify: ## Verify dependencies
	@echo "Verifying dependencies..."
	$(GOMOD) verify

deps-update: ## Update dependencies to latest versions
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

# Check targets (combines multiple quality checks)
check: fmt-check vet test ## Run all quality checks
	@echo "All checks passed!"

check-all: fmt-check vet lint security test ## Run all quality checks including linting and security
	@echo "All comprehensive checks passed!"

# Container targets (Podman/Docker)
redis-setup: ## Setup Redis for testing with Podman
	@echo "Setting up Redis for testing..."
	./scripts/setup-redis.sh

redis-cleanup: ## Clean up Redis testing environment
	@echo "Cleaning up Redis testing environment..."
	./scripts/cleanup-redis.sh

redis-logs: ## Show Redis container logs
	@echo "Showing Redis logs..."
	@if podman ps | grep -q gorly-redis; then \
		podman logs gorly-redis; \
	else \
		echo "❌ Redis container not running. Run 'make redis-setup' first."; \
	fi

redis-cli: ## Connect to Redis CLI
	@echo "Connecting to Redis CLI..."
	@if podman ps | grep -q gorly-redis; then \
		podman exec -it gorly-redis redis-cli; \
	else \
		echo "❌ Redis container not running. Run 'make redis-setup' first."; \
	fi

# Docker targets (legacy - prefer Podman targets above)
docker: ## Build Docker containers for testing
	@echo "Building Docker containers..."
	@if command -v podman-compose >/dev/null 2>&1; then \
		podman-compose build; \
	elif command -v docker-compose >/dev/null 2>&1; then \
		docker-compose build; \
	else \
		echo "❌ Neither podman-compose nor docker-compose found"; \
		exit 1; \
	fi

docker-test: ## Run tests in containers
	@echo "Running tests in containers..."
	@if command -v podman-compose >/dev/null 2>&1; then \
		podman-compose up --abort-on-container-exit; \
	elif command -v docker-compose >/dev/null 2>&1; then \
		docker-compose up --abort-on-container-exit; \
	else \
		echo "❌ Neither podman-compose nor docker-compose found"; \
		exit 1; \
	fi

docker-clean: ## Clean containers and images
	@echo "Cleaning containers and images..."
	@if command -v podman-compose >/dev/null 2>&1; then \
		podman-compose down --rmi all --volumes; \
	elif command -v docker-compose >/dev/null 2>&1; then \
		docker-compose down --rmi all --volumes; \
	else \
		echo "❌ Neither podman-compose nor docker-compose found"; \
	fi

# Example targets
examples: ## Build example applications
	@echo "Building example applications..."
	@if [ -d "examples" ]; then \
		for example in examples/*/; do \
			if [ -f "$$example/main.go" ]; then \
				echo "Building $$example..."; \
				(cd "$$example" && $(GOBUILD) $(BUILD_FLAGS) .); \
			fi; \
		done; \
	else \
		echo "No examples directory found"; \
	fi

examples-run: ## Run example applications
	@echo "Running example applications..."
	@if [ -d "examples" ]; then \
		for example in examples/*/; do \
			if [ -f "$$example/main.go" ]; then \
				echo "Running $$example..."; \
				(cd "$$example" && timeout 10s $(GOCMD) run . || true); \
			fi; \
		done; \
	fi

# Clean targets
clean: ## Clean build artifacts and temporary files
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	rm -f $(TEST_COVERAGE_PROFILE) $(TEST_COVERAGE_HTML)
	rm -f mem.prof cpu.prof
	rm -rf bin/
	find . -name '*.test' -delete
	find . -name '*.tmp' -delete

clean-all: clean docker-clean ## Clean everything including Docker artifacts

# Release targets
release-check: check-all ## Pre-release checks
	@echo "Running pre-release checks..."
	@echo "✓ All checks passed - ready for release!"

# Development targets
dev-setup: deps lint-install ## Set up development environment
	@echo "Setting up development environment..."
	@echo "Installing development tools..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install golang.org/x/tools/cmd/goimports@latest
	@echo "Development environment ready!"

dev-watch: ## Watch for file changes and run tests (requires entr)
	@echo "Watching for changes... (requires 'entr' to be installed)"
	@if command -v entr >/dev/null 2>&1; then \
		find . -name '*.go' -not -path './vendor/*' | entr -c make test; \
	else \
		echo "entr is not installed. Please install it to use file watching."; \
		echo "On macOS: brew install entr"; \
		echo "On Linux: apt-get install entr or yum install entr"; \
	fi

# Statistics targets
stats: ## Show code statistics
	@echo "Code Statistics:"
	@echo "==============="
	@find . -name '*.go' -not -path './vendor/*' -not -path './.git/*' | xargs wc -l | sort -n
	@echo ""
	@echo "Package Information:"
	@go list -f '{{.ImportPath}} - {{.Doc}}' ./...

# Tools validation
tools-check: ## Check if required tools are installed
	@echo "Checking required tools..."
	@echo -n "go: "; go version || echo "❌ NOT INSTALLED"
	@echo -n "golangci-lint: "; golangci-lint version 2>/dev/null || echo "❌ NOT INSTALLED (run 'make lint-install')"
	@echo -n "govulncheck: "; govulncheck -version 2>/dev/null || echo "❌ NOT INSTALLED (run 'make dev-setup')"
	@echo -n "podman: "; podman --version 2>/dev/null || echo "⚠️  NOT INSTALLED (required for Redis testing)"
	@echo -n "podman-compose: "; podman-compose --version 2>/dev/null || echo "⚠️  NOT INSTALLED (optional, fallback to podman)"
	@echo -n "docker: "; docker --version 2>/dev/null || echo "⚠️  NOT INSTALLED (optional alternative to podman)"
	@echo -n "nc (netcat): "; nc -h 2>/dev/null >/dev/null && echo "✅ INSTALLED" || echo "⚠️  NOT INSTALLED (needed for Redis port checking)"