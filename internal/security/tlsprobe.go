package security

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// TLSReport is what the vendor-neutral TLS probe observed on one port.
type TLSReport struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// MinVersion is the lowest protocol version the server accepted ("1.0".."1.3").
	MinVersion string `json:"minVersion"`
	// Versions lists every version the server negotiated.
	Versions   []string           `json:"versions"`
	Cipher     string             `json:"cipher,omitempty"` // suite of the default handshake
	Leaf       *model.Certificate `json:"leaf,omitempty"`
	ChainLen   int                `json:"chainLen"`
	Error      string             `json:"error,omitempty"`
	HandshakeV string             `json:"negotiated,omitempty"` // version of the default handshake
}

// TLSProber probes a host's TLS endpoints.
type TLSProber func(ctx context.Context, host string, ports []int) []TLSReport

// DefaultTLSPorts are the printer services that commonly speak TLS: HTTPS and IPPS.
var DefaultTLSPorts = []int{443, 631}

// tlsVersions maps protocol constants to the catalogue's enum values, oldest first.
var tlsVersions = []struct {
	v    uint16
	name string
}{
	{tls.VersionTLS10, "1.0"},
	{tls.VersionTLS11, "1.1"},
	{tls.VersionTLS12, "1.2"},
	{tls.VersionTLS13, "1.3"},
}

func tlsVersionName(v uint16) string {
	for _, t := range tlsVersions {
		if t.v == v {
			return t.name
		}
	}
	return fmt.Sprintf("0x%04x", v)
}

// ProbeTLS connects to host:port with TLS for every port, records the leaf
// certificate from a default handshake, then finds the lowest protocol
// version the server still accepts by offering exactly one version at a time
// (TLS 1.0, 1.1, 1.2, 1.3; RFC 8446 §4.2.1 supported_versions and the legacy
// version field). No credentials are sent; the probe never verifies the chain
// because printers almost always present self-signed certificates.
func ProbeTLS(ctx context.Context, host string, ports []int) []TLSReport {
	if len(ports) == 0 {
		ports = DefaultTLSPorts
	}
	out := make([]TLSReport, 0, len(ports))
	for _, port := range ports {
		out = append(out, probeTLSPort(ctx, host, port))
	}
	return out
}

func probeTLSPort(ctx context.Context, host string, port int) TLSReport {
	rep := TLSReport{Host: host, Port: port}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 4 * time.Second}

	// Default handshake: whatever the server prefers, to read the leaf.
	conn, err := dialTLS(ctx, dialer, addr, host, probeConfig(tls.VersionTLS10, 0))
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	st := conn.ConnectionState()
	conn.Close()
	rep.HandshakeV = tlsVersionName(st.Version)
	rep.Cipher = tls.CipherSuiteName(st.CipherSuite)
	rep.ChainLen = len(st.PeerCertificates)
	if len(st.PeerCertificates) > 0 {
		leaf := CertificateFromX509(st.PeerCertificates[0], "identity", fmt.Sprintf("tls:%d", port))
		rep.Leaf = &leaf
	}

	// One version at a time, oldest first. Go refuses to *offer* SSLv3, so the
	// probe starts at TLS 1.0; anything older is reported as unknown.
	for _, v := range tlsVersions {
		cfg := probeConfig(v.v, v.v)
		c, err := dialTLS(ctx, dialer, addr, host, cfg)
		if err != nil {
			continue
		}
		c.Close()
		rep.Versions = append(rep.Versions, v.name)
		if rep.MinVersion == "" {
			rep.MinVersion = v.name
		}
	}
	if rep.MinVersion == "" {
		// The default handshake worked but no single-version offer did; keep the negotiated one.
		rep.MinVersion = rep.HandshakeV
		rep.Versions = []string{rep.HandshakeV}
	}
	return rep
}

