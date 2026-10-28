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

func scanPackagePG(r rowScanner) (*model.FirmwarePackage, error) {
	var p model.FirmwarePackage
	var models string
	var created time.Time
	if err := r.Scan(&p.ID, &p.Vendor, &models, &p.Version, &p.MinVersion, &p.FileName, &p.Size, &p.SHA256, &p.StoragePath, &p.Notes, &p.CreatedBy, &created); err != nil {
		return nil, err
	}
	p.Models = []string{}
	fromJSON(sql.NullString{String: models, Valid: true}, &p.Models)
	p.CreatedAt = created.UTC()
	return &p, nil
}

func (s *Postgres) CreateFirmwarePackage(ctx context.Context, p *model.FirmwarePackage) error {
	if p.ID == "" {
		p.ID = newID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO firmware_packages(`+packageCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Vendor, toJSON(p.Models), p.Version, p.MinVersion, p.FileName, p.Size, p.SHA256, p.StoragePath, p.Notes, p.CreatedBy, pgt(p.CreatedAt))
	return err
}

func (s *Postgres) GetFirmwarePackage(ctx context.Context, id string) (*model.FirmwarePackage, error) {
	p, err := scanPackagePG(pgRow(ctx, s.db, `SELECT `+packageCols+` FROM firmware_packages WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Postgres) ListFirmwarePackages(ctx context.Context) ([]*model.FirmwarePackage, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+packageCols+` FROM firmware_packages ORDER BY vendor, version DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FirmwarePackage{}
	for rows.Next() {
		p, err := scanPackagePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteFirmwarePackage(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM firmware_packages WHERE id=?`, id))
}

// --- campaigns ---------------------------------------------------------------

// pgCampaignCols is campaignCols with the reserved word "window" quoted.
var pgCampaignCols = strings.Replace(campaignCols, ",window,", `,"window",`, 1)

func scanCampaignPG(r rowScanner) (*model.Campaign, error) {
	var c model.Campaign
	var deviceIDs string
	var created, updated time.Time
	var window sql.NullString
	var scheduled, started, finished sql.NullTime
	if err := r.Scan(&c.ID, &c.Name, &c.Description, &c.PackageID, &c.GroupID, &deviceIDs, &c.CanaryCount, &c.Concurrency, &window, &c.FailureThresholdPercent,
		&c.Status, &c.Wave, &c.Confirmed, &c.DryRun, &c.AllowDowngrade, &c.VerifyTimeoutSecs, &c.PauseReason, &c.JobID, &scheduled, &started, &finished,
		&c.CreatedBy, &c.StartedBy, &created, &updated, &c.Error); err != nil {
		return nil, err
	}
	c.DeviceIDs = []string{}
	fromJSON(sql.NullString{String: deviceIDs, Valid: true}, &c.DeviceIDs)
	if window.Valid && window.String != "" && window.String != "null" {
		c.Window = &model.MaintenanceWindow{}
		fromJSON(window, c.Window)
	}
	c.ScheduledAt, c.StartedAt, c.FinishedAt = timeOf(scheduled), timeOf(started), timeOf(finished)
	c.CreatedAt, c.UpdatedAt = created.UTC(), updated.UTC()
	return &c, nil
}

func (s *Postgres) CreateCampaign(ctx context.Context, c *model.Campaign) error {
	if c.ID == "" {
		c.ID = newID()
	}
	now := nowUTC()
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
	_, err := pgExec(ctx, s.db, `INSERT INTO firmware_campaigns(`+pgCampaignCols+`) VALUES(`+placeholders(26)+`)`,
		c.ID, c.Name, c.Description, c.PackageID, c.GroupID, toJSON(c.DeviceIDs), c.CanaryCount, c.Concurrency, windowJSON(c.Window), c.FailureThresholdPercent,
		string(c.Status), c.Wave, c.Confirmed, c.DryRun, c.AllowDowngrade, c.VerifyTimeoutSecs, c.PauseReason, c.JobID,
		pgtp(c.ScheduledAt), pgtp(c.StartedAt), pgtp(c.FinishedAt), c.CreatedBy, c.StartedBy, pgt(c.CreatedAt), pgt(c.UpdatedAt), c.Error)
	return err
}

