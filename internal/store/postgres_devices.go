package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// Device methods of Postgres: the same logic as sqlite_devices.go with native
// column types (see scanDevicePG).

func scanDevicePG(r rowScanner) (*model.Device, error) {
	var (
		d                                  model.Device
		caps, disc, ident, supplies, trays string
		identErrs, fieldSources            string
		status, counters                   sql.NullString
		firstSeen, lastSeen, created, upd  time.Time
		lastPolled, dnsCheckedAt           sql.NullTime
		dnsNameSuspect, dnsSync            int
		failures                           int
		severity                           string
	)
	err := r.Scan(&d.ID, &d.Address, &d.Hostname, &d.DNSName, &dnsNameSuspect, &dnsCheckedAt, &d.MAC, &d.Serial, &d.UUID, &d.Make, &d.Model, &d.DeviceType, &d.Firmware,
		&d.SysDescr, &d.SysName, &d.EnterpriseID, &d.AdminURL, &caps, &disc, &ident, &d.DisplayName, &d.DisplayNameSource, &dnsSync, &d.Location, &d.AssetTag,
		&d.Notes, &d.State, &status, &supplies, &trays, &counters, &severity, &failures, &firstSeen, &lastSeen, &lastPolled, &created, &upd,
		&d.WebURLProbed, &d.WebURLOverride, &identErrs, &fieldSources)
	if err != nil {
		return nil, err
	}
	fromJSON(sql.NullString{String: identErrs, Valid: true}, &d.IdentifyErrors)
	fromJSON(sql.NullString{String: fieldSources, Valid: true}, &d.FieldSources)
	if len(d.FieldSources) == 0 {
		d.FieldSources = nil
	}
	d.DNSNameSuspect = dnsNameSuspect != 0
	d.DNSCheckedAt = timeOf(dnsCheckedAt)
	d.DNSSync = dnsSync != 0
	d.DNSNameChanged = !d.DNSSync && d.DisplayNameSource == "dns" && d.DNSName != "" && d.DisplayName != "" && d.DNSName != d.DisplayName
	d.Capabilities = map[model.Capability]string{}
	fromJSON(sql.NullString{String: caps, Valid: true}, &d.Capabilities)
	d.DiscoveryMethods = []string{}
	fromJSON(sql.NullString{String: disc, Valid: true}, &d.DiscoveryMethods)
	d.IdentifyMethods = []string{}
	fromJSON(sql.NullString{String: ident, Valid: true}, &d.IdentifyMethods)
	d.Supplies = []model.Supply{}
	fromJSON(sql.NullString{String: supplies, Valid: true}, &d.Supplies)
	d.Trays = []model.Tray{}
	fromJSON(sql.NullString{String: trays, Valid: true}, &d.Trays)
	if status.Valid && status.String != "" && status.String != "null" {
		d.Status = &model.Status{}
		fromJSON(status, d.Status)
	}
	if counters.Valid && counters.String != "" && counters.String != "null" {
		d.Counters = &model.Counters{}
		fromJSON(counters, d.Counters)
	}
	d.WebURL = model.ResolveWebURL(&d)
	d.FirstSeen, d.LastSeen = firstSeen.UTC(), lastSeen.UTC()
	d.LastPolled = timeOf(lastPolled)
	d.CreatedAt, d.UpdatedAt = created.UTC(), upd.UTC()
	return &d, nil
}

func (s *Postgres) getDevice(q rowScanner) (*model.Device, error) {
	d, err := scanDevicePG(q)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return d, err
}

// GetDevice implements Store.
func (s *Postgres) GetDevice(ctx context.Context, id string) (*model.Device, error) {
	return s.getDevice(pgRow(ctx, s.db, `SELECT `+deviceCols+` FROM devices WHERE id=?`, id))
}

