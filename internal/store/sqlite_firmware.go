package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- firmware packages -------------------------------------------------------

const packageCols = `id,vendor,models,version,min_version,file_name,size,sha256,storage_path,notes,created_by,created_at`

func scanPackage(r rowScanner) (*model.FirmwarePackage, error) {
	var p model.FirmwarePackage
	var models, created string
	if err := r.Scan(&p.ID, &p.Vendor, &models, &p.Version, &p.MinVersion, &p.FileName, &p.Size, &p.SHA256, &p.StoragePath, &p.Notes, &p.CreatedBy, &created); err != nil {
		return nil, err
	}
	p.Models = []string{}
	fromJSON(sql.NullString{String: models, Valid: true}, &p.Models)
	p.CreatedAt = parseTime(created)
	return &p, nil
}

func (s *SQLite) CreateFirmwarePackage(ctx context.Context, p *model.FirmwarePackage) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO firmware_packages(`+packageCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Vendor, toJSON(p.Models), p.Version, p.MinVersion, p.FileName, p.Size, p.SHA256, p.StoragePath, p.Notes, p.CreatedBy, ts(p.CreatedAt))
	return err
}

func (s *SQLite) GetFirmwarePackage(ctx context.Context, id string) (*model.FirmwarePackage, error) {
	p, err := scanPackage(s.db.QueryRowContext(ctx, `SELECT `+packageCols+` FROM firmware_packages WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *SQLite) ListFirmwarePackages(ctx context.Context) ([]*model.FirmwarePackage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+packageCols+` FROM firmware_packages ORDER BY vendor, version DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FirmwarePackage{}
	for rows.Next() {
		p, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *SQLite) DeleteFirmwarePackage(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM firmware_packages WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- campaigns ---------------------------------------------------------------

const campaignCols = `id,name,description,package_id,group_id,device_ids,canary_count,concurrency,window,failure_threshold,status,wave,confirmed,dry_run,allow_downgrade,verify_timeout_secs,pause_reason,job_id,scheduled_at,started_at,finished_at,created_by,started_by,created_at,updated_at,error`

func scanCampaign(r rowScanner) (*model.Campaign, error) {
	var c model.Campaign
	var deviceIDs, created, updated string
	var window, scheduled, started, finished sql.NullString
	var confirmed, dryRun, allowDown int
	if err := r.Scan(&c.ID, &c.Name, &c.Description, &c.PackageID, &c.GroupID, &deviceIDs, &c.CanaryCount, &c.Concurrency, &window, &c.FailureThresholdPercent,
		&c.Status, &c.Wave, &confirmed, &dryRun, &allowDown, &c.VerifyTimeoutSecs, &c.PauseReason, &c.JobID, &scheduled, &started, &finished,
		&c.CreatedBy, &c.StartedBy, &created, &updated, &c.Error); err != nil {
		return nil, err
	}
	c.DeviceIDs = []string{}
	fromJSON(sql.NullString{String: deviceIDs, Valid: true}, &c.DeviceIDs)
	if window.Valid && window.String != "" && window.String != "null" {
		c.Window = &model.MaintenanceWindow{}
		fromJSON(window, c.Window)
	}
	c.Confirmed, c.DryRun, c.AllowDowngrade = confirmed != 0, dryRun != 0, allowDown != 0
	c.ScheduledAt, c.StartedAt, c.FinishedAt = parseTimeP(scheduled), parseTimeP(started), parseTimeP(finished)
	c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(updated)
	return &c, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func windowJSON(w *model.MaintenanceWindow) any {
	if w == nil {
		return nil
	}
	return toJSON(w)
}

func (s *SQLite) CreateCampaign(ctx context.Context, c *model.Campaign) error {
	if c.ID == "" {
		c.ID = newID()
	}
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.Status == "" {
		c.Status = model.CampaignDraft
	}
	if c.DeviceIDs == nil {
		c.DeviceIDs = []string{}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO firmware_campaigns(`+campaignCols+`) VALUES(`+placeholders(26)+`)`,
		c.ID, c.Name, c.Description, c.PackageID, c.GroupID, toJSON(c.DeviceIDs), c.CanaryCount, c.Concurrency, windowJSON(c.Window), c.FailureThresholdPercent,
		string(c.Status), c.Wave, b2i(c.Confirmed), b2i(c.DryRun), b2i(c.AllowDowngrade), c.VerifyTimeoutSecs, c.PauseReason, c.JobID,
		tsp(c.ScheduledAt), tsp(c.StartedAt), tsp(c.FinishedAt), c.CreatedBy, c.StartedBy, ts(c.CreatedAt), ts(c.UpdatedAt), c.Error)
	return err
}

