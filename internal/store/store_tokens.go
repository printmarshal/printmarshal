package store

import (
	"context"
	"time"

	"printmarshal/internal/model"
)

// APIToken is a service-account credential (audit A10): a named principal
// with a role, an optional group scope and an optional expiry. The secret is
// shown once at creation and stored as a SHA-256 hash, like sessions.
type APIToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Role       model.Role `json:"role"`
	Scope      []string   `json:"scope"` // group ids; empty = every device
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

// Active reports whether the token may authenticate at time now.
func (t *APIToken) Active(now time.Time) bool {
	if t.RevokedAt != nil {
		return false
	}
	return t.ExpiresAt == nil || t.ExpiresAt.After(now)
}

// TokenStore is the persistence surface for API tokens and the identity
// bits OIDC provisioning needs. It is defined here (own file), implemented
// on *SQLite in sqlite_tokens.go, with its table in schema_tokens.sql.
// Embed it in Store (store.go) next to the other sub-interfaces.
type TokenStore interface {
	CreateToken(ctx context.Context, t *APIToken, secretHash string) error
	ListTokens(ctx context.Context) ([]*APIToken, error)
	// GetTokenByHash returns the token for a secret hash, revoked or expired
	// ones included (the caller decides what to report).
	GetTokenByHash(ctx context.Context, secretHash string) (*APIToken, error)
	GetToken(ctx context.Context, id string) (*APIToken, error)
	RevokeToken(ctx context.Context, id string, at time.Time) error
	TouchToken(ctx context.Context, id string, at time.Time) error
	// SetUserRole updates a user's role (OIDC re-maps it on every sign-in).
	SetUserRole(ctx context.Context, userID string, role model.Role) error
}
