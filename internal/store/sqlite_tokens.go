package store

import (
	"context"
	"database/sql"
	"time"

	"printmarshal/internal/model"
)

const tokenCols = `id,name,role,scope,expires_at,created_by,created_at,last_used_at,revoked_at`

func scanToken(r rowScanner) (*APIToken, error) {
	var t APIToken
	var scope, created string
	var expires, lastUsed, revoked sql.NullString
	if err := r.Scan(&t.ID, &t.Name, &t.Role, &scope, &expires, &t.CreatedBy, &created, &lastUsed, &revoked); err != nil {
		return nil, err
	}
	t.Scope = []string{}
	fromJSON(sql.NullString{String: scope, Valid: true}, &t.Scope)
	t.CreatedAt = parseTime(created)
	t.ExpiresAt, t.LastUsedAt, t.RevokedAt = parseTimeP(expires), parseTimeP(lastUsed), parseTimeP(revoked)
	return &t, nil
}

func (s *SQLite) CreateToken(ctx context.Context, t *APIToken, secretHash string) error {
	if t.ID == "" {
		t.ID = newID()
	}
	n := now()
	t.CreatedAt = parseTime(n)
	if t.Scope == nil {
		t.Scope = []string{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO api_tokens(id,name,secret_hash,role,scope,expires_at,created_by,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		t.ID, t.Name, secretHash, string(t.Role), toJSON(t.Scope), tsp(t.ExpiresAt), t.CreatedBy, n)
	return err
}

func (s *SQLite) ListTokens(ctx context.Context) ([]*APIToken, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+tokenCols+` FROM api_tokens ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *SQLite) GetTokenByHash(ctx context.Context, secretHash string) (*APIToken, error) {
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE secret_hash=?`, secretHash))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *SQLite) GetToken(ctx context.Context, id string) (*APIToken, error) {
	t, err := scanToken(s.db.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *SQLite) RevokeToken(ctx context.Context, id string, at time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, ts(at), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) TouchToken(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_tokens SET last_used_at=? WHERE id=?`, ts(at), id)
	return err
}

func (s *SQLite) SetUserRole(ctx context.Context, userID string, role model.Role) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET role=? WHERE id=?`, string(role), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
