package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// TokenPrefix marks an API token secret (audit A10). It lets Authenticate tell
// a service-account credential from a session token without a lookup and
// makes secrets greppable in leaked logs or repositories.
const TokenPrefix = "pmt_"

// KindToken is model.User.Kind for an API-token principal.
const KindToken = "token"

// ErrNoTokenStore is returned when the store does not implement TokenStore.
var ErrNoTokenStore = errors.New("api tokens are not supported by this store")

// touchEvery throttles last-used updates so a busy integration does not turn
// every read into a write.
const touchEvery = time.Minute

func (s *Service) tokens() store.TokenStore {
	ts, _ := s.store.(store.TokenStore)
	return ts
}

// CreateToken mints a service-account token. The clear-text secret is
// returned exactly once; only its SHA-256 is stored.
func (s *Service) CreateToken(ctx context.Context, name string, role model.Role, scope []string, expires *time.Time, createdBy string) (string, *store.APIToken, error) {
	ts := s.tokens()
	if ts == nil {
		return "", nil, ErrNoTokenStore
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil, errors.New("name required")
	}
	if !role.Allows(model.RoleViewer) {
		return "", nil, fmt.Errorf("unknown role %q", role)
	}
	if expires != nil && !expires.After(time.Now()) {
		return "", nil, errors.New("expiry must be in the future")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	secret := TokenPrefix + hex.EncodeToString(b)
	if scope == nil {
		scope = []string{}
	}
	t := &store.APIToken{Name: name, Role: role, Scope: scope, ExpiresAt: expires, CreatedBy: createdBy}
	if err := ts.CreateToken(ctx, t, hashToken(secret)); err != nil {
		return "", nil, err
	}
	return secret, t, nil
}

// ListTokens lists every token, revoked ones included (the API shows them greyed out).
func (s *Service) ListTokens(ctx context.Context) ([]*store.APIToken, error) {
	ts := s.tokens()
	if ts == nil {
		return nil, ErrNoTokenStore
	}
	return ts.ListTokens(ctx)
}

// RevokeToken ends a token immediately; the next request with it gets 401.
func (s *Service) RevokeToken(ctx context.Context, id string) (*store.APIToken, error) {
	ts := s.tokens()
	if ts == nil {
		return nil, ErrNoTokenStore
	}
	if err := ts.RevokeToken(ctx, id, time.Now()); err != nil {
		return nil, err
	}
	return ts.GetToken(ctx, id)
}

// authenticateToken resolves a pmt_ secret to a principal. Revoked and expired
// tokens fail exactly like unknown ones.
func (s *Service) authenticateToken(ctx context.Context, secret string) (*model.User, error) {
	ts := s.tokens()
	if ts == nil {
		return nil, ErrNoTokenStore
	}
	t, err := ts.GetTokenByHash(ctx, hashToken(secret))
	if err != nil {
		return nil, errors.New("invalid token")
	}
	now := time.Now()
	if !t.Active(now) {
		return nil, errors.New("token revoked or expired")
	}
	if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) > touchEvery {
		if err := ts.TouchToken(ctx, t.ID, now); err != nil {
			s.log.Warn("token last-used update failed", "token", t.ID, "err", err)
		}
	}
	return TokenPrincipal(t), nil
}

// TokenPrincipal is the model.User an API token acts as. The username carries
// a "token:" prefix so audit entries never collide with a human account.
func TokenPrincipal(t *store.APIToken) *model.User {
	scope := t.Scope
	if len(scope) == 0 {
		scope = nil
	}
	return &model.User{ID: t.ID, Username: "token:" + t.Name, Role: t.Role, CreatedAt: t.CreatedAt, Kind: KindToken, Scope: scope}
}

// IsToken reports whether the principal is an API token.
func IsToken(u *model.User) bool { return u != nil && u.Kind == KindToken }

// ProvisionExternal returns the local user for an identity-provider subject,
// creating it on first sign-in with a random (unusable) password. When
// syncRole is set the role is re-applied from the provider on every sign-in,
// so a demotion at the IdP takes effect at the next login.
func (s *Service) ProvisionExternal(ctx context.Context, username string, role model.Role, syncRole bool) (*model.User, bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, false, errors.New("the identity provider returned no usable username")
	}
	if !role.Allows(model.RoleViewer) {
		return nil, false, fmt.Errorf("unknown role %q", role)
	}
	u, _, err := s.store.GetUserByName(ctx, username)
	if err == nil {
		if syncRole && u.Role != role {
			if ts := s.tokens(); ts != nil {
				if err := ts.SetUserRole(ctx, u.ID, role); err != nil {
					return nil, false, err
				}
				u.Role = role
			}
		}
		return u, false, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, false, err
	}
	// A random 32-byte password that nobody knows: the account can only be
	// used through the identity provider (or after an admin reset).
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, false, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(b)), bcrypt.DefaultCost)
	if err != nil {
		return nil, false, err
	}
	u = &model.User{Username: username, Role: role}
	if err := s.store.CreateUser(ctx, u, string(hash)); err != nil {
		return nil, false, err
	}
	return u, true, nil
}
