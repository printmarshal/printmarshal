// Package saml builds the SAML 2.0 service provider PrintMarshal exposes
// (docs/auth.md) on top of github.com/crewjam/saml: IdP metadata loading,
// the SP signing pair (generated under the data directory when none is
// configured), the assertion helpers the API layer needs, and a replay
// cache for assertion IDs.
package saml

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	csaml "github.com/crewjam/saml"
	xrv "github.com/mattermost/xml-roundtrip-validator"
	dsig "github.com/russellhaering/goxmldsig"
)

// Config is what the service provider needs; the API layer fills it from
// config.SAML.
type Config struct {
	IDPMetadataURL  string
	IDPMetadataFile string
	BaseURL         string // https://printmarshal.example.com
	EntityID        string // default <BaseURL>/api/auth/saml/metadata
	ACSURL          string // default <BaseURL>/api/auth/saml/acs
	CertFile        string
	KeyFile         string
	DataDir         string // where a generated pair is kept
	SignRequests    bool
	HTTPClient      *http.Client
	Log             *slog.Logger
}

const maxMetadata = 4 << 20

// MetadataPath and ACSPath are the SP routes under /api/auth/saml.
const (
	MetadataPath = "/api/auth/saml/metadata"
	ACSPath      = "/api/auth/saml/acs"
)

// MetadataURL returns the SP metadata URL (also the default entity ID).
func (c Config) MetadataURL() string { return strings.TrimRight(c.BaseURL, "/") + MetadataPath }

// EntityIDOrDefault is the audience assertions must name.
func (c Config) EntityIDOrDefault() string {
	if c.EntityID != "" {
		return c.EntityID
	}
	return c.MetadataURL()
}

// ACSURLOrDefault is where the IdP posts assertions.
func (c Config) ACSURLOrDefault() string {
	if c.ACSURL != "" {
		return c.ACSURL
	}
	return strings.TrimRight(c.BaseURL, "/") + ACSPath
}

// New loads the IdP metadata and the SP key pair and returns a configured
// service provider. A failure is not cached by callers, so an IdP that was
// unreachable at first use is retried.
func New(ctx context.Context, cfg Config) (*csaml.ServiceProvider, error) {
	if cfg.BaseURL == "" && (cfg.EntityID == "" || cfg.ACSURL == "") {
		return nil, errors.New("saml: baseUrl (or entityId and acsUrl) is required")
	}
	meta, err := loadIDPMetadata(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cert, key, err := LoadOrCreateKeyPair(cfg.CertFile, cfg.KeyFile, cfg.DataDir, cfg.Log)
	if err != nil {
		return nil, err
	}
	metaURL, err := url.Parse(cfg.MetadataURL())
	if err != nil {
		return nil, fmt.Errorf("saml: baseUrl: %w", err)
	}
	acs, err := url.Parse(cfg.ACSURLOrDefault())
	if err != nil {
		return nil, fmt.Errorf("saml: acsUrl: %w", err)
	}
	sp := &csaml.ServiceProvider{
		EntityID:          cfg.EntityIDOrDefault(),
		Key:               key,
		Certificate:       cert,
		MetadataURL:       *metaURL,
		AcsURL:            *acs,
		IDPMetadata:       meta,
		AuthnNameIDFormat: csaml.UnspecifiedNameIDFormat,
		AllowIDPInitiated: false,
		HTTPClient:        cfg.HTTPClient,
	}
	if cfg.SignRequests {
		sp.SignatureMethod = dsig.RSASHA256SignatureMethod
	}
	if sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding) == "" {
		return nil, errors.New("saml: the IdP metadata offers no HTTP-Redirect single sign-on endpoint")
	}
	return sp, nil
}

func loadIDPMetadata(ctx context.Context, cfg Config) (*csaml.EntityDescriptor, error) {
	var raw []byte
	switch {
	case cfg.IDPMetadataFile != "":
		b, err := os.ReadFile(cfg.IDPMetadataFile)
		if err != nil {
			return nil, fmt.Errorf("saml: idpMetadataFile: %w", err)
		}
		raw = b
	case cfg.IDPMetadataURL != "":
		if err := checkHTTPS(cfg.IDPMetadataURL); err != nil {
			return nil, err
		}
		client := cfg.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 15 * time.Second}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.IDPMetadataURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("saml: idp metadata: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("saml: idp metadata: status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadata))
		if err != nil {
			return nil, err
		}
		raw = b
	default:
		return nil, errors.New("saml: idpMetadataUrl or idpMetadataFile is required")
	}
	meta, err := ParseMetadata(raw)
	if err != nil {
		return nil, fmt.Errorf("saml: idp metadata: %w", err)
	}
	if len(meta.IDPSSODescriptors) == 0 {
		return nil, errors.New("saml: idp metadata has no IDPSSODescriptor")
	}
	return meta, nil
}

