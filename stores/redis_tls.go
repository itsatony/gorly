package stores

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	ratelimit "github.com/itsatony/gorly"
)

// pemBlockCertificate is the PEM block type of an X.509 certificate.
const pemBlockCertificate = "CERTIFICATE"

// Errors returned by the pinned-leaf verifier during the TLS handshake.
var (
	errPinnedNoPeerCertificate = errors.New("redis tls: server presented no certificate")
	errPinnedLeafMismatch      = errors.New("redis tls: server certificate is not the pinned leaf")
	errPinnedLeafExpired       = errors.New("redis tls: pinned server certificate is outside its validity period")
)

// PinnedLeafTLSConfig returns a TLS 1.2+ client config that trusts exactly the
// server certificate(s) in leafPEM — byte-for-byte (SHA-256 of the DER) — and
// nothing else. It is the way to connect to a server whose certificate carries
// no usable hostname, e.g. Scaleway's managed Redis (self-signed, CA:FALSE,
// CN-only, no SAN), which the normal hostname verification can never accept.
//
// Why a LEAF pin, not a CA pool: the hostname check has to be off for such a
// certificate, and without it a chain check against a CA accepts EVERY
// certificate that CA signed — for a provider-wide CA, other tenants' servers
// included. Pinning the leaf itself cannot be satisfied by any other
// certificate. The leaf's validity period is still enforced.
//
// leafPEM may hold several certificates (e.g. the current and the next one
// during a rotation); the server's leaf must equal one of them. The result can
// be used as RedisStoreConfig.TLSConfig or as a go-redis Options.TLSConfig.
func PinnedLeafTLSConfig(leafPEM []byte) (*tls.Config, error) {
	var pins [][sha256.Size]byte
	rest := leafPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != pemBlockCertificate {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, ratelimit.WrapConfigError(err, "pinned certificate is not a valid X.509 certificate",
				"pinned_cert_index", len(pins))
		}
		pins = append(pins, sha256.Sum256(block.Bytes))
	}
	if len(pins) == 0 {
		return nil, ratelimit.WrapConfigError(nil, "pinned certificate PEM contains no CERTIFICATE block",
			"pinned_cert_count", 0)
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Default verification (chain to system roots + hostname) is replaced,
		// not dropped: VerifyConnection below runs on every handshake,
		// including resumed ones, and is the only thing that can accept.
		InsecureSkipVerify: true, //nolint:gosec // replaced by the exact-leaf pin in VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			return verifyPinnedLeaf(cs, pins, time.Now())
		},
	}, nil
}

// verifyPinnedLeaf accepts the connection only if the peer's leaf certificate
// is byte-identical to one of pins and is within its validity period at now.
func verifyPinnedLeaf(cs tls.ConnectionState, pins [][sha256.Size]byte, now time.Time) error {
	if len(cs.PeerCertificates) == 0 {
		return errPinnedNoPeerCertificate
	}
	leaf := cs.PeerCertificates[0]
	sum := sha256.Sum256(leaf.Raw)
	matched := false
	for i := range pins {
		if bytes.Equal(sum[:], pins[i][:]) {
			matched = true
			break
		}
	}
	if !matched {
		return errPinnedLeafMismatch
	}
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return errPinnedLeafExpired
	}
	return nil
}
