package stores

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	ratelimit "github.com/itsatony/gorly"
)

// ============================================================================
// v1.3.1 review fixes (gorly#1 follow-up)
// ============================================================================

// selfSignedPEM makes a CN-only, SAN-less, CA:FALSE self-signed certificate —
// the shape Scaleway's managed Redis presents — and returns its PEM and key.
func selfSignedPEM(t *testing.T, cn string, notBefore, notAfter time.Time) ([]byte, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	aclNoErr(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	aclNoErr(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	aclNoErr(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: pemBlockCertificate, Bytes: der})
	return pemBytes, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// handshake runs a real TLS handshake against a loopback server presenting
// serverCert, with the client using clientCfg.
func handshake(t *testing.T, serverCert tls.Certificate, clientCfg *tls.Config) error {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		MinVersion:   tls.VersionTLS12,
	})
	aclNoErr(t, err)
	defer func() { _ = ln.Close() }()
	go func() {
		c, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_ = c.(*tls.Conn).Handshake()
		_ = c.Close()
	}()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", ln.Addr().String(), clientCfg)
	if err != nil {
		return err
	}
	return conn.Close()
}

func TestPinnedLeafTLSConfig_Handshake(t *testing.T) {
	now := time.Now()
	pinnedPEM, pinnedCert := selfSignedPEM(t, "fd00::6379", now.Add(-time.Hour), now.Add(time.Hour))
	_, sameShapeOther := selfSignedPEM(t, "fd00::6379", now.Add(-time.Hour), now.Add(time.Hour))

	tc, err := PinnedLeafTLSConfig(pinnedPEM)
	aclNoErr(t, err)
	aclTrue(t, tc.MinVersion == tls.VersionTLS12, "MinVersion = %x, want TLS 1.2", tc.MinVersion)
	aclNoErr(t, validateTLSConfig(tc), "the helper's own config must pass validation")

	t.Run("the pinned leaf connects (no SAN, CN only)", func(t *testing.T) {
		aclNoErr(t, handshake(t, pinnedCert, tc.Clone()))
	})
	t.Run("a different same-shape certificate is refused", func(t *testing.T) {
		err := handshake(t, sameShapeOther, tc.Clone())
		aclErr(t, err)
		aclTrue(t, strings.Contains(err.Error(), "not the pinned leaf"), "got %v", err)
	})
	t.Run("rotation: either of two pinned leaves connects", func(t *testing.T) {
		otherPEM, otherCert := selfSignedPEM(t, "fd00::6379", now.Add(-time.Hour), now.Add(time.Hour))
		both, err := PinnedLeafTLSConfig(append(append([]byte{}, pinnedPEM...), otherPEM...))
		aclNoErr(t, err)
		aclNoErr(t, handshake(t, pinnedCert, both.Clone()))
		aclNoErr(t, handshake(t, otherCert, both.Clone()))
	})
}

// A CA-pool "pin" with the hostname check off accepts ANY cert the CA signed;
// the leaf pin does not. This is the reason the helper exists.
func TestPinnedLeafTLSConfig_SharedCAIsNotAPin(t *testing.T) {
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	aclNoErr(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "provider-ca"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	aclNoErr(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	aclNoErr(t, err)
	issue := func(serial int64) ([]byte, tls.Certificate) {
		k, kErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		aclNoErr(t, kErr)
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "tenant"},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		der, cErr := x509.CreateCertificate(rand.Reader, tmpl, caCert, &k.PublicKey, caKey)
		aclNoErr(t, cErr)
		return pem.EncodeToMemory(&pem.Block{Type: pemBlockCertificate, Bytes: der}),
			tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}
	}
	minePEM, mine := issue(2)
	_, otherTenant := issue(3)

	// The hand-rolled pattern: skip hostname, verify chain against the CA.
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	chainOnly := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			_, vErr := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: pool})
			return vErr
		},
	}
	aclNoErr(t, handshake(t, otherTenant, chainOnly), "demonstration: the chain-only check admits another tenant")

	leafPin, err := PinnedLeafTLSConfig(minePEM)
	aclNoErr(t, err)
	aclNoErr(t, handshake(t, mine, leafPin.Clone()))
	aclErr(t, handshake(t, otherTenant, leafPin.Clone()))
}

