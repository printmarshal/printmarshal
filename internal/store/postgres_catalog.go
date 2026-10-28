package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"printmarshal/internal/catalog"
)

// Compile-time check that *Postgres implements CatalogStore.
var _ CatalogStore = (*Postgres)(nil)

func (s *Postgres) SetFamilyQualification(ctx context.Context, q *catalog.Qualification) error {
	if q.UpdatedAt.IsZero() {
		q.UpdatedAt = nowUTC()
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO catalog_qualifications(family_id,status,note,actor,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(family_id) DO UPDATE SET status=excluded.status, note=excluded.note, actor=excluded.actor, updated_at=excluded.updated_at`,
		q.FamilyID, q.Status, q.Note, q.Actor, pgt(q.UpdatedAt))
	return err
}

func (s *Postgres) GetFamilyQualification(ctx context.Context, familyID string) (*catalog.Qualification, error) {
	var q catalog.Qualification
	var at time.Time
	err := pgRow(ctx, s.db, `SELECT family_id,status,note,actor,updated_at FROM catalog_qualifications WHERE family_id=?`, familyID).
		Scan(&q.FamilyID, &q.Status, &q.Note, &q.Actor, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	q.UpdatedAt = at.UTC()
	return &q, nil
}

func (s *Postgres) ListFamilyQualifications(ctx context.Context) (map[string]*catalog.Qualification, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT family_id,status,note,actor,updated_at FROM catalog_qualifications`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*catalog.Qualification{}
	for rows.Next() {
		var q catalog.Qualification
		var at time.Time
		if err := rows.Scan(&q.FamilyID, &q.Status, &q.Note, &q.Actor, &at); err != nil {
			return nil, err
		}
		q.UpdatedAt = at.UTC()
		out[q.FamilyID] = &q
	}
	return out, rows.Err()
}
