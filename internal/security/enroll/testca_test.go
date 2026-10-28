package enroll

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// testCA is a throwaway certificate authority generated per test.
type testCA struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
	n    int64
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "PrintMarshal Test CA", Organization: []string{"test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &testCA{cert: cert, key: key, n: 100}
}

// sign issues a 90-day certificate for a CSR.
func (ca *testCA) sign(t *testing.T, csr *x509.CertificateRequest) *x509.Certificate {
	t.Helper()
	if csr.Raw != nil { // synthetic CSRs (SCEP GetCertInitial) carry only subject + key
		if err := csr.CheckSignature(); err != nil {
			t.Fatalf("csr signature: %v", err)
		}
	}
	ca.n++
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(ca.n),
		Subject:      csr.Subject,
		DNSNames:     csr.DNSNames,
		IPAddresses:  csr.IPAddresses,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return cert
}

func TestKeysAndCSR(t *testing.T) {
	for _, kt := range []string{"", KeyRSA2048, KeyECDSAP256} {
		key, err := GenerateKey(kt)
		if err != nil {
			t.Fatal(kt, err)
		}
		csrDER, err := NewCSR(key, "printer.example.test", []string{"printer.example.test", "10.1.2.3"})
		if err != nil {
			t.Fatal(err)
		}
		csr, err := x509.ParseCertificateRequest(csrDER)
		if err != nil || csr.CheckSignature() != nil {
			t.Fatal(err)
		}
		if csr.Subject.CommonName != "printer.example.test" || len(csr.DNSNames) != 1 || len(csr.IPAddresses) != 1 {
			t.Errorf("csr = %+v", csr.Subject)
		}
		pemKey, err := EncodeKeyPEM(key)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseKeyPEM(pemKey); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := GenerateKey("dsa"); err == nil {
		t.Error("unknown key type must fail")
	}
	if _, err := NewCSR(mustKey(t), "", nil); err == nil {
		t.Error("CSR without any name must fail")
	}
	// CN falls back to the first SAN.
	der, _ := NewCSR(mustKey(t), "", []string{"a.example.test"})
	if csr, _ := x509.ParseCertificateRequest(der); csr.Subject.CommonName != "a.example.test" {
		t.Error("CN should default to the first DNS SAN")
	}
}

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}
