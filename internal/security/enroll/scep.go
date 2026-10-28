package enroll

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/smallstep/pkcs7"
)

// SCEP (RFC 8894) attribute OIDs (§3.2.1) and message values.
var (
	oidSCEPMessageType    = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 2}
	oidSCEPPKIStatus      = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 3}
	oidSCEPFailInfo       = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 4}
	oidSCEPSenderNonce    = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 5}
	oidSCEPRecipientNonce = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 6}
	oidSCEPTransactionID  = asn1.ObjectIdentifier{2, 16, 840, 1, 113733, 1, 9, 7}
	oidChallengePassword  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 7} // PKCS#9 challengePassword
)

// SCEP messageType values (RFC 8894 §3.2.1.2) and pkiStatus values (§3.2.1.3).
const (
	SCEPMsgCertRep        = "3"
	SCEPMsgPKCSReq        = "19"
	SCEPMsgGetCertInitial = "20"

	SCEPStatusSuccess = "0"
	SCEPStatusFailure = "2"
	SCEPStatusPending = "3"
)

// SCEP failInfo values (RFC 8894 §3.2.1.4).
var scepFailInfo = map[string]string{"0": "badAlg", "1": "badMessageCheck", "2": "badRequest", "3": "badTime", "4": "badCertId"}

// Content types (RFC 8894 §4).
const (
	ctSCEPCACert   = "application/x-x509-ca-cert"
	ctSCEPCARACert = "application/x-x509-ca-ra-cert"
	ctSCEPPKI      = "application/x-pki-message"
)

// SCEPClient enrolls with a SCEP server (RFC 8894). SCEP is RSA-only: the
// request is signed with a self-signed certificate for the new key and the
// CA's reply is encrypted to it (§2.3, §3.1).
type SCEPClient struct {
	cfg Config
	// PollInterval/PollAttempts bound GetCertInitial polling of a pending
	// request (§3.3.2); tests shorten them.
	PollInterval time.Duration
	PollAttempts int
}

var encryptMu sync.Mutex

// CACerts implements Client with GetCACert (RFC 8894 §4.2).
func (c *SCEPClient) CACerts(ctx context.Context) ([]*x509.Certificate, error) {
	resp, body, err := c.get(ctx, "GetCACert", c.cfg.Profile.Options["caIdent"])
	if err != nil {
		return nil, err
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	switch ct {
	case ctSCEPCACert:
		cert, err := x509.ParseCertificate(body)
		if err != nil {
			return nil, fmt.Errorf("scep GetCACert: %w", err)
		}
		return []*x509.Certificate{cert}, nil
	case ctSCEPCARACert, "application/x-pkcs7-certificates", "application/pkcs7-mime":
		p7, err := pkcs7.Parse(body)
		if err != nil {
			return nil, fmt.Errorf("scep GetCACert: %w", err)
		}
		return p7.Certificates, nil
	}
	// Some servers omit the content type; try both.
	if cert, err := x509.ParseCertificate(body); err == nil {
		return []*x509.Certificate{cert}, nil
	}
	if p7, err := pkcs7.Parse(body); err == nil {
		return p7.Certificates, nil
	}
	return nil, fmt.Errorf("scep GetCACert: unexpected content type %q", ct)
}

// CACaps fetches the server capabilities (RFC 8894 §3.5.2).
func (c *SCEPClient) CACaps(ctx context.Context) (map[string]bool, error) {
	_, body, err := c.get(ctx, "GetCACaps", c.cfg.Profile.Options["caIdent"])
	if err != nil {
		return nil, err
	}
	caps := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			caps[l] = true
		}
	}
	return caps, nil
}

