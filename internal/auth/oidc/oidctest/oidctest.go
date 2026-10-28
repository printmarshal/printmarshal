// Package oidctest is a fake OpenID Connect issuer for tests: discovery,
// a JWKS with one generated RSA key (plus an optional P-256 key), and a
// token endpoint that returns an ID token built from the claims the test
// chose. It never talks to the network beyond its own httptest server.
package oidctest

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// Issuer is the running fake.
type Issuer struct {
	Server *httptest.Server
	URL    string
	RSA    *rsa.PrivateKey
	EC     *ecdsa.PrivateKey

	mu sync.Mutex
	// Claims are merged into every ID token the token endpoint mints
	// (iss, aud, exp, iat and nonce are filled in unless already present).
	Claims map[string]any
	// Nonce is what the token endpoint puts in the ID token; set it from the
	// nonce the authorization request carried.
	Nonce string
	// ClientID is the expected audience.
	ClientID string
	// WantCode, when set, makes the token endpoint reject any other code.
	WantCode string
	// LastTokenForm records the last token-endpoint form for assertions.
	LastTokenForm url.Values
}

// New starts the fake. clientID is the audience it issues tokens for.
func New(t *testing.T, clientID string) *Issuer {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	is := &Issuer{RSA: rk, EC: ek, ClientID: clientID, Claims: map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                 is.URL,
			"authorization_endpoint": is.URL + "/authorize",
			"token_endpoint":         is.URL + "/token",
			"jwks_uri":               is.URL + "/keys",
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"keys": []any{is.rsaJWK(), is.ecJWK()}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		is.mu.Lock()
		is.LastTokenForm = r.PostForm
		want, nonce := is.WantCode, is.Nonce
		is.mu.Unlock()
		if r.PostForm.Get("grant_type") != "authorization_code" || (want != "" && r.PostForm.Get("code") != want) || r.PostForm.Get("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			writeJSON(w, map[string]any{"error": "invalid_grant", "error_description": "bad code"})
			return
		}
		writeJSON(w, map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": is.IDToken(map[string]any{"nonce": nonce})})
	})
	is.Server = httptest.NewServer(mux)
	is.URL = is.Server.URL
	t.Cleanup(is.Server.Close)
	return is
}

// IDToken signs (RS256) an ID token: the issuer's Claims, then extra on top,
// with iss/aud/exp/iat defaulted.
func (is *Issuer) IDToken(extra map[string]any) string {
	return is.Sign("RS256", "rsa-1", is.claims(extra))
}

// IDTokenES256 is IDToken signed with the P-256 key.
func (is *Issuer) IDTokenES256(extra map[string]any) string {
	return is.Sign("ES256", "ec-1", is.claims(extra))
}

func (is *Issuer) claims(extra map[string]any) map[string]any {
	is.mu.Lock()
	defer is.mu.Unlock()
	now := time.Now()
	c := map[string]any{"iss": is.URL, "aud": is.ClientID, "sub": "subject-1", "exp": now.Add(5 * time.Minute).Unix(), "iat": now.Unix()}
	for k, v := range is.Claims {
		c[k] = v
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// Sign produces a compact JWS with the given alg/kid over claims. alg
// "none" yields an unsigned token (for negative tests).
func (is *Issuer) Sign(alg, kid string, claims map[string]any) string {
	hdr, _ := json.Marshal(map[string]any{"alg": alg, "kid": kid, "typ": "JWT"})
	body, _ := json.Marshal(claims)
	signing := b64(hdr) + "." + b64(body)
	digest := sha256.Sum256([]byte(signing))
	var sig []byte
	switch alg {
	case "RS256":
		sig, _ = rsa.SignPKCS1v15(rand.Reader, is.RSA, crypto.SHA256, digest[:])
	case "ES256":
		r, s, _ := ecdsa.Sign(rand.Reader, is.EC, digest[:])
		sig = make([]byte, 64)
		r.FillBytes(sig[:32])
		s.FillBytes(sig[32:])
	case "HS256":
		sig = digest[:] // nonsense, but shaped like a MAC
	}
	return signing + "." + b64(sig)
}

func (is *Issuer) rsaJWK() map[string]any {
	pub := is.RSA.PublicKey
	return map[string]any{"kty": "RSA", "kid": "rsa-1", "use": "sig", "alg": "RS256", "n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes())}
}

func (is *Issuer) ecJWK() map[string]any {
	pub := is.EC.PublicKey
	x, y := make([]byte, 32), make([]byte, 32)
	pub.X.FillBytes(x)
	pub.Y.FillBytes(y)
	return map[string]any{"kty": "EC", "kid": "ec-1", "use": "sig", "alg": "ES256", "crv": "P-256", "x": b64(x), "y": b64(y)}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
