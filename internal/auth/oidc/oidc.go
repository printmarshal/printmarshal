// Package oidc is a dependency-free OpenID Connect relying party for the
// authorization-code flow with PKCE (docs/auth.md, audit A10): discovery,
// JWKS caching, ID-token verification (RS256 / ES256 with the crypto stdlib)
// and the claim helpers the API layer needs to provision a local user.
package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/model"
)

// Config is what the relying party needs. ClientSecret is optional (public
// clients rely on PKCE alone).
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string // besides "openid"; default profile, email
	HTTPClient   *http.Client
}

// Provider is a discovered issuer with a cached key set.
type Provider struct {
	cfg  Config
	http *http.Client
	meta metadata

	mu        sync.Mutex
	keys      map[string]crypto.PublicKey
	keysAt    time.Time
	lastFetch time.Time
}

type metadata struct {
	Issuer        string `json:"issuer"`
	AuthEndpoint  string `json:"authorization_endpoint"`
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
}

const (
	keyTTL        = time.Hour   // refresh JWKS at least this often
	refetchNoMore = time.Minute // unknown kid: refetch at most once a minute
	leeway        = time.Minute // clock skew tolerated on exp / iat
	maxBody       = 1 << 20
)

// Discover fetches <issuer>/.well-known/openid-configuration and validates it.
func Discover(ctx context.Context, cfg Config) (*Provider, error) {
	if err := checkIssuer(cfg.Issuer); err != nil {
		return nil, err
	}
	if cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, errors.New("oidc: clientId and redirectUrl are required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	p := &Provider{cfg: cfg, http: client}
	u := strings.TrimSuffix(cfg.Issuer, "/") + "/.well-known/openid-configuration"
	if err := p.getJSON(ctx, u, &p.meta); err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	if strings.TrimSuffix(p.meta.Issuer, "/") != strings.TrimSuffix(cfg.Issuer, "/") {
		return nil, fmt.Errorf("oidc discovery: issuer mismatch (%q vs configured %q)", p.meta.Issuer, cfg.Issuer)
	}
	if p.meta.AuthEndpoint == "" || p.meta.TokenEndpoint == "" || p.meta.JWKSURI == "" {
		return nil, errors.New("oidc discovery: document lacks authorization_endpoint, token_endpoint or jwks_uri")
	}
	for _, ep := range []string{p.meta.AuthEndpoint, p.meta.TokenEndpoint, p.meta.JWKSURI} {
		if err := checkIssuer(ep); err != nil {
			return nil, fmt.Errorf("oidc discovery: %w", err)
		}
	}
	return p, nil
}

// checkIssuer insists on https except for loopback (development, tests).
func checkIssuer(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("oidc: %q is not an absolute URL", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || host == "localhost" {
			return nil
		}
		return fmt.Errorf("oidc: %q must use https (plain http is only allowed on loopback)", raw)
	}
	return fmt.Errorf("oidc: unsupported scheme in %q", raw)
}

// Issuer returns the discovered issuer identifier.
func (p *Provider) Issuer() string { return p.meta.Issuer }

// AuthURL builds the authorization request for the browser redirect.
func (p *Provider) AuthURL(state, nonce, verifier string) string {
	scopes := append([]string{"openid"}, p.cfg.Scopes...)
	if len(p.cfg.Scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {p.cfg.ClientID},
		"redirect_uri":          {p.cfg.RedirectURL},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {Challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	sep := "?"
	if strings.Contains(p.meta.AuthEndpoint, "?") {
		sep = "&"
	}
	return p.meta.AuthEndpoint + sep + q.Encode()
}

// Claims are the verified ID-token claims.
type Claims struct {
	Subject           string
	Email             string
	EmailVerified     bool
	PreferredUsername string
	Name              string
	Raw               map[string]any
}

// Username is the local account name: preferred_username, else email, else
// the subject.
func (c *Claims) Username() string {
	for _, v := range []string{c.PreferredUsername, c.Email, c.Subject} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// Lookup returns the values at a dot path in the raw claims ("roles",
// "realm_access.roles"); a string becomes a one-element list.
func (c *Claims) Lookup(path string) []string {
	if path == "" {
		return nil
	}
	var cur any = c.Raw
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[part]
		if !ok {
			return nil
		}
	}
	switch v := cur.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// MapRole picks the highest local role that roleMap assigns to any of the
// claim values; def (or viewer) when nothing matches.
func MapRole(values []string, roleMap map[string]string, def string) model.Role {
	best := model.Role(def)
	if !best.Allows(model.RoleViewer) {
		best = model.RoleViewer
	}
	for _, v := range values {
		r := model.Role(roleMap[v])
		if r.Allows(model.RoleViewer) && r.Allows(best) {
			best = r
		}
	}
	return best
}

// Exchange redeems the authorization code and verifies the ID token,
// including the nonce bound to this sign-in.
func (p *Provider) Exchange(ctx context.Context, code, verifier, nonce string) (*Claims, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.cfg.RedirectURL},
		"client_id":     {p.cfg.ClientID},
		"code_verifier": {verifier},
	}
	if p.cfg.ClientSecret != "" {
		form.Set("client_secret", p.cfg.ClientSecret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.meta.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	var tr struct {
		IDToken   string `json:"id_token"`
		TokenType string `json:"token_type"`
		Error     string `json:"error"`
		ErrorDesc string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("oidc token response: status %d, not JSON", resp.StatusCode)
	}
	if tr.Error != "" || resp.StatusCode != http.StatusOK {
		msg := tr.Error
		if tr.ErrorDesc != "" {
			msg += ": " + tr.ErrorDesc
		}
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("oidc token endpoint: %s", msg)
	}
	if tr.IDToken == "" {
		return nil, errors.New("oidc token response carries no id_token")
	}
	return p.VerifyIDToken(ctx, tr.IDToken, nonce)
}

// VerifyIDToken checks signature (RS256 / ES256 only), issuer, audience,
// expiry and nonce, and returns the claims.
func (p *Provider) VerifyIDToken(ctx context.Context, raw, nonce string) (*Claims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("id token: not a compact JWS")
	}
	hb, err := b64(parts[0])
	if err != nil {
		return nil, errors.New("id token: bad header")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		return nil, errors.New("id token: bad header")
	}
	if hdr.Alg != "RS256" && hdr.Alg != "ES256" {
		return nil, fmt.Errorf("id token: algorithm %q not accepted (RS256 or ES256 only)", hdr.Alg)
	}
	sig, err := b64(parts[2])
	if err != nil {
		return nil, errors.New("id token: bad signature encoding")
	}
	key, err := p.key(ctx, hdr.Kid)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	switch k := key.(type) {
	case *rsa.PublicKey:
		if hdr.Alg != "RS256" {
			return nil, errors.New("id token: key/algorithm mismatch")
		}
		if err := rsa.VerifyPKCS1v15(k, crypto.SHA256, digest[:], sig); err != nil {
			return nil, errors.New("id token: signature invalid")
		}
	case *ecdsa.PublicKey:
		if hdr.Alg != "ES256" || len(sig) != 64 {
			return nil, errors.New("id token: key/algorithm mismatch")
		}
		r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(k, digest[:], r, s) {
			return nil, errors.New("id token: signature invalid")
		}
	default:
		return nil, errors.New("id token: unsupported key type")
	}
	pb, err := b64(parts[1])
	if err != nil {
		return nil, errors.New("id token: bad payload")
	}
	var c struct {
		Iss      string          `json:"iss"`
		Sub      string          `json:"sub"`
		Aud      json.RawMessage `json:"aud"`
		Azp      string          `json:"azp"`
		Exp      int64           `json:"exp"`
		Iat      int64           `json:"iat"`
		Nonce    string          `json:"nonce"`
		Email    string          `json:"email"`
		EmailVer bool            `json:"email_verified"`
		PrefUser string          `json:"preferred_username"`
		Name     string          `json:"name"`
	}
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, errors.New("id token: bad payload")
	}
	if strings.TrimSuffix(c.Iss, "/") != strings.TrimSuffix(p.meta.Issuer, "/") {
		return nil, errors.New("id token: issuer mismatch")
	}
	auds, _ := audiences(c.Aud)
	found := false
	for _, a := range auds {
		if a == p.cfg.ClientID {
			found = true
		}
	}
	if !found {
		return nil, errors.New("id token: audience mismatch")
	}
	if len(auds) > 1 && c.Azp != "" && c.Azp != p.cfg.ClientID {
		return nil, errors.New("id token: authorized party mismatch")
	}
	now := time.Now()
	if c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(leeway)) {
		return nil, errors.New("id token: expired")
	}
	if c.Iat != 0 && time.Unix(c.Iat, 0).After(now.Add(leeway)) {
		return nil, errors.New("id token: issued in the future")
	}
	if nonce == "" || c.Nonce != nonce {
		return nil, errors.New("id token: nonce mismatch")
	}
	if c.Sub == "" {
		return nil, errors.New("id token: no subject")
	}
	all := map[string]any{}
	_ = json.Unmarshal(pb, &all)
	return &Claims{Subject: c.Sub, Email: c.Email, EmailVerified: c.EmailVer, PreferredUsername: c.PrefUser, Name: c.Name, Raw: all}, nil
}

func audiences(raw json.RawMessage) ([]string, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return []string{one}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	return many, nil
}

// key returns the public key for kid, refreshing the JWKS when the cache is
// stale or the kid is unknown (key rotation), at most once a minute.
func (p *Provider) key(ctx context.Context, kid string) (crypto.PublicKey, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	stale := p.keys == nil || now.Sub(p.keysAt) > keyTTL
	if k, ok := p.keys[kid]; ok && !stale {
		return k, nil
	}
	if !stale && now.Sub(p.lastFetch) < refetchNoMore {
		return nil, fmt.Errorf("id token: unknown key id %q", kid)
	}
	p.lastFetch = now
	keys, err := p.fetchKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("oidc jwks: %w", err)
	}
	p.keys, p.keysAt = keys, now
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	if kid == "" && len(keys) == 1 {
		for _, k := range keys {
			return k, nil
		}
	}
	return nil, fmt.Errorf("id token: unknown key id %q", kid)
}

func (p *Provider) fetchKeys(ctx context.Context) (map[string]crypto.PublicKey, error) {
	var set struct {
		Keys []struct {
			Kty, Kid, Use, Alg, N, E, Crv, X, Y string
		} `json:"keys"`
	}
	if err := p.getJSON(ctx, p.meta.JWKSURI, &set); err != nil {
		return nil, err
	}
	out := map[string]crypto.PublicKey{}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		switch k.Kty {
		case "RSA":
			n, err1 := b64(k.N)
			e, err2 := b64(k.E)
			if err1 != nil || err2 != nil || len(n) == 0 || len(e) == 0 {
				continue
			}
			out[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		case "EC":
			if k.Crv != "P-256" {
				continue
			}
			x, err1 := b64(k.X)
			y, err2 := b64(k.Y)
			if err1 != nil || err2 != nil {
				continue
			}
			pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
			if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
				continue
			}
			out[k.Kid] = pub
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no usable RSA or P-256 signing keys")
	}
	return out, nil
}

func (p *Provider) getJSON(ctx context.Context, u string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(dst)
}

func b64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// Random returns a URL-safe random string for state, nonce and PKCE verifiers.
func Random() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge is the S256 PKCE code challenge of a verifier.
func Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
