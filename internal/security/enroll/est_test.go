package enroll

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// estServer speaks RFC 7030 /cacerts and /simpleenroll with HTTP Basic auth,
// optionally answering 202 once.
type estServer struct {
	t         *testing.T
	ca        *testCA
	user, pw  string
	pendFirst bool
	calls     atomic.Int32
}

func (s *estServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/est/cacerts", func(w http.ResponseWriter, r *http.Request) {
		b, err := CertsOnlyPKCS7(s.ca.cert)
		if err != nil {
			s.t.Fatal(err)
		}
		w.Header().Set("Content-Type", ctPKCS7MIME)
		w.Header().Set("Content-Transfer-Encoding", "base64")
		w.Write([]byte(b))
	})
	mux.HandleFunc("POST /.well-known/est/simpleenroll", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); s.user != "" && (!ok || u != s.user || p != s.pw) {
			w.Header().Set("WWW-Authenticate", `Basic realm="est"`)
			http.Error(w, "unauthorized", 401)
			return
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, ctPKCS10) {
			http.Error(w, "content type "+ct, 400)
			return
		}
		body, _ := io.ReadAll(r.Body)
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		csr, err := x509.ParseCertificateRequest(der)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if s.pendFirst && s.calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusAccepted)
			return
		}
		cert := s.ca.sign(s.t, csr)
		b, _ := CertsOnlyPKCS7(cert, s.ca.cert)
		w.Header().Set("Content-Type", ctPKCS7MIME)
		w.Header().Set("Content-Transfer-Encoding", "base64")
		w.Write([]byte(b))
	})
	return mux
}

func newESTClient(t *testing.T, srv *httptest.Server, user, pw string) *ESTClient {
	t.Helper()
	// The test server has a self-signed TLS certificate: pin it, as an
	// operator would for a private CA.
	fp := Fingerprint(srv.Certificate())
	c, err := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollEST, URL: srv.URL + "/.well-known/est", CAFingerprint: fp},
		Username: user, Password: pw, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ec := c.(*ESTClient)
	ec.PollInterval, ec.PollAttempts = 10*time.Millisecond, 3
	return ec
}

func TestESTEnroll(t *testing.T) {
	ca := newTestCA(t)
	ts := httptest.NewTLSServer((&estServer{t: t, ca: ca, user: "printer", pw: "s3cret"}).handler())
	defer ts.Close()
	client := newESTClient(t, ts, "printer", "s3cret")

	cas, err := client.CACerts(context.Background())
	if err != nil || len(cas) != 1 || !cas[0].Equal(ca.cert) {
		t.Fatalf("cacerts: %v %v", cas, err)
	}
	key, _ := GenerateKey(KeyECDSAP256) // EST is fine with EC keys
	csr, _ := NewCSR(key, "mfp.example.test", []string{"mfp.example.test"})
	res, err := client.Enroll(context.Background(), csr, key)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cert.Subject.CommonName != "mfp.example.test" || res.Cert.CheckSignatureFrom(ca.cert) != nil {
		t.Errorf("issued = %+v", res.Cert.Subject)
	}
	if len(res.Chain) != 1 || !res.Chain[0].Equal(ca.cert) {
		t.Errorf("chain = %v", res.Chain)
	}
}

func TestESTPendingAndAuthFailures(t *testing.T) {
	ca := newTestCA(t)
	srv := &estServer{t: t, ca: ca, user: "u", pw: "p", pendFirst: true}
	ts := httptest.NewTLSServer(srv.handler())
	defer ts.Close()
	key, _ := GenerateKey(KeyRSA2048)
	csr, _ := NewCSR(key, "p.example.test", nil)

	client := newESTClient(t, ts, "u", "p")
	if _, err := client.Enroll(context.Background(), csr, key); err != nil {
		t.Fatalf("202 then 200 should succeed: %v", err)
	}
	if srv.calls.Load() != 2 {
		t.Errorf("expected a retry after 202, got %d calls", srv.calls.Load())
	}
	bad := newESTClient(t, ts, "u", "wrong")
	if _, err := bad.Enroll(context.Background(), csr, key); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("wrong password should be a 401 error: %v", err)
	}
	// Without the pin, the self-signed test server is rejected.
	unpinned, _ := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollEST, URL: ts.URL + "/.well-known/est"}, Timeout: 5 * time.Second})
	if _, err := unpinned.CACerts(context.Background()); err == nil {
		t.Error("unpinned self-signed server must be rejected")
	}
	// A wrong pin is rejected too.
	wrongPin, _ := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollEST, URL: ts.URL + "/.well-known/est", CAFingerprint: strings.Repeat("ab", 32)}, Timeout: 5 * time.Second})
	if _, err := wrongPin.CACerts(context.Background()); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Errorf("wrong pin: %v", err)
	}
}
