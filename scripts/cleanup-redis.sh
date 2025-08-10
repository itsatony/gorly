#!/bin/bash
# scripts/cleanup-redis.sh - Clean up Redis testing environment

set -e

echo "🧹 Cleaning up Redis testing environment..."

# Check if podman-compose is available
if command -v podman-compose &> /dev/null; then
    echo "🐳 Using podman-compose for cleanup..."
    podman-compose down -v 2>/dev/null || true
else
    echo "🐳 Using podman for cleanup..."
fi

# Stop and remove containers
echo "🛑 Stopping Redis containers..."
podman stop gorly-redis gorly-redis-commander 2>/dev/null || true

echo "🗑️  Removing Redis containers..."
podman rm gorly-redis gorly-redis-commander 2>/dev/null || true

# Ask user if they want to remove volumes and network
echo ""
read -p "❓ Do you want to remove Redis data volume? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "💾 Removing Redis data volume..."
    podman volume rm gorly-redis-data 2>/dev/null || true
    podman volume rm gorly_redis_data 2>/dev/null || true
else
    echo "💾 Keeping Redis data volume for next startup..."
fi

echo ""
read -p "❓ Do you want to remove the gorly network? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🌐 Removing gorly network..."
    podman network rm gorly-network 2>/dev/null || true
    podman network rm gorly_gorly-network 2>/dev/null || true
else
    echo "🌐 Keeping gorly network for next startup..."
fi

# Clean up any dangling images (optional)
echo ""
read -p "❓ Do you want to remove unused Redis images? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🖼️  Removing unused images..."
    podman image prune -f 2>/dev/null || true
fi

echo ""
echo "✅ Redis cleanup complete!"
echo ""
echo "📋 To restart Redis:"
echo "   ./scripts/setup-redis.sh"
echo ""
echo "📋 To check what's still running:"
echo "   podman ps -a"
echo "   podman volume ls"
echo "   podman network ls"