// ParseMetadata parses an EntityDescriptor, or the first IdP entity of an
// EntitiesDescriptor (federation metadata), after checking the XML
// round-trips safely.
func ParseMetadata(data []byte) (*csaml.EntityDescriptor, error) {
	if err := xrv.Validate(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	entity := &csaml.EntityDescriptor{}
	err := xml.Unmarshal(data, entity)
	if err != nil && strings.Contains(err.Error(), "<EntitiesDescriptor>") {
		entities := &csaml.EntitiesDescriptor{}
		if err := xml.Unmarshal(data, entities); err != nil {
			return nil, err
		}
		for i := range entities.EntityDescriptors {
			if len(entities.EntityDescriptors[i].IDPSSODescriptors) > 0 {
				return &entities.EntityDescriptors[i], nil
			}
		}
		return nil, errors.New("no entity with an IDPSSODescriptor")
	}
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// checkHTTPS insists on https except for loopback (development, tests).
func checkHTTPS(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("saml: %q is not an absolute URL", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || host == "localhost" {
			return nil
		}
		return fmt.Errorf("saml: %q must use https (plain http is only allowed on loopback)", raw)
	}
	return fmt.Errorf("saml: unsupported scheme in %q", raw)
}

// LoadOrCreateKeyPair returns the SP signing certificate and key: the
// configured PEM files when set, else <dataDir>/saml-sp.crt and
// saml-sp.key, generated (RSA 2048, self-signed, 10 years) on first use.
func LoadOrCreateKeyPair(certFile, keyFile, dataDir string, log *slog.Logger) (*x509.Certificate, *rsa.PrivateKey, error) {
	if (certFile == "") != (keyFile == "") {
		return nil, nil, errors.New("saml: spCertFile and spKeyFile must be set together")
	}
	generated := false
	if certFile == "" {
		if dataDir == "" {
			return nil, nil, errors.New("saml: no spCertFile/spKeyFile and no data directory to generate them in")
		}
		certFile, keyFile = filepath.Join(dataDir, "saml-sp.crt"), filepath.Join(dataDir, "saml-sp.key")
		if _, err := os.Stat(keyFile); errors.Is(err, os.ErrNotExist) {
			if err := generatePair(certFile, keyFile); err != nil {
				return nil, nil, err
			}
			generated = true
		}
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: spCertFile: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: spKeyFile: %w", err)
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil || cb.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("saml: %s is not a PEM certificate", certFile)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("saml: %s: %w", certFile, err)
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, nil, fmt.Errorf("saml: %s is not a PEM key", keyFile)
	}
	var key *rsa.PrivateKey
	switch kb.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(kb.Bytes)
	case "PRIVATE KEY":
		var k any
		k, err = x509.ParsePKCS8PrivateKey(kb.Bytes)
		if err == nil {
			var ok bool
			if key, ok = k.(*rsa.PrivateKey); !ok {
				err = errors.New("not an RSA key")
			}
		}
	default:
		err = fmt.Errorf("unsupported PEM type %q", kb.Type)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("saml: %s: %w", keyFile, err)
	}
	if generated && log != nil {
		log.Warn("generated a self-signed SAML service-provider signing certificate; back it up with the data directory and re-upload the SP metadata if it changes", "cert", certFile, "key", keyFile)
	}
	return cert, key, nil
}

func generatePair(certFile, keyFile string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "PrintMarshal SAML SP"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		return err
	}
	return os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// NameID returns the assertion's subject NameID value and format.
func NameID(a *csaml.Assertion) (value, format string) {
	if a == nil || a.Subject == nil || a.Subject.NameID == nil {
		return "", ""
	}
	return strings.TrimSpace(a.Subject.NameID.Value), a.Subject.NameID.Format
}

// StableNameID reports whether a NameID format may serve as the account's
// stable subject: persistent, unspecified or absent. Transient and
// emailAddress identifiers can change or be reissued.
func StableNameID(format string) bool {
	switch format {
	case "", string(csaml.PersistentNameIDFormat), string(csaml.UnspecifiedNameIDFormat), "urn:oasis:names:tc:SAML:2.0:nameid-format:unspecified":
		return true
	}
	return false
}

// Values returns every value of the attribute whose Name or FriendlyName is
// name (case-insensitive), across all attribute statements.
func Values(a *csaml.Assertion, name string) []string {
	if a == nil || strings.TrimSpace(name) == "" {
		return nil
	}
	var out []string
	for _, st := range a.AttributeStatements {
		for _, attr := range st.Attributes {
			if !strings.EqualFold(attr.Name, name) && !strings.EqualFold(attr.FriendlyName, name) {
				continue
			}
			for _, v := range attr.Values {
				if s := strings.TrimSpace(v.Value); s != "" {
					out = append(out, s)
				} else if v.NameID != nil && strings.TrimSpace(v.NameID.Value) != "" {
					out = append(out, strings.TrimSpace(v.NameID.Value))
				}
			}
		}
	}
	return out
}

// Expiry is the latest instant the assertion may be presented (the larger
// of the Conditions and bearer SubjectConfirmationData NotOnOrAfter), used
// to size the replay cache entry.
func Expiry(a *csaml.Assertion, now time.Time) time.Time {
	t := now.Add(csaml.MaxIssueDelay + csaml.MaxClockSkew)
	if a.Conditions != nil && a.Conditions.NotOnOrAfter.After(t) {
		t = a.Conditions.NotOnOrAfter
	}
	if a.Subject != nil {
		for _, sc := range a.Subject.SubjectConfirmations {
			if sc.SubjectConfirmationData != nil && sc.SubjectConfirmationData.NotOnOrAfter.After(t) {
				t = sc.SubjectConfirmationData.NotOnOrAfter
			}
		}
	}
	return t.Add(csaml.MaxClockSkew)
}

// ReplayCache remembers assertion IDs until they expire so a captured
// response cannot be posted twice.
type ReplayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// Seen records id and reports whether it was already present (and not yet
// expired). Expired entries are pruned on every call.
func (c *ReplayCache) Seen(id string, until time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.seen == nil {
		c.seen = map[string]time.Time{}
	}
	for k, t := range c.seen {
		if !t.After(now) {
			delete(c.seen, k)
		}
	}
	if t, ok := c.seen[id]; ok && t.After(now) {
		return true
	}
	c.seen[id] = until
	return false
}
