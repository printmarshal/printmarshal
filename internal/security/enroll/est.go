package enroll

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smallstep/pkcs7"
)

// EST content types (RFC 7030 §4.1.3, §4.2.1).
const (
	ctPKCS7MIME = "application/pkcs7-mime"
	ctPKCS10    = "application/pkcs10"
)

// ESTClient enrolls with an EST server (RFC 7030) over HTTPS: /cacerts
// (§4.1) and /simpleenroll (§4.2). The profile URL is the server base, e.g.
// https://ca.example.com/.well-known/est; Options["label"] appends an
// optional CA label (§3.2.2). Client authentication is HTTP Basic (§3.2.3)
// with the profile credential's username/password; TLS client-certificate
// authentication (§3.3.2) is not implemented.
type ESTClient struct {
	cfg Config
	// PollInterval/PollAttempts bound the 202 "retry later" loop (§4.2.3).
	PollInterval time.Duration
	PollAttempts int
}

func (c *ESTClient) base() string {
	u := strings.TrimRight(c.cfg.Profile.URL, "/")
	if label := strings.Trim(c.cfg.Profile.Options["label"], "/"); label != "" {
		u += "/" + label
	}
	return u
}

// CACerts implements Client with GET /cacerts (RFC 7030 §4.1.2).
func (c *ESTClient) CACerts(ctx context.Context) ([]*x509.Certificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/cacerts", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", ctPKCS7MIME)
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("est cacerts: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("est cacerts: HTTP %d: %s", resp.StatusCode, truncate(body))
	}
	return parsePKCS7Certs(body)
}

// Enroll implements Client with POST /simpleenroll (RFC 7030 §4.2.1): the
// body is the base64 DER CSR, the reply a base64 certs-only PKCS#7.
func (c *ESTClient) Enroll(ctx context.Context, csrDER []byte, key crypto.Signer) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	chain, _ := c.CACerts(ctx) // best effort: the CA chain is returned alongside the leaf
	interval, attempts := c.PollInterval, c.PollAttempts
	if interval == 0 {
		interval = 5 * time.Second
	}
	if attempts == 0 {
		attempts = 12
	}
	for i := 0; i < attempts; i++ {
		certs, retry, err := c.simpleEnroll(ctx, csrDER)
		if err != nil {
			return nil, err
		}
		if retry > 0 {
			c.cfg.Notify(fmt.Sprintf("est: request pending at the CA; retrying in %s", retry))
			if retry > interval*4 {
				retry = interval * 4
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retry):
			}
			continue
		}
		leaf, rest := leafFor(certs, key)
		if leaf == nil {
			return nil, errors.New("est simpleenroll: no certificate for our key in the reply")
		}
		for _, ca := range chain {
			if !containsCert(rest, ca) && !bytes.Equal(ca.Raw, leaf.Raw) {
				rest = append(rest, ca)
			}
		}
		return &Result{Cert: leaf, Chain: rest}, nil
	}
	return nil, errors.New("est: the CA left the request pending (manual approval needed); retry the enrollment later")
}

// simpleEnroll performs one POST; retry > 0 means HTTP 202 with Retry-After.
func (c *ESTClient) simpleEnroll(ctx context.Context, csrDER []byte) ([]*x509.Certificate, time.Duration, error) {
	body := base64.StdEncoding.EncodeToString(csrDER)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/simpleenroll", strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", ctPKCS10)
	req.Header.Set("Content-Transfer-Encoding", "base64")
	req.Header.Set("Accept", ctPKCS7MIME)
	if c.cfg.Username != "" || c.cfg.Password != "" {
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("est simpleenroll: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		certs, err := parsePKCS7Certs(raw)
		return certs, 0, err
	case http.StatusAccepted:
		retry := 5 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if n, err := strconv.Atoi(ra); err == nil && n > 0 {
				retry = time.Duration(n) * time.Second
			} else if t, err := http.ParseTime(ra); err == nil {
				retry = time.Until(t)
			}
		}
		if retry <= 0 {
			retry = time.Second
		}
		return nil, retry, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, 0, fmt.Errorf("est simpleenroll: HTTP %d: the CA rejected the credentials", resp.StatusCode)
	}
	return nil, 0, fmt.Errorf("est simpleenroll: HTTP %d: %s", resp.StatusCode, truncate(raw))
}

// parsePKCS7Certs decodes a base64 (or raw DER) certs-only PKCS#7.
func parsePKCS7Certs(body []byte) ([]*x509.Certificate, error) {
	der := body
	trimmed := bytes.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, body)
	if dec, err := base64.StdEncoding.DecodeString(string(trimmed)); err == nil {
		der = dec
	}
	p7, err := pkcs7.Parse(der)
	if err != nil {
		return nil, fmt.Errorf("pkcs7: %w", err)
	}
	return p7.Certificates, nil
}

// CertsOnlyPKCS7 encodes certificates as a degenerate PKCS#7 SignedData
// (RFC 7030 §4.1.3 / RFC 8894 §3.4), base64 as EST transports it.
func CertsOnlyPKCS7(certs ...*x509.Certificate) (string, error) {
	var der []byte
	for _, c := range certs {
		der = append(der, c.Raw...)
	}
	p7, err := pkcs7.DegenerateCertificate(der)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(p7), nil
}
