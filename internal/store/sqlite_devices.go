package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"printmarshal/internal/model"
)

const deviceCols = `id, address, hostname, dns_name, dns_name_suspect, dns_checked_at, mac, serial, uuid, make, model, device_type, firmware, sys_descr, sys_name,
 enterprise_id, admin_url, capabilities, discovery_methods, identify_methods, display_name, display_name_source, dns_sync, location, asset_tag, notes,
 state, status, supplies, trays, counters, severity, consecutive_failures, first_seen, last_seen, last_polled, created_at, updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanDevice(r rowScanner) (*model.Device, error) {
	var (
		d                                  model.Device
		caps, disc, ident, supplies, trays string
		status, counters                   sql.NullString
		firstSeen, lastSeen, created, upd  string
		lastPolled, dnsCheckedAt           sql.NullString
		dnsNameSuspect, dnsSync            int
		failures                           int
		severity                           string
	)
	err := r.Scan(&d.ID, &d.Address, &d.Hostname, &d.DNSName, &dnsNameSuspect, &dnsCheckedAt, &d.MAC, &d.Serial, &d.UUID, &d.Make, &d.Model, &d.DeviceType, &d.Firmware,
		&d.SysDescr, &d.SysName, &d.EnterpriseID, &d.AdminURL, &caps, &disc, &ident, &d.DisplayName, &d.DisplayNameSource, &dnsSync, &d.Location, &d.AssetTag,
		&d.Notes, &d.State, &status, &supplies, &trays, &counters, &severity, &failures, &firstSeen, &lastSeen, &lastPolled, &created, &upd)
	if err != nil {
		return nil, err
	}
	d.DNSNameSuspect = dnsNameSuspect != 0
	d.DNSCheckedAt = parseTimeP(dnsCheckedAt)
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
	d.FirstSeen, d.LastSeen = parseTime(firstSeen), parseTime(lastSeen)
	d.LastPolled = parseTimeP(lastPolled)
	d.CreatedAt, d.UpdatedAt = parseTime(created), parseTime(upd)
	return &d, nil
}

func (s *SQLite) getDevice(ctx context.Context, q rowScanner) (*model.Device, error) {
	d, err := scanDevice(q)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return d, err
}

// GetDevice implements Store.
func (s *SQLite) GetDevice(ctx context.Context, id string) (*model.Device, error) {
	return s.getDevice(ctx, s.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE id=?`, id))
}

// findByIdentity de-duplicates by serial, then UUID, then MAC, then address (spec §8.1).
func (s *SQLite) findByIdentity(ctx context.Context, id *model.Identity) (*model.Device, error) {
	try := func(col, val string) (*model.Device, error) {
		if val == "" {
			return nil, ErrNotFound
		}
		return s.getDevice(ctx, s.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE `+col+`=? ORDER BY last_seen DESC LIMIT 1`, val))
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

// IdentityConflicts reports whether an address-matched record and an incoming
// identity carry different, non-empty stable identifiers (serial, UUID or
// MAC). Such a pair is two devices that have shared an address, never one.
func IdentityConflicts(existing *model.Device, id *model.Identity) bool {
	differs := func(a, b string) bool {
		a, b = strings.TrimSpace(a), strings.TrimSpace(b)
		return a != "" && b != "" && !strings.EqualFold(a, b)
	}
	return differs(existing.Serial, id.Serial) || differs(existing.UUID, id.UUID) || differs(existing.MAC, id.MAC)
}

// UpsertDevice implements Store. Automatic fields are refreshed; operator
// fields, state and telemetry are untouched (spec §3 principle 5).
func (s *SQLite) UpsertDevice(ctx context.Context, id *model.Identity, caps map[model.Capability]string, discoveryMethods []string) (*model.Device, bool, error) {
	existing, err := s.findByIdentity(ctx, id)
	if err != nil && err != ErrNotFound {
		return nil, false, err
	}
	identMethods := strings.Split(id.Method, ",")
	n := now()
	if existing == nil {
		d := &model.Device{ID: newID()}
		_, err := s.db.ExecContext(ctx, `INSERT INTO devices (`+deviceCols+`) VALUES (`+placeholders(38)+`)`,
			d.ID, id.Address, id.Hostname, "", 0, nil, strings.ToLower(id.MAC), id.Serial, id.UUID, id.Make, id.Model, id.DeviceType, id.Firmware,
			id.SysDescr, id.SysName, id.EnterpriseID, id.AdminURL, toJSON(caps), toJSON(discoveryMethods), toJSON(identMethods),
			"", "", 0, id.SysLocation, "", "", string(model.StateDiscovered), nil, "[]", "[]", nil, "ok", 0, n, n, nil, n, n)
		if err != nil {
			return nil, false, err
		}
		dev, err := s.GetDevice(ctx, d.ID)
		return dev, true, err
	}
	// Merge: keep existing values where the new identity is silent.
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
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET address=?, hostname=?, mac=?, serial=?, uuid=?, make=?, model=?, device_type=?,
		firmware=?, sys_descr=?, sys_name=?, enterprise_id=?, admin_url=?, capabilities=?, discovery_methods=?, identify_methods=?,
		last_seen=?, updated_at=? WHERE id=?`,
		id.Address, pick(id.Hostname, existing.Hostname), strings.ToLower(pick(id.MAC, existing.MAC)), pick(id.Serial, existing.Serial),
		pick(id.UUID, existing.UUID), pick(id.Make, existing.Make), pick(id.Model, existing.Model), pick(id.DeviceType, existing.DeviceType),
		pick(id.Firmware, existing.Firmware), pick(id.SysDescr, existing.SysDescr), pick(id.SysName, existing.SysName),
		maxInt(id.EnterpriseID, existing.EnterpriseID), pick(id.AdminURL, existing.AdminURL), toJSON(merged), toJSON(disc), toJSON(ident),
		n, n, existing.ID)
	if err != nil {
		return nil, false, err
	}
	dev, err := s.GetDevice(ctx, existing.ID)
	return dev, false, err
}

func maxInt(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		if v == "" {
			continue
		}
		dup := false
		for _, d := range dst {
			if d == v {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, v)
		}
	}
	return dst
}

