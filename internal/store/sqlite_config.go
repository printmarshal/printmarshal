package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- profiles ----------------------------------------------------------------

const profileCols = `id,name,description,settings,created_by,created_at,updated_at`

func scanProfile(r rowScanner) (*model.Profile, error) {
	var p model.Profile
	var settings, created, upd string
	if err := r.Scan(&p.ID, &p.Name, &p.Description, &settings, &p.CreatedBy, &created, &upd); err != nil {
		return nil, err
	}
	p.Settings = []model.SettingChange{}
	fromJSON(sql.NullString{String: settings, Valid: true}, &p.Settings)
	p.CreatedAt, p.UpdatedAt = parseTime(created), parseTime(upd)
	return &p, nil
}

func (s *SQLite) CreateProfile(ctx context.Context, p *model.Profile) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Settings == nil {
		p.Settings = []model.SettingChange{}
	}
	n := now()
	p.CreatedAt, p.UpdatedAt = parseTime(n), parseTime(n)
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_profiles(`+profileCols+`) VALUES(?,?,?,?,?,?,?)`,
		p.ID, strings.TrimSpace(p.Name), p.Description, toJSON(p.Settings), p.CreatedBy, n, n)
	return err
}

func (s *SQLite) UpdateProfile(ctx context.Context, p *model.Profile) error {
	if p.Settings == nil {
		p.Settings = []model.SettingChange{}
	}
	n := now()
	res, err := s.db.ExecContext(ctx, `UPDATE config_profiles SET name=?, description=?, settings=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(p.Name), p.Description, toJSON(p.Settings), n, p.ID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	p.UpdatedAt = parseTime(n)
	return nil
}

func (s *SQLite) DeleteProfile(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM config_profiles WHERE id=?`, id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM config_drift WHERE profile_id=?`, id)
	return nil
}

