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

func (s *Postgres) CreateCredential(ctx context.Context, c *model.Credential, secret *model.Secret) error {
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
	n := nowUTC()
	_, err = pgExec(ctx, s.db, `INSERT INTO credentials(id,name,type,scope,username,priority,purpose,secret,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, string(c.Type), c.Scope, c.Username, c.Priority, string(c.Purpose), enc, n, n)
	if err == nil {
		c.CreatedAt, c.UpdatedAt = n, n
	}
	return err
}

func (s *Postgres) ListCredentials(ctx context.Context) ([]*model.Credential, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+credentialCols+` FROM credentials ORDER BY priority DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Credential{}
	for rows.Next() {
		c, err := scanCredentialPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteCredential(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM credentials WHERE id=?`, id))
}

// ResolveCreds implements Store: the read-capable credentials (purpose read or
// both), device-scoped first, then group-scoped, then global. Write-only
// credentials are never returned here; see ResolveWriteCreds.
func (s *Postgres) ResolveCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error) {
	creds, _, err := s.resolveCreds(ctx, deviceID, groupIDs, model.CredentialPurpose.CanRead)
	return creds, err
}

// --- jobs ------------------------------------------------------------------

func scanJobPG(r rowScanner) (*model.Job, error) {
	var j model.Job
	var target, params string
	var created time.Time
	var started, finished sql.NullTime
	if err := r.Scan(&j.ID, &j.Type, &target, &params, &j.Status, &j.Total, &j.Done, &j.Failed, &j.Error, &j.CreatedBy, &created, &started, &finished); err != nil {
		return nil, err
	}
	j.Target, j.Params = map[string]any{}, map[string]any{}
	fromJSON(sql.NullString{String: target, Valid: true}, &j.Target)
	fromJSON(sql.NullString{String: params, Valid: true}, &j.Params)
	j.CreatedAt = created.UTC()
	j.StartedAt, j.FinishedAt = timeOf(started), timeOf(finished)
	return &j, nil
}

func (s *Postgres) CreateJob(ctx context.Context, j *model.Job) error {
	if j.ID == "" {
		j.ID = newID()
	}
	if j.Status == "" {
		j.Status = model.JobQueued
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO jobs(`+jobCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.Type, toJSON(j.Target), toJSON(j.Params), string(j.Status), j.Total, j.Done, j.Failed, j.Error, j.CreatedBy,
		pgt(j.CreatedAt), pgtp(j.StartedAt), pgtp(j.FinishedAt))
	return err
}

func (s *Postgres) UpdateJob(ctx context.Context, j *model.Job) error {
	_, err := pgExec(ctx, s.db, `UPDATE jobs SET status=?, total=?, done=?, failed=?, error=?, started_at=?, finished_at=? WHERE id=?`,
		string(j.Status), j.Total, j.Done, j.Failed, j.Error, pgtp(j.StartedAt), pgtp(j.FinishedAt), j.ID)
	return err
}

func (s *Postgres) GetJob(ctx context.Context, id string) (*model.Job, error) {
	j, err := scanJobPG(pgRow(ctx, s.db, `SELECT `+jobCols+` FROM jobs WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return j, err
}

func (s *Postgres) ListJobs(ctx context.Context, f JobFilter) ([]*model.Job, error) {
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
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Job{}
	for rows.Next() {
		j, err := scanJobPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Postgres) AddJobResult(ctx context.Context, r *model.JobResult) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	var data any
	if r.Data != nil {
		data = toJSON(r.Data)
	}
	err := pgRow(ctx, s.db, `INSERT INTO job_results(job_id,device_id,address,status,message,data,at) VALUES(?,?,?,?,?,?,?) RETURNING id`,
		r.JobID, r.DeviceID, r.Address, r.Status, r.Message, data, pgt(r.At)).Scan(&r.ID)
	return pgErr(err)
}

func (s *Postgres) ListJobResults(ctx context.Context, jobID string) ([]*model.JobResult, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT id,job_id,device_id,address,status,message,data,at FROM job_results WHERE job_id=? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.JobResult{}
	for rows.Next() {
		var r model.JobResult
		var data sql.NullString
		var at time.Time
		if err := rows.Scan(&r.ID, &r.JobID, &r.DeviceID, &r.Address, &r.Status, &r.Message, &data, &at); err != nil {
			return nil, err
		}
		if data.Valid {
			r.Data = map[string]any{}
			fromJSON(data, &r.Data)
		}
		r.At = at.UTC()
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkInterruptedJobs fails jobs left running by a previous process (spec §3 principle 7).
func (s *Postgres) MarkInterruptedJobs(ctx context.Context) (int, error) {
	res, err := pgExec(ctx, s.db, `UPDATE jobs SET status='failed', error='interrupted by restart', finished_at=? WHERE status IN ('queued','running')`, nowUTC())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// --- telemetry -------------------------------------------------------------

func (s *Postgres) AddSamples(ctx context.Context, samples []model.TelemetrySample) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, pgq(`INSERT INTO telemetry(device_id,metric,label,value,at) VALUES(?,?,?,?,?)`))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, sm := range samples {
		at := sm.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, sm.DeviceID, sm.Metric, sm.Label, sm.Value, pgt(at)); err != nil {
			return pgErr(err)
		}
	}
	return tx.Commit()
}

func (s *Postgres) QueryTelemetry(ctx context.Context, q TelemetryQuery) ([]model.TelemetrySample, error) {
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
		args = append(args, pgt(q.From))
	}
	if !q.To.IsZero() {
		where = append(where, "at<=?")
		args = append(args, pgt(q.To))
	}
	limit := 5000
	if q.Limit > 0 {
		limit = q.Limit
	}
	rows, err := pgQuery(ctx, s.db, `SELECT device_id,metric,label,value,at FROM telemetry WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(" ORDER BY at LIMIT %d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TelemetrySample{}
	for rows.Next() {
		var sm model.TelemetrySample
		var at time.Time
		if err := rows.Scan(&sm.DeviceID, &sm.Metric, &sm.Label, &sm.Value, &at); err != nil {
			return nil, err
		}
		sm.At = at.UTC()
		out = append(out, sm)
	}
	return out, rows.Err()
}

func (s *Postgres) PruneTelemetry(ctx context.Context, before time.Time) (int64, error) {
	res, err := pgExec(ctx, s.db, `DELETE FROM telemetry WHERE at < ?`, pgt(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- alerts ----------------------------------------------------------------

func scanAlertPG(r rowScanner) (*model.Alert, error) {
	var a model.Alert
	var raised, upd time.Time
	var cleared, acked sql.NullTime
	if err := r.Scan(&a.ID, &a.DeviceID, &a.Severity, &a.Source, &a.Key, &a.Description, &raised, &upd, &cleared, &acked, &a.AckedBy); err != nil {
		return nil, err
	}
	a.RaisedAt, a.UpdatedAt = raised.UTC(), upd.UTC()
	a.ClearedAt, a.AckedAt = timeOf(cleared), timeOf(acked)
	return &a, nil
}

func (s *Postgres) ListAlerts(ctx context.Context, f AlertFilter) ([]*model.Alert, error) {
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
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Alert{}
	for rows.Next() {
		a, err := scanAlertPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Postgres) GetAlert(ctx context.Context, id string) (*model.Alert, error) {
	a, err := scanAlertPG(pgRow(ctx, s.db, `SELECT `+alertCols+` FROM alerts WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return a, err
}

// RaiseAlert creates an active alert for (device,key) or refreshes the existing one.
func (s *Postgres) RaiseAlert(ctx context.Context, deviceID, key, source string, sev model.Severity, desc string) (*model.Alert, bool, error) {
	existing, err := scanAlertPG(pgRow(ctx, s.db, `SELECT `+alertCols+` FROM alerts WHERE device_id=? AND key=? AND cleared_at IS NULL ORDER BY raised_at DESC LIMIT 1`, deviceID, key))
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}
	n := nowUTC()
	if existing != nil {
		if existing.Severity != sev || existing.Description != desc {
			if _, err := pgExec(ctx, s.db, `UPDATE alerts SET severity=?, description=?, updated_at=? WHERE id=?`, string(sev), desc, n, existing.ID); err != nil {
				return nil, false, err
			}
			existing.Severity, existing.Description = sev, desc
		}
		return existing, false, nil
	}
	a := &model.Alert{ID: newID(), DeviceID: deviceID, Severity: sev, Source: source, Key: key, Description: desc, RaisedAt: n, UpdatedAt: n}
	_, err = pgExec(ctx, s.db, `INSERT INTO alerts(`+alertCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.DeviceID, string(a.Severity), a.Source, a.Key, a.Description, n, n, nil, nil, "")
	return a, true, err
}

func (s *Postgres) ClearAlert(ctx context.Context, id string) (*model.Alert, error) {
	n := nowUTC()
	if err := affected(pgExec(ctx, s.db, `UPDATE alerts SET cleared_at=?, updated_at=? WHERE id=? AND cleared_at IS NULL`, n, n, id)); err != nil {
		return nil, err
	}
	return s.GetAlert(ctx, id)
}

// ClearAlertsExcept clears active alerts of a source on a device whose key is not in keepKeys.
func (s *Postgres) ClearAlertsExcept(ctx context.Context, deviceID, source string, keepKeys []string) ([]*model.Alert, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+alertCols+` FROM alerts WHERE device_id=? AND source=? AND cleared_at IS NULL`, deviceID, source)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, k := range keepKeys {
		keep[k] = true
	}
	var toClear []*model.Alert
	for rows.Next() {
		a, err := scanAlertPG(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if !keep[a.Key] {
			toClear = append(toClear, a)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	n := nowUTC()
	for _, a := range toClear {
		if _, err := pgExec(ctx, s.db, `UPDATE alerts SET cleared_at=?, updated_at=? WHERE id=?`, n, n, a.ID); err != nil {
			return nil, err
		}
		t := n
		a.ClearedAt = &t
	}
	return toClear, nil
}

func (s *Postgres) AckAlert(ctx context.Context, id, by string) (*model.Alert, error) {
	n := nowUTC()
	if err := affected(pgExec(ctx, s.db, `UPDATE alerts SET acked_at=?, acked_by=?, updated_at=? WHERE id=?`, n, by, n, id)); err != nil {
		return nil, err
	}
	return s.GetAlert(ctx, id)
}

// --- notifications / audit --------------------------------------------------

func (s *Postgres) AddNotificationLog(ctx context.Context, l *model.NotificationLog) error {
	if l.At.IsZero() {
		l.At = time.Now().UTC()
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO notification_log(notifier,event,target,ok,error,at) VALUES(?,?,?,?,?,?)`, l.Notifier, l.Event, l.Target, l.OK, l.Error, pgt(l.At))
	return err
}

func (s *Postgres) ListNotificationLog(ctx context.Context, limit int) ([]*model.NotificationLog, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := pgQuery(ctx, s.db, fmt.Sprintf(`SELECT id,notifier,event,target,ok,error,at FROM notification_log ORDER BY id DESC LIMIT %d`, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.NotificationLog{}
	for rows.Next() {
		var l model.NotificationLog
		var at time.Time
		if err := rows.Scan(&l.ID, &l.Notifier, &l.Event, &l.Target, &l.OK, &l.Error, &at); err != nil {
			return nil, err
		}
		l.At = at.UTC()
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *Postgres) AddAudit(ctx context.Context, e *model.AuditEntry) error {
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
	err := pgRow(ctx, s.db, `INSERT INTO audit(actor,action,target,before,after,result,at) VALUES(?,?,?,?,?,?,?) RETURNING id`,
		e.Actor, e.Action, e.Target, before, after, e.Result, pgt(e.At)).Scan(&e.ID)
	return pgErr(err)
}

func (s *Postgres) ListAudit(ctx context.Context, limit int) ([]*model.AuditEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := pgQuery(ctx, s.db, fmt.Sprintf(`SELECT id,actor,action,target,before,after,result,at FROM audit ORDER BY id DESC LIMIT %d`, limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.AuditEntry{}
	for rows.Next() {
		var e model.AuditEntry
		var before, after sql.NullString
		var at time.Time
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
		e.At = at.UTC()
		out = append(out, &e)
	}
	return out, rows.Err()
}

// --- users / sessions ------------------------------------------------------

func (s *Postgres) CreateUser(ctx context.Context, u *model.User, passwordHash string) error {
	if u.ID == "" {
		u.ID = newID()
	}
	n := nowUTC()
	u.CreatedAt = n
	_, err := pgExec(ctx, s.db, `INSERT INTO users(id,username,password_hash,role,created_at) VALUES(?,?,?,?,?)`, u.ID, u.Username, passwordHash, string(u.Role), n)
	return err
}

func (s *Postgres) GetUserByName(ctx context.Context, username string) (*model.User, string, error) {
	var u model.User
	var hash string
	var created time.Time
	err := pgRow(ctx, s.db, `SELECT id,username,password_hash,role,created_at FROM users WHERE username=?`, username).Scan(&u.ID, &u.Username, &hash, &u.Role, &created)
	if err == sql.ErrNoRows {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	u.CreatedAt = created.UTC()
	return &u, hash, nil
}

func (s *Postgres) GetUser(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	var created time.Time
	err := pgRow(ctx, s.db, `SELECT id,username,role,created_at FROM users WHERE id=?`, id).Scan(&u.ID, &u.Username, &u.Role, &created)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.CreatedAt = created.UTC()
	return &u, nil
}

func (s *Postgres) ListUsers(ctx context.Context) ([]*model.User, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT id,username,role,created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.User{}
	for rows.Next() {
		var u model.User
		var created time.Time
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created); err != nil {
			return nil, err
		}
		u.CreatedAt = created.UTC()
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (s *Postgres) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := pgRow(ctx, s.db, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Postgres) DeleteUser(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM users WHERE id=?`, id))
}

func (s *Postgres) SetPassword(ctx context.Context, id, passwordHash string) error {
	return affected(pgExec(ctx, s.db, `UPDATE users SET password_hash=? WHERE id=?`, passwordHash, id))
}

func (s *Postgres) CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error {
	_, err := pgExec(ctx, s.db, `INSERT INTO sessions(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, tokenHash, userID, pgt(expires), nowUTC())
	return err
}

func (s *Postgres) GetSession(ctx context.Context, tokenHash string) (*model.User, error) {
	var userID string
	var expires time.Time
	err := pgRow(ctx, s.db, `SELECT user_id,expires_at FROM sessions WHERE token_hash=?`, tokenHash).Scan(&userID, &expires)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if expires.Before(time.Now()) {
		_, _ = pgExec(ctx, s.db, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
		return nil, ErrNotFound
	}
	return s.GetUser(ctx, userID)
}

func (s *Postgres) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := pgExec(ctx, s.db, `DELETE FROM sessions WHERE token_hash=?`, tokenHash)
	return err
}

func (s *Postgres) DeleteUserSessions(ctx context.Context, userID, keepHash string) (int64, error) {
	res, err := pgExec(ctx, s.db, `DELETE FROM sessions WHERE user_id=? AND token_hash<>?`, userID, keepHash)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