func TestVerifyPinnedLeaf_ValidityAndEmpty(t *testing.T) {
	now := time.Now()
	pemBytes, cert := selfSignedPEM(t, "x", now.Add(-2*time.Hour), now.Add(-time.Hour))
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	aclNoErr(t, err)
	block, _ := pem.Decode(pemBytes)
	pins := [][sha256.Size]byte{sha256.Sum256(block.Bytes)}

	err = verifyPinnedLeaf(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, pins, now)
	aclTrue(t, errors.Is(err, errPinnedLeafExpired), "expired pinned leaf: got %v", err)
	aclNoErr(t, verifyPinnedLeaf(tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}}, pins, now.Add(-90*time.Minute)))
	err = verifyPinnedLeaf(tls.ConnectionState{}, pins, now)
	aclTrue(t, errors.Is(err, errPinnedNoPeerCertificate), "no peer cert: got %v", err)
}

func TestPinnedLeafTLSConfig_RefusesBadPEM(t *testing.T) {
	for name, in := range map[string][]byte{
		"empty":        nil,
		"not PEM":      []byte("hello"),
		"no CERT":      pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1}}),
		"garbage CERT": pem.EncodeToMemory(&pem.Block{Type: pemBlockCertificate, Bytes: []byte{1, 2, 3}}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PinnedLeafTLSConfig(in)
			aclErr(t, err)
			aclTrue(t, ratelimit.IsConfigError(err), "want config error, got %v", err)
		})
	}
}

func TestValidateTLSConfig_RefusesOldMinVersion(t *testing.T) {
	aclErr(t, validateTLSConfig(&tls.Config{MinVersion: tls.VersionTLS11}))
	aclNoErr(t, validateTLSConfig(&tls.Config{})) // 0 = Go's client default (TLS 1.2)
	aclNoErr(t, validateTLSConfig(&tls.Config{MinVersion: tls.VersionTLS13}))
}

func TestFlushDB_RefusedOnCallerOwnedClient(t *testing.T) {
	// No Redis needed: the refusal precedes any command.
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer func() { _ = client.Close() }()
	rs := &RedisStore{client: client, config: DefaultRedisStoreConfig(), ownsClient: false, logger: ratelimit.NewNopLogger()}
	err := rs.FlushDB(context.Background())
	aclErr(t, err)
	aclTrue(t, errors.Is(err, ratelimit.ErrOperationNotSupported), "want ErrOperationNotSupported, got %v", err)
	aclTrue(t, !ratelimit.IsConfigError(err), "not a config error: %v", err)
}

func TestNewRedisStoreFromClient_RefusesShortDialTimeout(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer func() { _ = client.Close() }()
	_, err := NewRedisStoreFromClient(client, &RedisStoreConfig{DialTimeout: 100 * time.Millisecond})
	aclErr(t, err)
	aclTrue(t, ratelimit.IsConfigError(err), "want config error, got %v", err)
}

func TestNewRedisStoreFromClient_PartialConfigGetsDefaultPrefix(t *testing.T) {
	client := requirePlainRedis(t)
	store, err := NewRedisStoreFromClient(client, &RedisStoreConfig{})
	aclNoErr(t, err)
	defer func() { _ = store.Close() }()
	aclTrue(t, store.config.KeyPrefix == DefaultRedisStoreConfig().KeyPrefix,
		"KeyPrefix = %q, want the default", store.config.KeyPrefix)

	key := "prefix-" + t.Name()
	_, err = store.Increment(context.Background(), key, time.Minute)
	aclNoErr(t, err)
	n, err := client.Exists(context.Background(), DefaultRedisStoreConfig().KeyPrefix+key).Result()
	aclNoErr(t, err)
	aclTrue(t, n == 1, "the key was not written under the default prefix")
	aclNoErr(t, store.Delete(context.Background(), key))
}

func TestNewRedisStore_CloseClosesOwnedClient(t *testing.T) {
	requirePlainRedis(t)
	cfg := DefaultRedisStoreConfig()
	cfg.Database = 15
	store, err := NewRedisStore(cfg)
	aclNoErr(t, err)
	aclNoErr(t, store.Close())
	err = store.client.Ping(context.Background()).Err()
	aclTrue(t, errors.Is(err, redis.ErrClosed), "owned client not closed by Close: %v", err)
}

func TestNewRedisStore_FailedPingClosesClient(t *testing.T) {
	var built *redis.Client
	orig := newGoRedisClient
	newGoRedisClient = func(o *redis.Options) *redis.Client {
		built = orig(o)
		return built
	}
	t.Cleanup(func() { newGoRedisClient = orig })

	cfg := DefaultRedisStoreConfig()
	cfg.Address = "127.0.0.1:1"
	cfg.DialTimeout = time.Second
	cfg.MaxRetries = 0
	_, err := NewRedisStore(cfg)
	aclErr(t, err)
	aclTrue(t, built != nil, "client was not built")
	err = built.Ping(context.Background()).Err()
	aclTrue(t, errors.Is(err, redis.ErrClosed), "client leaked after a failed ping: %v", err)
}
