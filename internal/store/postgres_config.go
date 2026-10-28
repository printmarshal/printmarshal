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

func scanProfilePG(r rowScanner) (*model.Profile, error) {
	var p model.Profile
	var settings string
	var created, upd time.Time
	if err := r.Scan(&p.ID, &p.Name, &p.Description, &settings, &p.CreatedBy, &created, &upd); err != nil {
		return nil, err
	}
	p.Settings = []model.SettingChange{}
	fromJSON(sql.NullString{String: settings, Valid: true}, &p.Settings)
	p.CreatedAt, p.UpdatedAt = created.UTC(), upd.UTC()
	return &p, nil
}

func (s *Postgres) CreateProfile(ctx context.Context, p *model.Profile) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.Settings == nil {
		p.Settings = []model.SettingChange{}
	}
	n := nowUTC()
	p.CreatedAt, p.UpdatedAt = n, n
	_, err := pgExec(ctx, s.db, `INSERT INTO config_profiles(`+profileCols+`) VALUES(?,?,?,?,?,?,?)`,
		p.ID, strings.TrimSpace(p.Name), p.Description, toJSON(p.Settings), p.CreatedBy, n, n)
	return err
}

func (s *Postgres) UpdateProfile(ctx context.Context, p *model.Profile) error {
	if p.Settings == nil {
		p.Settings = []model.SettingChange{}
	}
	n := nowUTC()
	if err := affected(pgExec(ctx, s.db, `UPDATE config_profiles SET name=?, description=?, settings=?, updated_at=? WHERE id=?`,
		strings.TrimSpace(p.Name), p.Description, toJSON(p.Settings), n, p.ID)); err != nil {
		return err
	}
	p.UpdatedAt = n
	return nil
}

func (s *Postgres) DeleteProfile(ctx context.Context, id string) error {
	if err := affected(pgExec(ctx, s.db, `DELETE FROM config_profiles WHERE id=?`, id)); err != nil {
		return err
	}
	_, _ = pgExec(ctx, s.db, `DELETE FROM config_drift WHERE profile_id=?`, id)
	return nil
}

