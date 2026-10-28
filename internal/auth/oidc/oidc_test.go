package oidc_test

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth/oidc"
	"printmarshal/internal/auth/oidc/oidctest"
	"printmarshal/internal/model"
)

func provider(t *testing.T) (*oidctest.Issuer, *oidc.Provider) {
	t.Helper()
	is := oidctest.New(t, "pm-client")
	p, err := oidc.Discover(context.Background(), oidc.Config{Issuer: is.URL, ClientID: "pm-client", ClientSecret: "s3cret", RedirectURL: "http://127.0.0.1/api/auth/oidc/callback"})
	if err != nil {
		t.Fatal(err)
	}
	return is, p
}

func TestDiscoveryAndAuthURL(t *testing.T) {
	is, p := provider(t)
	if p.Issuer() != is.URL {
		t.Fatalf("issuer %q", p.Issuer())
	}
	raw := p.AuthURL("st", "no", "verifier-value")
	u, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, is.URL+"/authorize?") {
		t.Fatalf("auth url %q: %v", raw, err)
	}
	q := u.Query()
	if q.Get("response_type") != "code" || q.Get("client_id") != "pm-client" || q.Get("state") != "st" || q.Get("nonce") != "no" ||
		q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != oidc.Challenge("verifier-value") || !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("auth query %v", q)
	}
	// Non-loopback plain http is refused; https is the rule.
	if _, err := oidc.Discover(context.Background(), oidc.Config{Issuer: "http://idp.example.com", ClientID: "x", RedirectURL: "x"}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("http issuer accepted: %v", err)
	}
}

func TestExchangeAndVerify(t *testing.T) {
	is, p := provider(t)
	is.Nonce, is.WantCode = "nonce-1", "code-1"
	is.Claims["email"] = "ann@example.com"
	is.Claims["preferred_username"] = "ann"
	is.Claims["groups"] = []string{"printer-admins", "staff"}

	c, err := p.Exchange(context.Background(), "code-1", "verifier", "nonce-1")
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "subject-1" || c.Username() != "ann" || c.Email != "ann@example.com" {
		t.Fatalf("claims %+v", c)
	}
	if got := c.Lookup("groups"); len(got) != 2 || got[0] != "printer-admins" {
		t.Fatalf("lookup groups %v", got)
	}
	if f := is.LastTokenForm; f.Get("client_secret") != "s3cret" || f.Get("code_verifier") != "verifier" || f.Get("redirect_uri") == "" {
		t.Fatalf("token form %v", f)
	}
	// Wrong nonce and wrong code are refused.
	if _, err := p.Exchange(context.Background(), "code-1", "verifier", "other"); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("nonce mismatch accepted: %v", err)
	}
	if _, err := p.Exchange(context.Background(), "code-2", "verifier", "nonce-1"); err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("bad code accepted: %v", err)
	}
}

func TestVerifyIDTokenRejections(t *testing.T) {
	is, p := provider(t)
	ctx := context.Background()
	ok := is.IDToken(map[string]any{"nonce": "n"})
	if _, err := p.VerifyIDToken(ctx, ok, "n"); err != nil {
		t.Fatalf("valid RS256 token refused: %v", err)
	}
	es := is.IDTokenES256(map[string]any{"nonce": "n"})
	if _, err := p.VerifyIDToken(ctx, es, "n"); err != nil {
		t.Fatalf("valid ES256 token refused: %v", err)
	}
	cases := map[string]string{
		"alg none":     is.Sign("none", "rsa-1", map[string]any{"iss": is.URL, "aud": "pm-client", "sub": "s", "exp": time.Now().Add(time.Minute).Unix(), "nonce": "n"}),
		"alg HS256":    is.Sign("HS256", "rsa-1", map[string]any{"iss": is.URL, "aud": "pm-client", "sub": "s", "exp": time.Now().Add(time.Minute).Unix(), "nonce": "n"}),
		"wrong aud":    is.IDToken(map[string]any{"nonce": "n", "aud": "someone-else"}),
		"wrong iss":    is.IDToken(map[string]any{"nonce": "n", "iss": "https://evil.example"}),
		"expired":      is.IDToken(map[string]any{"nonce": "n", "exp": time.Now().Add(-10 * time.Minute).Unix()}),
		"unknown kid":  is.Sign("RS256", "rsa-9", map[string]any{"iss": is.URL, "aud": "pm-client", "sub": "s", "exp": time.Now().Add(time.Minute).Unix(), "nonce": "n"}),
		"tampered":     ok[:len(ok)-6] + "AAAAAA",
		"no signature": strings.Join(strings.Split(ok, ".")[:2], ".") + ".",
		"not a jwt":    "hello",
		"key mismatch": is.Sign("RS256", "ec-1", map[string]any{"iss": is.URL, "aud": "pm-client", "sub": "s", "exp": time.Now().Add(time.Minute).Unix(), "nonce": "n"}),
	}
	for name, tok := range cases {
		if _, err := p.VerifyIDToken(ctx, tok, "n"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Multi-audience with azp for this client is fine; with another azp it is not.
	multi := is.IDToken(map[string]any{"nonce": "n", "aud": []string{"pm-client", "other"}, "azp": "pm-client"})
	if _, err := p.VerifyIDToken(ctx, multi, "n"); err != nil {
		t.Fatalf("multi-aud refused: %v", err)
	}
	bad := is.IDToken(map[string]any{"nonce": "n", "aud": []string{"pm-client", "other"}, "azp": "other"})
	if _, err := p.VerifyIDToken(ctx, bad, "n"); err == nil {
		t.Fatal("azp mismatch accepted")
	}
}

func TestMapRoleAndLookup(t *testing.T) {
	m := map[string]string{"printer-admins": "admin", "helpdesk": "operator", "everyone": "viewer"}
	if r := oidc.MapRole([]string{"everyone", "helpdesk"}, m, ""); r != model.RoleOperator {
		t.Fatalf("got %s", r)
	}
	if r := oidc.MapRole([]string{"helpdesk", "printer-admins"}, m, "viewer"); r != model.RoleAdmin {
		t.Fatalf("got %s", r)
	}
	if r := oidc.MapRole(nil, m, "operator"); r != model.RoleOperator {
		t.Fatalf("default: %s", r)
	}
	if r := oidc.MapRole([]string{"x"}, m, "bogus"); r != model.RoleViewer {
		t.Fatalf("bogus default: %s", r)
	}
	c := &oidc.Claims{Raw: map[string]any{"realm_access": map[string]any{"roles": []any{"a", "b", 3}}, "role": "admin"}}
	if got := c.Lookup("realm_access.roles"); len(got) != 2 || got[1] != "b" {
		t.Fatalf("nested lookup %v", got)
	}
	if got := c.Lookup("role"); len(got) != 1 || got[0] != "admin" {
		t.Fatalf("string lookup %v", got)
	}
	if got := c.Lookup("missing.path"); got != nil {
		t.Fatalf("missing lookup %v", got)
	}
}
