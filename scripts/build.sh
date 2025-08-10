#!/bin/bash

# build.sh - Professional build script with version information
# Usage: ./scripts/build.sh [version] [target]

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
print_success() { echo -e "${GREEN}✅ $1${NC}"; }
print_warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
print_error() { echo -e "${RED}❌ $1${NC}"; }

# Default values
VERSION=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
TARGET=${2:-"gorly-ops"}

# Build information
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER=${USER:-$(whoami)}

print_info "Building Gorly with version information..."
print_info "Version: $VERSION"
print_info "Git Commit: $GIT_COMMIT"
print_info "Build Time: $BUILD_TIME"
print_info "Build User: $BUILD_USER"

# Determine package and binary path based on target
case $TARGET in
    "gorly-ops"|"ops")
        PACKAGE="./cmd/gorly-ops"
        BINARY="gorly-ops"
        ;;
    "gorly"|"cli")
        PACKAGE="./cmd/gorly"
        BINARY="gorly"
        ;;
    "library"|"lib")
        print_info "Building library (validation only - no binary output)"
        go build ./...
        print_success "Library build successful!"
        exit 0
        ;;
    *)
        print_error "Unknown target: $TARGET"
        print_info "Available targets: gorly-ops, gorly, library"
        exit 1
        ;;
esac

# Create dist directory
mkdir -p dist

# Build with version information embedded via ldflags
print_info "Building $BINARY..."

LDFLAGS="-X github.com/itsatony/gorly.gitCommit=$GIT_COMMIT"
LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.buildTime=$BUILD_TIME"
LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.buildUser=$BUILD_USER"

# Add version if it looks like a release tag
if [[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
    # Extract version number without 'v' prefix
    VERSION_NUM=${VERSION#v}
    LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.Version=$VERSION_NUM"
    print_info "Setting version to: $VERSION_NUM"
fi

go build -ldflags "$LDFLAGS" -o "dist/$BINARY" "$PACKAGE"

print_success "Built: dist/$BINARY"

# Test the binary
print_info "Testing binary version output..."
if ./dist/$BINARY version > /dev/null 2>&1; then
    print_success "Version command works!"
    echo -e "${PURPLE}Version output:${NC}"
    ./dist/$BINARY version
else
    print_warning "Version command not available or failed"
fi

# Show binary info
print_info "Binary information:"
ls -lh "dist/$BINARY"
file "dist/$BINARY" 2>/dev/null || true

print_success "Build complete!"

# Example usage information
cat << EOF

${PURPLE}📚 Usage Examples:${NC}
  
  # Build with automatic version detection
  ./scripts/build.sh
  
  # Build with specific version
  ./scripts/build.sh v1.2.3
  
  # Build different targets
  ./scripts/build.sh v1.2.3 gorly-ops     # CLI operations tool
  ./scripts/build.sh v1.2.3 library       # Library only (no binary)
  
  # Test the built binary
  ./dist/$BINARY version
  ./dist/$BINARY help

${PURPLE}🔧 Advanced Build Options:${NC}

  # Cross-compilation example
  GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "dist/${BINARY}-linux-amd64" "$PACKAGE"
  
  # Optimized release build
  go build -ldflags "$LDFLAGS -s -w" -o "dist/$BINARY" "$PACKAGE"
  
  # Build for multiple platforms
  ./scripts/build-all-platforms.sh $VERSION

EOF