package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/auth/oidc"
)

// OIDC sign-in (docs/auth.md): /start redirects the browser to the identity
// provider with PKCE + state + nonce held in a short-lived cookie; /callback
// redeems the code, verifies the ID token, provisions the local user and
// issues an ordinary session. /providers (providers.go) tells the login
// page what to show; the SAML, LDAP and RADIUS methods live beside it.

const oidcCookie = "pm_oidc"

type oidcState struct {
	mu   sync.Mutex
	p    *oidc.Provider
	when time.Time
}

type oidcFlow struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
}

func (s *Server) oidcRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/providers", s.authProviders)
	mux.HandleFunc("GET /api/auth/oidc/start", s.oidcStart)
	mux.HandleFunc("GET /api/auth/oidc/callback", s.oidcCallback)
	s.samlRoutes(mux)
}

func (s *Server) oidcEnabled() bool {
	c := s.Cfg.Auth.OIDC
	return c.Issuer != "" && c.ClientID != ""
}

func (s *Server) oidcName() string {
	c := s.Cfg.Auth.OIDC
	if c.DisplayName != "" {
		return c.DisplayName
	}
	if u, err := url.Parse(c.Issuer); err == nil && u.Host != "" {
		return u.Host
	}
	return "single sign-on"
}

// oidcProvider discovers the issuer on first use and keeps it; a failed
// discovery is not cached, so a provider that was down comes back.
func (s *Server) oidcProvider(ctx context.Context) (*oidc.Provider, error) {
	if !s.oidcEnabled() {
		return nil, errors.New("oidc is not configured")
	}
	if s.oidc == nil {
		s.oidc = &oidcState{}
	}
	s.oidc.mu.Lock()
	defer s.oidc.mu.Unlock()
	if s.oidc.p != nil {
		return s.oidc.p, nil
	}
	c := s.Cfg.Auth.OIDC
	secret := ""
	if c.ClientSecretEnv != "" {
		secret = os.Getenv(c.ClientSecretEnv)
	}
	p, err := oidc.Discover(ctx, oidc.Config{Issuer: c.Issuer, ClientID: c.ClientID, ClientSecret: secret, RedirectURL: c.RedirectURL, Scopes: c.Scopes})
	if err != nil {
		return nil, err
	}
	s.oidc.p, s.oidc.when = p, time.Now()
	return p, nil
}

func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	p, err := s.oidcProvider(r.Context())
	if err != nil {
		s.Log.Error("oidc start", "err", err)
		s.oidcFail(w, r, "single sign-on is unavailable; an administrator can find the reason in the server log")
		return
	}
	state, err1 := oidc.Random()
	nonce, err2 := oidc.Random()
	verifier, err3 := oidc.Random()
	if err1 != nil || err2 != nil || err3 != nil {
		writeErr(w, http.StatusInternalServerError, "random source failed")
		return
	}
	b, _ := json.Marshal(oidcFlow{State: state, Nonce: nonce, Verifier: verifier})
	// SameSite=Lax: the callback is a top-level navigation from the IdP, which
	// Strict would strip the cookie from. Path-limited so it is never sent elsewhere.
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/api/auth/oidc/",
		HttpOnly: true, Secure: s.Cfg.Auth.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, p.AuthURL(state, nonce, verifier), http.StatusFound)
}

