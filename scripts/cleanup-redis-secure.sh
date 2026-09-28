#!/usr/bin/env bash
# scripts/cleanup-redis-secure.sh — remove the containers setup-redis-secure.sh started.
set -euo pipefail
for c in gorly-redis-plain gorly-redis-acl gorly-redis-tls; do
  podman rm -f "$c" >/dev/null 2>&1 || true
done