// Enroll implements Client: PKCSReq (§3.3.1) and, while the CA answers
// PENDING, GetCertInitial polling (§3.3.2).
func (c *SCEPClient) Enroll(ctx context.Context, csrDER []byte, key crypto.Signer) (*Result, error) {
	if _, ok := key.(*rsa.PrivateKey); !ok {
		return nil, errors.New("scep: the key must be RSA (RFC 8894 §2.3)")
	}
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	caCerts, err := c.CACerts(ctx)
	if err != nil {
		return nil, err
	}
	if len(caCerts) == 0 {
		return nil, errors.New("scep: GetCACert returned no certificate")
	}
	caps, _ := c.CACaps(ctx) // optional; a failure means "no capabilities"
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, err
	}
	if c.cfg.Password != "" {
		// The challenge password travels as a PKCS#9 attribute of the CSR (§3.3.1).
		csrDER, err = addChallengePassword(csr, key, c.cfg.Password)
		if err != nil {
			return nil, err
		}
	}
	signer, err := selfSignedCert(key, csr.Subject)
	if err != nil {
		return nil, err
	}
	// Recipients: every RSA CA/RA certificate (an RA may hold the encryption key).
	var recipients []*x509.Certificate
	for _, ca := range caCerts {
		if _, ok := ca.PublicKey.(*rsa.PublicKey); ok {
			recipients = append(recipients, ca)
		}
	}
	if len(recipients) == 0 {
		return nil, errors.New("scep: the CA has no RSA certificate to encrypt to")
	}
	txID := transactionID(key)
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	msg, err := c.pkiMessage(csrDER, SCEPMsgPKCSReq, txID, nonce, signer, key, recipients, caps)
	if err != nil {
		return nil, err
	}
	res, status, err := c.pkiOperation(ctx, msg, txID, nonce, signer, key, caCerts, caps)
	if err != nil {
		return nil, err
	}
	interval, attempts := c.PollInterval, c.PollAttempts
	if interval == 0 {
		interval = 5 * time.Second
	}
	if attempts == 0 {
		attempts = 12
	}
	for i := 0; status == SCEPStatusPending && i < attempts; i++ {
		c.cfg.Notify("scep: request pending at the CA; polling with GetCertInitial")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		ias, err := asn1.Marshal(issuerAndSubject{Issuer: asn1.RawValue{FullBytes: caCerts[0].RawSubject}, Subject: asn1.RawValue{FullBytes: csr.RawSubject}})
		if err != nil {
			return nil, err
		}
		_, _ = rand.Read(nonce)
		msg, err := c.pkiMessage(ias, SCEPMsgGetCertInitial, txID, nonce, signer, key, recipients, caps)
		if err != nil {
			return nil, err
		}
		res, status, err = c.pkiOperation(ctx, msg, txID, nonce, signer, key, caCerts, caps)
		if err != nil {
			return nil, err
		}
	}
	if status == SCEPStatusPending {
		return nil, errors.New("scep: the CA left the request pending (manual approval needed); retry the enrollment later")
	}
	return res, nil
}

// pkiMessage builds a SCEP pkiMessage: content enveloped to the CA (§3.1
// pkcsPKIEnvelope) inside SignedData carrying the SCEP attributes (§3.2.1).
func (c *SCEPClient) pkiMessage(content []byte, msgType, txID string, nonce []byte, signer *x509.Certificate, key crypto.Signer, recipients []*x509.Certificate, caps map[string]bool) ([]byte, error) {
	encryptMu.Lock()
	// AES-256-CBC when the CA advertises AES (RFC 8894 §3.5.2 GetCACaps), else
	// the library's DES-CBC fallback for legacy servers (it cannot encrypt 3DES).
	prev := pkcs7.ContentEncryptionAlgorithm
	if caps["AES"] {
		pkcs7.ContentEncryptionAlgorithm = pkcs7.EncryptionAlgorithmAES256CBC
	}
	enveloped, err := pkcs7.Encrypt(content, recipients)
	pkcs7.ContentEncryptionAlgorithm = prev
	encryptMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("scep: envelope: %w", err)
	}
	sd, err := pkcs7.NewSignedData(enveloped)
	if err != nil {
		return nil, err
	}
	if caps["SHA-256"] || len(caps) == 0 {
		sd.SetDigestAlgorithm(pkcs7.OIDDigestAlgorithmSHA256)
	}
	attrs := []pkcs7.Attribute{
		{Type: oidSCEPTransactionID, Value: printable(txID)},
		{Type: oidSCEPMessageType, Value: printable(msgType)},
		{Type: oidSCEPSenderNonce, Value: nonce},
	}
	if err := sd.AddSigner(signer, key, pkcs7.SignerInfoConfig{ExtraSignedAttributes: attrs}); err != nil {
		return nil, err
	}
	return sd.Finish()
}

