package store

import (
	"context"
	"database/sql"
	"errors"

	"printmarshal/internal/catalog"
)

// Compile-time check that *SQLite implements CatalogStore.
var _ CatalogStore = (*SQLite)(nil)

func (s *SQLite) SetFamilyQualification(ctx context.Context, q *catalog.Qualification) error {
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = parseTime(now())
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO catalog_qualifications(family_id,status,note,actor,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(family_id) DO UPDATE SET status=excluded.status, note=excluded.note, actor=excluded.actor, updated_at=excluded.updated_at`,
		q.FamilyID, q.Status, q.Note, q.Actor, ts(q.UpdatedAt))
	return err
}

func (s *SQLite) GetFamilyQualification(ctx context.Context, familyID string) (*catalog.Qualification, error) {
	var q catalog.Qualification
	var at string
	err := s.db.QueryRowContext(ctx, `SELECT family_id,status,note,actor,updated_at FROM catalog_qualifications WHERE family_id=?`, familyID).
		Scan(&q.FamilyID, &q.Status, &q.Note, &q.Actor, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	q.UpdatedAt = parseTime(at)
	return &q, nil
}

func (s *SQLite) ListFamilyQualifications(ctx context.Context) (map[string]*catalog.Qualification, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT family_id,status,note,actor,updated_at FROM catalog_qualifications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*catalog.Qualification{}
	for rows.Next() {
		var q catalog.Qualification
		var at string
		if err := rows.Scan(&q.FamilyID, &q.Status, &q.Note, &q.Actor, &at); err != nil {
			return nil, err
		}
		q.UpdatedAt = parseTime(at)
		out[q.FamilyID] = &q
	}
	return out, rows.Err()
}