// findByIdentity de-duplicates by serial, then UUID, then MAC, then address (spec §8.1).
func (s *Postgres) findByIdentity(ctx context.Context, id *model.Identity) (*model.Device, error) {
	try := func(col, val string) (*model.Device, error) {
		if val == "" {
			return nil, ErrNotFound
		}
		return s.getDevice(pgRow(ctx, s.db, `SELECT `+deviceCols+` FROM devices WHERE `+col+`=? ORDER BY last_seen DESC LIMIT 1`, val))
	}
	for _, k := range []struct{ col, val string }{
		{"serial", id.Serial}, {"uuid", id.UUID}, {"mac", strings.ToLower(id.MAC)},
	} {
		d, err := try(k.col, k.val)
		if err == nil {
			return d, nil
		}
		if err != ErrNotFound {
			return nil, err
		}
	}
	d, err := try("address", id.Address)
	if err != nil {
		return nil, err
	}
	if IdentityConflicts(d, id) {
		// Another printer now answers at this address: a replacement must
		// become its own record, not inherit the old one's groups,
		// credentials and history (code review H-006).
		return nil, ErrNotFound
	}
	return d, nil
}

// UpsertDevice implements Store. Automatic fields are refreshed; operator
// fields, state and telemetry are untouched (spec §3 principle 5).
func (s *Postgres) UpsertDevice(ctx context.Context, id *model.Identity, caps map[model.Capability]string, discoveryMethods []string) (*model.Device, bool, error) {
	existing, err := s.findByIdentity(ctx, id)
	if err != nil && err != ErrNotFound {
		return nil, false, err
	}
	identMethods := strings.Split(id.Method, ",")
	n := nowUTC()
	if existing == nil {
		d := &model.Device{ID: newID()}
		_, err := pgExec(ctx, s.db, `INSERT INTO devices (`+deviceCols+`) VALUES (`+placeholders(42)+`)`,
			d.ID, id.Address, id.Hostname, "", 0, nil, strings.ToLower(id.MAC), id.Serial, id.UUID, id.Make, id.Model, id.DeviceType, id.Firmware,
			id.SysDescr, id.SysName, id.EnterpriseID, id.AdminURL, toJSON(caps), toJSON(discoveryMethods), toJSON(identMethods),
			"", "", 0, id.SysLocation, "", "", string(model.StateDiscovered), nil, "[]", "[]", nil, "ok", 0, n, n, nil, n, n, "", "", identifyErrorsJSON(id, nil), "{}")
		if err != nil {
			return nil, false, err
		}
		dev, err := s.GetDevice(ctx, d.ID)
		return dev, true, err
	}
	pick := func(newv, old string) string {
		if strings.TrimSpace(newv) != "" {
			return newv
		}
		return old
	}
	merged := map[model.Capability]string{}
	for k, v := range existing.Capabilities {
		merged[k] = v
	}
	for k, v := range caps {
		merged[k] = v
	}
	disc := appendUnique(existing.DiscoveryMethods, discoveryMethods...)
	ident := appendUnique(existing.IdentifyMethods, identMethods...)
	_, err = pgExec(ctx, s.db, `UPDATE devices SET address=?, hostname=?, mac=?, serial=?, uuid=?, make=?, model=?, device_type=?,
		firmware=?, sys_descr=?, sys_name=?, enterprise_id=?, admin_url=?, capabilities=?, discovery_methods=?, identify_methods=?, identify_errors=?,
		last_seen=?, updated_at=? WHERE id=?`,
		id.Address, pick(id.Hostname, existing.Hostname), strings.ToLower(pick(id.MAC, existing.MAC)), pick(id.Serial, existing.Serial),
		pick(id.UUID, existing.UUID), pick(id.Make, existing.Make), pickModel(id.Model, existing.Model), pick(id.DeviceType, existing.DeviceType),
		pick(id.Firmware, existing.Firmware), pick(id.SysDescr, existing.SysDescr), pick(id.SysName, existing.SysName),
		maxInt(id.EnterpriseID, existing.EnterpriseID), pick(id.AdminURL, existing.AdminURL), toJSON(merged), toJSON(disc), toJSON(ident), identifyErrorsJSON(id, existing),
		n, n, existing.ID)
	if err != nil {
		return nil, false, err
	}
	dev, err := s.GetDevice(ctx, existing.ID)
	return dev, false, err
}