// pkiOperation sends a pkiMessage and parses the CertRep (§3.3.2, §4.3).
func (c *SCEPClient) pkiOperation(ctx context.Context, msg []byte, txID string, nonce []byte, signer *x509.Certificate, key crypto.Signer, caCerts []*x509.Certificate, caps map[string]bool) (*Result, string, error) {
	var resp *http.Response
	var body []byte
	var err error
	if caps["POSTPKIOperation"] {
		resp, body, err = c.post(ctx, "PKIOperation", msg)
	} else {
		resp, body, err = c.get(ctx, "PKIOperation", base64.StdEncoding.EncodeToString(msg))
	}
	if err != nil {
		return nil, "", err
	}
	if ct := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0]); strings.TrimSpace(ct) != ctSCEPPKI {
		return nil, "", fmt.Errorf("scep PKIOperation: unexpected content type %q: %s", ct, truncate(body))
	}
	p7, err := pkcs7.Parse(body)
	if err != nil {
		return nil, "", fmt.Errorf("scep CertRep: %w", err)
	}
	// The reply is signed by the CA or RA; make sure its certificate is known
	// and the signature verifies against the CA certificates we fetched.
	pool := x509.NewCertPool()
	for _, ca := range caCerts {
		pool.AddCert(ca)
		if !containsCert(p7.Certificates, ca) {
			p7.Certificates = append(p7.Certificates, ca)
		}
	}
	if err := p7.VerifyWithChain(pool); err != nil {
		if err := p7.Verify(); err != nil {
			return nil, "", fmt.Errorf("scep CertRep: signature: %w", err)
		}
		if s := p7.GetOnlySigner(); s == nil || !containsCert(caCerts, s) {
			return nil, "", errors.New("scep CertRep: not signed by the CA/RA")
		}
	}
	var msgType, status, gotTx string
	var rNonce []byte
	_ = p7.UnmarshalSignedAttribute(oidSCEPMessageType, &msgType)
	_ = p7.UnmarshalSignedAttribute(oidSCEPPKIStatus, &status)
	_ = p7.UnmarshalSignedAttribute(oidSCEPTransactionID, &gotTx)
	_ = p7.UnmarshalSignedAttribute(oidSCEPRecipientNonce, &rNonce)
	if msgType != SCEPMsgCertRep {
		return nil, "", fmt.Errorf("scep: expected CertRep, got messageType %q", msgType)
	}
	if gotTx != txID {
		return nil, "", errors.New("scep CertRep: transactionID mismatch")
	}
	if !bytes.Equal(rNonce, nonce) {
		return nil, "", errors.New("scep CertRep: recipientNonce does not match our senderNonce")
	}
	switch status {
	case SCEPStatusPending:
		return nil, status, nil
	case SCEPStatusFailure:
		var fi string
		_ = p7.UnmarshalSignedAttribute(oidSCEPFailInfo, &fi)
		if name, ok := scepFailInfo[fi]; ok {
			fi = name
		}
		return nil, status, fmt.Errorf("scep: the CA rejected the request (failInfo %s)", nonEmpty(fi, "unknown"))
	case SCEPStatusSuccess:
	default:
		return nil, status, fmt.Errorf("scep: unknown pkiStatus %q", status)
	}
	env, err := pkcs7.Parse(p7.Content)
	if err != nil {
		return nil, status, fmt.Errorf("scep CertRep envelope: %w", err)
	}
	plain, err := env.Decrypt(signer, key)
	if err != nil {
		return nil, status, fmt.Errorf("scep CertRep: decrypt: %w", err)
	}
	certs, err := pkcs7.Parse(plain)
	if err != nil {
		return nil, status, fmt.Errorf("scep CertRep: certificates: %w", err)
	}
	leaf, rest := leafFor(certs.Certificates, key)
	if leaf == nil {
		return nil, status, errors.New("scep CertRep: no certificate for our key in the reply")
	}
	return &Result{Cert: leaf, Chain: rest}, status, nil
}