func (s *SQLite) UpdateCampaign(ctx context.Context, c *model.Campaign) error {
	c.UpdatedAt = time.Now().UTC()
	if c.DeviceIDs == nil {
		c.DeviceIDs = []string{}
	}
	res, err := s.db.ExecContext(ctx, `UPDATE firmware_campaigns SET name=?, description=?, package_id=?, group_id=?, device_ids=?, canary_count=?, concurrency=?, window=?,
		failure_threshold=?, status=?, wave=?, confirmed=?, dry_run=?, allow_downgrade=?, verify_timeout_secs=?, pause_reason=?, job_id=?,
		scheduled_at=?, started_at=?, finished_at=?, started_by=?, updated_at=?, error=? WHERE id=?`,
		c.Name, c.Description, c.PackageID, c.GroupID, toJSON(c.DeviceIDs), c.CanaryCount, c.Concurrency, windowJSON(c.Window),
		c.FailureThresholdPercent, string(c.Status), c.Wave, b2i(c.Confirmed), b2i(c.DryRun), b2i(c.AllowDowngrade), c.VerifyTimeoutSecs, c.PauseReason, c.JobID,
		tsp(c.ScheduledAt), tsp(c.StartedAt), tsp(c.FinishedAt), c.StartedBy, ts(c.UpdatedAt), c.Error, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) GetCampaign(ctx context.Context, id string) (*model.Campaign, error) {
	c, err := scanCampaign(s.db.QueryRowContext(ctx, `SELECT `+campaignCols+` FROM firmware_campaigns WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *SQLite) ListCampaigns(ctx context.Context, f CampaignFilter) ([]*model.Campaign, error) {
	var where []string
	var args []any
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.PackageID != "" {
		where = append(where, "package_id=?")
		args = append(args, f.PackageID)
	}
	if f.Active {
		where = append(where, "status NOT IN ('completed','failed','cancelled')")
	}
	q := `SELECT ` + campaignCols + ` FROM firmware_campaigns`
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
	out := []*model.Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) DeleteCampaign(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM firmware_campaigns WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- campaign devices --------------------------------------------------------

const campaignDeviceCols = `campaign_id,device_id,wave,status,from_version,to_version,message,started_at,finished_at`

func scanCampaignDevice(r rowScanner) (*model.CampaignDevice, error) {
	var d model.CampaignDevice
	var started, finished sql.NullString
	if err := r.Scan(&d.CampaignID, &d.DeviceID, &d.Wave, &d.Status, &d.FromVersion, &d.ToVersion, &d.Message, &started, &finished); err != nil {
		return nil, err
	}
	d.StartedAt, d.FinishedAt = parseTimeP(started), parseTimeP(finished)
	return &d, nil
}

func (s *SQLite) UpsertCampaignDevice(ctx context.Context, d *model.CampaignDevice) error {
	if d.Status == "" {
		d.Status = model.CampaignDevicePending
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO firmware_campaign_devices(`+campaignDeviceCols+`,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(campaign_id, device_id) DO UPDATE SET wave=excluded.wave, status=excluded.status, from_version=excluded.from_version,
		to_version=excluded.to_version, message=excluded.message, started_at=excluded.started_at, finished_at=excluded.finished_at, updated_at=excluded.updated_at`,
		d.CampaignID, d.DeviceID, d.Wave, string(d.Status), d.FromVersion, d.ToVersion, d.Message, tsp(d.StartedAt), tsp(d.FinishedAt), now())
	return err
}

func (s *SQLite) ListCampaignDevices(ctx context.Context, campaignID string) ([]*model.CampaignDevice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+campaignDeviceCols+` FROM firmware_campaign_devices WHERE campaign_id=? ORDER BY wave, rowid`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.CampaignDevice{}
	for rows.Next() {
		d, err := scanCampaignDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *SQLite) DeleteCampaignDevices(ctx context.Context, campaignID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM firmware_campaign_devices WHERE campaign_id=?`, campaignID)
	return err
}

func (s *SQLite) FirmwareHistory(ctx context.Context, deviceID string, limit int) ([]*model.FirmwareHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.campaign_id,d.device_id,d.wave,d.status,d.from_version,d.to_version,d.message,d.started_at,d.finished_at,c.name,c.package_id,c.dry_run
		FROM firmware_campaign_devices d JOIN firmware_campaigns c ON c.id=d.campaign_id WHERE d.device_id=? ORDER BY d.updated_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FirmwareHistoryEntry{}
	for rows.Next() {
		var e model.FirmwareHistoryEntry
		var started, finished sql.NullString
		var dryRun int
		if err := rows.Scan(&e.CampaignID, &e.DeviceID, &e.Wave, &e.Status, &e.FromVersion, &e.ToVersion, &e.Message, &started, &finished, &e.CampaignName, &e.PackageID, &dryRun); err != nil {
			return nil, err
		}
		e.StartedAt, e.FinishedAt = parseTimeP(started), parseTimeP(finished)
		e.DryRun = dryRun != 0
		out = append(out, &e)
	}
	return out, rows.Err()
}