func buildDeviceWhere(f DeviceFilter) (string, []any) {
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
		where = append(where, "make = ? COLLATE NOCASE")
		args = append(args, f.Make)
	}
	if f.Model != "" {
		where = append(where, "model LIKE ?")
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
func (s *SQLite) ListDevices(ctx context.Context, f DeviceFilter) ([]*model.Device, error) {
	where, args := buildDeviceWhere(f)
	order := " ORDER BY COALESCE(NULLIF(display_name,''), NULLIF(sys_name,''), NULLIF(hostname,''), address) COLLATE NOCASE"
	switch f.Sort {
	case "model":
		order = " ORDER BY model COLLATE NOCASE, address"
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
	rows, err := s.db.QueryContext(ctx, `SELECT `+deviceCols+` FROM devices`+where+order+limit, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if out == nil {
		out = []*model.Device{}
	}
	return out, rows.Err()
}

// CountDevices implements Store.
func (s *SQLite) CountDevices(ctx context.Context, f DeviceFilter) (int, error) {
	where, args := buildDeviceWhere(f)
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`+where, args...).Scan(&n)
	return n, err
}

// PatchDevice implements Store (operator fields only).
func (s *SQLite) PatchDevice(ctx context.Context, id string, p DevicePatch) (*model.Device, error) {
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
	dnsSync := 0
	if d.DNSSync {
		dnsSync = 1
	}
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET display_name=?, display_name_source=?, dns_sync=?, location=?, asset_tag=?, notes=?, updated_at=? WHERE id=?`,
		d.DisplayName, d.DisplayNameSource, dnsSync, d.Location, d.AssetTag, d.Notes, now(), id)
	if err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// SetDeviceDNS implements Store.
func (s *SQLite) SetDeviceDNS(ctx context.Context, id, dnsName string, suspect bool, checkedAt time.Time) (*model.Device, error) {
	susp := 0
	if suspect {
		susp = 1
	}
	if checkedAt.IsZero() {
		checkedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET dns_name=?, dns_name_suspect=?, dns_checked_at=?, updated_at=? WHERE id=?`,
		dnsName, susp, ts(checkedAt), now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetDevice(ctx, id)
}

// SetDeviceState implements Store.
func (s *SQLite) SetDeviceState(ctx context.Context, id string, state model.LifecycleState) (*model.Device, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET state=?, updated_at=? WHERE id=?`, string(state), now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetDevice(ctx, id)
}

// ApplyPoll implements Store: writes automatic telemetry fields only.
func (s *SQLite) ApplyPoll(ctx context.Context, id string, u PollUpdate) (*model.Device, error) {
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
	_, err = s.db.ExecContext(ctx, `UPDATE devices SET status=?, supplies=?, trays=?, counters=?, firmware=?, severity=?,
		consecutive_failures=?, last_polled=?, last_seen=?, updated_at=? WHERE id=?`,
		toJSON(status), toJSON(supplies), toJSON(trays), toJSON(counters), fw, sev, failures, ts(at), ts(lastSeen), now(), id)
	if err != nil {
		return nil, err
	}
	return s.GetDevice(ctx, id)
}

// DeviceFailures implements Store.
func (s *SQLite) DeviceFailures(ctx context.Context, id string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT consecutive_failures FROM devices WHERE id=?`, id).Scan(&n)
	return n, err
}

// DeleteDevice implements Store.
func (s *SQLite) DeleteDevice(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM devices WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM telemetry WHERE device_id=?`, id)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM alerts WHERE device_id=?`, id)
	_, _ = s.db.ExecContext(ctx, `DELETE FROM supply_links WHERE device_id=?`, id)
	return nil
}

// Dashboard implements Store.
func (s *SQLite) Dashboard(ctx context.Context) (*DashboardSummary, error) {
	ds := &DashboardSummary{ByState: map[string]int{}, BySeverity: map[string]int{}}
	rows, err := s.db.QueryContext(ctx, `SELECT state, severity, COUNT(*) FROM devices GROUP BY state, severity`)
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
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL`).Scan(&ds.ActiveAlerts)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL AND severity='critical'`).Scan(&ds.CriticalAlerts)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE cleared_at IS NULL AND source='supply'`).Scan(&ds.LowSupplies)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM consumables WHERE on_hand <= 0 OR on_hand <= warning_at`).Scan(&ds.LowStock)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status IN ('queued','running')`).Scan(&ds.RunningJobs)
	return ds, nil
}

