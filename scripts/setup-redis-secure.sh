#!/usr/bin/env bash
# scripts/setup-redis-secure.sh — start the Redis instances the ACL/TLS store
# tests run against (stores/redis_acl_tls_test.go), with Podman.
#
# It mirrors a managed Redis that enforces ACLs and TLS (e.g. Scaleway managed
# Redis): TLS only (no plaintext port), the default user disabled so a
# password-only AUTH is refused, and a self-signed, CA:FALSE, CN-only server
# certificate with NO SAN — which clients can only verify by pinning.
#
#   gorly-redis-plain  localhost:6379   plaintext, no auth (the existing tests)
#   gorly-redis-acl    localhost:16380  plaintext, ACL user only
#   gorly-redis-tls    localhost:16381  TLS only,  ACL user only
#
# Prints the env exports the tests read. Stop with scripts/cleanup-redis-secure.sh.
set -euo pipefail

IMAGE="${GORLY_REDIS_IMAGE:-docker.io/library/redis:7-alpine}"
ACL_USER="gorlytest"
ACL_PASS="gorly-acl-secret"
CERT_DIR="${GORLY_REDIS_CERT_DIR:-${TMPDIR:-/tmp}/gorly-redis-tls}"

mkdir -p "$CERT_DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days 2 \
  -keyout "$CERT_DIR/server.key" -out "$CERT_DIR/server.crt" \
  -subj "/CN=fd00::6379" \
  -addext "basicConstraints=critical,CA:FALSE" >/dev/null 2>&1
chmod 644 "$CERT_DIR/server.key" "$CERT_DIR/server.crt"

for c in gorly-redis-plain gorly-redis-acl gorly-redis-tls; do
  podman rm -f "$c" >/dev/null 2>&1 || true
done

ACL_ARGS=(--user default off --user "$ACL_USER" on ">$ACL_PASS" "~*" "&*" "+@all")

podman run -d --name gorly-redis-plain -p 6379:6379 "$IMAGE" >/dev/null
podman run -d --name gorly-redis-acl -p 16380:6379 "$IMAGE" \
  redis-server "${ACL_ARGS[@]}" >/dev/null
podman run -d --name gorly-redis-tls -p 16381:6379 \
  -v "$CERT_DIR:/tls:ro,Z" "$IMAGE" \
  redis-server --port 0 --tls-port 6379 \
  --tls-cert-file /tls/server.crt --tls-key-file /tls/server.key \
  --tls-auth-clients no "${ACL_ARGS[@]}" >/dev/null

for _ in $(seq 1 30); do
  if podman exec gorly-redis-plain redis-cli ping >/dev/null 2>&1 &&
     podman exec gorly-redis-acl redis-cli --user "$ACL_USER" --pass "$ACL_PASS" ping >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

cat <<ENV
export GORLY_TEST_REDIS_ACL_ADDR=localhost:16380
export GORLY_TEST_REDIS_TLS_ADDR=localhost:16381
export GORLY_TEST_REDIS_ACL_USER=$ACL_USER
export GORLY_TEST_REDIS_ACL_PASSWORD=$ACL_PASS
export GORLY_TEST_REDIS_TLS_CERT=$CERT_DIR/server.crt
ENV
