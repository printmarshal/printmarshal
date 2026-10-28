package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- credentials -----------------------------------------------------------

func (s *SQLite) CreateCredential(ctx context.Context, c *model.Credential, secret *model.Secret) error {
	if s.box == nil {
		return fmt.Errorf("credential store has no encryption key")
	}
	if c.ID == "" {
		c.ID = newID()
	}
	plain, err := json.Marshal(secret)
	if err != nil {
		return err
	}
	enc, err := s.box.Seal(plain)
	if err != nil {
		return err
	}
	if c.Purpose == "" {
		c.Purpose = model.PurposeRead
	}
	if !c.Purpose.Valid() {
		return fmt.Errorf("credential purpose %q is not read, write or both", c.Purpose)
	}
	n := now()
	_, err = s.db.ExecContext(ctx, `INSERT INTO credentials(id,name,type,scope,username,priority,purpose,secret,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, string(c.Type), c.Scope, c.Username, c.Priority, string(c.Purpose), enc, n, n)
	if err == nil {
		c.CreatedAt, c.UpdatedAt = parseTime(n), parseTime(n)
	}
	return err
}

func (s *SQLite) ListCredentials(ctx context.Context) ([]*model.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+credentialCols+` FROM credentials ORDER BY priority DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Credential{}
	for rows.Next() {
		c, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) DeleteCredential(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveCreds implements Store: the read-capable credentials (purpose read or
// both), device-scoped first, then group-scoped, then global. Write-only
// credentials are never returned here; see ResolveWriteCreds.
func (s *SQLite) ResolveCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error) {
	creds, _, err := s.resolveCreds(ctx, deviceID, groupIDs, model.CredentialPurpose.CanRead)
	return creds, err
}

// --- jobs ------------------------------------------------------------------

const jobCols = `id,type,target,params,status,total,done,failed,error,created_by,created_at,started_at,finished_at`

func scanJob(r rowScanner) (*model.Job, error) {
	var j model.Job
	var target, params, created string
	var started, finished sql.NullString
	if err := r.Scan(&j.ID, &j.Type, &target, &params, &j.Status, &j.Total, &j.Done, &j.Failed, &j.Error, &j.CreatedBy, &created, &started, &finished); err != nil {
		return nil, err
	}
	j.Target, j.Params = map[string]any{}, map[string]any{}
	fromJSON(sql.NullString{String: target, Valid: true}, &j.Target)
	fromJSON(sql.NullString{String: params, Valid: true}, &j.Params)
	j.CreatedAt = parseTime(created)
	j.StartedAt, j.FinishedAt = parseTimeP(started), parseTimeP(finished)
	return &j, nil
}

func (s *SQLite) CreateJob(ctx context.Context, j *model.Job) error {
	if j.ID == "" {
		j.ID = newID()
	}
	if j.Status == "" {
		j.Status = model.JobQueued
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO jobs(`+jobCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Type, toJSON(j.Target), toJSON(j.Params), string(j.Status), j.Total, j.Done, j.Failed, j.Error, j.CreatedBy,
		ts(j.CreatedAt), tsp(j.StartedAt), tsp(j.FinishedAt))
	return err
}

func (s *SQLite) UpdateJob(ctx context.Context, j *model.Job) error {
	_, err := s.db.ExecContext(ctx, `UPDATE jobs SET status=?, total=?, done=?, failed=?, error=?, started_at=?, finished_at=? WHERE id=?`,
		string(j.Status), j.Total, j.Done, j.Failed, j.Error, tsp(j.StartedAt), tsp(j.FinishedAt), j.ID)
	return err
}

func (s *SQLite) GetJob(ctx context.Context, id string) (*model.Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return j, err
}

func (s *SQLite) ListJobs(ctx context.Context, f JobFilter) ([]*model.Job, error) {
	var where []string
	var args []any
	if f.Type != "" {
		where = append(where, "type=?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	q := `SELECT ` + jobCols + ` FROM jobs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *SQLite) AddJobResult(ctx context.Context, r *model.JobResult) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	var data any
	if r.Data != nil {
		data = toJSON(r.Data)
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO job_results(job_id,device_id,address,status,message,data,at) VALUES(?,?,?,?,?,?,?)`,
		r.JobID, r.DeviceID, r.Address, r.Status, r.Message, data, ts(r.At))
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLite) ListJobResults(ctx context.Context, jobID string) ([]*model.JobResult, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,job_id,device_id,address,status,message,data,at FROM job_results WHERE job_id=? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.JobResult{}
	for rows.Next() {
		var r model.JobResult
		var data sql.NullString
		var at string
		if err := rows.Scan(&r.ID, &r.JobID, &r.DeviceID, &r.Address, &r.Status, &r.Message, &data, &at); err != nil {
			return nil, err
		}
		if data.Valid {
			r.Data = map[string]any{}
			fromJSON(data, &r.Data)
		}
		r.At = parseTime(at)
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkInterruptedJobs fails jobs left running by a previous process (spec §3 principle 7).
func (s *SQLite) MarkInterruptedJobs(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE jobs SET status='failed', error='interrupted by restart', finished_at=? WHERE status IN ('queued','running')`, now())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// --- telemetry -------------------------------------------------------------

func (s *SQLite) AddSamples(ctx context.Context, samples []model.TelemetrySample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO telemetry(device_id,metric,label,value,at) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, sm := range samples {
		at := sm.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, sm.DeviceID, sm.Metric, sm.Label, sm.Value, ts(at)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) QueryTelemetry(ctx context.Context, q TelemetryQuery) ([]model.TelemetrySample, error) {
	where := []string{"device_id=?"}
	args := []any{q.DeviceID}
	if q.Metric != "" {
		where = append(where, "metric=?")
		args = append(args, q.Metric)
	}
	if q.Label != "" {
		where = append(where, "label=?")
		args = append(args, q.Label)
	}
	if !q.From.IsZero() {
		where = append(where, "at>=?")
		args = append(args, ts(q.From))
	}
	if !q.To.IsZero() {
		where = append(where, "at<=?")
		args = append(args, ts(q.To))
	}
	limit := 5000
	if q.Limit > 0 {
		limit = q.Limit
	}
	rows, err := s.db.QueryContext(ctx, `SELECT device_id,metric,label,value,at FROM telemetry WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(" ORDER BY at LIMIT %d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TelemetrySample{}
	for rows.Next() {
		var sm model.TelemetrySample
		var at string
		if err := rows.Scan(&sm.DeviceID, &sm.Metric, &sm.Label, &sm.Value, &at); err != nil {
			return nil, err
		}
		sm.At = parseTime(at)
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *SQLite) PruneTelemetry(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM telemetry WHERE at < ?`, ts(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- alerts ----------------------------------------------------------------

const alertCols = `id,device_id,severity,source,key,description,raised_at,updated_at,cleared_at,acked_at,acked_by`

func scanAlert(r rowScanner) (*model.Alert, error) {
	var a model.Alert
	var raised, upd string
	var cleared, acked sql.NullString
	if err := r.Scan(&a.ID, &a.DeviceID, &a.Severity, &a.Source, &a.Key, &a.Description, &raised, &upd, &cleared, &acked, &a.AckedBy); err != nil {
		return nil, err
	}
	a.RaisedAt, a.UpdatedAt = parseTime(raised), parseTime(upd)
	a.ClearedAt, a.AckedAt = parseTimeP(cleared), parseTimeP(acked)
	return &a, nil
}

func (s *SQLite) ListAlerts(ctx context.Context, f AlertFilter) ([]*model.Alert, error) {
	var where []string
	var args []any
	if f.DeviceID != "" {
		where = append(where, "device_id=?")
		args = append(args, f.DeviceID)
	}
	if f.ActiveOnly {
		where = append(where, "cleared_at IS NULL")
	}
	if f.Severity != "" {
		where = append(where, "severity=?")
		args = append(args, f.Severity)
	}
	q := `SELECT ` + alertCols + ` FROM alerts`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY (cleared_at IS NOT NULL), CASE severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, raised_at DESC"
	limit := 500
	if f.Limit > 0 {
		limit = f.Limit
	}
	q += fmt.Sprintf(" LIMIT %d", limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLite) GetAlert(ctx context.Context, id string) (*model.Alert, error) {
	a, err := scanAlert(s.db.QueryRowContext(ctx, `SELECT `+alertCols+` FROM alerts WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return a, err
}

// RaiseAlert creates an active alert for (device,key) or refreshes the existing one.
func (s *SQLite) RaiseAlert(ctx context.Context, deviceID, key, source string, sev model.Severity, desc string) (*model.Alert, bool, error) {
	existing, err := scanAlert(s.db.QueryRowContext(ctx, `SELECT `+alertCols+` FROM alerts WHERE device_id=? AND key=? AND cleared_at IS NULL`, deviceID, key))
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}
	n := now()
	if existing != nil {
		if existing.Severity != sev || existing.Description != desc {
			if _, err := s.db.ExecContext(ctx, `UPDATE alerts SET severity=?, description=?, updated_at=? WHERE id=?`, string(sev), desc, n, existing.ID); err != nil {
				return nil, false, err
			}
			existing.Severity, existing.Description = sev, desc
		}
		return existing, false, nil
	}
	a := &model.Alert{ID: newID(), DeviceID: deviceID, Severity: sev, Source: source, Key: key, Description: desc, RaisedAt: parseTime(n), UpdatedAt: parseTime(n)}
	_, err = s.db.ExecContext(ctx, `INSERT INTO alerts(`+alertCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.DeviceID, string(a.Severity), a.Source, a.Key, a.Description, n, n, nil, nil, "")
	return a, true, err
}

func (s *SQLite) ClearAlert(ctx context.Context, id string) (*model.Alert, error) {
	n := now()
	res, err := s.db.ExecContext(ctx, `UPDATE alerts SET cleared_at=?, updated_at=? WHERE id=? AND cleared_at IS NULL`, n, n, id)
	if err != nil {
		return nil, err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return nil, ErrNotFound
	}
	return s.GetAlert(ctx, id)
}

// ClearAlertsExcept clears active alerts of a source on a device whose key is not in keepKeys.
func (s *SQLite) ClearAlertsExcept(ctx context.Context, deviceID, source string, keepKeys []string) ([]*model.Alert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+alertCols+` FROM alerts WHERE device_id=? AND source=? AND cleared_at IS NULL`, deviceID, source)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, k := range keepKeys {
		keep[k] = true
	}
	var toClear []*model.Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if !keep[a.Key] {
			toClear = append(toClear, a)
		}
	}
	rows.Close()
	n := now()
	for _, a := range toClear {
		if _, err := s.db.ExecContext(ctx, `UPDATE alerts SET cleared_at=?, updated_at=? WHERE id=?`, n, n, a.ID); err != nil {
			return nil, err
		}
		t := parseTime(n)
		a.ClearedAt = &t
	}
	return toClear, nil
}

func (s *SQLite) AckAlert(ctx context.Context, id, by string) (*model.Alert, error) {
	n := now()
	res, err := s.db.ExecContext(ctx, `UPDATE alerts SET acked_at=?, acked_by=?, updated_at=? WHERE id=?`, n, by, n, id)
	if err != nil {
		return nil, err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return nil, ErrNotFound
	}
	return s.GetAlert(ctx, id)
}

// --- notifications / audit --------------------------------------------------

func (s *SQLite) AddNotificationLog(ctx context.Context, l *model.NotificationLog) error {
	if l.At.IsZero() {
		l.At = time.Now().UTC()
	}
	ok := 0
	if l.OK {
		ok = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_log(notifier,event,target,ok,error,at) VALUES(?,?,?,?,?,?)`, l.Notifier, l.Event, l.Target, ok, l.Error, ts(l.At))
	return err
}

func (s *SQLite) ListNotificationLog(ctx context.Context, limit int) ([]*model.NotificationLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id,notifier,event,target,ok,error,at FROM notification_log ORDER BY id DESC LIMIT %d`, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.NotificationLog{}
	for rows.Next() {
		var l model.NotificationLog
		var ok int
		var at string
		if err := rows.Scan(&l.ID, &l.Notifier, &l.Event, &l.Target, &ok, &l.Error, &at); err != nil {
			return nil, err
		}
		l.OK, l.At = ok == 1, parseTime(at)
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *SQLite) AddAudit(ctx context.Context, e *model.AuditEntry) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	var before, after any
	if e.Before != nil {
		before = toJSON(e.Before)
	}
	if e.After != nil {
		after = toJSON(e.After)
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO audit(actor,action,target,before,after,result,at) VALUES(?,?,?,?,?,?,?)`,
		e.Actor, e.Action, e.Target, before, after, e.Result, ts(e.At))
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

func (s *SQLite) ListAudit(ctx context.Context, limit int) ([]*model.AuditEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id,actor,action,target,before,after,result,at FROM audit ORDER BY id DESC LIMIT %d`, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		var before, after sql.NullString
		var at string
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &before, &after, &e.Result, &at); err != nil {
			return nil, err
		}
		if before.Valid {
			e.Before = map[string]any{}
			fromJSON(before, &e.Before)
		}
		if after.Valid {
			e.After = map[string]any{}
			fromJSON(after, &e.After)
		}
		e.At = parseTime(at)
		out = append(out, &e)
	}
	return out, rows.Err()
}

// --- users / sessions ------------------------------------------------------

func (s *SQLite) CreateUser(ctx context.Context, u *model.User, passwordHash string) error {
	if u.ID == "" {
		u.ID = newID()
	}
	n := now()
	u.CreatedAt = parseTime(n)
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id,username,password_hash,role,created_at) VALUES(?,?,?,?,?)`, u.ID, u.Username, passwordHash, string(u.Role), n)
	return err
}

func (s *SQLite) GetUserByName(ctx context.Context, username string) (*model.User, string, error) {
	var u model.User
	var hash, created string
	err := s.db.QueryRowContext(ctx, `SELECT id,username,password_hash,role,created_at FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &hash, &u.Role, &created)
	if err == sql.ErrNoRows {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	u.CreatedAt = parseTime(created)
	return &u, hash, nil
}

func (s *SQLite) GetUser(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id,username,role,created_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Username, &u.Role, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = parseTime(created)
	return &u, nil
}

func (s *SQLite) ListUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,role,created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		var u model.User
		var created string
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created); err != nil {
			return nil, err
		}
		u.CreatedAt = parseTime(created)
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (s *SQLite) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *SQLite) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) SetPassword(ctx context.Context, id, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, passwordHash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, tokenHash, userID, ts(expires), now())
	return err
}

func (s *SQLite) GetSession(ctx context.Context, tokenHash string) (*model.User, error) {
	var userID, expires string
	err := s.db.QueryRowContext(ctx, `SELECT user_id,expires_at FROM sessions WHERE token_hash=?`, tokenHash).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if parseTime(expires).Before(time.Now()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
		return nil, ErrNotFound
	}
	return s.GetUser(ctx, userID)
}

func (s *SQLite) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *SQLite) DeleteUserSessions(ctx context.Context, userID, keepHash string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, userID, keepHash)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
