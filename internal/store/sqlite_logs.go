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

func (s *SQLite) AddDeviceLog(ctx context.Context, l *model.DeviceLog) error {
	if l.At.IsZero() {
		l.At = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO device_logs(device_id, at, facility, severity, message) VALUES (?,?,?,?,?)`,
		l.DeviceID, ts(l.At), l.Facility, l.Severity, l.Message)
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLite) ListDeviceLogs(ctx context.Context, deviceID string, limit int) ([]*model.DeviceLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, device_id, at, facility, severity, message FROM device_logs WHERE device_id=? ORDER BY id DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.DeviceLog{}
	for rows.Next() {
		var l model.DeviceLog
		var at string
		if err := rows.Scan(&l.ID, &l.DeviceID, &at, &l.Facility, &l.Severity, &l.Message); err != nil {
			return nil, err
		}
		l.At = parseTime(at)
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *SQLite) PruneDeviceLogs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM device_logs WHERE at < ?`, ts(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- tombstones --------------------------------------------------------------

func (s *SQLite) AddTombstone(ctx context.Context, collection, id string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO tombstones(collection, id, deleted_at) VALUES (?,?,?)`, collection, id, now())
	return err
}

// --- incremental sync ----------------------------------------------------------
//
// Every query orders by (<updated column>, id) and applies the cursor with a
// strict comparison on the same expression, fetching limit+1 rows to learn
// whether more remain. Timestamps are compared as the stored strings; every
// timestamp this store writes is ts() (UTC RFC 3339 with nanoseconds), and
// ts(parseTime(x)) == x for such strings, so a cursor rebuilt from the parsed
// model value is byte-identical to what SQL compared.

// syncWhere builds the cursor predicate for a text id column.
func syncWhere(col string, c SyncCursor) (string, []any) {
	if c.Zero() {
		return "", nil
	}
	return fmt.Sprintf(" WHERE (%s > ? OR (%s = ? AND id > ?))", col, col), []any{c.At, c.At, c.ID}
}

// syncWhereInt is syncWhere for an integer id column.
func syncWhereInt(col string, c SyncCursor) (string, []any) {
	if c.Zero() {
		return "", nil
	}
	id, _ := strconv.ParseInt(c.ID, 10, 64)
	return fmt.Sprintf(" WHERE (%s > ? OR (%s = ? AND id > ?))", col, col), []any{c.At, c.At, id}
}

func syncLimit(limit int) int {
	if limit <= 0 {
		limit = 200
	}
	return limit
}

// cut trims a limit+1 result to limit and reports whether more remain.
func cut(rows []SyncRow, limit int) ([]SyncRow, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

func (s *SQLite) SyncDevices(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhere("updated_at", after)
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices`+where+` ORDER BY updated_at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var live []SyncRow
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, false, err
		}
		live = append(live, SyncRow{Cursor: SyncCursor{At: ts(d.UpdatedAt), ID: d.ID}, Item: d})
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

// syncTombstones lists the tombstones of a collection after the cursor.
func (s *SQLite) syncTombstones(ctx context.Context, collection string, after SyncCursor, limit int) ([]SyncRow, error) {
	q := `SELECT id, deleted_at FROM tombstones WHERE collection=?`
	args := []any{collection}
	if !after.Zero() {
		q += ` AND (deleted_at > ? OR (deleted_at = ? AND id > ?))`
		args = append(args, after.At, after.At, after.ID)
	}
	q += ` ORDER BY deleted_at, id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var id, at string
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out = append(out, SyncRow{Cursor: SyncCursor{At: at, ID: id}, Item: &Tombstone{ID: id, Deleted: true, UpdatedAt: parseTime(at)}})
	}
	return out, rows.Err()
}

// mergeSync merges two (cursor-ordered) row lists into one ordered list.
func mergeSync(a, b []SyncRow) []SyncRow {
	out := make([]SyncRow, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i].Cursor, b[j].Cursor
		if ca.At < cb.At || (ca.At == cb.At && ca.ID <= cb.ID) {
			out = append(out, a[i])
			i++
		} else {
			out = append(out, b[j])
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}

func (s *SQLite) SyncAlerts(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhere("updated_at", after)
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+alertCols+` FROM alerts`+where+` ORDER BY updated_at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var a model.Alert
		var raised, upd string
		var cleared, acked sql.NullString
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.Severity, &a.Source, &a.Key, &a.Description, &raised, &upd, &cleared, &acked, &a.AckedBy); err != nil {
			return nil, false, err
		}
		a.RaisedAt, a.UpdatedAt = parseTime(raised), parseTime(upd)
		a.ClearedAt, a.AckedAt = parseTimeP(cleared), parseTimeP(acked)
		out = append(out, SyncRow{Cursor: SyncCursor{At: upd, ID: a.ID}, Item: &a})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

func (s *SQLite) SyncAudit(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhereInt("at", after)
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT id,actor,action,target,before,after,result,at FROM audit`+where+` ORDER BY at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var e model.AuditEntry
		var before, afterJSON sql.NullString
		var at string
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
		e.At = parseTime(at)
		out = append(out, SyncRow{Cursor: SyncCursor{At: at, ID: strconv.FormatInt(e.ID, 10)}, Item: &e})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

// jobUpdated is the "last changed" expression for jobs, which have no
// updated_at column: the latest of finished, started and created.
const jobUpdated = `COALESCE(finished_at, started_at, created_at)`

func (s *SQLite) SyncJobs(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhere(jobUpdated, after)
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobCols+`, `+jobUpdated+` AS cur FROM jobs`+where+` ORDER BY cur, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		var j model.Job
		var target, params, created, cur string
		var started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.Type, &target, &params, &j.Status, &j.Total, &j.Done, &j.Failed, &j.Error, &j.CreatedBy, &created, &started, &finished, &cur); err != nil {
			return nil, false, err
		}
		j.Target, j.Params = map[string]any{}, map[string]any{}
		fromJSON(sql.NullString{String: target, Valid: true}, &j.Target)
		fromJSON(sql.NullString{String: params, Valid: true}, &j.Params)
		j.CreatedAt = parseTime(created)
		j.StartedAt, j.FinishedAt = parseTimeP(started), parseTimeP(finished)
		out = append(out, SyncRow{Cursor: SyncCursor{At: cur, ID: j.ID}, Item: &j})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}

func (s *SQLite) SyncConfigWrites(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error) {
	limit = syncLimit(limit)
	where, args := syncWhere("at", after)
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT `+writeCols+` FROM config_writes`+where+` ORDER BY at, id LIMIT ?`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []SyncRow
	for rows.Next() {
		w, err := scanWrite(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, SyncRow{Cursor: SyncCursor{At: ts(w.Result.At), ID: w.ID}, Item: w})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	r, more := cut(out, limit)
	return r, more, nil
}
