package stores

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	ratelimit "github.com/itsatony/gorly"
)

// Minimal assertion helpers (the package's tests use only the stdlib).
func aclNoErr(t *testing.T, err error, msg ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error %v %v", err, msg)
	}
}

func aclErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func aclTrue(t *testing.T, ok bool, format string, args ...interface{}) {
	t.Helper()
	if !ok {
		t.Fatalf(format, args...)
	}
}

// ============================================================================
// ACL USERNAME + TLS + CALLER-OWNED CLIENT (gorly#1)
//
// The live cases run against the instances scripts/setup-redis-secure.sh
// starts (TLS-only + ACL-only, CN-only SAN-less self-signed cert — the shape
// of a managed Redis such as Scaleway's) and skip when its env is absent.
// ============================================================================

const (
	envRedisACLAddr     = "GORLY_TEST_REDIS_ACL_ADDR"
	envRedisTLSAddr     = "GORLY_TEST_REDIS_TLS_ADDR"
	envRedisACLUser     = "GORLY_TEST_REDIS_ACL_USER"
	envRedisACLPassword = "GORLY_TEST_REDIS_ACL_PASSWORD"
	envRedisTLSCert     = "GORLY_TEST_REDIS_TLS_CERT"
)

type secureRedisEnv struct {
	aclAddr, tlsAddr, user, password, certPath string
}

func requireSecureRedisEnv(t *testing.T) secureRedisEnv {
	t.Helper()
	e := secureRedisEnv{
		aclAddr:  os.Getenv(envRedisACLAddr),
		tlsAddr:  os.Getenv(envRedisTLSAddr),
		user:     os.Getenv(envRedisACLUser),
		password: os.Getenv(envRedisACLPassword),
		certPath: os.Getenv(envRedisTLSCert),
	}
	if e.aclAddr == "" || e.tlsAddr == "" || e.user == "" || e.password == "" || e.certPath == "" {
		skipOrFailInCI(t, "ACL/TLS Redis not configured; run scripts/setup-redis-secure.sh and export its env")
	}
	return e
}

func secureTestConfig(addr string) *RedisStoreConfig {
	cfg := DefaultRedisStoreConfig()
	cfg.Address = addr
	cfg.DialTimeout = 2 * time.Second
	cfg.KeyPrefix = "gorly-acltls-test:"
	cfg.RetryMaxAttempts = 0
	return cfg
}

// skipOrFailInCI skips a live test locally but FAILS it in CI (CI=true), so a
// CI whose Redis fixtures went missing cannot pass by skipping the very tests
// that cover ACL and TLS.
func skipOrFailInCI(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("CI") == "true" {
		t.Fatalf("CI=true but %s", reason)
	}
	t.Skip(reason)
}

// requirePlainRedis returns a client on the plaintext test Redis (DB 15), or
// skips (fails in CI) when it is not reachable.
func requirePlainRedis(t *testing.T) *redis.Client {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15, DialTimeout: time.Second})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		skipOrFailInCI(t, "Redis not available on localhost:6379")
	}
	return client
}

// pinnedTLSConfig pins the test server's leaf with the shipped helper — the
// same call a consumer makes for a CN-only, SAN-less managed-Redis cert.
func pinnedTLSConfig(t *testing.T, certPath string) *tls.Config {
	t.Helper()
	pemBytes, err := os.ReadFile(certPath)
	aclNoErr(t, err)
	tc, err := PinnedLeafTLSConfig(pemBytes)
	aclNoErr(t, err)
	return tc
}

func exerciseStore(t *testing.T, store *RedisStore) {
	t.Helper()
	ctx := context.Background()
	key := "k-" + t.Name()
	n, err := store.Increment(ctx, key, time.Minute)
	aclNoErr(t, err)
	aclTrue(t, n == 1, "increment = %d, want 1", n)
	aclNoErr(t, store.Delete(ctx, key))
}