// probeConfig offers every cipher suite Go can speak, including the legacy
// RSA-key-exchange and CBC suites that crypto/tls no longer offers by
// default (Go 1.22+), because many printers support nothing else. The probe
// wants to learn what the device accepts, not to negotiate a safe session;
// no data is sent over the connection and the chain is never verified.
func probeConfig(min, max uint16) *tls.Config {
	var suites []uint16
	for _, cs := range tls.CipherSuites() {
		suites = append(suites, cs.ID)
	}
	for _, cs := range tls.InsecureCipherSuites() {
		suites = append(suites, cs.ID)
	}
	return &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // probe of self-signed devices
		MinVersion:         min,
		MaxVersion:         max,
		CipherSuites:       suites,
	}
}

func dialTLS(ctx context.Context, d *net.Dialer, addr, host string, cfg *tls.Config) (*tls.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(host) == nil {
		cfg.ServerName = host
	}
	conn := tls.Client(raw, cfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		raw.Close()
		return nil, err
	}
	return conn, nil
}

// CertificateFromX509 converts a parsed certificate into the model type.
func CertificateFromX509(c *x509.Certificate, use, id string) model.Certificate {
	sum := sha256.Sum256(c.Raw)
	return model.Certificate{
		ID:          id,
		Use:         use,
		Subject:     c.Subject.String(),
		Issuer:      c.Issuer.String(),
		Serial:      c.SerialNumber.Text(16),
		NotBefore:   c.NotBefore.UTC(),
		NotAfter:    c.NotAfter.UTC(),
		Fingerprint: hex.EncodeToString(sum[:]),
		SelfSigned:  isSelfSigned(c),
		KeyType:     keyTypeName(c),
	}
}

func isSelfSigned(c *x509.Certificate) bool {
	if !strings.EqualFold(c.Subject.String(), c.Issuer.String()) {
		return false
	}
	// Not CheckSignatureFrom: printer self-signed leafs usually lack the CA
	// basic constraint / keyCertSign it insists on. Verify with the cert's own key.
	return c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
}

func keyTypeName(c *x509.Certificate) string {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return fmt.Sprintf("rsa%d", k.N.BitLen())
	case *ecdsa.PublicKey:
		return "ecdsa-" + strings.ToLower(k.Curve.Params().Name)
	case ed25519.PublicKey:
		return "ed25519"
	}
	return c.PublicKeyAlgorithm.String()
}

// TLSSettings turns probe reports into security settings: the weakest
// accepted TLS version across ports and the identity certificate facts
// (security.cert.*). Every setting is marked Source "probe" and read-only
// because it is an observation, not a device setting.
func TLSSettings(reports []TLSReport, now time.Time) []model.Setting {
	var minV string
	var leaf *model.Certificate
	for i := range reports {
		r := reports[i]
		if r.Error != "" {
			continue
		}
		if minV == "" || versionLess(r.MinVersion, minV) {
			minV = r.MinVersion
		}
		if leaf == nil && r.Leaf != nil {
			leaf = r.Leaf
		}
	}
	if minV == "" {
		return nil
	}
	out := []model.Setting{probeSetting("security.tls.minVersion", minV)}
	if leaf != nil {
		days := int(leaf.NotAfter.Sub(now).Hours() / 24)
		out = append(out,
			probeSetting("security.cert.identityPresent", true),
			probeSetting("security.cert.identitySelfSigned", leaf.SelfSigned),
			probeSetting("security.cert.identityExpiresAt", leaf.NotAfter.Format(time.RFC3339)),
			probeSetting("security.cert.identityDaysLeft", days),
		)
	}
	return out
}

func versionLess(a, b string) bool {
	fa, _ := strconv.ParseFloat(a, 64)
	fb, _ := strconv.ParseFloat(b, 64)
	return fa < fb
}

func probeSetting(key string, v any) model.Setting {
	def := model.CatalogByKey[key]
	return model.Setting{Key: key, Area: def.Area, Name: def.Name, Value: v, Type: def.Type, Options: def.Options,
		Writable: false, Source: "probe", ReadOnlyReason: "observed by network probe, not read from the device's settings"}
}
