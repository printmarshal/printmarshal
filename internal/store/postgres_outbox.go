package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"printmarshal/internal/model"
)

const outboxCols = `id, event_id, event, notifier, subject, body, payload, state, attempts, last_error, next_at, created_at, done_at`

func (s *Postgres) EnqueueNotification(ctx context.Context, o *model.Outbox) error {
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	if o.NextAt.IsZero() {
		o.NextAt = o.CreatedAt
	}
	if o.State == "" {
		o.State = "pending"
	}
	var payload []byte
	if o.Payload != nil {
		payload, _ = json.Marshal(o.Payload)
	}
	// INSERT OR IGNORE semantics: a duplicate (event, notifier) leaves ID at 0.
	err := pgRow(ctx, s.db, `INSERT INTO notification_outbox(event_id, event, notifier, subject, body, payload, state, attempts, last_error, next_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT (event_id, notifier) DO NOTHING RETURNING id`,
		o.EventID, o.Event, o.Notifier, o.Subject, o.Body, nullIfEmpty(string(payload)), o.State, o.Attempts, o.LastError, pgt(o.NextAt), pgt(o.CreatedAt)).Scan(&o.ID)
	if err == sql.ErrNoRows {
		return nil
	}
	return pgErr(err)
}

func (s *Postgres) DueNotifications(ctx context.Context, limit int) ([]*model.Outbox, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+outboxCols+` FROM notification_outbox WHERE state='pending' AND next_at <= ? ORDER BY next_at LIMIT ?`, nowUTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOutboxPG(rows)
}

func (s *Postgres) ListOutbox(ctx context.Context, state string, limit int) ([]*model.Outbox, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := pgQuery(ctx, s.db, `SELECT `+outboxCols+` FROM notification_outbox WHERE (?::text='' OR state=?) ORDER BY id DESC LIMIT ?`, state, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOutboxPG(rows)
}

func (s *Postgres) UpdateNotification(ctx context.Context, o *model.Outbox) error {
	_, err := pgExec(ctx, s.db, `UPDATE notification_outbox SET state=?, attempts=?, last_error=?, next_at=?, done_at=? WHERE id=?`,
		o.State, o.Attempts, o.LastError, pgt(o.NextAt), pgtp(o.DoneAt), o.ID)
	return err
}

func (s *Postgres) RequeueDead(ctx context.Context, id int64) error {
	return affected(pgExec(ctx, s.db, `UPDATE notification_outbox SET state='pending', attempts=0, next_at=?, done_at=NULL WHERE id=? AND state='dead'`, nowUTC(), id))
}

func scanOutboxPG(rows *sql.Rows) ([]*model.Outbox, error) {
	out := []*model.Outbox{}
	for rows.Next() {
		var o model.Outbox
		var payload sql.NullString
		var next, created time.Time
		var done sql.NullTime
		if err := rows.Scan(&o.ID, &o.EventID, &o.Event, &o.Notifier, &o.Subject, &o.Body, &payload, &o.State, &o.Attempts, &o.LastError, &next, &created, &done); err != nil {
			return nil, err
		}
		if payload.Valid && payload.String != "" {
			var v any
			if json.Unmarshal([]byte(payload.String), &v) == nil {
				o.Payload = v
			}
		}
		o.NextAt = next.UTC()
		o.CreatedAt = created.UTC()
		o.DoneAt = timeOf(done)
		out = append(out, &o)
	}
	return out, rows.Err()
}

func (s *Postgres) UpdateNotificationText(ctx context.Context, o *model.Outbox) error {
	_, err := pgExec(ctx, s.db, `UPDATE notification_outbox SET subject=?, body=? WHERE id=?`, o.Subject, o.Body, o.ID)
	return err
}
