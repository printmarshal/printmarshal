package api

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	csaml "github.com/crewjam/saml"

	"printmarshal/internal/auth"
	"printmarshal/internal/auth/oidc"
	"printmarshal/internal/auth/saml"
)

// SAML 2.0 service provider (docs/auth.md): /metadata publishes the SP
// descriptor, /start sends the browser to the IdP with the HTTP-Redirect
// binding and keeps the request ID in a short-lived cookie, /acs validates
// the posted response (signature, issuer, audience, destination, validity
// window, InResponseTo against the cookie, replay of the assertion ID) and
// issues an ordinary session.

const samlCookie = "pm_saml"

var samlIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

type samlState struct {
	mu sync.Mutex
	sp *csaml.ServiceProvider
}

func (s *Server) samlRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/saml/metadata", s.samlMetadata)
	mux.HandleFunc("GET /api/auth/saml/start", s.samlStart)
	mux.HandleFunc("POST /api/auth/saml/acs", s.samlACS)
}

func (s *Server) samlEnabled() bool {
	c := s.Cfg.Auth.SAML
	return c.IDPMetadataURL != "" || c.IDPMetadataFile != ""
}

func (s *Server) samlName() string {
	c := s.Cfg.Auth.SAML
	if c.DisplayName != "" {
		return c.DisplayName
	}
	if c.IDPMetadataURL != "" {
		if u, err := url.Parse(c.IDPMetadataURL); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return "SAML single sign-on"
}

// samlProvider builds the service provider on first use (IdP metadata, SP
// key pair) and keeps it; a failure is not cached.
func (s *Server) samlProvider(ctx context.Context) (*csaml.ServiceProvider, error) {
	if !s.samlEnabled() {
		return nil, errors.New("saml is not configured")
	}
	if s.saml == nil {
		s.saml = &samlState{}
	}
	s.saml.mu.Lock()
	defer s.saml.mu.Unlock()
	if s.saml.sp != nil {
		return s.saml.sp, nil
	}
	c := s.Cfg.Auth.SAML
	sp, err := saml.New(ctx, saml.Config{IDPMetadataURL: c.IDPMetadataURL, IDPMetadataFile: c.IDPMetadataFile, BaseURL: c.BaseURL, EntityID: c.EntityID, ACSURL: c.ACSURL,
		CertFile: c.SPCertFile, KeyFile: c.SPKeyFile, DataDir: s.Cfg.DataDir, SignRequests: c.SignRequests, Log: s.Log})
	if err != nil {
		return nil, err
	}
	s.saml.sp = sp
	return sp, nil
}

func (s *Server) samlMetadata(w http.ResponseWriter, r *http.Request) {
	sp, err := s.samlProvider(r.Context())
	if err != nil {
		s.Log.Error("saml metadata", "err", err)
		writeErr(w, http.StatusServiceUnavailable, "single sign-on is unavailable; an administrator can find the reason in the server log")
		return
	}
	b, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(append([]byte(xml.Header), b...))
}

func (s *Server) samlFlowCookie(value string, maxAge int) *http.Cookie {
	c := &http.Cookie{Name: samlCookie, Value: value, Path: "/api/auth/saml/", HttpOnly: true, Secure: s.Cfg.Auth.SecureCookies, MaxAge: maxAge}
	// The IdP posts the response cross-site, which SameSite=Lax would strip
	// the cookie from. Behind TLS it is Secure + None; on plain http the
	// attribute is omitted (browsers then allow a recent top-level POST).
	if s.Cfg.Auth.SecureCookies {
		c.SameSite = http.SameSiteNoneMode
	} else {
		c.SameSite = http.SameSiteDefaultMode
	}
	return c
}

func (s *Server) samlStart(w http.ResponseWriter, r *http.Request) {
	sp, err := s.samlProvider(r.Context())
	if err != nil {
		s.Log.Error("saml start", "err", err)
		s.ssoFail(w, r, "single sign-on is unavailable; an administrator can find the reason in the server log")
		return
	}
	req, err := sp.MakeAuthenticationRequest(sp.GetSSOBindingLocation(csaml.HTTPRedirectBinding), csaml.HTTPRedirectBinding, csaml.HTTPPostBinding)
	if err != nil {
		s.Log.Error("saml start", "err", err)
		s.ssoFail(w, r, "single sign-on request could not be built")
		return
	}
	// RelayState carries nothing: the landing page is always "/".
	dest, err := req.Redirect("", sp)
	if err != nil {
		s.Log.Error("saml start", "err", err)
		s.ssoFail(w, r, "single sign-on request could not be built")
		return
	}
	http.SetCookie(w, s.samlFlowCookie(req.ID, 600))
	http.Redirect(w, r, dest.String(), http.StatusFound)
}

func (s *Server) samlACS(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if s.logins.Blocked(ip) {
		s.security(r.Context(), r, "auth.failed", "saml", "rate limited", "throttled")
		s.ssoFail(w, r, "too many failed sign-ins; try again in a minute")
		return
	}
	reqID := ""
	if c, err := r.Cookie(samlCookie); err == nil && samlIDPattern.MatchString(c.Value) {
		reqID = c.Value
	}
	http.SetCookie(w, s.samlFlowCookie("", -1))
	if err := r.ParseForm(); err != nil || r.PostForm.Get("SAMLResponse") == "" {
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", "saml", "no SAMLResponse posted", "denied")
		s.ssoFail(w, r, "sign-in could not be verified; start again")
		return
	}
	if reqID == "" {
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", "saml", "no pending request (missing or expired cookie)", "denied")
		s.ssoFail(w, r, "sign-in could not be verified; start again")
		return
	}
	sp, err := s.samlProvider(r.Context())
	if err != nil {
		s.ssoFail(w, r, "single sign-on is unavailable: "+err.Error())
		return
	}
	assertion, err := sp.ParseResponse(r, []string{reqID})
	if err != nil {
		s.logins.Fail(ip)
		detail := err.Error()
		var ire *csaml.InvalidResponseError
		if errors.As(err, &ire) && ire.PrivateErr != nil {
			detail = ire.PrivateErr.Error()
		}
		s.Log.Warn("saml acs", "err", detail)
		s.security(r.Context(), r, "auth.failed", "saml", detail, "denied")
		s.ssoFail(w, r, "the identity provider's response could not be verified")
		return
	}
	if assertion.ID == "" || s.samlReplay.Seen(assertion.ID, saml.Expiry(assertion, time.Now())) {
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", "saml", "assertion replayed: "+assertion.ID, "denied")
		s.ssoFail(w, r, "this sign-in response was already used; start again")
		return
	}

	cfg := s.Cfg.Auth.SAML
	nameID, format := saml.NameID(assertion)
	username := nameID
	if cfg.UsernameAttr != "" {
		username = first(saml.Values(assertion, cfg.UsernameAttr))
	}
	subject := ""
	switch {
	case cfg.SubjectAttr != "":
		subject = first(saml.Values(assertion, cfg.SubjectAttr))
	case saml.StableNameID(format):
		subject = nameID
	}
	if subject == "" {
		s.logins.Fail(ip)
		detail := "no stable subject: configure subjectAttr (NameID format " + format + ")"
		s.security(r.Context(), r, "auth.failed", username, detail, "denied")
		s.ssoFail(w, r, "the identity provider sent no stable identifier; an administrator must configure subjectAttr")
		return
	}
	groups := saml.Values(assertion, cfg.RoleAttr)
	if len(cfg.AllowedGroups) > 0 && !anyEqualFold(groups, cfg.AllowedGroups) {
		s.logins.Fail(ip)
		s.security(r.Context(), r, "auth.failed", username, "not in an allowed group", "denied")
		s.ssoFail(w, r, "your account is not in a group allowed to sign in")
		return
	}
	syncRole := cfg.RoleAttr != "" && len(cfg.RoleMap) > 0
	role := oidc.MapRole(groups, cfg.RoleMap, cfg.DefaultRole)
	issuer := "saml:" + sp.IDPMetadata.EntityID
	u, created, err := s.Auth.ProvisionExternal(r.Context(), auth.ExternalIdentity{Issuer: issuer, Subject: subject, Username: username}, role, syncRole)
	if err != nil {
		if errors.Is(err, auth.ErrUsernameTaken) || strings.Contains(err.Error(), "no usable username") {
			s.logins.Fail(ip)
			s.security(r.Context(), r, "auth.failed", username, err.Error(), "denied")
			s.ssoFail(w, r, err.Error())
			return
		}
		s.Log.Error("saml provisioning", "err", err)
		s.security(r.Context(), r, "auth.failed", username, "provisioning failed: "+err.Error(), "error")
		s.ssoFail(w, r, "the account could not be provisioned")
		return
	}
	if created {
		s.auditAs(r.Context(), u.Username, ip, "user.create", u.ID, nil, map[string]any{"username": u.Username, "role": u.Role, "source": "saml", "issuer": issuer, "subject": subject}, "ok")
	}
	token, err := s.Auth.IssueSession(r.Context(), u)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.logins.Reset(ip)
	s.security(r.Context(), r, "auth.login", u.Username, "signed in via "+s.samlName(), "ok")
	s.Auth.SetCookie(w, token)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ssoFail sends the browser back to the login page with a short message.
func (s *Server) ssoFail(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?sso_error="+url.QueryEscape(msg), http.StatusSeeOther)
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func anyEqualFold(have, allowed []string) bool {
	for _, a := range allowed {
		for _, h := range have {
			if strings.EqualFold(strings.TrimSpace(a), h) {
				return true
			}
		}
	}
	return false
}