// buildDeviceWherePG is buildDeviceWhere with PostgreSQL case folding: SQLite's
// COLLATE NOCASE and ASCII-insensitive LIKE become lower()/ILIKE.
func buildDeviceWherePG(f DeviceFilter) (string, []any) {
	var where []string
	var args []any
	if !f.IncludeDecommissioned && f.State == "" {
		where = append(where, "state != 'decommissioned'")
	}
	if f.State != "" {
		where = append(where, "state = ?")
		args = append(args, f.State)
	}
	if f.Make != "" {
		where = append(where, "lower(make) = lower(?)")
		args = append(args, f.Make)
	}
	if f.Model != "" {
		where = append(where, "model ILIKE ?")
		args = append(args, "%"+f.Model+"%")
	}
	if f.Severity != "" {
		where = append(where, "severity = ?")
		args = append(args, f.Severity)
	}
	if f.Q != "" {
		q := "%" + strings.ToLower(f.Q) + "%"
		where = append(where, `(lower(display_name) LIKE ? OR lower(sys_name) LIKE ? OR lower(hostname) LIKE ? OR lower(model) LIKE ?
			OR lower(make) LIKE ? OR lower(serial) LIKE ? OR address LIKE ? OR lower(location) LIKE ? OR lower(asset_tag) LIKE ? OR lower(mac) LIKE ?)`)
		for i := 0; i < 10; i++ {
			args = append(args, q)
		}
	}
	if len(f.IDs) > 0 {
		where = append(where, "id IN ("+placeholders(len(f.IDs))+")")
		args = append(args, toAny(f.IDs)...)
	}
	if len(f.GroupIDs) > 0 {
		where = append(where, "id IN (SELECT device_id FROM group_members WHERE group_id IN ("+placeholders(len(f.GroupIDs))+"))")
		args = append(args, toAny(f.GroupIDs)...)
	}
	if len(where) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(where, " AND "), args
}

