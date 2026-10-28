package store

import (
	"context"
	"database/sql"
	"time"

	"printmarshal/internal/model"
)

func scanTokenPG(r rowScanner) (*APIToken, error) {
	var t APIToken
	var scope string
	var created time.Time
	var expires, lastUsed, revoked sql.NullTime
	if err := r.Scan(&t.ID, &t.Name, &t.Role, &scope, &expires, &t.CreatedBy, &created, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	t.Scope = []string{}
	fromJSON(sql.NullString{String: scope, Valid: true}, &t.Scope)
	t.CreatedAt = created.UTC()
	t.ExpiresAt, t.LastUsedAt, t.RevokedAt = timeOf(expires), timeOf(lastUsed), timeOf(revoked)
	return &t, nil
}

func (s *Postgres) CreateToken(ctx context.Context, t *APIToken, secretHash string) error {
	if t.ID == "" {
		t.ID = newID()
	}
	n := nowUTC()
	t.CreatedAt = n
	if t.Scope == nil {
		t.Scope = []string{}
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO api_tokens(id,name,secret_hash,role,scope,expires_at,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, secretHash, string(t.Role), toJSON(t.Scope), pgtp(t.ExpiresAt), t.CreatedBy, n)
	return err
}

func (s *Postgres) ListTokens(ctx context.Context) ([]*APIToken, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+tokenCols+` FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*APIToken{}
	for rows.Next() {
		t, err := scanTokenPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Postgres) GetTokenByHash(ctx context.Context, secretHash string) (*APIToken, error) {
	t, err := scanTokenPG(pgRow(ctx, s.db, `SELECT `+tokenCols+` FROM api_tokens WHERE secret_hash=?`, secretHash))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Postgres) GetToken(ctx context.Context, id string) (*APIToken, error) {
	t, err := scanTokenPG(pgRow(ctx, s.db, `SELECT `+tokenCols+` FROM api_tokens WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Postgres) RevokeToken(ctx context.Context, id string, at time.Time) error {
	return affected(pgExec(ctx, s.db, `UPDATE api_tokens SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, pgt(at), id))
}

func (s *Postgres) TouchToken(ctx context.Context, id string, at time.Time) error {
	_, err := pgExec(ctx, s.db, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, pgt(at), id)
	return err
}

func (s *Postgres) SetUserRole(ctx context.Context, userID string, role model.Role) error {
	return affected(pgExec(ctx, s.db, `UPDATE users SET role=? WHERE id=?`, string(role), userID))
}
