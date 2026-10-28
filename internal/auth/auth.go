// Package auth provides local users, hashed passwords, bearer/cookie sessions
// and role checks (spec §11 access control).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Service authenticates requests.
type Service struct {
	store  store.Store
	ttl    time.Duration
	secure bool
	log    *slog.Logger
}

// CookieName carries the session token for the SPA.
const CookieName = "pm_session"

// New returns a service.
func New(st store.Store, ttl time.Duration, secureCookies bool, log *slog.Logger) *Service {
	return &Service{store: st, ttl: ttl, secure: secureCookies, log: log}
}

// Bootstrap creates the first admin if no users exist. The password comes from
// PM_ADMIN_PASSWORD; otherwise a random one is generated and logged once.
func (s *Service) Bootstrap(ctx context.Context, envPassword string) error {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	pw := envPassword
	generated := false
	if pw == "" {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		pw = hex.EncodeToString(b)
		generated = true
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u := &model.User{Username: "admin", Role: model.RoleAdmin}
	if err := s.store.CreateUser(ctx, u, string(hash)); err != nil {
		return err
	}
	if generated {
		// This is the one place a secret is deliberately printed: there is no
		// other way to hand the operator their initial password.
		s.log.Warn("created initial admin user; change this password after first login", "username", "admin", "password", pw)
	} else {
		s.log.Info("created initial admin user from PM_ADMIN_PASSWORD", "username", "admin")
	}
	return nil
}

// Login verifies a password and returns a new session token.
func (s *Service) Login(ctx context.Context, username, password string) (string, *model.User, error) {
	u, hash, err := s.store.GetUserByName(ctx, username)
	if err != nil {
		// Burn comparable time so user enumeration is not trivial.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz0123456"), []byte(password))
		return "", nil, errors.New("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", nil, errors.New("invalid credentials")
	}
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return "", nil, err
	}
	token := hex.EncodeToString(tok)
	if err := s.store.CreateSession(ctx, hashToken(token), u.ID, time.Now().Add(s.ttl)); err != nil {
		return "", nil, err
	}
	return token, u, nil
}

// Logout deletes the session.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, hashToken(token))
}

// CreateUser adds a user.
func (s *Service) CreateUser(ctx context.Context, username, password string, role model.Role) (*model.User, error) {
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	if !role.Allows(model.RoleViewer) {
		return nil, fmt.Errorf("unknown role %q", role)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &model.User{Username: strings.TrimSpace(username), Role: role}
	if u.Username == "" {
		return nil, errors.New("username required")
	}
	return u, s.store.CreateUser(ctx, u, string(hash))
}

// SetPassword changes a user's password.
func (s *Service) SetPassword(ctx context.Context, userID, password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.store.SetPassword(ctx, userID, string(hash))
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// TokenFromRequest extracts the session token from the Authorization header,
// the session cookie, or (for WebSocket clients) the access_token query param.
func TokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return r.URL.Query().Get("access_token")
}

// Authenticate resolves the user for a request.
func (s *Service) Authenticate(r *http.Request) (*model.User, error) {
	tok := TokenFromRequest(r)
	if tok == "" {
		return nil, errors.New("no session")
	}
	return s.store.GetSession(r.Context(), hashToken(tok))
}

// SetCookie writes the session cookie.
func (s *Service) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.secure,
		SameSite: http.SameSiteStrictMode, MaxAge: int(s.ttl.Seconds())})
}

// ClearCookie deletes the session cookie.
func (s *Service) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

type ctxKey struct{}

// WithUser stores the user on a context.
func WithUser(ctx context.Context, u *model.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// UserFrom reads the user from a context.
func UserFrom(ctx context.Context) *model.User {
	u, _ := ctx.Value(ctxKey{}).(*model.User)
	return u
}