// ListDevices implements Store.
func (s *Postgres) ListDevices(ctx context.Context, f DeviceFilter) ([]*model.Device, error) {
	where, args := buildDeviceWherePG(f)
	order := " ORDER BY lower(COALESCE(NULLIF(display_name,''), NULLIF(sys_name,''), NULLIF(hostname,''), address))"
	switch f.Sort {
	case "model":
		order = " ORDER BY lower(model), address"
	case "address":
		order = " ORDER BY address"
	case "lastSeen":
		order = " ORDER BY last_seen DESC"
	case "severity":
		order = " ORDER BY CASE severity WHEN 'critical' THEN 0 WHEN 'offline' THEN 1 WHEN 'warning' THEN 2 ELSE 3 END, address"
	}
	limit := ""
	if f.Limit > 0 {
		limit = fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := pgQuery(ctx, s.db, `SELECT `+deviceCols+` FROM devices`+where+order+limit, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Device{}
	for rows.Next() {
		d, err := scanDevicePG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// CountDevices implements Store.
func (s *Postgres) CountDevices(ctx context.Context, f DeviceFilter) (int, error) {
	where, args := buildDeviceWherePG(f)
	var n int
	err := pgRow(ctx, s.db, `SELECT COUNT(*) FROM devices`+where, args...).Scan(&n)
	return n, err
}

// PatchDevice implements Store (operator fields only).
func (s *Postgres) PatchDevice(ctx context.Context, id string, p DevicePatch) (*model.Device, error) {
	d, err := s.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.DisplayName != nil {
		d.DisplayName = *p.DisplayName
		if p.DisplayNameSource == nil {
			d.DisplayNameSource = ""
		}
	}
	if p.DisplayNameSource != nil {
		d.DisplayNameSource = *p.DisplayNameSource
	}
	if p.DNSSync != nil {
		d.DNSSync = *p.DNSSync
	}
	if p.Location != nil {
		d.Location = *p.Location
	}
	if p.AssetTag != nil {
		d.AssetTag = *p.AssetTag
	}
	if p.Notes != nil {
		d.Notes = *p.Notes
	}
	if p.WebURLOverride != nil {
		d.WebURLOverride = strings.TrimSpace(*p.WebURLOverride)
	}
	d.FieldSources = mergeFieldSources(d.FieldSources, p.FieldSources)
	dnsSync := 0
	if d.DNSSync {
		dnsSync = 1
	}
	_, err = pgExec(ctx, s.db, `UPDATE devices SET display_name=?, display_name_source=?, dns_sync=?, location=?, asset_tag=?, notes=?, web_url_override=?, field_sources=?, updated_at=? WHERE id=?`,
		d.DisplayName, d.DisplayNameSource, dnsSync, d.Location, d.AssetTag, d.Notes, d.WebURLOverride, fieldSourcesJSON(d.FieldSources), nowUTC(), id)
	if err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// SetDeviceDNS implements Store.
func (s *Postgres) SetDeviceDNS(ctx context.Context, id, dnsName string, suspect bool, checkedAt time.Time) (*model.Device, error) {
	susp := 0
	if suspect {
		susp = 1
	}
	if checkedAt.IsZero() {
		checkedAt = nowUTC()
	}
	if err := affected(pgExec(ctx, s.db, `UPDATE devices SET dns_name=?, dns_name_suspect=?, dns_checked_at=?, updated_at=? WHERE id=?`,
		dnsName, susp, pgt(checkedAt), nowUTC(), id)); err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// SetDeviceState implements Store.
func (s *Postgres) SetDeviceState(ctx context.Context, id string, state model.LifecycleState) (*model.Device, error) {
	if err := affected(pgExec(ctx, s.db, `UPDATE devices SET state=?, updated_at=? WHERE id=?`, string(state), nowUTC(), id)); err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// ApplyPoll implements Store: writes automatic telemetry fields only.
func (s *Postgres) ApplyPoll(ctx context.Context, id string, u PollUpdate) (*model.Device, error) {
	d, err := s.GetDevice(ctx, id)
	if err != nil {
		return nil, err
	}
	sev := "offline"
	failures, _ := s.DeviceFailures(ctx, d.ID)
	if u.Reachable {
		failures = 0
		sev = "ok"
		if u.Status != nil {
			sev = string(u.Status.Severity)
		}
	} else {
		failures++
	}
	status := d.Status
	if u.Status != nil {
		status = u.Status
	}
	supplies, trays, counters := d.Supplies, d.Trays, d.Counters
	if u.Reachable {
		if u.Supplies != nil {
			supplies = u.Supplies
		}
		if u.Trays != nil {
			trays = u.Trays
		}
		if u.Counters != nil {
			counters = u.Counters
		}
	}
	fw := d.Firmware
	if u.Firmware != "" {
		fw = u.Firmware
	}
	at := u.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	lastSeen := d.LastSeen
	if u.Reachable {
		lastSeen = at
	}
	var statusJSON, countersJSON any
	if status != nil {
		statusJSON = toJSON(status)
	}
	if counters != nil {
		countersJSON = toJSON(counters)
	}
	_, err = pgExec(ctx, s.db, `UPDATE devices SET status=?, supplies=?, trays=?, counters=?, firmware=?, severity=?,
		consecutive_failures=?, last_polled=?, last_seen=?, updated_at=? WHERE id=?`,
		statusJSON, toJSON(supplies), toJSON(trays), countersJSON, fw, sev, failures, pgt(at), pgt(lastSeen), nowUTC(), id)
	if err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// DeviceFailures implements Store.
func (s *Postgres) DeviceFailures(ctx context.Context, id string) (int, error) {
	var n int
	err := pgRow(ctx, s.db, `SELECT consecutive_failures FROM devices WHERE id=?`, id).Scan(&n)
	return n, err
}

// DeleteDevice implements Store.
func (s *Postgres) DeleteDevice(ctx context.Context, id string) error {
	if err := affected(pgExec(ctx, s.db, `DELETE FROM devices WHERE id=?`, id)); err != nil {
		return err
	}
	_, _ = pgExec(ctx, s.db, `DELETE FROM telemetry WHERE device_id=?`, id)
	_, _ = pgExec(ctx, s.db, `DELETE FROM alerts WHERE device_id=?`, id)
	_, _ = pgExec(ctx, s.db, `DELETE FROM supply_links WHERE device_id=?`, id)
	return nil
}

// Dashboard implements Store.
func (s *Postgres) Dashboard(ctx context.Context) (*DashboardSummary, error) {
	ds := &DashboardSummary{ByState: map[string]int{}, BySeverity: map[string]int{}}
	rows, err := pgQuery(ctx, s.db, `SELECT state, severity, COUNT(*) FROM devices GROUP BY state, severity`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st, sev string
		var n int
		if err := rows.Scan(&st, &sev, &n); err != nil {
			return nil, err
		}
		ds.ByState[st] += n
		if st != string(model.StateDecommissioned) {
			ds.Devices += n
			ds.BySeverity[sev] += n
			if sev == "offline" {
				ds.Offline += n
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = pgRow(ctx, s.db, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL`).Scan(&ds.ActiveAlerts)
	_ = pgRow(ctx, s.db, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL AND severity='critical'`).Scan(&ds.CriticalAlerts)
	_ = pgRow(ctx, s.db, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL AND source='supply'`).Scan(&ds.LowSupplies)
	_ = pgRow(ctx, s.db, `SELECT COUNT(*) FROM consumables WHERE on_hand <= 0 OR on_hand <= warning_at`).Scan(&ds.LowStock)
	_ = pgRow(ctx, s.db, `SELECT COUNT(*) FROM jobs WHERE status IN ('queued','running')`).Scan(&ds.RunningJobs)
	return ds, nil
}

// --- groups ----------------------------------------------------------------

func scanGroupPG(r rowScanner) (*model.Group, error) {
	var g model.Group
	var rule sql.NullString
	var c, u time.Time
	if err := r.Scan(&g.ID, &g.Name, &g.Description, &g.Kind, &rule, &c, &u); err != nil {
		return nil, err
	}
	if rule.Valid && rule.String != "" && rule.String != "null" {
		g.Rule = &model.GroupRule{}
		fromJSON(rule, g.Rule)
	}
	g.CreatedAt, g.UpdatedAt = c.UTC(), u.UTC()
	g.MemberIDs = []string{}
	return &g, nil
}

func (s *Postgres) CreateGroup(ctx context.Context, g *model.Group) error {
	if g.ID == "" {
		g.ID = newID()
	}
	if g.Kind == "" {
		g.Kind = "static"
	}
	n := nowUTC()
	g.CreatedAt, g.UpdatedAt = n, n
	var rule any
	if g.Rule != nil {
		rule = toJSON(g.Rule)
	}
	_, err := pgExec(ctx, s.db, `INSERT INTO groups(id,name,description,kind,rule,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		g.ID, g.Name, g.Description, g.Kind, rule, n, n)
	if err != nil {
		return err
	}
	if len(g.MemberIDs) > 0 {
		return s.SetGroupMembers(ctx, g.ID, g.MemberIDs)
	}
	return nil
}

func (s *Postgres) UpdateGroup(ctx context.Context, g *model.Group) error {
	var rule any
	if g.Rule != nil {
		rule = toJSON(g.Rule)
	}
	return affected(pgExec(ctx, s.db, `UPDATE groups SET name=?, description=?, kind=?, rule=?, updated_at=? WHERE id=?`,
		g.Name, g.Description, g.Kind, rule, nowUTC(), g.ID))
}

func (s *Postgres) DeleteGroup(ctx context.Context, id string) error {
	return affected(pgExec(ctx, s.db, `DELETE FROM groups WHERE id=?`, id))
}

func (s *Postgres) GetGroup(ctx context.Context, id string) (*model.Group, error) {
	g, err := scanGroupPG(pgRow(ctx, s.db, `SELECT id,name,description,kind,rule,created_at,updated_at FROM groups WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.MemberIDs, err = s.members(ctx, id)
	return g, err
}

func (s *Postgres) members(ctx context.Context, groupID string) ([]string, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT device_id FROM group_members WHERE group_id=?`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Postgres) ListGroups(ctx context.Context) ([]*model.Group, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT id,name,description,kind,rule,created_at,updated_at FROM groups ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Group{}
	for rows.Next() {
		g, err := scanGroupPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, g := range out {
		if g.MemberIDs, err = s.members(ctx, g.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Postgres) SetGroupMembers(ctx context.Context, groupID string, deviceIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := pgExec(ctx, tx, `DELETE FROM group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, id := range deviceIDs {
		if _, err := pgExec(ctx, tx, `INSERT INTO group_members(group_id,device_id) VALUES(?,?) ON CONFLICT DO NOTHING`, groupID, id); err != nil {
			return err
		}
	}
	if _, err := pgExec(ctx, tx, `UPDATE groups SET updated_at=? WHERE id=?`, nowUTC(), groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Postgres) GroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	rows, err := pgQuery(ctx, s.db, `SELECT group_id FROM group_members WHERE device_id=?`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetDeviceWebURLProbed records the web-page probe result (see model.ResolveWebURL).
func (s *Postgres) SetDeviceWebURLProbed(ctx context.Context, id, url string) error {
	res, err := pgExec(ctx, s.db, `UPDATE devices SET web_url_probed=?, updated_at=? WHERE id=?`, url, nowUTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