func (c *SCEPClient) get(ctx context.Context, op, message string) (*http.Response, []byte, error) {
	u, err := url.Parse(c.cfg.Profile.URL)
	if err != nil {
		return nil, nil, err
	}
	q := u.Query()
	q.Set("operation", op)
	if message != "" {
		q.Set("message", message)
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	return c.do(req)
}

func (c *SCEPClient) post(ctx context.Context, op string, body []byte) (*http.Response, []byte, error) {
	u, err := url.Parse(c.cfg.Profile.URL)
	if err != nil {
		return nil, nil, err
	}
	q := u.Query()
	q.Set("operation", op)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", ctSCEPPKI)
	return c.do(req)
}

func (c *SCEPClient) do(req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("scep %s: %w", req.URL.Query().Get("operation"), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("scep %s: HTTP %d: %s", req.URL.Query().Get("operation"), resp.StatusCode, truncate(body))
	}
	return resp, body, nil
}

// transactionID is a hash of the public key (RFC 8894 §3.2.1.1).
func transactionID(key crypto.Signer) string {
	der, _ := x509.MarshalPKIXPublicKey(key.Public())
	sum := sha256.Sum256(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func printable(s string) asn1.RawValue {
	return asn1.RawValue{Tag: asn1.TagPrintableString, Bytes: []byte(s)}
}

// issuerAndSubject is the GetCertInitial content (RFC 8894 §3.2.3).
type issuerAndSubject struct {
	Issuer  asn1.RawValue
	Subject asn1.RawValue
}

// PKCS#10 structures (RFC 2986 §4) used to add attributes to a CSR.
type certificationRequestInfo struct {
	Version    int
	Subject    asn1.RawValue
	PublicKey  asn1.RawValue
	Attributes []asn1.RawValue `asn1:"tag:0,set,optional"`
}

type certificationRequest struct {
	Info      certificationRequestInfo
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

type csrAttribute struct {
	Type   asn1.ObjectIdentifier
	Values []asn1.RawValue `asn1:"set"`
}

var oidSHA256WithRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}

// addChallengePassword rebuilds the CSR with a PKCS#9 challengePassword
// attribute (RFC 2985 §5.4.1; SCEP's PKCSReq, RFC 8894 §3.3.1) and re-signs
// it. crypto/x509 cannot emit this attribute shape itself, so the
// CertificationRequestInfo is re-encoded here and signed with
// sha256WithRSAEncryption.
func addChallengePassword(csr *x509.CertificateRequest, key crypto.Signer, password string) ([]byte, error) {
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("scep: challenge password needs an RSA key")
	}
	var req certificationRequest
	if rest, err := asn1.Unmarshal(csr.Raw, &req); err != nil {
		return nil, fmt.Errorf("scep: parse CSR: %w", err)
	} else if len(rest) > 0 {
		return nil, errors.New("scep: trailing data after CSR")
	}
	attr, err := asn1.Marshal(csrAttribute{Type: oidChallengePassword, Values: []asn1.RawValue{printable(password)}})
	if err != nil {
		return nil, err
	}
	req.Info.Attributes = append(req.Info.Attributes, asn1.RawValue{FullBytes: attr})
	info, err := asn1.Marshal(req.Info)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(info)
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	if err != nil {
		return nil, err
	}
	out, err := asn1.Marshal(certificationRequest{
		Info:      req.Info,
		Algorithm: pkix.AlgorithmIdentifier{Algorithm: oidSHA256WithRSA, Parameters: asn1.NullRawValue},
		Signature: asn1.BitString{Bytes: sig, BitLength: len(sig) * 8},
	})
	if err != nil {
		return nil, err
	}
	// Round-trip through the standard parser so a malformed request never leaves.
	parsed, err := x509.ParseCertificateRequest(out)
	if err != nil {
		return nil, fmt.Errorf("scep: rebuilt CSR: %w", err)
	}
	if err := parsed.CheckSignature(); err != nil {
		return nil, fmt.Errorf("scep: rebuilt CSR signature: %w", err)
	}
	return out, nil
}

// ChallengePasswordOf extracts the PKCS#9 challengePassword of a CSR (test
// servers and RAs use it to authorise a request).
func ChallengePasswordOf(csrDER []byte) (string, error) {
	var req certificationRequest
	if _, err := asn1.Unmarshal(csrDER, &req); err != nil {
		return "", err
	}
	for _, raw := range req.Info.Attributes {
		var a csrAttribute
		if _, err := asn1.Unmarshal(raw.FullBytes, &a); err != nil || !a.Type.Equal(oidChallengePassword) {
			continue
		}
		if len(a.Values) > 0 {
			return string(a.Values[0].Bytes), nil
		}
	}
	return "", nil
}

func containsCert(list []*x509.Certificate, c *x509.Certificate) bool {
	for _, x := range list {
		if bytes.Equal(x.Raw, c.Raw) {
			return true
		}
	}
	return false
}

func truncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
