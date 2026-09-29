package stores

import (
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	ratelimit "github.com/itsatony/gorly"
)

// v1.3.2 review fixes.

// resumingServer accepts connections and completes a TLS 1.2 handshake with
// session tickets, so a client with a ClientSessionCache resumes on reconnect.
func resumingServer(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
	})
	aclNoErr(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.(*tls.Conn).Handshake()
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}

func dialResume(t *testing.T, addr string, cfg *tls.Config) (didResume bool, err error) {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, cfg)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close() }()
	return conn.ConnectionState().DidResume, nil
}

// The reason InsecureSkipVerify now REQUIRES VerifyConnection: on a resumed
// session VerifyPeerCertificate is not called, VerifyConnection is.
func TestTLSResumption_OnlyVerifyConnectionRunsOnResume(t *testing.T) {
	now := time.Now()
	_, cert := selfSignedPEM(t, "redis.example", now.Add(-time.Hour), now.Add(time.Hour))
	addr := resumingServer(t, cert)

	var peerCalls, connCalls atomic.Int32
	cfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
		ClientSessionCache: tls.NewLRUClientSessionCache(4),
		VerifyPeerCertificate: func([][]byte, [][]*x509.Certificate) error {
			peerCalls.Add(1)
			return nil
		},
		VerifyConnection: func(tls.ConnectionState) error {
			connCalls.Add(1)
			return nil
		},
	}
	resumed1, err := dialResume(t, addr, cfg)
	aclNoErr(t, err)
	resumed2, err := dialResume(t, addr, cfg)
	aclNoErr(t, err)
	aclTrue(t, !resumed1 && resumed2, "second connection must resume (got %v,%v)", resumed1, resumed2)
	aclTrue(t, peerCalls.Load() == 1, "VerifyPeerCertificate skipped on resume: %d", peerCalls.Load())
	aclTrue(t, connCalls.Load() == 2, "VerifyConnection runs on every handshake: %d", connCalls.Load())
}

func TestPinnedLeaf_VerifiedOnResumedSessions(t *testing.T) {
	now := time.Now()
	pinPEM, _ := selfSignedPEM(t, "pinned", now.Add(-time.Hour), now.Add(time.Hour))
	_, other := selfSignedPEM(t, "other", now.Add(-time.Hour), now.Add(time.Hour))
	cfg, err := PinnedLeafTLSConfig(pinPEM)
	aclNoErr(t, err)
	cfg.ClientSessionCache = tls.NewLRUClientSessionCache(4)
	addr := resumingServer(t, other)
	for i := 0; i < 2; i++ {
		_, err := dialResume(t, addr, cfg)
		aclErr(t, err) // refused on every attempt, resumed or not
	}
}

func TestValidateTLSConfig_RefusesVerifyPeerCertificateOnly(t *testing.T) {
	aclErr(t, validateTLSConfig(&tls.Config{
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: func([][]byte, [][]*x509.Certificate) error { return nil },
	}))
	aclNoErr(t, validateTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
		VerifyConnection:   func(tls.ConnectionState) error { return nil },
	}))
}

func TestParsePinnedLeaves_Strict(t *testing.T) {
	now := time.Now()
	a, _ := selfSignedPEM(t, "a", now.Add(-time.Hour), now.Add(time.Hour))
	b, _ := selfSignedPEM(t, "b", now.Add(-time.Hour), now.Add(2*time.Hour))
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}})

	certs, err := ParsePinnedLeaves(append(append([]byte("\n"), a...), append([]byte("\n\n"), b...)...))
	aclNoErr(t, err)
	aclTrue(t, len(certs) == 2 && certs[1].NotAfter.After(certs[0].NotAfter), "two certs with NotAfter exposed")

	for name, in := range map[string][]byte{
		"key after cert":   append(append([]byte{}, a...), key...),
		"trailing garbage": append(append([]byte{}, a...), []byte("garbage")...),
		"leading garbage":  append([]byte("garbage\n"), a...),
		"truncated":        a[:len(a)/2],
		"empty":            nil,
	} {
		_, err := ParsePinnedLeaves(in)
		aclTrue(t, err != nil && ratelimit.IsConfigError(err), "%s: want config error, got %v", name, err)
		_, err = PinnedLeafTLSConfig(in)
		aclTrue(t, err != nil, "%s: PinnedLeafTLSConfig must refuse too", name)
	}
}

func TestPinnedLeaf_RefuseCAOption(t *testing.T) {
	now := time.Now()
	leaf, _ := selfSignedPEM(t, "leaf", now.Add(-time.Hour), now.Add(time.Hour))
	ca := caPEM(t)
	_, err := PinnedLeafTLSConfigWithOptions(ca, PinnedLeafOptions{RefuseCA: true})
	aclErr(t, err)
	_, err = PinnedLeafTLSConfig(ca)
	aclNoErr(t, err) // opt-in only: a CA:TRUE self-signed server cert may be the leaf
	_, err = PinnedLeafTLSConfigWithOptions(leaf, PinnedLeafOptions{RefuseCA: true})
	aclNoErr(t, err)
}

// caPEM makes a CA:TRUE certificate by flipping the helper's template.
func caPEM(t *testing.T) []byte {
	t.Helper()
	now := time.Now()
	p, _ := selfSignedPEM(t, "ca", now.Add(-time.Hour), now.Add(time.Hour))
	block, _ := pem.Decode(p)
	cert, err := x509.ParseCertificate(block.Bytes)
	aclNoErr(t, err)
	_, key := selfSignedPEM(t, "ca-key", now.Add(-time.Hour), now.Add(time.Hour))
	signer := key.PrivateKey.(crypto.Signer)
	tmpl := &x509.Certificate{
		SerialNumber:          cert.SerialNumber,
		Subject:               cert.Subject,
		NotBefore:             cert.NotBefore,
		NotAfter:              cert.NotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, signer.Public(), signer)
	aclNoErr(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: pemBlockCertificate, Bytes: der})
}

type recordingLogger struct {
	ratelimit.Logger
	mu    sync.Mutex
	warns []string
}

func (l *recordingLogger) Warn(msg string, _ ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
}

func TestNewRedisStoreFromClient_WarnsPlaintextPassword(t *testing.T) {
	for name, tc := range map[string]struct {
		opts *redis.Options
		want bool
	}{
		"password no tls": {&redis.Options{Addr: "127.0.0.1:1", Password: "pw"}, true},
		"password + tls":  {&redis.Options{Addr: "127.0.0.1:1", Password: "pw", TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}, false},
		"no password":     {&redis.Options{Addr: "127.0.0.1:1"}, false},
	} {
		client := redis.NewClient(tc.opts)
		logger := &recordingLogger{Logger: ratelimit.NewNopLogger()}
		_, _ = NewRedisStoreFromClient(client, &RedisStoreConfig{Logger: logger, DialTimeout: time.Second})
		_ = client.Close()
		got := false
		for _, w := range logger.warns {
			if strings.Contains(w, "PLAINTEXT") {
				got = true
			}
		}
		aclTrue(t, got == tc.want, "%s: plaintext warn=%v want %v", name, got, tc.want)
	}
}
