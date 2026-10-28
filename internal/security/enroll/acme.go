package enroll

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

// DNSProvider publishes the TXT record an ACME dns-01 challenge needs
// (RFC 8555 §8.4). Printers cannot answer http-01 or tls-alpn-01 (ports 80
// and 443 are the printer's own), so dns-01 is the only workable method.
type DNSProvider interface {
	// Present makes "<fqdn> IN TXT <value>" resolvable, or returns an error
	// explaining what the operator must do.
	Present(ctx context.Context, fqdn, value string) error
	// CleanUp removes the record once validation is over.
	CleanUp(ctx context.Context, fqdn, value string) error
}

// ErrDNSRecordRequired is returned by the Manual provider when it cannot
// wait for the record: the enrollment stops and the message tells the
// operator exactly which record to create before retrying.
type ErrDNSRecordRequired struct {
	FQDN  string
	Value string
}

func (e *ErrDNSRecordRequired) Error() string {
	return fmt.Sprintf("acme dns-01: create the DNS record %q IN TXT %q, then run the enrollment again", e.FQDN, e.Value)
}

// Manual is the DNS provider for operators who edit DNS by hand. It reports
// the record to create and, when Wait > 0, polls DNS until the record is
// visible (or Wait elapses) before letting the CA validate. With Wait == 0
// it returns ErrDNSRecordRequired immediately; the record value is
// deterministic for a given account key and challenge token, so a retry
// after creating the record succeeds only if the CA reuses the token —
// most CAs issue a new token per order, so prefer a non-zero Wait.
type Manual struct {
	Wait   time.Duration
	Notify func(msg string)
	// Lookup resolves TXT records (net.LookupTXT by default; tests stub it).
	Lookup func(ctx context.Context, name string) ([]string, error)
}

// Present implements DNSProvider.
func (m *Manual) Present(ctx context.Context, fqdn, value string) error {
	if m.Notify != nil {
		m.Notify(fmt.Sprintf("acme dns-01: create the DNS record %q IN TXT %q", fqdn, value))
	}
	if m.Wait <= 0 {
		return &ErrDNSRecordRequired{FQDN: fqdn, Value: value}
	}
	lookup := m.Lookup
	if lookup == nil {
		lookup = net.DefaultResolver.LookupTXT
	}
	deadline := time.Now().Add(m.Wait)
	for {
		if txts, err := lookup(ctx, fqdn); err == nil {
			for _, t := range txts {
				if t == value {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("acme dns-01: the record %q IN TXT %q did not appear within %s", fqdn, value, m.Wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

// CleanUp implements DNSProvider: nothing to do for a manual record beyond
// telling the operator it can go.
func (m *Manual) CleanUp(_ context.Context, fqdn, _ string) error {
	if m.Notify != nil {
		m.Notify(fmt.Sprintf("acme dns-01: the TXT record %q can be removed", fqdn))
	}
	return nil
}

// ACMEClient enrolls with an ACME CA (RFC 8555) using dns-01 challenges.
// The profile URL is the directory URL. The account key comes from the
// profile credential's PEM; without one a fresh key (and account) is
// created per enrollment, which works but gives the CA no stable identity.
// Only DNS names can be ordered: IP identifiers (RFC 8738) need http-01 or
// tls-alpn-01, which a printer cannot serve, so IP SANs are refused here.
type ACMEClient struct {
	cfg Config
}

// CACerts implements Client. ACME has no CA-certificate operation: the chain
// arrives with every issued certificate, so this returns nothing.
func (c *ACMEClient) CACerts(_ context.Context) ([]*x509.Certificate, error) {
	return nil, nil
}

// Enroll implements Client: newOrder → dns-01 per authorization → finalize
// with the CSR → download the chain (RFC 8555 §7.4).
func (c *ACMEClient) Enroll(ctx context.Context, csrDER []byte, _ crypto.Signer) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		return nil, err
	}
	if len(csr.IPAddresses) > 0 {
		return nil, errors.New("acme: IP address identifiers cannot be validated with dns-01 (and a printer cannot serve http-01/tls-alpn-01); request DNS names only")
	}
	names := append([]string(nil), csr.DNSNames...)
	if cn := csr.Subject.CommonName; cn != "" && !contains(names, cn) {
		if net.ParseIP(cn) != nil {
			return nil, errors.New("acme: the common name must be a DNS name, not an IP address")
		}
		names = append([]string{cn}, names...)
	}
	if len(names) == 0 {
		return nil, errors.New("acme: at least one DNS name is required")
	}
	accountKey, err := c.accountKey()
	if err != nil {
		return nil, err
	}
	client := &acme.Client{Key: accountKey, DirectoryURL: c.cfg.Profile.URL, HTTPClient: c.cfg.HTTPClient, UserAgent: "printmarshal"}
	acct := &acme.Account{}
	if email := strings.TrimSpace(c.cfg.Profile.Options["email"]); email != "" {
		acct.Contact = []string{"mailto:" + email}
	}
	if _, err := client.Register(ctx, acct, acme.AcceptTOS); err != nil && !errors.Is(err, acme.ErrAccountAlreadyExists) {
		return nil, fmt.Errorf("acme register: %w", err)
	}
	ids := make([]acme.AuthzID, 0, len(names))
	for _, n := range names {
		ids = append(ids, acme.AuthzID{Type: "dns", Value: n})
	}
	order, err := client.AuthorizeOrder(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("acme new-order: %w", err)
	}
	dns := c.cfg.DNS
	if dns == nil {
		dns = &Manual{Notify: c.cfg.Notify}
	}
	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			return nil, fmt.Errorf("acme authorization: %w", err)
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, ch := range authz.Challenges {
			if ch.Type == "dns-01" {
				chal = ch
				break
			}
		}
		if chal == nil {
			return nil, fmt.Errorf("acme: the CA offers no dns-01 challenge for %s", authz.Identifier.Value)
		}
		value, err := client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			return nil, err
		}
		fqdn := "_acme-challenge." + strings.TrimSuffix(authz.Identifier.Value, ".")
		if err := dns.Present(ctx, fqdn, value); err != nil {
			return nil, err
		}
		if _, err := client.Accept(ctx, chal); err != nil {
			_ = dns.CleanUp(ctx, fqdn, value)
			return nil, fmt.Errorf("acme accept challenge: %w", err)
		}
		_, err = client.WaitAuthorization(ctx, authz.URI)
		_ = dns.CleanUp(ctx, fqdn, value)
		if err != nil {
			return nil, fmt.Errorf("acme validation of %s: %w", authz.Identifier.Value, err)
		}
	}
	if _, err := client.WaitOrder(ctx, order.URI); err != nil {
		return nil, fmt.Errorf("acme order: %w", err)
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csrDER, true)
	if err != nil {
		return nil, fmt.Errorf("acme finalize: %w", err)
	}
	var certs []*x509.Certificate
	for _, d := range der {
		cert, err := x509.ParseCertificate(d)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("acme: the CA returned no certificate")
	}
	return &Result{Cert: certs[0], Chain: certs[1:]}, nil
}

func (c *ACMEClient) accountKey() (crypto.Signer, error) {
	if c.cfg.AccountKeyPEM != "" {
		return ParseKeyPEM(c.cfg.AccountKeyPEM)
	}
	c.cfg.Notify("acme: no account key credential configured; using a fresh account for this enrollment")
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