func TestRedisStore_ACLUsername(t *testing.T) {
	e := requireSecureRedisEnv(t)

	t.Run("password-only AUTH is refused by an ACL-only server", func(t *testing.T) {
		cfg := secureTestConfig(e.aclAddr)
		cfg.Password = e.password
		_, err := NewRedisStore(cfg)
		aclErr(t, err)
		aclTrue(t, strings.Contains(err.Error(), "WRONGPASS"), "want WRONGPASS, got %v", err)
	})

	t.Run("username + password authenticates", func(t *testing.T) {
		cfg := secureTestConfig(e.aclAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		store, err := NewRedisStore(cfg)
		aclNoErr(t, err)
		defer func() { _ = store.Close() }()
		exerciseStore(t, store)
	})
}

func TestRedisStore_TLS(t *testing.T) {
	e := requireSecureRedisEnv(t)

	t.Run("plaintext to a TLS-only port fails", func(t *testing.T) {
		cfg := secureTestConfig(e.tlsAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		_, err := NewRedisStore(cfg)
		aclErr(t, err)
	})

	t.Run("default hostname verification rejects a SAN-less cert", func(t *testing.T) {
		pem, err := os.ReadFile(e.certPath)
		aclNoErr(t, err)
		pool := x509.NewCertPool()
		aclTrue(t, pool.AppendCertsFromPEM(pem), "pinned cert is not PEM")
		cfg := secureTestConfig(e.tlsAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		cfg.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
		_, err = NewRedisStore(cfg)
		aclErr(t, err)
		aclTrue(t, strings.Contains(err.Error(), "x509"), "want an x509 verification failure, got %v", err)
	})

	t.Run("pinned TLS + ACL user connects", func(t *testing.T) {
		cfg := secureTestConfig(e.tlsAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		cfg.TLSConfig = pinnedTLSConfig(t, e.certPath)
		store, err := NewRedisStore(cfg)
		aclNoErr(t, err)
		defer func() { _ = store.Close() }()
		exerciseStore(t, store)
	})

	t.Run("a pin to the wrong certificate is refused", func(t *testing.T) {
		cfg := secureTestConfig(e.tlsAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		// A different certificate with the SAME shape (CN-only, SAN-less).
		otherPEM, _ := selfSignedPEM(t, "fd00::6379", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
		tc, err := PinnedLeafTLSConfig(otherPEM)
		aclNoErr(t, err)
		cfg.TLSConfig = tc
		_, err = NewRedisStore(cfg)
		aclErr(t, err)
		aclTrue(t, strings.Contains(err.Error(), "not the pinned leaf"), "want a pin mismatch, got %v", err)
	})

	t.Run("the caller's tls.Config is cloned, not retained", func(t *testing.T) {
		cfg := secureTestConfig(e.tlsAddr)
		cfg.Username = e.user
		cfg.Password = e.password
		tc := pinnedTLSConfig(t, e.certPath)
		cfg.TLSConfig = tc
		store, err := NewRedisStore(cfg)
		aclNoErr(t, err)
		defer func() { _ = store.Close() }()

		c, ok := store.client.(*redis.Client)
		aclTrue(t, ok, "client is not *redis.Client")
		aclTrue(t, tc != c.Options().TLSConfig, "tls.Config was retained, not cloned")
		// Mutating the caller's struct after construction must not reach the store.
		tc.ServerName = "mutated.invalid"
		aclTrue(t, c.Options().TLSConfig.ServerName == "", "caller mutation reached the store")
		exerciseStore(t, store)
	})
}

func TestNewRedisStoreFromClient_Secure(t *testing.T) {
	e := requireSecureRedisEnv(t)

	client := redis.NewClient(&redis.Options{
		Addr:        e.tlsAddr,
		Username:    e.user,
		Password:    e.password,
		TLSConfig:   pinnedTLSConfig(t, e.certPath),
		DialTimeout: 2 * time.Second,
	})
	defer func() { _ = client.Close() }()

	cfg := secureTestConfig("ignored:1") // connection fields are ignored
	store, err := NewRedisStoreFromClient(client, cfg)
	aclNoErr(t, err)
	exerciseStore(t, store)

	stats := store.Stats()
	aclTrue(t, stats.Address == e.tlsAddr, "stats address %q, want the client's %q", stats.Address, e.tlsAddr)

	aclNoErr(t, store.Close())
	_, err = store.Get(context.Background(), "x")
	aclTrue(t, errors.Is(err, ratelimit.ErrClosed), "want ErrClosed, got %v", err)

	// The caller-owned client stays open after the store is closed.
	aclNoErr(t, client.Ping(context.Background()).Err())
}

// ============================================================================
// Infra-free cases
// ============================================================================

func TestNewRedisStoreFromClient_NilClient(t *testing.T) {
	_, err := NewRedisStoreFromClient(nil, nil)
	aclErr(t, err)
	aclTrue(t, ratelimit.IsConfigError(err), "want config error, got %v", err)
}

func TestNewRedisStoreFromClient_NegativeRetries(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer func() { _ = client.Close() }()
	cfg := DefaultRedisStoreConfig()
	cfg.RetryMaxAttempts = -1
	_, err := NewRedisStoreFromClient(client, cfg)
	aclErr(t, err)
	aclTrue(t, ratelimit.IsConfigError(err), "want config error, got %v", err)
}

func TestNewRedisStoreFromClient_UnreachableLeavesClientOpen(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = client.Close() }()
	cfg := DefaultRedisStoreConfig()
	cfg.DialTimeout = time.Second
	_, err := NewRedisStoreFromClient(client, cfg)
	aclErr(t, err)
	aclTrue(t, strings.Contains(err.Error(), "redis connection failed"), "want a connection failure, got %v", err)
	// Not closed by the failed constructor: a closed client reports ErrClosed.
	pingErr := client.Ping(context.Background()).Err()
	aclErr(t, pingErr)
	aclTrue(t, !errors.Is(pingErr, redis.ErrClosed), "caller's client must not be closed: %v", pingErr)
}

func TestNewRedisStoreFromClient_DoesNotMutateCallerConfig(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = client.Close() }()
	cfg := &RedisStoreConfig{} // Logger nil, DialTimeout 0: both defaulted internally
	_, _ = NewRedisStoreFromClient(client, cfg)
	aclTrue(t, cfg.Logger == nil, "caller config Logger was written")
	aclTrue(t, cfg.DialTimeout == 0, "caller config DialTimeout was written")
}

func TestNewRedisStoreFromClient_PlainRedis(t *testing.T) {
	client := requirePlainRedis(t)
	store, err := NewRedisStoreFromClient(client, nil)
	aclNoErr(t, err)
	exerciseStore(t, store)
	aclTrue(t, store.Stats().Database == 15, "database %d, want 15", store.Stats().Database)
	aclTrue(t, strings.Contains(store.String(), "localhost:6379"), "String() = %s", store.String())
	aclNoErr(t, store.Close())
	aclNoErr(t, store.Close())
	aclNoErr(t, client.Ping(context.Background()).Err())
}

func TestRedisStoreConfig_RefusesUnverifiedTLS(t *testing.T) {
	cfg := DefaultRedisStoreConfig()
	cfg.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	err := cfg.Validate()
	aclErr(t, err)
	aclTrue(t, ratelimit.IsConfigError(err), "want config error, got %v", err)

	cfg.TLSConfig.VerifyConnection = func(tls.ConnectionState) error { return nil }
	aclNoErr(t, cfg.Validate())

	cfg.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	aclNoErr(t, cfg.Validate())
}

func TestNewRedisStoreFromClient_DefaultsZeroRetryBackoff(t *testing.T) {
	client := requirePlainRedis(t)
	store, err := NewRedisStoreFromClient(client, &RedisStoreConfig{RetryMaxAttempts: 2})
	aclNoErr(t, err)
	def := DefaultRedisStoreConfig()
	aclTrue(t, store.config.RetryInitialBackoff == def.RetryInitialBackoff &&
		store.config.RetryMaxBackoff == def.RetryMaxBackoff &&
		store.config.RetryBackoffMultiplier == def.RetryBackoffMultiplier,
		"zero retry backoff fields were not defaulted: %+v", store.config)
	_ = store.Close()
}
