package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/model"
)

// tokenRoutes registers API tokens (service accounts, audit A10) and the
// OIDC sign-in endpoints (oidc.go). Tokens are admin-only to manage; an API
// token itself can never mint or revoke one (scope.go).
func (s *Server) tokenRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/tokens", s.require(model.RoleAdmin, s.listTokens))
	mux.Handle("POST /api/tokens", s.require(model.RoleAdmin, s.createToken))
	mux.Handle("DELETE /api/tokens/{id}", s.require(model.RoleAdmin, s.revokeToken))
	s.oidcRoutes(mux)
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.Auth.ListTokens(r.Context())
	if err != nil {
		s.tokenFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// createToken mints a token. The response carries the clear-text secret
// exactly once; it is never stored or logged.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string     `json:"name"`
		Role      model.Role `json:"role"`
		Scope     []string   `json:"scope"`     // group ids; empty = unscoped
		ExpiresAt *time.Time `json:"expiresAt"` // RFC 3339, optional
		TTL       string     `json:"ttl"`       // Go duration alternative, e.g. "720h"
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Role == "" {
		body.Role = model.RoleViewer
	}
	expires := body.ExpiresAt
	if body.TTL != "" {
		d, err := time.ParseDuration(body.TTL)
		if err != nil || d <= 0 {
			writeErr(w, http.StatusBadRequest, "ttl must be a positive Go duration, e.g. 720h")
			return
		}
		t := time.Now().Add(d)
		expires = &t
	}
	scope := []string{}
	for _, gid := range body.Scope {
		gid = strings.TrimSpace(gid)
		if gid == "" {
			continue
		}
		if _, err := s.Store.GetGroup(r.Context(), gid); err != nil {
			writeErr(w, http.StatusBadRequest, "unknown group in scope: "+gid)
			return
		}
		scope = append(scope, gid)
	}
	secret, t, err := s.Auth.CreateToken(r.Context(), body.Name, body.Role, scope, expires, actor(r))
	if err != nil {
		if errors.Is(err, auth.ErrNoTokenStore) {
			s.tokenFail(w, err)
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	after := map[string]any{"name": t.Name, "role": t.Role, "scope": t.Scope, "expiresAt": t.ExpiresAt}
	s.audit(r.Context(), r, "token.create", t.ID, nil, after, "ok")
	s.security(r.Context(), r, "auth.token.created", actor(r), "created api token "+t.Name+" ("+string(t.Role)+")", "ok")
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.Auth.RevokeToken(r.Context(), id)
	if err != nil {
		s.tokenFail(w, err)
		return
	}
	s.audit(r.Context(), r, "token.revoke", id, map[string]any{"name": t.Name, "role": t.Role}, nil, "ok")
	s.security(r.Context(), r, "auth.token.revoked", actor(r), "revoked api token "+t.Name, "ok")
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) tokenFail(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrNoTokenStore) {
		writeErr(w, http.StatusNotImplemented, err.Error())
		return
	}
	s.fail(w, err)
}
