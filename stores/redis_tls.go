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

// pemBeginMarker is how every PEM block starts.
var pemBeginMarker = []byte("-----BEGIN ")

// PinnedLeafOptions tunes PinnedLeafTLSConfigWithOptions.
type PinnedLeafOptions struct {
	// RefuseCA refuses a pinned certificate whose basic constraints mark it as
	// a CA. A CA certificate is almost never what a server presents as its
	// leaf — pinning one usually means a CA/intermediate was supplied where the
	// server's own certificate belongs, and every handshake would fail. It is
	// opt-in because a self-signed server certificate made with a default
	// `openssl req -x509` carries CA:TRUE and IS the leaf.
	RefuseCA bool
}

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
// certificate. The leaf's validity period is still enforced, so when the
// pinned certificate expires every connection fails at once: log or export
// NotAfter (ParsePinnedLeaves) to see the rotation coming.
//
// leafPEM must contain only CERTIFICATE blocks (v1.3.2: any other block, e.g. a
// private key, or non-PEM bytes are refused); several are allowed (the current
// and the next one during a rotation) and the server's leaf must equal one of
// them. The result can be used as RedisStoreConfig.TLSConfig or as a go-redis
// Options.TLSConfig.
func PinnedLeafTLSConfig(leafPEM []byte) (*tls.Config, error) {
	return PinnedLeafTLSConfigWithOptions(leafPEM, PinnedLeafOptions{})
}

// PinnedLeafTLSConfigWithOptions is PinnedLeafTLSConfig with options.
func PinnedLeafTLSConfigWithOptions(leafPEM []byte, opts PinnedLeafOptions) (*tls.Config, error) {
	certs, err := ParsePinnedLeaves(leafPEM)
	if err != nil {
		return nil, err
	}
	pins := make([][sha256.Size]byte, 0, len(certs))
	for i, cert := range certs {
		if opts.RefuseCA && cert.BasicConstraintsValid && cert.IsCA {
			return nil, ratelimit.WrapConfigError(nil,
				"pinned certificate is a CA certificate; pin the server's own LEAF certificate",
				"pinned_cert_index", i)
		}
		pins = append(pins, sha256.Sum256(cert.Raw))
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

// ParsePinnedLeaves strictly parses a pin file: one or more CERTIFICATE blocks
// separated only by whitespace. Anything else — another block type, leading
// or trailing non-PEM bytes, an unparsable certificate — is refused, so a key
// or a truncated paste is reported at boot instead of silently ignored. The
// certificates are returned so a caller can log NotAfter or inspect IsCA.
func ParsePinnedLeaves(leafPEM []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := leafPEM
	for {
		rest = bytes.TrimSpace(rest)
		if len(rest) == 0 {
			break
		}
		if !bytes.HasPrefix(rest, pemBeginMarker) {
			return nil, ratelimit.WrapConfigError(nil, "pinned certificate file contains non-PEM data",
				"pinned_cert_index", len(certs))
		}
		block, next := pem.Decode(rest)
		if block == nil {
			return nil, ratelimit.WrapConfigError(nil, "pinned certificate file contains a malformed PEM block",
				"pinned_cert_index", len(certs))
		}
		if block.Type != pemBlockCertificate {
			return nil, ratelimit.WrapConfigError(nil, "pinned certificate file contains a non-CERTIFICATE PEM block",
				"pinned_cert_index", len(certs), "pem_type", block.Type)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, ratelimit.WrapConfigError(err, "pinned certificate is not a valid X.509 certificate",
				"pinned_cert_index", len(certs))
		}
		certs = append(certs, cert)
		rest = next
	}
	if len(certs) == 0 {
		return nil, ratelimit.WrapConfigError(nil, "pinned certificate PEM contains no CERTIFICATE block",
			"pinned_cert_count", 0)
	}
	return certs, nil
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