func (s *Postgres) UpdateCampaign(ctx context.Context, c *model.Campaign) error {
	c.UpdatedAt = nowUTC()
	if c.DeviceIDs == nil {
		c.DeviceIDs = []string{}
	}
	return affected(pgExec(ctx, s.db, `UPDATE firmware_campaigns SET name=?, description=?, package_id=?, group_id=?, device_ids=?, canary_count=?, concurrency=?, "window"=?,
		failure_threshold=?, status=?, wave=?, confirmed=?, dry_run=?, allow_downgrade=?, verify_timeout_secs=?, pause_reason=?, job_id=?,
		scheduled_at=?, started_at=?, finished_at=?, started_by=?, updated_at=?, error=? WHERE id=?`,
		c.Name, c.Description, c.PackageID, c.GroupID, toJSON(c.DeviceIDs), c.CanaryCount, c.Concurrency, windowJSON(c.Window),
		c.FailureThresholdPercent, string(c.Status), c.Wave, c.Confirmed, c.DryRun, c.AllowDowngrade, c.VerifyTimeoutSecs, c.PauseReason, c.JobID,
		pgtp(c.ScheduledAt), pgtp(c.StartedAt), pgtp(c.FinishedAt), c.StartedBy, pgt(c.UpdatedAt), c.Error, c.ID))
}

func (s *Postgres) GetCampaign(ctx context.Context, id string) (*model.Campaign, error) {
	c, err := scanCampaignPG(pgRow(ctx, s.db, `SELECT `+pgCampaignCols+` FROM firmware_campaigns WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Postgres) ListCampaigns(ctx context.Context, f CampaignFilter) ([]*model.Campaign, error) {
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
	q := `SELECT ` + pgCampaignCols + ` FROM firmware_campaigns`
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
	out := []*model.Campaign{}
	for rows.Next() {
		c, err := scanCampaignPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteCampaign(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM firmware_campaigns WHERE id=?`, id))
}

// --- campaign devices --------------------------------------------------------

func scanCampaignDevicePG(r rowScanner) (*model.CampaignDevice, error) {
	var d model.CampaignDevice
	var started, finished sql.NullTime
	if err := r.Scan(&d.CampaignID, &d.DeviceID, &d.Wave, &d.Status, &d.FromVersion, &d.ToVersion, &d.Message, &started, &finished); err != nil {
		return nil, err
	}
	d.StartedAt, d.FinishedAt = timeOf(started), timeOf(finished)
	return &d, nil
}

func (s *Postgres) UpsertCampaignDevice(ctx context.Context, d *model.CampaignDevice) error {
	if d.Status == "" {
		d.Status = model.CampaignDevicePending
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO firmware_campaign_devices(`+campaignDeviceCols+`,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(campaign_id, device_id) DO UPDATE SET wave=excluded.wave, status=excluded.status, from_version=excluded.from_version,
		to_version=excluded.to_version, message=excluded.message, started_at=excluded.started_at, finished_at=excluded.finished_at, updated_at=excluded.updated_at`,
		d.CampaignID, d.DeviceID, d.Wave, string(d.Status), d.FromVersion, d.ToVersion, d.Message, pgtp(d.StartedAt), pgtp(d.FinishedAt), nowUTC())
	return err
}

func (s *Postgres) ListCampaignDevices(ctx context.Context, campaignID string) ([]*model.CampaignDevice, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT `+campaignDeviceCols+` FROM firmware_campaign_devices WHERE campaign_id=? ORDER BY wave, seq`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.CampaignDevice{}
	for rows.Next() {
		d, err := scanCampaignDevicePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Postgres) DeleteCampaignDevices(ctx context.Context, campaignID string) error {
	_, err := pgExec(ctx, s.db, `DELETE FROM firmware_campaign_devices WHERE campaign_id=?`, campaignID)
	return err
}

func (s *Postgres) FirmwareHistory(ctx context.Context, deviceID string, limit int) ([]*model.FirmwareHistoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := pgQuery(ctx, s.db, `SELECT d.campaign_id,d.device_id,d.wave,d.status,d.from_version,d.to_version,d.message,d.started_at,d.finished_at,c.name,c.package_id,c.dry_run
		FROM firmware_campaign_devices d JOIN firmware_campaigns c ON c.id=d.campaign_id WHERE d.device_id=? ORDER BY d.updated_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FirmwareHistoryEntry{}
	for rows.Next() {
		var e model.FirmwareHistoryEntry
		var started, finished sql.NullTime
		if err := rows.Scan(&e.CampaignID, &e.DeviceID, &e.Wave, &e.Status, &e.FromVersion, &e.ToVersion, &e.Message, &started, &finished, &e.CampaignName, &e.PackageID, &e.DryRun); err != nil {
			return nil, err
		}
		e.StartedAt, e.FinishedAt = timeOf(started), timeOf(finished)
		out = append(out, &e)
	}
	return out, rows.Err()
}
