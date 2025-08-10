#!/bin/bash
# scripts/setup-redis.sh - Setup Redis for testing with Podman

set -e

echo "🚀 Setting up Redis for Gorly rate limiting tests with Podman..."

# Check if podman is installed
if ! command -v podman &> /dev/null; then
    echo "❌ Podman is not installed. Please install podman first."
    echo "   On Ubuntu/Debian: sudo apt-get install podman"
    echo "   On RHEL/CentOS: sudo dnf install podman"
    echo "   On macOS: brew install podman"
    exit 1
fi

# Check if podman-compose is available
if ! command -v podman-compose &> /dev/null; then
    echo "⚠️  podman-compose not found. Using podman directly..."
    USE_COMPOSE=false
else
    echo "✅ Found podman-compose"
    USE_COMPOSE=true
fi

# Function to start Redis with podman-compose
start_with_compose() {
    echo "🐳 Starting Redis with podman-compose..."
    podman-compose up -d redis
    
    echo "⏳ Waiting for Redis to be ready..."
    for i in {1..30}; do
        if podman exec gorly-redis redis-cli ping >/dev/null 2>&1; then
            echo "✅ Redis is ready!"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "❌ Redis failed to start within 30 seconds"
            exit 1
        fi
        sleep 1
    done
    
    echo "🎯 Starting Redis Commander (optional web UI)..."
    podman-compose up -d redis-commander
    
    echo ""
    echo "✅ Redis setup complete!"
    echo "   Redis:           localhost:6379"
    echo "   Redis Commander: http://localhost:8081 (web UI)"
}

# Function to start Redis with plain podman
start_with_podman() {
    echo "🐳 Starting Redis with podman..."
    
    # Create network if it doesn't exist
    if ! podman network exists gorly-network; then
        echo "🌐 Creating gorly-network..."
        podman network create gorly-network
    fi
    
    # Create volume if it doesn't exist
    if ! podman volume exists gorly-redis-data; then
        echo "💾 Creating Redis data volume..."
        podman volume create gorly-redis-data
    fi
    
    # Stop and remove existing containers
    echo "🧹 Cleaning up existing containers..."
    podman stop gorly-redis gorly-redis-commander 2>/dev/null || true
    podman rm gorly-redis gorly-redis-commander 2>/dev/null || true
    
    # Start Redis
    echo "🚀 Starting Redis container..."
    podman run -d \
        --name gorly-redis \
        --network gorly-network \
        -p 6379:6379 \
        -v gorly-redis-data:/data \
        --restart unless-stopped \
        docker.io/redis:7-alpine \
        redis-server --appendonly yes --maxmemory 256mb --maxmemory-policy allkeys-lru
    
    echo "⏳ Waiting for Redis to be ready..."
    for i in {1..30}; do
        if podman exec gorly-redis redis-cli ping >/dev/null 2>&1; then
            echo "✅ Redis is ready!"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "❌ Redis failed to start within 30 seconds"
            exit 1
        fi
        sleep 1
    done
    
    # Start Redis Commander (optional)
    echo "🎯 Starting Redis Commander (web UI)..."
    podman run -d \
        --name gorly-redis-commander \
        --network gorly-network \
        -p 8081:8081 \
        -e REDIS_HOSTS=local:gorly-redis:6379 \
        --restart unless-stopped \
        docker.io/rediscommander/redis-commander:latest 2>/dev/null || true
    
    echo ""
    echo "✅ Redis setup complete!"
    echo "   Redis:           localhost:6379"
    echo "   Redis Commander: http://localhost:8081 (web UI)"
    if ! podman ps | grep -q gorly-redis-commander; then
        echo "   (Redis Commander failed to start - Redis is still available)"
    fi
}

# Main execution
if [ "$USE_COMPOSE" = true ]; then
    start_with_compose
else
    start_with_podman
fi

echo ""
echo "🧪 Testing Redis connection..."
if podman exec gorly-redis redis-cli ping | grep -q PONG; then
    echo "✅ Redis connection test successful!"
else
    echo "❌ Redis connection test failed!"
    exit 1
fi

echo ""
echo "📋 Quick Redis commands:"
echo "   Connect to Redis: podman exec -it gorly-redis redis-cli"
echo "   View logs:        podman logs gorly-redis"
echo "   Stop Redis:       podman stop gorly-redis gorly-redis-commander"
echo "   Start Redis:      podman start gorly-redis gorly-redis-commander"
echo "   Remove all:       ./scripts/cleanup-redis.sh"
echo ""
echo "🎉 Ready to run Redis-based tests!"
echo "   Run: make test-redis"
echo "   Run: go test -tags=redis ./..."