func (s *Postgres) GetProfile(ctx context.Context, id string) (*model.Profile, error) {
	p, err := scanProfilePG(pgRow(ctx, s.db, `SELECT `+profileCols+` FROM config_profiles WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Postgres) ListProfiles(ctx context.Context) ([]*model.Profile, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+profileCols+` FROM config_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Profile{}
	for rows.Next() {
		p, err := scanProfilePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- snapshots ---------------------------------------------------------------

func scanSnapshotPG(r rowScanner) (*model.ConfigSnapshot, error) {
	var sn model.ConfigSnapshot
	var doc string
	var at time.Time
	if err := r.Scan(&sn.ID, &sn.DeviceID, &doc, &at); err != nil {
		return nil, err
	}
	sn.Document = &model.ConfigDocument{}
	fromJSON(sql.NullString{String: doc, Valid: true}, sn.Document)
	sn.At = at.UTC()
	return &sn, nil
}

func (s *Postgres) SaveConfigSnapshot(ctx context.Context, sn *model.ConfigSnapshot) error {
	if sn.ID == "" {
		sn.ID = newID()
	}
	if sn.At.IsZero() {
		sn.At = time.Now().UTC()
	}
	if sn.Document == nil {
		return fmt.Errorf("snapshot has no document")
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO config_snapshots(id,device_id,document,at) VALUES(?,?,?,?)`,
		sn.ID, sn.DeviceID, toJSON(sn.Document), pgt(sn.At))
	return err
}

func (s *Postgres) GetConfigSnapshot(ctx context.Context, id string) (*model.ConfigSnapshot, error) {
	sn, err := scanSnapshotPG(pgRow(ctx, s.db, `SELECT id,device_id,document,at FROM config_snapshots WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return sn, err
}

func (s *Postgres) LatestConfigSnapshot(ctx context.Context, deviceID string) (*model.ConfigSnapshot, error) {
	sn, err := scanSnapshotPG(pgRow(ctx, s.db, `SELECT id,device_id,document,at FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT 1`, deviceID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return sn, err
}

func (s *Postgres) ListConfigSnapshots(ctx context.Context, deviceID string, limit int) ([]*model.ConfigSnapshot, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := pgQuery(ctx, s.db, fmt.Sprintf(`SELECT id,device_id,document,at FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT %d`, limit), deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.ConfigSnapshot{}
	for rows.Next() {
		sn, err := scanSnapshotPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// PruneConfigSnapshots keeps the newest `keep` snapshots of a device.
func (s *Postgres) PruneConfigSnapshots(ctx context.Context, deviceID string, keep int) (int64, error) {
	if keep <= 0 {
		keep = 20
	}
	res, err := pgExec(ctx, s.db, fmt.Sprintf(`DELETE FROM config_snapshots WHERE device_id=? AND id NOT IN (SELECT id FROM config_snapshots WHERE device_id=? ORDER BY at DESC LIMIT %d)`, keep), deviceID, deviceID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- write records -----------------------------------------------------------

func scanWritePG(r rowScanner) (*model.WriteRecord, error) {
	var w model.WriteRecord
	var deviceID, changes, result string
	var at time.Time
	var dry bool
	if err := r.Scan(&w.ID, &deviceID, &w.JobID, &w.ProfileID, &w.RevertOf, &w.Actor, &dry, &changes, &result, &w.Written, &w.Failed, &w.Skipped, &w.Unchanged, &at); err != nil {
		return nil, err
	}
	w.Changes = []model.SettingChange{}
	fromJSON(sql.NullString{String: changes, Valid: true}, &w.Changes)
	fromJSON(sql.NullString{String: result, Valid: true}, &w.Result)
	if w.Result.Results == nil {
		w.Result.Results = []model.SettingResult{}
	}
	w.Result.DeviceID, w.Result.DryRun = deviceID, dry
	if w.Result.At.IsZero() {
		w.Result.At = at.UTC()
	}
	return &w, nil
}

func (s *Postgres) SaveWriteRecord(ctx context.Context, w *model.WriteRecord) error {
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
	_, err := pgExec(ctx, s.db, `INSERT INTO config_writes(`+writeCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.Result.DeviceID, w.JobID, w.ProfileID, w.RevertOf, w.Actor, w.Result.DryRun, toJSON(w.Changes), toJSON(w.Result),
		w.Written, w.Failed, w.Skipped, w.Unchanged, pgt(w.Result.At))
	return err
}

func (s *Postgres) GetWriteRecord(ctx context.Context, id string) (*model.WriteRecord, error) {
	w, err := scanWritePG(pgRow(ctx, s.db, `SELECT `+writeCols+` FROM config_writes WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return w, err
}

func (s *Postgres) ListWriteRecords(ctx context.Context, f WriteRecordFilter) ([]*model.WriteRecord, error) {
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
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.WriteRecord{}
	for rows.Next() {
		w, err := scanWritePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// --- drift reports -----------------------------------------------------------

func (s *Postgres) SaveDriftReport(ctx context.Context, r *model.DriftReport) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO config_drift(device_id,profile_id,report,at) VALUES(?,?,?,?)
		ON CONFLICT(device_id,profile_id) DO UPDATE SET report=excluded.report, at=excluded.at`,
		r.DeviceID, r.ProfileID, toJSON(r), pgt(r.At))
	return err
}

func (s *Postgres) ListDriftReports(ctx context.Context, profileID string, limit int) ([]*model.DriftReport, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := pgQuery(ctx, s.db, fmt.Sprintf(`SELECT report FROM config_drift WHERE profile_id=? ORDER BY at DESC LIMIT %d`, limit), profileID)
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