func (s *SQLite) GetProfile(ctx context.Context, id string) (*model.Profile, error) {
	p, err := scanProfile(s.db.QueryRowContext(ctx, `SELECT `+profileCols+` FROM config_profiles WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *SQLite) ListProfiles(ctx context.Context) ([]*model.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+profileCols+` FROM config_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- snapshots ---------------------------------------------------------------

func scanSnapshot(r rowScanner) (*model.ConfigSnapshot, error) {
	var sn model.ConfigSnapshot
	var doc, at string
	if err := r.Scan(&sn.ID, &sn.DeviceID, &doc, &at); err != nil {
		return nil, err
	}
	sn.Document = &model.ConfigDocument{}
	fromJSON(sql.NullString{String: doc, Valid: true}, sn.Document)
	sn.At = parseTime(at)
	return &sn, nil
}

func (s *SQLite) SaveConfigSnapshot(ctx context.Context, sn *model.ConfigSnapshot) error {
	if sn.ID == "" {
		sn.ID = newID()
	}
	if sn.At.IsZero() {
		sn.At = time.Now().UTC()
	}
	if sn.Document == nil {
		return fmt.Errorf("snapshot has no document")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_snapshots(id,device_id,document,at) VALUES(?,?,?,?)`,
		sn.ID, sn.DeviceID, toJSON(sn.Document), ts(sn.At))
	return err
}

func (s *SQLite) GetConfigSnapshot(ctx context.Context, id string) (*model.ConfigSnapshot, error) {
	sn, err := scanSnapshot(s.db.QueryRowContext(ctx, `SELECT id,device_id,document,at FROM config_snapshots WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return sn, err
}

func (s *SQLite) LatestConfigSnapshot(ctx context.Context, deviceID string) (*model.ConfigSnapshot, error) {
	sn, err := scanSnapshot(s.db.QueryRowContext(ctx, `SELECT id,device_id,document,at FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT 1`, deviceID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return sn, err
}

func (s *SQLite) ListConfigSnapshots(ctx context.Context, deviceID string, limit int) ([]*model.ConfigSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT id,device_id,document,at FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT %d`, limit), deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ConfigSnapshot{}
	for rows.Next() {
		sn, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// PruneConfigSnapshots keeps the newest `keep` snapshots of a device.
func (s *SQLite) PruneConfigSnapshots(ctx context.Context, deviceID string, keep int) (int64, error) {
	if keep <= 0 {
		keep = 20
	}
	res, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM config_snapshots WHERE device_id=? AND id NOT IN (SELECT id FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT %d)`, keep), deviceID, deviceID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- write records -----------------------------------------------------------

const writeCols = `id,device_id,job_id,profile_id,revert_of,actor,dry_run,changes,result,written,failed,skipped,unchanged,at`

func scanWrite(r rowScanner) (*model.WriteRecord, error) {
	var w model.WriteRecord
	var deviceID, changes, result, at string
	var dry int
	if err := r.Scan(&w.ID, &deviceID, &w.JobID, &w.ProfileID, &w.RevertOf, &w.Actor, &dry, &changes, &result, &w.Written, &w.Failed, &w.Skipped, &w.Unchanged, &at); err != nil {
		return nil, err
	}
	w.Changes = []model.SettingChange{}
	fromJSON(sql.NullString{String: changes, Valid: true}, &w.Changes)
	fromJSON(sql.NullString{String: result, Valid: true}, &w.Result)
	if w.Result.Results == nil {
		w.Result.Results = []model.SettingResult{}
	}
	w.Result.DeviceID, w.Result.DryRun = deviceID, dry == 1
	if w.Result.At.IsZero() {
		w.Result.At = parseTime(at)
	}
	// Verified/diverged live in the result JSON only (audit A20); recount so
	// every counter is derived from the same per-setting results.
	w.Count()
	return &w, nil
}

func (s *SQLite) SaveWriteRecord(ctx context.Context, w *model.WriteRecord) error {
	if w.ID == "" {
		w.ID = newID()
	}
	if w.Result.At.IsZero() {
		w.Result.At = time.Now().UTC()
	}
	if w.Changes == nil {
		w.Changes = []model.SettingChange{}
	}
	w.Count()
	dry := 0
	if w.Result.DryRun {
		dry = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_writes(`+writeCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.Result.DeviceID, w.JobID, w.ProfileID, w.RevertOf, w.Actor, dry, toJSON(w.Changes), toJSON(w.Result),
		w.Written, w.Failed, w.Skipped, w.Unchanged, ts(w.Result.At))
	return err
}

func (s *SQLite) GetWriteRecord(ctx context.Context, id string) (*model.WriteRecord, error) {
	w, err := scanWrite(s.db.QueryRowContext(ctx, `SELECT `+writeCols+` FROM config_writes WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return w, err
}

func (s *SQLite) ListWriteRecords(ctx context.Context, f WriteRecordFilter) ([]*model.WriteRecord, error) {
	var where []string
	var args []any
	if f.DeviceID != "" {
		where = append(where, "device_id=?")
		args = append(args, f.DeviceID)
	}
	if f.ProfileID != "" {
		where = append(where, "profile_id=?")
		args = append(args, f.ProfileID)
	}
	if f.JobID != "" {
		where = append(where, "job_id=?")
		args = append(args, f.JobID)
	}
	q := `SELECT ` + writeCols + ` FROM config_writes`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := 200
	if f.Limit > 0 {
		limit = f.Limit
	}
	q += fmt.Sprintf(" ORDER BY at DESC LIMIT %d", limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.WriteRecord{}
	for rows.Next() {
		w, err := scanWrite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- drift reports -----------------------------------------------------------

func (s *SQLite) SaveDriftReport(ctx context.Context, r *model.DriftReport) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO config_drift(device_id,profile_id,report,at) VALUES(?,?,?,?)
		ON CONFLICT(device_id,profile_id) DO UPDATE SET report=excluded.report, at=excluded.at`,
		r.DeviceID, r.ProfileID, toJSON(r), ts(r.At))
	return err
}

func (s *SQLite) ListDriftReports(ctx context.Context, profileID string, limit int) ([]*model.DriftReport, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT report FROM config_drift WHERE profile_id=? ORDER BY at DESC LIMIT %d`, limit), profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.DriftReport{}
	for rows.Next() {
		var report string
		if err := rows.Scan(&report); err != nil {
			return nil, err
		}
		var r model.DriftReport
		fromJSON(sql.NullString{String: report, Valid: true}, &r)
		if r.Items == nil {
			r.Items = []model.DriftItem{}
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}
