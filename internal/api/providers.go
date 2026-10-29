package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// External sign-in methods (docs/auth.md). Browser-redirect providers (OIDC,
// SAML) have their own routes; password providers (LDAP, RADIUS) sit behind
// POST /api/auth/login, which tries the local account first and falls back
// to them only when no local account owns the name or the account is
// already bound to that provider. Every method provisions through
// auth.Service.ProvisionExternal, so accounts bind by (issuer, subject).

// provider is one row of GET /api/auth/providers.
type provider struct {
	Kind   string `json:"kind"`   // oidc | saml | ldap | radius
	Name   string `json:"name"`   // login-page label
	Method string `json:"method"` // redirect | password
	Start  string `json:"start,omitempty"`
}

func (s *Server) providers() []provider {
	var out []provider
	if s.oidcEnabled() {
		out = append(out, provider{Kind: "oidc", Name: s.oidcName(), Method: "redirect", Start: "/api/auth/oidc/start"})
	}
	if s.samlEnabled() {
		out = append(out, provider{Kind: "saml", Name: s.samlName(), Method: "redirect", Start: "/api/auth/saml/start"})
	}
	if s.ldapEnabled() {
		out = append(out, provider{Kind: "ldap", Name: s.ldapName(), Method: "password"})
	}
	if s.radiusEnabled() {
		out = append(out, provider{Kind: "radius", Name: s.radiusName(), Method: "password"})
	}
	if out == nil {
		out = []provider{}
	}
	return out
}

func (s *Server) authProviders(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"local": true, "oidc": map[string]any{"enabled": s.oidcEnabled()}, "providers": s.providers()}
	if s.oidcEnabled() {
		out["oidc"] = map[string]any{"enabled": true, "name": s.oidcName(), "start": "/api/auth/oidc/start"}
	}
	writeJSON(w, http.StatusOK, out)
}

// externalIdentity is what a password provider returns on success.
type externalIdentity struct {
	auth.ExternalIdentity
	Role     model.Role
	SyncRole bool
}

// loginError carries an HTTP status and message for the login handler plus
// the detail recorded in the security event.
type loginError struct {
	status int
	msg    string
	detail string
	result string
}

func (e *loginError) Error() string { return e.detail }

// errInvalidCredentials is the one answer wrong passwords, unknown users and
// names a provider will not claim all map to.
var errInvalidCredentials = errors.New("invalid credentials")

// passwordProvider is LDAP or RADIUS behind the login form.
type passwordProvider struct {
	name, issuer string
	authenticate func(ctx context.Context, username, password string) (*externalIdentity, error)
}

func (s *Server) passwordProviders() []passwordProvider {
	var out []passwordProvider
	if s.ldapEnabled() {
		out = append(out, passwordProvider{name: s.ldapName(), issuer: s.ldapIssuer(), authenticate: s.ldapAuthenticate})
	}
	if s.radiusEnabled() {
		out = append(out, passwordProvider{name: s.radiusName(), issuer: s.radiusIssuer(), authenticate: s.radiusAuthenticate})
	}
	return out
}

// externalLogin authenticates the form credentials against the password
// providers and returns a session. It is only called after the local
// password check failed. Rules:
//
//   - a local (unbound) account with that name is never handed to a
//     provider: the local password is the only way in;
//   - an account bound to a provider is checked with that provider alone;
//   - a free name is offered to each provider in turn (LDAP, then RADIUS);
//   - rejections stay "invalid credentials"; a provider that could not be
//     reached is reported as 503 so an operator knows to look at the log.
func (s *Server) externalLogin(r *http.Request, username, password string) (string, *model.User, string, error) {
	providers := s.passwordProviders()
	if len(providers) == 0 {
		return "", nil, "", errInvalidCredentials
	}
	ctx := r.Context()
	ip := clientIP(r)
	boundTo := ""
	if existing, _, err := s.Store.GetUserByName(ctx, strings.TrimSpace(username)); err == nil {
		if !existing.IsExternal() {
			return "", nil, "", errInvalidCredentials
		}
		boundTo = existing.ExternalIssuer
	} else if !errors.Is(err, store.ErrNotFound) {
		s.Log.Error("login lookup", "err", err)
		return "", nil, "", errInvalidCredentials
	}
	var unavailable error
	for _, p := range providers {
		if boundTo != "" && boundTo != p.issuer {
			continue
		}
		id, err := p.authenticate(ctx, username, password)
		if err != nil {
			var le *loginError
			if errors.As(err, &le) {
				return "", nil, "", le
			}
			if errors.Is(err, errInvalidCredentials) {
				continue
			}
			// Transport / configuration problem: the user is not at fault.
			s.Log.Error("external sign-in", "provider", p.name, "err", err)
			s.security(ctx, r, "auth.failed", username, p.name+": "+err.Error(), "error")
			unavailable = err
			continue
		}
		u, created, err := s.Auth.ProvisionExternal(ctx, id.ExternalIdentity, id.Role, id.SyncRole)
		if err != nil {
			if errors.Is(err, auth.ErrUsernameTaken) {
				return "", nil, "", &loginError{status: http.StatusUnauthorized, msg: err.Error(), detail: err.Error(), result: "denied"}
			}
			s.Log.Error("external provisioning", "provider", p.name, "err", err)
			return "", nil, "", &loginError{status: http.StatusInternalServerError, msg: "the account could not be provisioned", detail: "provisioning failed: " + err.Error(), result: "error"}
		}
		if created {
			s.auditAs(ctx, u.Username, ip, "user.create", u.ID, nil, map[string]any{"username": u.Username, "role": u.Role, "source": strings.SplitN(id.Issuer, ":", 2)[0], "issuer": id.Issuer, "subject": id.Subject}, "ok")
		}
		token, err := s.Auth.IssueSession(ctx, u)
		if err != nil {
			return "", nil, "", &loginError{status: http.StatusInternalServerError, msg: "session could not be created", detail: err.Error(), result: "error"}
		}
		return token, u, p.name, nil
	}
	if unavailable != nil {
		return "", nil, "", &loginError{status: http.StatusServiceUnavailable, msg: "the directory is unavailable; an administrator can find the reason in the server log", detail: "directory unavailable", result: "error"}
	}
	return "", nil, "", errInvalidCredentials
}

// hostOf returns the host of a URL or address for display names.
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	if h, _, err := splitHostPortLoose(raw); err == nil {
		return h
	}
	return raw
}
