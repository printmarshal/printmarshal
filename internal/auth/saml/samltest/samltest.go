// Package samltest is an in-process SAML 2.0 identity provider for tests,
// built on crewjam/saml's IdentityProvider: it serves its metadata over an
// httptest server, fetches the service provider's metadata from the entity
// ID it is asked for, and answers AuthnRequests with a signed Response for
// the Session the test chose. It never talks to the network beyond loopback.
package samltest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	csaml "github.com/crewjam/saml"
	"github.com/crewjam/saml/logger"

	pmsaml "printmarshal/internal/auth/saml"
)

// IdP is the running fake.
type IdP struct {
	Server *httptest.Server
	URL    string
	IDP    *csaml.IdentityProvider
	Key    *rsa.PrivateKey
	Cert   *x509.Certificate

	mu sync.Mutex
	// Session is asserted for every sign-in; tests set NameID, NameIDFormat,
	// UserName, Groups and CustomAttributes before starting a flow.
	Session csaml.Session
	// Mutate, when set, edits the assertion before it is signed (expiry tests).
	Mutate func(a *csaml.Assertion)
	// Encrypt makes the IdP encrypt the assertion to the SP's certificate,
	// as crewjam does whenever the SP metadata offers an encryption key.
	// Off by default so tests can inspect and tamper with the signed XML,
	// which is also how most production IdPs behave out of the box.
	Encrypt bool
}

// New starts the IdP.
func New(t *testing.T) *IdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "samltest idp"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	i := &IdP{Key: key, Cert: cert}
	i.Session = csaml.Session{ID: "sess-1", CreateTime: time.Now(), ExpireTime: time.Now().Add(time.Hour), Index: "idx-1", NameID: "ann", NameIDFormat: string(csaml.PersistentNameIDFormat), UserName: "ann"}
	mux := http.NewServeMux()
	i.Server = httptest.NewServer(mux)
	i.URL = i.Server.URL
	t.Cleanup(i.Server.Close)
	metaURL, _ := url.Parse(i.URL + "/metadata")
	ssoURL, _ := url.Parse(i.URL + "/sso")
	i.IDP = &csaml.IdentityProvider{Key: key, Certificate: cert, Logger: logger.DefaultLogger, MetadataURL: *metaURL, SSOURL: *ssoURL,
		ServiceProviderProvider: i, SessionProvider: i, AssertionMaker: i}
	mux.HandleFunc("/metadata", i.IDP.ServeMetadata)
	mux.HandleFunc("/sso", i.IDP.ServeSSO)
	return i
}

// EntityID is the IdP's entity ID (its metadata URL).
func (i *IdP) EntityID() string { return i.URL + "/metadata" }

// GetServiceProvider fetches the SP metadata from its entity ID, which in
// tests is the SP's metadata URL (loopback only). Unless Encrypt is set the
// encryption key descriptor is dropped so the assertion stays readable.
func (i *IdP) GetServiceProvider(r *http.Request, id string) (*csaml.EntityDescriptor, error) {
	if !strings.HasPrefix(id, "http://127.0.0.1") && !strings.HasPrefix(id, "http://localhost") {
		return nil, os.ErrNotExist
	}
	resp, err := http.Get(id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	m, err := pmsaml.ParseMetadata(b)
	if err != nil {
		return nil, err
	}
	i.mu.Lock()
	encrypt := i.Encrypt
	i.mu.Unlock()
	if !encrypt {
		for d := range m.SPSSODescriptors {
			var keep []csaml.KeyDescriptor
			for _, kd := range m.SPSSODescriptors[d].KeyDescriptors {
				if kd.Use != "encryption" {
					keep = append(keep, kd)
				}
			}
			m.SPSSODescriptors[d].KeyDescriptors = keep
		}
	}
	return m, nil
}

// GetSession returns the configured session for every request.
func (i *IdP) GetSession(w http.ResponseWriter, r *http.Request, req *csaml.IdpAuthnRequest) *csaml.Session {
	i.mu.Lock()
	defer i.mu.Unlock()
	s := i.Session
	return &s
}

// MakeAssertion builds the default assertion, then applies Mutate.
func (i *IdP) MakeAssertion(req *csaml.IdpAuthnRequest, session *csaml.Session) error {
	if err := (csaml.DefaultAssertionMaker{}).MakeAssertion(req, session); err != nil {
		return err
	}
	i.mu.Lock()
	m := i.Mutate
	i.mu.Unlock()
	if m != nil {
		m(req.Assertion)
	}
	return nil
}

// Response is what the IdP would post to the SP's ACS.
type Response struct {
	ACSURL       string
	SAMLResponse string // base64
	RelayState   string
}

// Respond takes the redirect-binding URL the SP sent the browser to (the
// IdP's /sso with SAMLRequest) and returns the signed response for its ACS,
// without going through HTML.
func (i *IdP) Respond(t *testing.T, ssoRedirect string) Response {
	t.Helper()
	httpReq, err := http.NewRequest(http.MethodGet, ssoRedirect, nil)
	if err != nil {
		t.Fatal(err)
	}
	httpReq.RemoteAddr = "127.0.0.1:1234"
	req, err := csaml.NewIdpAuthnRequest(i.IDP, httpReq)
	if err != nil {
		t.Fatal(err)
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("validate authn request: %v", err)
	}
	session := i.GetSession(nil, httpReq, req)
	if err := i.MakeAssertion(req, session); err != nil {
		t.Fatal(err)
	}
	form, err := req.PostBinding()
	if err != nil {
		t.Fatal(err)
	}
	return Response{ACSURL: form.URL, SAMLResponse: form.SAMLResponse, RelayState: form.RelayState}
}

// Tamper edits the decoded XML of a response (old → new) and re-encodes
// it, leaving the signatures stale.
func Tamper(r Response, old, new string) (Response, error) {
	raw, err := base64.StdEncoding.DecodeString(r.SAMLResponse)
	if err != nil {
		return r, err
	}
	if !strings.Contains(string(raw), old) {
		return r, errors.New("tamper: text not found in response")
	}
	r.SAMLResponse = base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(string(raw), old, new)))
	return r, nil
}
