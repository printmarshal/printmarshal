// Package enroll obtains X.509 certificates from a certificate authority on
// a device's behalf using the standard enrollment protocols (spec §8.5):
// SCEP (RFC 8894), EST (RFC 7030) and ACME (RFC 8555). Keys and CSRs are
// generated here, in Go; installing the result on the device is the job of a
// driver implementing engine.CertManager.
package enroll

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// Key types a CA profile may ask for.
const (
	KeyRSA2048   = "rsa2048"
	KeyRSA3072   = "rsa3072"
	KeyECDSAP256 = "ecdsa-p256"
)

// GenerateKey creates a fresh private key. RSA 2048 is the default because
// it is what printers most widely accept (and what SCEP requires).
func GenerateKey(keyType string) (crypto.Signer, error) {
	switch keyType {
	case "", KeyRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case KeyRSA3072:
		return rsa.GenerateKey(rand.Reader, 3072)
	case KeyECDSAP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
	return nil, fmt.Errorf("unknown key type %q (rsa2048, rsa3072, ecdsa-p256)", keyType)
}

// SplitSANs sorts subject alternative names into DNS names and IP addresses.
func SplitSANs(sans []string) (dns []string, ips []net.IP) {
	for _, s := range sans {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip)
			continue
		}
		dns = append(dns, s)
	}
	return
}

// NewCSR builds a PKCS#10 certificate request (RFC 2986) in DER form.
func NewCSR(key crypto.Signer, commonName string, sans []string) ([]byte, error) {
	dns, ips := SplitSANs(sans)
	tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: commonName}, DNSNames: dns, IPAddresses: ips}
	if commonName == "" && len(dns) > 0 {
		tmpl.Subject.CommonName = dns[0]
	}
	if tmpl.Subject.CommonName == "" && len(ips) > 0 {
		tmpl.Subject.CommonName = ips[0].String()
	}
	if tmpl.Subject.CommonName == "" {
		return nil, errors.New("a common name or at least one subject alternative name is required")
	}
	return x509.CreateCertificateRequest(rand.Reader, tmpl, key)
}

// EncodeKeyPEM serialises a private key as PKCS#8 PEM.
func EncodeKeyPEM(key crypto.Signer) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), nil
}

// ParseKeyPEM reads a PKCS#8, PKCS#1 or EC private key PEM.
func ParseKeyPEM(s string) (crypto.Signer, error) {
	blk, _ := pem.Decode([]byte(s))
	if blk == nil {
		return nil, errors.New("no PEM block in key")
	}
	if k, err := x509.ParsePKCS8PrivateKey(blk.Bytes); err == nil {
		if s, ok := k.(crypto.Signer); ok {
			return s, nil
		}
		return nil, errors.New("unsupported private key type")
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	return nil, errors.New("unrecognised private key format")
}

// EncodeCertPEM serialises certificates as PEM, in order.
func EncodeCertPEM(certs ...*x509.Certificate) string {
	var b strings.Builder
	for _, c := range certs {
		if c == nil {
			continue
		}
		b.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
	}
	return b.String()
}

// ParseCertsPEM reads every CERTIFICATE block.
func ParseCertsPEM(s string) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	rest := []byte(s)
	for {
		var blk *pem.Block
		blk, rest = pem.Decode(rest)
		if blk == nil {
			break
		}
		if blk.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(blk.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Fingerprint is the SHA-256 hex fingerprint of a certificate.
func Fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return hex.EncodeToString(sum[:])
}

// Result is an issued certificate with whatever chain the CA returned.
type Result struct {
	Cert  *x509.Certificate
	Chain []*x509.Certificate // intermediates/roots returned alongside, leaf excluded
}

// Client enrolls a CSR with a CA.
type Client interface {
	// Enroll submits csrDER (signed by key) and returns the issued certificate.
	Enroll(ctx context.Context, csrDER []byte, key crypto.Signer) (*Result, error)
	// CACerts fetches the CA's certificates (for trust and for installing as CA certs).
	CACerts(ctx context.Context) ([]*x509.Certificate, error)
}

// Config builds a client for a CA profile. Secrets come from the profile's
// credential: Username/Password for EST HTTP auth or the SCEP challenge
// password (Password), AccountKeyPEM for the ACME account key.
type Config struct {
	Profile       *model.CAProfile
	Username      string
	Password      string
	AccountKeyPEM string
	// DNS answers ACME dns-01 challenges; nil = Manual with no wait.
	DNS DNSProvider
	// Notify receives operator-facing progress messages (never secrets).
	Notify func(msg string)
	// HTTPClient overrides the transport (tests); nil builds one from the profile.
	HTTPClient *http.Client
	// Timeout bounds the whole enrollment (default 5 minutes).
	Timeout time.Duration
}

// NewClient returns the protocol client for the profile.
func NewClient(cfg Config) (Client, error) {
	if cfg.Profile == nil {
		return nil, errors.New("CA profile is required")
	}
	if cfg.Profile.URL == "" {
		return nil, errors.New("CA profile has no URL")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = HTTPClient(cfg.Profile.CAFingerprint, 30*time.Second)
	}
	if cfg.Notify == nil {
		cfg.Notify = func(string) {}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	switch cfg.Profile.Protocol {
	case model.EnrollSCEP:
		return &SCEPClient{cfg: cfg}, nil
	case model.EnrollEST:
		return &ESTClient{cfg: cfg}, nil
	case model.EnrollACME:
		return &ACMEClient{cfg: cfg}, nil
	}
	return nil, fmt.Errorf("unknown enrollment protocol %q (scep, est, acme)", cfg.Profile.Protocol)
}

// HTTPClient builds an HTTP client. With a fingerprint (SHA-256 hex of the
// server's leaf or any chain certificate) the TLS connection is pinned to it
// instead of the system roots, which is how private CAs are reached safely.
func HTTPClient(fingerprint string, timeout time.Duration) *http.Client {
	fp := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(fingerprint), ":", ""))
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 10 * time.Second}
	if fp != "" {
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // replaced by fingerprint pinning below
			MinVersion:         tls.VersionTLS12,
			VerifyConnection: func(cs tls.ConnectionState) error {
				for _, c := range cs.PeerCertificates {
					if Fingerprint(c) == fp {
						return nil
					}
				}
				return fmt.Errorf("enrollment server certificate does not match the pinned fingerprint %s", fp)
			},
		}
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// selfSignedCert makes a short-lived self-signed certificate for a key: SCEP
// signs its requests with one (RFC 8894 §2.3), and it is the recipient of
// the CA's encrypted reply.
func selfSignedCert(key crypto.Signer, subject pkix.Name) (*x509.Certificate, error) {
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      subject,
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(der)
}

func serial() *big.Int {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[0] &= 0x7f
	return new(big.Int).SetBytes(b)
}

// leafFor picks, from certs, the one whose public key matches key.
func leafFor(certs []*x509.Certificate, key crypto.Signer) (*x509.Certificate, []*x509.Certificate) {
	pub, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil {
		return nil, certs
	}
	var leaf *x509.Certificate
	var rest []*x509.Certificate
	for _, c := range certs {
		if leaf == nil {
			if cp, err := x509.MarshalPKIXPublicKey(c.PublicKey); err == nil && string(cp) == string(pub) {
				leaf = c
				continue
			}
		}
		rest = append(rest, c)
	}
	return leaf, rest
}
