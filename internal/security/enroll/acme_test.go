package enroll

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// acmeServer is a minimal RFC 8555 CA: directory, nonces, one account, one
// order with one dns-01 authorization, finalize and certificate download.
// It does not verify JWS signatures; it only decodes the payloads.
type acmeServer struct {
	t      *testing.T
	ca     *testCA
	url    string
	mu     sync.Mutex
	ident  string
	token  string
	authz  string // pending | valid
	order  string // pending | ready | valid
	cert   *x509.Certificate
	dnsTXT map[string]string // records the fake DNS provider "published"
}

func (s *acmeServer) nonce(w http.ResponseWriter) {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	w.Header().Set("Replay-Nonce", hex.EncodeToString(b))
	w.Header().Set("Cache-Control", "no-store")
}

func (s *acmeServer) payload(r *http.Request, dst any) {
	var jws struct {
		Payload string `json:"payload"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &jws); err != nil {
		s.t.Fatalf("jws: %v", err)
	}
	if jws.Payload == "" || dst == nil {
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(jws.Payload)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		s.t.Fatalf("payload %s: %v", raw, err)
	}
}

func (s *acmeServer) writeJSON(w http.ResponseWriter, status int, v any) {
	s.nonce(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *acmeServer) orderJSON() map[string]any {
	o := map[string]any{"status": s.order, "expires": time.Now().Add(time.Hour).Format(time.RFC3339),
		"identifiers":    []map[string]string{{"type": "dns", "value": s.ident}},
		"authorizations": []string{s.url + "/authz/1"}, "finalize": s.url + "/order/1/finalize"}
	if s.order == "valid" {
		o["certificate"] = s.url + "/cert/1"
	}
	return o
}

func (s *acmeServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /directory", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, 200, map[string]any{"newNonce": s.url + "/new-nonce", "newAccount": s.url + "/new-account", "newOrder": s.url + "/new-order",
			"meta": map[string]any{"termsOfService": s.url + "/tos"}})
	})
	mux.HandleFunc("/new-nonce", func(w http.ResponseWriter, r *http.Request) {
		s.nonce(w)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /new-account", func(w http.ResponseWriter, r *http.Request) {
		var acct struct {
			TOS     bool     `json:"termsOfServiceAgreed"`
			Contact []string `json:"contact"`
		}
		s.payload(r, &acct)
		if !acct.TOS {
			s.writeJSON(w, 400, map[string]any{"type": "urn:ietf:params:acme:error:userActionRequired"})
			return
		}
		w.Header().Set("Location", s.url+"/acct/1")
		s.writeJSON(w, 201, map[string]any{"status": "valid", "contact": acct.Contact})
	})
	mux.HandleFunc("POST /new-order", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Identifiers []struct{ Type, Value string } `json:"identifiers"`
		}
		s.payload(r, &req)
		s.mu.Lock()
		defer s.mu.Unlock()
		if len(req.Identifiers) != 1 || req.Identifiers[0].Type != "dns" {
			s.writeJSON(w, 400, map[string]any{"type": "urn:ietf:params:acme:error:malformed", "detail": "one dns identifier expected"})
			return
		}
		s.ident, s.token, s.authz, s.order = req.Identifiers[0].Value, "tok-"+hex.EncodeToString([]byte(req.Identifiers[0].Value))[:8], "pending", "pending"
		w.Header().Set("Location", s.url+"/order/1")
		s.writeJSON(w, 201, s.orderJSON())
	})
	mux.HandleFunc("POST /authz/1", func(w http.ResponseWriter, r *http.Request) {
		s.payload(r, nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.writeJSON(w, 200, map[string]any{"status": s.authz, "expires": time.Now().Add(time.Hour).Format(time.RFC3339),
			"identifier": map[string]string{"type": "dns", "value": s.ident},
			"challenges": []map[string]any{{"type": "dns-01", "url": s.url + "/chall/1", "token": s.token, "status": s.authz}}})
	})
	mux.HandleFunc("POST /chall/1", func(w http.ResponseWriter, r *http.Request) {
		s.payload(r, nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		// "Validate": the record for _acme-challenge.<ident> must have been published.
		if _, ok := s.dnsTXT["_acme-challenge."+s.ident]; !ok {
			s.writeJSON(w, 400, map[string]any{"type": "urn:ietf:params:acme:error:dns", "detail": "TXT record not found"})
			return
		}
		s.authz, s.order = "valid", "ready"
		s.writeJSON(w, 200, map[string]any{"type": "dns-01", "url": s.url + "/chall/1", "token": s.token, "status": "valid"})
	})
	mux.HandleFunc("POST /order/1", func(w http.ResponseWriter, r *http.Request) {
		s.payload(r, nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.writeJSON(w, 200, s.orderJSON())
	})
	mux.HandleFunc("POST /order/1/finalize", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CSR string `json:"csr"`
		}
		s.payload(r, &req)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.order != "ready" {
			s.writeJSON(w, 403, map[string]any{"type": "urn:ietf:params:acme:error:orderNotReady"})
			return
		}
		der, err := base64.RawURLEncoding.DecodeString(req.CSR)
		if err != nil {
			s.t.Fatal(err)
		}
		csr, err := x509.ParseCertificateRequest(der)
		if err != nil {
			s.t.Fatal(err)
		}
		if len(csr.DNSNames) != 1 || csr.DNSNames[0] != s.ident {
			s.writeJSON(w, 400, map[string]any{"type": "urn:ietf:params:acme:error:badCSR", "detail": fmt.Sprintf("csr names %v != order %s", csr.DNSNames, s.ident)})
			return
		}
		s.cert = s.ca.sign(s.t, csr)
		s.order = "valid"
		w.Header().Set("Location", s.url+"/order/1")
		s.writeJSON(w, 200, s.orderJSON())
	})
	mux.HandleFunc("POST /cert/1", func(w http.ResponseWriter, r *http.Request) {
		s.payload(r, nil)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.nonce(w)
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		w.Write([]byte(EncodeCertPEM(s.cert, s.ca.cert)))
	})
	return mux
}

// fakeDNS records what would be published and tells the CA.
type fakeDNS struct {
	srv       *acmeServer
	presented []string
	cleaned   []string
}

func (f *fakeDNS) Present(_ context.Context, fqdn, value string) error {
	f.presented = append(f.presented, fqdn+"="+value)
	f.srv.mu.Lock()
	f.srv.dnsTXT[fqdn] = value
	f.srv.mu.Unlock()
	return nil
}

func (f *fakeDNS) CleanUp(_ context.Context, fqdn, _ string) error {
	f.cleaned = append(f.cleaned, fqdn)
	return nil
}

func newACMEServer(t *testing.T) (*acmeServer, *httptest.Server) {
	ca := newTestCA(t)
	s := &acmeServer{t: t, ca: ca, dnsTXT: map[string]string{}}
	ts := httptest.NewTLSServer(s.handler())
	s.url = ts.URL
	return s, ts
}

func TestACMEEnrollDNS01(t *testing.T) {
	srv, ts := newACMEServer(t)
	defer ts.Close()
	dns := &fakeDNS{srv: srv}
	var notes []string
	c, err := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollACME, URL: ts.URL + "/directory", CAFingerprint: Fingerprint(ts.Certificate()),
		Options: map[string]string{"email": "ops@example.test"}}, DNS: dns, Notify: func(m string) { notes = append(notes, m) }, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := GenerateKey(KeyECDSAP256)
	csr, _ := NewCSR(key, "printer.example.test", []string{"printer.example.test"})
	res, err := c.Enroll(context.Background(), csr, key)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cert.DNSNames[0] != "printer.example.test" || res.Cert.CheckSignatureFrom(srv.ca.cert) != nil {
		t.Errorf("issued = %+v", res.Cert.Subject)
	}
	if len(res.Chain) != 1 || !res.Chain[0].Equal(srv.ca.cert) {
		t.Errorf("chain = %v", res.Chain)
	}
	if len(dns.presented) != 1 || !strings.HasPrefix(dns.presented[0], "_acme-challenge.printer.example.test=") || len(dns.cleaned) != 1 {
		t.Errorf("dns provider calls: %v %v", dns.presented, dns.cleaned)
	}
	// Without an account key credential the client says so (keep it honest).
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, "\n"), "fresh account") {
		t.Errorf("expected a note about the ephemeral account, got %v", notes)
	}
}

func TestACMERefusesIPAndManualProviderExplains(t *testing.T) {
	srv, ts := newACMEServer(t)
	defer ts.Close()
	c, _ := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollACME, URL: ts.URL + "/directory", CAFingerprint: Fingerprint(ts.Certificate())}, Timeout: 10 * time.Second})
	key, _ := GenerateKey(KeyRSA2048)
	csr, _ := NewCSR(key, "10.0.0.1", []string{"10.0.0.1"})
	if _, err := c.Enroll(context.Background(), csr, key); err == nil || !strings.Contains(err.Error(), "IP") {
		t.Errorf("IP identifiers must be refused: %v", err)
	}
	// The default Manual provider with no wait stops with the record to create.
	csr, _ = NewCSR(key, "p.example.test", []string{"p.example.test"})
	_, err := c.Enroll(context.Background(), csr, key)
	var dnsErr *ErrDNSRecordRequired
	if err == nil || !errorsAs(err, &dnsErr) || dnsErr.FQDN != "_acme-challenge.p.example.test" || dnsErr.Value == "" {
		t.Fatalf("manual provider should report the TXT record: %v", err)
	}
	_ = srv
	// Manual with a wait and a stubbed resolver completes once the record "appears".
	manual := &Manual{Wait: 5 * time.Second, Lookup: func(_ context.Context, name string) ([]string, error) {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		v, ok := srv.dnsTXT[name]
		if !ok {
			return nil, fmt.Errorf("NXDOMAIN")
		}
		return []string{v}, nil
	}}
	manual.Notify = func(m string) {
		// Simulate the operator creating the record as soon as it is announced.
		if strings.Contains(m, "create the DNS record") {
			parts := strings.Split(m, `"`)
			srv.mu.Lock()
			srv.dnsTXT[parts[1]] = parts[3]
			srv.mu.Unlock()
		}
	}
	c2, _ := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollACME, URL: ts.URL + "/directory", CAFingerprint: Fingerprint(ts.Certificate())}, DNS: manual, Timeout: 20 * time.Second})
	if _, err := c2.Enroll(context.Background(), csr, key); err != nil {
		t.Fatalf("manual provider with wait: %v", err)
	}
}

func errorsAs(err error, target **ErrDNSRecordRequired) bool {
	for err != nil {
		if e, ok := err.(*ErrDNSRecordRequired); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