// --- groups ----------------------------------------------------------------

func scanGroup(r rowScanner) (*model.Group, error) {
	var g model.Group
	var rule sql.NullString
	var c, u string
	if err := r.Scan(&g.ID, &g.Name, &g.Description, &g.Kind, &rule, &c, &u); err != nil {
		return nil, err
	}
	if rule.Valid && rule.String != "" && rule.String != "null" {
		g.Rule = &model.GroupRule{}
		fromJSON(rule, g.Rule)
	}
	g.CreatedAt, g.UpdatedAt = parseTime(c), parseTime(u)
	g.MemberIDs = []string{}
	return &g, nil
}

func (s *SQLite) CreateGroup(ctx context.Context, g *model.Group) error {
	if g.ID == "" {
		g.ID = newID()
	}
	if g.Kind == "" {
		g.Kind = "static"
	}
	n := now()
	g.CreatedAt, g.UpdatedAt = parseTime(n), parseTime(n)
	var rule any
	if g.Rule != nil {
		rule = toJSON(g.Rule)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO groups(id,name,description,kind,rule,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`,
		g.ID, g.Name, g.Description, g.Kind, rule, n, n)
	if err != nil {
		return err
	}
	if len(g.MemberIDs) > 0 {
		return s.SetGroupMembers(ctx, g.ID, g.MemberIDs)
	}
	return nil
}

func (s *SQLite) UpdateGroup(ctx context.Context, g *model.Group) error {
	var rule any
	if g.Rule != nil {
		rule = toJSON(g.Rule)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE groups SET name=?, description=?, kind=?, rule=?, updated_at=? WHERE id=?`,
		g.Name, g.Description, g.Kind, rule, now(), g.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteGroup(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) GetGroup(ctx context.Context, id string) (*model.Group, error) {
	g, err := scanGroup(s.db.QueryRowContext(ctx, `SELECT id,name,description,kind,rule,created_at,updated_at FROM groups WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.MemberIDs, err = s.members(ctx, id)
	return g, err
}

func (s *SQLite) members(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT device_id FROM group_members WHERE group_id=?`, groupID)
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

func (s *SQLite) ListGroups(ctx context.Context) ([]*model.Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,description,kind,rule,created_at,updated_at FROM groups ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Group{}
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	rows.Close()
	for _, g := range out {
		if g.MemberIDs, err = s.members(ctx, g.ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *SQLite) SetGroupMembers(ctx context.Context, groupID string, deviceIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	for _, id := range deviceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO group_members(group_id,device_id) VALUES(?,?)`, groupID, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE groups SET updated_at=? WHERE id=?`, now(), groupID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLite) GroupsForDevice(ctx context.Context, deviceID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT group_id FROM group_members WHERE device_id=?`, deviceID)
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

// MatchRule evaluates a rule-based group against a device (spec §8.2).
func MatchRule(r *model.GroupRule, d *model.Device) bool {
	if r == nil {
		return false
	}
	has := func(hay, needle string) bool {
		return needle == "" || strings.Contains(strings.ToLower(hay), strings.ToLower(needle))
	}
	if !has(d.Make, r.MakeContains) || !has(d.Model, r.ModelContains) || !has(d.Location, r.LocationContains) {
		return false
	}
	if r.DeviceType != "" && d.DeviceType != r.DeviceType {
		return false
	}
	if r.State != "" && string(d.State) != r.State {
		return false
	}
	if r.HasCapability != "" {
		if _, ok := d.Capabilities[r.HasCapability]; !ok {
			return false
		}
	}
	if r.Subnet != "" {
		pfx, err := netip.ParsePrefix(r.Subnet)
		if err != nil {
			return false
		}
		addr, err := netip.ParseAddr(d.Address)
		if err != nil || !pfx.Contains(addr) {
			return false
		}
	}
	return true
}

// SortDevicesByName orders devices by display name (helper for callers).
func SortDevicesByName(devs []*model.Device) {
	sort.SliceStable(devs, func(i, j int) bool { return strings.ToLower(devs[i].Name()) < strings.ToLower(devs[j].Name()) })
}