func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	clearFlow := func() {
		http.SetCookie(w, &http.Cookie{Name: oidcCookie, Value: "", Path: "/api/auth/oidc/", HttpOnly: true, Secure: s.Cfg.Auth.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}
	ip := clientIP(r)
	if s.logins.Blocked(ip) {
		s.security(r.Context(), r, "auth.failed", "oidc", "rate limited", "throttled")
		s.oidcFail(w, r, "too many failed sign-ins; try again in a minute")
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		clearFlow()
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", "oidc", "provider returned "+e+": "+q.Get("error_description"), "denied")
		s.oidcFail(w, r, "the identity provider refused the sign-in ("+e+")")
		return
	}
	var flow oidcFlow
	if c, err := r.Cookie(oidcCookie); err == nil {
		if b, err := base64.RawURLEncoding.DecodeString(c.Value); err == nil {
			_ = json.Unmarshal(b, &flow)
		}
	}
	clearFlow()
	code, state := q.Get("code"), q.Get("state")
	if flow.State == "" || state == "" || state != flow.State || code == "" {
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", "oidc", "state mismatch or missing code", "denied")
		s.oidcFail(w, r, "sign-in could not be verified; start again")
		return
	}
	p, err := s.oidcProvider(r.Context())
	if err != nil {
		s.oidcFail(w, r, "single sign-on is unavailable: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	claims, err := p.Exchange(ctx, code, flow.Verifier, flow.Nonce)
	if err != nil {
		s.logins.Fail(ip)
		s.Log.Warn("oidc callback", "err", err)
		s.security(r.Context(), r, "auth.failed", "oidc", err.Error(), "denied")
		s.oidcFail(w, r, "the identity provider's response could not be verified")
		return
	}
	cfg := s.Cfg.Auth.OIDC
	if len(cfg.AllowedDomains) > 0 {
		// The domain is an access boundary, so an email the provider itself
		// marks unverified (email_verified: false) must not pass it; a token
		// without the claim (Entra ID omits it) is accepted.
		if v, ok := claims.Raw["email_verified"].(bool); ok && !v {
			s.logins.Fail(ip)
			s.security(r.Context(), r, "auth.failed", claims.Username(), "email not verified by the provider", "denied")
			s.oidcFail(w, r, "your account's email address is not verified by the identity provider")
			return
		}
		if !domainAllowed(claims.Email, cfg.AllowedDomains) {
			s.logins.Fail(ip)
			s.security(r.Context(), r, "auth.failed", claims.Username(), "email domain not allowed", "denied")
			s.oidcFail(w, r, "your account's email domain is not allowed to sign in")
			return
		}
	}
	syncRole := cfg.RoleClaim != "" && len(cfg.RoleMap) > 0
	role := oidc.MapRole(claims.Lookup(cfg.RoleClaim), cfg.RoleMap, cfg.DefaultRole)
	u, created, err := s.Auth.ProvisionExternal(r.Context(), auth.ExternalIdentity{Issuer: p.Issuer(), Subject: claims.Subject, Username: claims.Username()}, role, syncRole)
	if err != nil {
		if errors.Is(err, auth.ErrUsernameTaken) {
			s.logins.Fail(ip)
			s.security(r.Context(), r, "auth.failed", claims.Username(), err.Error(), "denied")
			s.oidcFail(w, r, err.Error())
			return
		}
		s.Log.Error("oidc provisioning", "err", err)
		s.security(r.Context(), r, "auth.failed", claims.Username(), "provisioning failed: "+err.Error(), "error")
		s.oidcFail(w, r, "the account could not be provisioned")
		return
	}
	if created {
		s.auditAs(r.Context(), u.Username, ip, "user.create", u.ID, nil, map[string]any{"username": u.Username, "role": u.Role, "source": "oidc", "issuer": p.Issuer(), "subject": claims.Subject}, "ok")
	}
	token, err := s.Auth.IssueSession(r.Context(), u)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.logins.Reset(ip)
	s.security(r.Context(), r, "auth.login", u.Username, "signed in via "+s.oidcName(), "ok")
	s.Auth.SetCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// oidcFail sends the browser back to the login page with a short message.
func (s *Server) oidcFail(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?oidc_error="+url.QueryEscape(msg), http.StatusSeeOther)
}

func domainAllowed(email string, domains []string) bool {
	_, dom, ok := strings.Cut(strings.ToLower(strings.TrimSpace(email)), "@")
	if !ok || dom == "" {
		return false
	}
	for _, d := range domains {
		if strings.EqualFold(strings.TrimPrefix(d, "@"), dom) {
			return true
		}
	}
	return false
}
