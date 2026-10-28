package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"printmarshal/internal/model"
)

func (s *SQLite) EnqueueNotification(ctx context.Context, o *model.Outbox) error {
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
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO notification_outbox(event_id, event, notifier, subject, body, payload, state, attempts, last_error, next_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		o.EventID, o.Event, o.Notifier, o.Subject, o.Body, nullIfEmpty(string(payload)), o.State, o.Attempts, o.LastError, ts(o.NextAt), ts(o.CreatedAt))
	if err != nil {
		return err
	}
	o.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLite) DueNotifications(ctx context.Context, limit int) ([]*model.Outbox, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, event, notifier, subject, body, payload, state, attempts, last_error, next_at, created_at, done_at
		FROM notification_outbox WHERE state='pending' AND next_at <= ? ORDER BY next_at LIMIT ?`, ts(time.Now().UTC()), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOutbox(rows)
}

func (s *SQLite) ListOutbox(ctx context.Context, state string, limit int) ([]*model.Outbox, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, event_id, event, notifier, subject, body, payload, state, attempts, last_error, next_at, created_at, done_at
		FROM notification_outbox WHERE (?='' OR state=?) ORDER BY id DESC LIMIT ?`, state, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOutbox(rows)
}

func (s *SQLite) UpdateNotification(ctx context.Context, o *model.Outbox) error {
	var done *string
	if o.DoneAt != nil {
		v := ts(*o.DoneAt)
		done = &v
	}
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET state=?, attempts=?, last_error=?, next_at=?, done_at=? WHERE id=?`,
		o.State, o.Attempts, o.LastError, ts(o.NextAt), done, o.ID)
	return err
}

func (s *SQLite) RequeueDead(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET state='pending', attempts=0, next_at=?, done_at=NULL WHERE id=? AND state='dead'`, ts(time.Now().UTC()), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanOutbox(rows *sql.Rows) ([]*model.Outbox, error) {
	out := []*model.Outbox{}
	for rows.Next() {
		var o model.Outbox
		var payload, next, created sql.NullString
		var done sql.NullString
		if err := rows.Scan(&o.ID, &o.EventID, &o.Event, &o.Notifier, &o.Subject, &o.Body, &payload, &o.State, &o.Attempts, &o.LastError, &next, &created, &done); err != nil {
			return nil, err
		}
		if payload.Valid && payload.String != "" {
			var v any
			if json.Unmarshal([]byte(payload.String), &v) == nil {
				o.Payload = v
			}
		}
		o.NextAt = parseTime(next.String)
		o.CreatedAt = parseTime(created.String)
		o.DoneAt = parseTimeP(done)
		out = append(out, &o)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *SQLite) UpdateNotificationText(ctx context.Context, o *model.Outbox) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET subject=?, body=? WHERE id=?`, o.Subject, o.Body, o.ID)
	return err
}
