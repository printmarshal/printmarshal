package enroll

import (
	"context"
	"crypto/x509"
	"encoding/asn1"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smallstep/pkcs7"

	"printmarshal/internal/model"
)

// scepServer speaks enough of RFC 8894 to enroll: GetCACert, GetCACaps,
// PKIOperation (PKCSReq and GetCertInitial) with an optional PENDING first.
type scepServer struct {
	t         *testing.T
	ca        *testCA
	challenge string
	pendFirst bool
	requests  atomic.Int32
}

func (s *scepServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("operation") {
		case "GetCACert":
			w.Header().Set("Content-Type", ctSCEPCACert)
			w.Write(s.ca.cert.Raw)
		case "GetCACaps":
			w.Write([]byte("POSTPKIOperation\nSHA-256\nAES\nSCEPStandard\n"))
		case "PKIOperation":
			s.pkiOperation(w, r)
		default:
			http.Error(w, "bad operation", 400)
		}
	})
}

func (s *scepServer) pkiOperation(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	p7, err := pkcs7.Parse(body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := p7.Verify(); err != nil {
		http.Error(w, "signature: "+err.Error(), 400)
		return
	}
	signer := p7.GetOnlySigner()
	var msgType, txID string
	var nonce []byte
	_ = p7.UnmarshalSignedAttribute(oidSCEPMessageType, &msgType)
	_ = p7.UnmarshalSignedAttribute(oidSCEPTransactionID, &txID)
	_ = p7.UnmarshalSignedAttribute(oidSCEPSenderNonce, &nonce)
	env, err := pkcs7.Parse(p7.Content)
	if err != nil {
		http.Error(w, "envelope: "+err.Error(), 400)
		return
	}
	plain, err := env.Decrypt(s.ca.cert, s.ca.key)
	if err != nil {
		http.Error(w, "decrypt: "+err.Error(), 400)
		return
	}
	n := s.requests.Add(1)
	reply := func(status string, content []byte) {
		attrs := []pkcs7.Attribute{
			{Type: oidSCEPTransactionID, Value: printable(txID)},
			{Type: oidSCEPMessageType, Value: printable(SCEPMsgCertRep)},
			{Type: oidSCEPPKIStatus, Value: printable(status)},
			{Type: oidSCEPRecipientNonce, Value: nonce},
		}
		if status == SCEPStatusFailure {
			attrs = append(attrs, pkcs7.Attribute{Type: oidSCEPFailInfo, Value: printable("2")})
		}
		sd, _ := pkcs7.NewSignedData(content)
		sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
		if err := sd.AddSigner(s.ca.cert, s.ca.key, pkcs7.SignerInfoConfig{ExtraSignedAttributes: attrs}); err != nil {
			s.t.Fatal(err)
		}
		out, _ := sd.Finish()
		w.Header().Set("Content-Type", ctSCEPPKI)
		w.Write(out)
	}
	var csr *x509.CertificateRequest
	switch msgType {
	case SCEPMsgPKCSReq:
		csr, err = x509.ParseCertificateRequest(plain)
		if err != nil {
			http.Error(w, "csr: "+err.Error(), 400)
			return
		}
		if pw, _ := ChallengePasswordOf(plain); pw != s.challenge {
			reply(SCEPStatusFailure, []byte{})
			return
		}
		if s.pendFirst && n == 1 {
			reply(SCEPStatusPending, []byte{})
			return
		}
	case SCEPMsgGetCertInitial:
		var ias issuerAndSubject
		if _, err := asn1.Unmarshal(plain, &ias); err != nil {
			http.Error(w, "issuerAndSubject: "+err.Error(), 400)
			return
		}
		// Issue for the signer's key (the self-signed request cert carries the subject).
		csr = &x509.CertificateRequest{Subject: signer.Subject, PublicKey: signer.PublicKey}
	default:
		http.Error(w, "unexpected messageType "+msgType, 400)
		return
	}
	var cert *x509.Certificate
	if msgType == SCEPMsgPKCSReq {
		cert = s.ca.sign(s.t, csr)
	} else {
		cert = s.ca.sign(s.t, &x509.CertificateRequest{Subject: csr.Subject, PublicKey: csr.PublicKey, Raw: nil, Signature: nil})
	}
	degenerate, err := pkcs7.DegenerateCertificate(cert.Raw)
	if err != nil {
		s.t.Fatal(err)
	}
	enc, err := pkcs7.Encrypt(degenerate, []*x509.Certificate{signer})
	if err != nil {
		s.t.Fatal(err)
	}
	reply(SCEPStatusSuccess, enc)
}

func newSCEPClient(t *testing.T, url, password string) *SCEPClient {
	t.Helper()
	c, err := NewClient(Config{Profile: &model.CAProfile{Protocol: model.EnrollSCEP, URL: url}, Password: password, Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	sc := c.(*SCEPClient)
	sc.PollInterval, sc.PollAttempts = 10*time.Millisecond, 3
	return sc
}

func TestSCEPEnroll(t *testing.T) {
	ca := newTestCA(t)
	srv := &scepServer{t: t, ca: ca, challenge: "secret-challenge"}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()

	client := newSCEPClient(t, ts.URL, "secret-challenge")
	cas, err := client.CACerts(context.Background())
	if err != nil || len(cas) != 1 || !cas[0].Equal(ca.cert) {
		t.Fatalf("cacerts: %v %v", cas, err)
	}
	key, _ := GenerateKey(KeyRSA2048)
	csr, _ := NewCSR(key, "printer1.example.test", []string{"printer1.example.test", "192.0.2.10"})
	res, err := client.Enroll(context.Background(), csr, key)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cert.Subject.CommonName != "printer1.example.test" || len(res.Cert.IPAddresses) != 1 {
		t.Errorf("issued = %v", res.Cert.Subject)
	}
	if err := res.Cert.CheckSignatureFrom(ca.cert); err != nil {
		t.Errorf("not signed by the CA: %v", err)
	}
	if leaf, _ := leafFor([]*x509.Certificate{res.Cert}, key); leaf == nil {
		t.Error("issued certificate does not match our key")
	}
}

func TestSCEPPendingThenGetCertInitial(t *testing.T) {
	ca := newTestCA(t)
	srv := &scepServer{t: t, ca: ca, challenge: "", pendFirst: true}
	ts := httptest.NewServer(srv.handler())
	defer ts.Close()
	client := newSCEPClient(t, ts.URL, "")
	key, _ := GenerateKey(KeyRSA2048)
	csr, _ := NewCSR(key, "printer2.example.test", nil)
	res, err := client.Enroll(context.Background(), csr, key)
	if err != nil {
		t.Fatal(err)
	}
	if srv.requests.Load() < 2 {
		t.Errorf("expected a GetCertInitial poll after PENDING, got %d requests", srv.requests.Load())
	}
	if res.Cert.Subject.CommonName != "printer2.example.test" {
		t.Errorf("issued = %v", res.Cert.Subject)
	}
}

func TestSCEPRejectsWrongChallengeAndNonRSA(t *testing.T) {
	ca := newTestCA(t)
	ts := httptest.NewServer((&scepServer{t: t, ca: ca, challenge: "right"}).handler())
	defer ts.Close()
	client := newSCEPClient(t, ts.URL, "wrong")
	key, _ := GenerateKey(KeyRSA2048)
	csr, _ := NewCSR(key, "p.example.test", nil)
	if _, err := client.Enroll(context.Background(), csr, key); err == nil {
		t.Error("wrong challenge must fail")
	}
	ec, _ := GenerateKey(KeyECDSAP256)
	csr, _ = NewCSR(ec, "p.example.test", nil)
	if _, err := client.Enroll(context.Background(), csr, ec); err == nil {
		t.Error("ECDSA must be refused for SCEP")
	}
}

func TestChallengePasswordAttribute(t *testing.T) {
	key := mustKey(t)
	der, _ := NewCSR(key, "x.example.test", []string{"x.example.test"})
	csr, _ := x509.ParseCertificateRequest(der)
	out, err := addChallengePassword(csr, key, "pw-123")
	if err != nil {
		t.Fatal(err)
	}
	pw, err := ChallengePasswordOf(out)
	if err != nil || pw != "pw-123" {
		t.Fatalf("challenge = %q %v", pw, err)
	}
	re, err := x509.ParseCertificateRequest(out)
	if err != nil || re.CheckSignature() != nil || len(re.DNSNames) != 1 {
		t.Fatalf("rebuilt CSR lost its extensions: %v %+v", err, re)
	}
	if got, _ := ChallengePasswordOf(der); got != "" {
		t.Error("plain CSR has no challenge password")
	}
}
