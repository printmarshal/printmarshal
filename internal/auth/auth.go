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
	"os"
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
	// PasswordFile, when set, receives the generated initial admin password
	// (mode 0600) instead of the log.
	PasswordFile string
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
		// The generated password goes to a 0600 file in the data directory, not
		// to the log, which is usually shipped elsewhere (audit A18).
		if s.PasswordFile != "" {
			if err := os.WriteFile(s.PasswordFile, []byte(pw+"\n"), 0o600); err == nil {
				s.log.Warn("created initial admin user; the generated password is in a file. Change it after first login, then delete the file", "username", "admin", "file", s.PasswordFile)
				return nil
			} else {
				s.log.Error("could not write the initial password file; printing it once instead", "file", s.PasswordFile, "err", err)
			}
		}
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
	token, err := s.IssueSession(ctx, u)
	return token, u, err
}

// IssueSession creates a session for an already-authenticated user (password
// login or an OIDC callback) and returns the clear-text token.
func (s *Service) IssueSession(ctx context.Context, u *model.User) (string, error) {
	tok := make([]byte, 32)
	if _, err := rand.Read(tok); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tok)
	if err := s.store.CreateSession(ctx, hashToken(token), u.ID, time.Now().Add(s.ttl)); err != nil {
		return "", err
	}
	return token, nil
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
// the session cookie. Tokens are never read from the query string: URLs end
// up in proxy and browser logs (audit A05).
func TokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}

// Authenticate resolves the principal for a request: a session token (cookie
// or bearer) for a signed-in user, or a "pmt_" API token (bearer only) for a
// service account, which comes back with Kind "token" and its group scope.
func (s *Service) Authenticate(r *http.Request) (*model.User, error) {
	tok := TokenFromRequest(r)
	if tok == "" {
		return nil, errors.New("no session")
	}
	if strings.HasPrefix(tok, TokenPrefix) {
		if !strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") {
			return nil, errors.New("api tokens are accepted in the Authorization header only")
		}
		return s.authenticateToken(r.Context(), tok)
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

// RevokeOtherSessions ends every session of the user except the one holding
// keepToken (used after a password change, audit A04). It returns how many
// sessions were revoked.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID, keepToken string) (int64, error) {
	keep := ""
	if keepToken != "" {
		keep = hashToken(keepToken)
	}
	return s.store.DeleteUserSessions(ctx, userID, keep)
}

// RevokeAllSessions ends every session of the user (admin reset / user delete).
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) (int64, error) {
	return s.store.DeleteUserSessions(ctx, userID, "")
}
