#!/bin/bash

# build-all-platforms.sh - Cross-platform release build script
# Usage: ./scripts/build-all-platforms.sh [version]

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m'

print_info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
print_success() { echo -e "${GREEN}✅ $1${NC}"; }

VERSION=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}

# Build information
GIT_COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER=${USER:-$(whoami)}

print_info "Building Gorly for all platforms..."
print_info "Version: $VERSION"

# Platforms to build for
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
    "freebsd/amd64"
    "openbsd/amd64"
)

# Create dist directory
rm -rf dist/releases
mkdir -p dist/releases

# Common ldflags
LDFLAGS="-X github.com/itsatony/gorly.gitCommit=$GIT_COMMIT"
LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.buildTime=$BUILD_TIME"
LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.buildUser=$BUILD_USER"

# Add version if it looks like a release tag
if [[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
    VERSION_NUM=${VERSION#v}
    LDFLAGS="$LDFLAGS -X github.com/itsatony/gorly.Version=$VERSION_NUM"
fi

# Optimized release build flags
LDFLAGS="$LDFLAGS -s -w"  # Strip debug info and symbol table

print_info "Building for ${#PLATFORMS[@]} platforms..."

for platform in "${PLATFORMS[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    
    binary_name="gorly-ops-$VERSION-$GOOS-$GOARCH"
    
    if [ "$GOOS" = "windows" ]; then
        binary_name="${binary_name}.exe"
    fi
    
    print_info "Building for $GOOS/$GOARCH..."
    
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags "$LDFLAGS" \
        -o "dist/releases/$binary_name" \
        ./cmd/gorly-ops
    
    # Create archive
    cd dist/releases
    if [ "$GOOS" = "windows" ]; then
        zip "${binary_name%.*}.zip" "$binary_name"
        rm "$binary_name"
    else
        tar -czf "${binary_name}.tar.gz" "$binary_name"
        rm "$binary_name"
    fi
    cd ../..
    
    print_success "Built: $binary_name"
done

# Create checksums
print_info "Creating checksums..."
cd dist/releases
shasum -a 256 * > "gorly-$VERSION-checksums.txt"
cd ../..

# Show results
print_success "Cross-platform build complete!"
echo -e "${PURPLE}📦 Release artifacts:${NC}"
ls -la dist/releases/

print_info "Total size: $(du -sh dist/releases | cut -f1)"

# Example usage in release notes
cat << EOF

${PURPLE}📋 For Release Notes:${NC}

## Downloads

| Platform | Architecture | Download |
|----------|-------------|----------|
EOF

cd dist/releases
for platform in "${PLATFORMS[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    
    if [ "$GOOS" = "windows" ]; then
        filename="gorly-ops-$VERSION-$GOOS-$GOARCH.zip"
    else
        filename="gorly-ops-$VERSION-$GOOS-$GOARCH.tar.gz"
    fi
    
    if [ -f "$filename" ]; then
        size=$(du -h "$filename" | cut -f1)
        echo "| $GOOS | $GOARCH | [$filename](./releases/download/$VERSION/$filename) ($size) |"
    fi
done

echo ""
echo "**Checksums**: [gorly-$VERSION-checksums.txt](./releases/download/$VERSION/gorly-$VERSION-checksums.txt)"

cd ../..

cat << EOF

${PURPLE}🔐 Verification:${NC}
  
  # Download and verify (example)
  curl -L -O https://github.com/itsatony/gorly/releases/download/$VERSION/gorly-ops-$VERSION-linux-amd64.tar.gz
  curl -L -O https://github.com/itsatony/gorly/releases/download/$VERSION/gorly-$VERSION-checksums.txt
  shasum -c gorly-$VERSION-checksums.txt --ignore-missing

EOF