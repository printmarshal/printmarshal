package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"printmarshal/internal/model"
)

// --- device logs (printer syslog receiver) ---------------------------------

func (s *Postgres) AddDeviceLog(ctx context.Context, l *model.DeviceLog) error {
	if l.At.IsZero() {
		l.At = time.Now().UTC()
	}
	err := pgRow(ctx, s.db, `INSERT INTO device_logs(device_id, at, facility, severity, message) VALUES (?,?,?,?,?) RETURNING id`,
		l.DeviceID, pgt(l.At), l.Facility, l.Severity, l.Message).Scan(&l.ID)
	return pgErr(err)
}

func (s *Postgres) ListDeviceLogs(ctx context.Context, deviceID string, limit int) ([]*model.DeviceLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := pgQuery(ctx, s.db, `SELECT id, device_id, at, facility, severity, message FROM device_logs WHERE device_id=? ORDER BY id DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.DeviceLog{}
	for rows.Next() {
		var l model.DeviceLog
		var at time.Time
		if err := rows.Scan(&l.ID, &l.DeviceID, &at, &l.Facility, &l.Severity, &l.Message); err != nil {
			return nil, err
		}
		l.At = at.UTC()
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *Postgres) PruneDeviceLogs(ctx context.Context, before time.Time) (int64, error) {
	res, err := pgExec(ctx, s.db, `DELETE FROM device_logs WHERE at < ?`, pgt(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- tombstones --------------------------------------------------------------

func (s *Postgres) AddTombstone(ctx context.Context, collection, id string) error {
	_, err := pgExec(ctx, s.db, `INSERT INTO tombstones(collection, id, deleted_at) VALUES (?,?,?)
		ON CONFLICT(collection, id) DO UPDATE SET deleted_at=excluded.deleted_at`, collection, id, nowUTC())
	return err
}

// --- incremental sync ----------------------------------------------------------
//
// Same contract as the SQLite store: rows strictly after the cursor, ordered
// by (<updated column>, id). Timestamps are TIMESTAMPTZ (microseconds), so the
// cursor carries a fixed-width UTC string (cursorTime) that sorts the same
// way as the column, parses back to the exact stored instant, and keeps
// mergeSync's string comparison correct for tombstones.

const cursorLayout = "2006-01-02T15:04:05.000000Z"

func cursorTime(t time.Time) string { return t.UTC().Format(cursorLayout) }

// cursorArg turns a cursor timestamp back into the TIMESTAMPTZ it compares
// against. Both this store's layout and RFC 3339 cursors are accepted.
func cursorArg(at string) time.Time {
	if t, err := time.Parse(cursorLayout, at); err == nil {
		return t
	}
	return parseTime(at)
}

func syncWherePG(col string, c SyncCursor) (string, []any) {
	if c.Zero() {
		return "", nil
	}
	at := cursorArg(c.At)
	return fmt.Sprintf(" WHERE (%s > ? OR (%s = ? AND id > ?))", col, col), []any{at, at, c.ID}
}

func syncWhereIntPG(col string, c SyncCursor) (string, []any) {
	if c.Zero() {
		return "", nil
	}
	id, _ := strconv.ParseInt(c.ID, 10, 64)
	at := cursorArg(c.At)
	return fmt.Sprintf(" WHERE (%s > ? OR (%s = ? AND id > ?))", col, col), []any{at, at, id}
}

func (s *Postgres) SyncDevices(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWherePG("updated_at", after)
	args = append(args, limit+1)
	rows, err := pgQuery(ctx, s.db, `SELECT `+deviceCols+` FROM devices`+where+` ORDER BY updated_at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var live []SyncRow
	for rows.Next() {
		d, err := scanDevicePG(rows)
		if err != nil {
			return nil, false, err
		}
		live = append(live, SyncRow{Cursor: SyncCursor{At: cursorTime(d.UpdatedAt), ID: d.ID}, Item: d})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	dead, err := s.syncTombstones(ctx, "devices", after, limit+1)
	if err != nil {
		return nil, false, err
	}
	r, more := cut(mergeSync(live, dead), limit)
	return r, more, nil
}

func (s *Postgres) syncTombstones(ctx context.Context, collection string, after SyncCursor, limit int) ([]SyncRow, error) {
	q := `SELECT id, deleted_at FROM tombstones WHERE collection=?`
	args := []any{collection}
	if !after.Zero() {
		at := cursorArg(after.At)
		q += ` AND (deleted_at > ? OR (deleted_at = ? AND id > ?))`
		args = append(args, at, at, after.ID)
	}
	q += ` ORDER BY deleted_at, id LIMIT ?`
	args = append(args, limit)
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out = append(out, SyncRow{Cursor: SyncCursor{At: cursorTime(at), ID: id}, Item: &Tombstone{ID: id, Deleted: true, UpdatedAt: at.UTC()}})
	}
	return out, rows.Err()
}

func (s *Postgres) SyncAlerts(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWherePG("updated_at", after)
	args = append(args, limit+1)
	rows, err := pgQuery(ctx, s.db, `SELECT `+alertCols+` FROM alerts`+where+` ORDER BY updated_at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		a, err := scanAlertPG(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, SyncRow{Cursor: SyncCursor{At: cursorTime(a.UpdatedAt), ID: a.ID}, Item: a})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

func (s *Postgres) SyncAudit(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhereIntPG("at", after)
	args = append(args, limit+1)
	rows, err := pgQuery(ctx, s.db, `SELECT id,actor,action,target,before,after,result,at FROM audit`+where+` ORDER BY at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var e model.AuditEntry
		var before, afterJSON sql.NullString
		var at time.Time
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &before, &afterJSON, &e.Result, &at); err != nil {
			return nil, false, err
		}
		if before.Valid {
			e.Before = map[string]any{}
			fromJSON(before, &e.Before)
		}
		if afterJSON.Valid {
			e.After = map[string]any{}
			fromJSON(afterJSON, &e.After)
		}
		e.At = at.UTC()
		out = append(out, SyncRow{Cursor: SyncCursor{At: cursorTime(e.At), ID: strconv.FormatInt(e.ID, 10)}, Item: &e})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

func (s *Postgres) SyncJobs(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWherePG(jobUpdated, after)
	args = append(args, limit+1)
	rows, err := pgQuery(ctx, s.db, `SELECT `+jobCols+`, `+jobUpdated+` AS cur FROM jobs`+where+` ORDER BY cur, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var j model.Job
		var target, params string
		var created, cur time.Time
		var started, finished sql.NullTime
		if err := rows.Scan(&j.ID, &j.Type, &target, &params, &j.Status, &j.Total, &j.Done, &j.Failed, &j.Error, &j.CreatedBy, &created, &started, &finished, &cur); err != nil {
			return nil, false, err
		}
		j.Target, j.Params = map[string]any{}, map[string]any{}
		fromJSON(sql.NullString{String: target, Valid: true}, &j.Target)
		fromJSON(sql.NullString{String: params, Valid: true}, &j.Params)
		j.CreatedAt = created.UTC()
		j.StartedAt, j.FinishedAt = timeOf(started), timeOf(finished)
		out = append(out, SyncRow{Cursor: SyncCursor{At: cursorTime(cur), ID: j.ID}, Item: &j})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

func (s *Postgres) SyncConfigWrites(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWherePG("at", after)
	args = append(args, limit+1)
	rows, err := pgQuery(ctx, s.db, `SELECT `+writeCols+` FROM config_writes`+where+` ORDER BY at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		w, err := scanWritePG(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, SyncRow{Cursor: SyncCursor{At: cursorTime(w.Result.At), ID: w.ID}, Item: w})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}
