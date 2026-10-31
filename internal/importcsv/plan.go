package importcsv

import (
	"fmt"
	"net/netip"
	"strings"

	"printmarshal/internal/model"
)

// Outcomes of a row.
const (
	OutcomeCreate = "create"
	OutcomeUpdate = "update"
	OutcomeSkip   = "skip"
)

// Why a supplied value is left alone when its column is not marked replace
// (spec R10), by where the present value came from.
const (
	ReasonOperatorSet = "set by an operator; column not marked replace"
	ReasonDNSSync     = "kept in sync with DNS; column not marked replace"
	// ReasonHasValue is for a value with no recorded source: an operator's
	// edit from before sources were recorded, or one seeded from the agent's
	// sysLocation. Which of them cannot be told.
	ReasonHasValue = "already has a value; column not marked replace"
)

// ReasonOutOfScope skips a row whose device or group the caller may not touch (R12).
const ReasonOutOfScope = "outside your groups"

// fieldColumns are the operator fields a row may set, in the order the plan
// reports them.
var fieldColumns = []string{ColDisplayName, ColLocation, ColAssetTag, ColNotes, ColWebURL, ColDNSSync, ColState, ColGroup}

// replaceable are the columns a "replace" flag applies to: every field but
// group, which only ever adds a membership.
var replaceable = map[string]bool{ColDisplayName: true, ColLocation: true, ColAssetTag: true, ColNotes: true, ColWebURL: true, ColDNSSync: true, ColState: true}

// Lookup answers the plan's questions about the inventory. Implementations
// must not contact devices; ResolveHostname is one forward DNS lookup.
type Lookup interface {
	DeviceByAddress(address string) *model.Device
	DeviceBySerial(serial string) *model.Device
	GroupByName(name string) *model.Group
	ResolveHostname(host string) (address string, ok bool)
	DeviceInScope(deviceID string) bool
	GroupInScope(groupID string) bool
}

// Options are the caller's choices and limits.
type Options struct {
	// Replace names the columns whose values replace ones an operator set.
	Replace map[string]bool
	// Scoped is true for a group-scoped caller: a device it creates must go
	// into one of its groups, or it would land outside them.
	Scoped bool
}

// PlanResult is the preview (spec R6) and what the import job applies.
type PlanResult struct {
	Separator      string   `json:"separator"`
	IgnoredColumns []string `json:"ignoredColumns"`
	KnownColumns   []string `json:"knownColumns"`
	Totals         Totals   `json:"totals"`
	Rows           []Row    `json:"rows"`
}

// Totals counts rows per outcome.
type Totals struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Skip   int `json:"skip"`
}

// HeldBack is a supplied value the plan leaves alone, and why.
type HeldBack struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// Row is the plan for one row of the file.
type Row struct {
	Row      int            `json:"row"`
	Key      string         `json:"key"`
	Outcome  string         `json:"outcome"`
	DeviceID string         `json:"deviceId,omitempty"`
	Fields   map[string]any `json:"fields"`
	HeldBack []HeldBack     `json:"heldBack"`
	Reason   string         `json:"reason,omitempty"`

	// For the apply; never sent.
	Address  string         `json:"-"` // the row's address, typed or resolved
	Hostname string         `json:"-"` // the hostname cell, a hint for identification
	GroupID  string         `json:"-"` // the group cell's group
	Values   map[string]any `json:"-"` // every validated value the row supplied
}

// ParseReplace reads the comma-separated replace list of the API.
func ParseReplace(s string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, name := range strings.Split(s, ",") {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		c, ok := Canonical(name)
		if !ok || !replaceable[c] {
			return nil, fmt.Errorf("replace: %q is not a column that can be replaced", name)
		}
		out[c] = true
	}
	return out, nil
}

// Hostnames lists, once each and in file order, the hostnames of rows that
// have no address and so need a lookup, for a caller that wants to resolve
// them ahead of Plan (concurrently).
func Hostnames(t *Table) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range t.Records {
		h := r.Cells[ColHostname]
		if r.Cells[ColAddress] == "" && h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

type resolution struct {
	addr string
	ok   bool
}

type planner struct {
	lk       Lookup
	opts     Options
	byAddr   map[string]int
	bySerial map[string]int
	byDevice map[string]int
	resolved map[string]resolution
}

// Plan works out, without changing anything, what importing t would do. It
// is deterministic for the same table, inventory and options (spec R8).
func Plan(t *Table, lk Lookup, opts Options) PlanResult {
	p := &planner{lk: lk, opts: opts, byAddr: map[string]int{}, bySerial: map[string]int{}, byDevice: map[string]int{}, resolved: map[string]resolution{}}
	res := PlanResult{Separator: t.Separator, IgnoredColumns: append([]string{}, t.Ignored...), KnownColumns: append([]string{}, t.Known...), Rows: make([]Row, 0, len(t.Records))}
	for _, rec := range t.Records {
		row := p.row(rec)
		switch row.Outcome {
		case OutcomeCreate:
			res.Totals.Create++
		case OutcomeUpdate:
			res.Totals.Update++
		default:
			res.Totals.Skip++
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

func (p *planner) resolve(host string) (string, bool) {
	if r, ok := p.resolved[host]; ok {
		return r.addr, r.ok
	}
	a, ok := p.lk.ResolveHostname(host)
	if ok {
		if ip, err := netip.ParseAddr(strings.TrimSpace(a)); err == nil {
			a = ip.Unmap().String()
		} else {
			a, ok = "", false
		}
	}
	p.resolved[host] = resolution{a, ok}
	return a, ok
}

func (p *planner) row(rec Record) Row {
	c := rec.Cells
	row := Row{Row: rec.Line, Fields: map[string]any{}, HeldBack: []HeldBack{}}
	addrCell, host, serial := c[ColAddress], c[ColHostname], c[ColSerial]
	row.Hostname = host
	switch {
	case addrCell != "":
		row.Key = addrCell
	case host != "":
		row.Key = "host:" + host
	case serial != "":
		row.Key = "serial:" + serial
	}
	skip := func(reason string) Row {
		row.Outcome, row.Reason = OutcomeSkip, reason
		row.Fields = map[string]any{}
		return row
	}

	// The address: typed, or looked up from the hostname.
	addr := ""
	if addrCell != "" {
		if ip, err := netip.ParseAddr(addrCell); err == nil {
			addr = ip.Unmap().String()
			row.Key = addr
		} else if p.lk.DeviceByAddress(addrCell) != nil {
			addr = addrCell // a device discovered by name is recorded under it
		} else {
			return skip("address is not an IP address")
		}
	} else if host != "" {
		addr, _ = p.resolve(host)
	}
	row.Address = addr

	// Duplicate keys (R5): every copy after the first is skipped.
	serialKey := strings.ToUpper(serial)
	if n, ok := p.byAddr[addr]; ok && addr != "" {
		return skip(fmt.Sprintf("duplicate of row %d", n))
	}
	if n, ok := p.bySerial[serialKey]; ok && serial != "" {
		return skip(fmt.Sprintf("duplicate of row %d", n))
	}
	if addr != "" {
		p.byAddr[addr] = row.Row
	}
	if serial != "" {
		p.bySerial[serialKey] = row.Row
	}

	// Cells (R4): an invalid one skips the whole row.
	values, reason := p.values(c, &row)
	if reason != "" {
		return skip(reason)
	}
	row.Values = values

	// Matching (R7): address first, then serial.
	var byAddr, bySerial *model.Device
	if addr != "" {
		byAddr = p.lk.DeviceByAddress(addr)
	}
	if serial != "" {
		bySerial = p.lk.DeviceBySerial(serial)
	}
	if byAddr != nil && bySerial != nil && byAddr.ID != bySerial.ID {
		if !p.lk.DeviceInScope(byAddr.ID) || !p.lk.DeviceInScope(bySerial.ID) {
			return skip(ReasonOutOfScope)
		}
		return skip("address and serial belong to different devices")
	}
	dev := byAddr
	if dev == nil {
		dev = bySerial
	}

	if dev == nil {
		switch {
		case addr == "" && addrCell == "" && host != "":
			return skip("no address: hostname " + host + " did not resolve")
		case addr == "":
			return skip("no address; scan first")
		case p.opts.Scoped && row.GroupID == "":
			return skip(ReasonOutOfScope)
		}
		row.Outcome = OutcomeCreate
		row.Fields, row.HeldBack = Diff(nil, values, false, nil)
		return row
	}

	if n, ok := p.byDevice[dev.ID]; ok {
		return skip(fmt.Sprintf("duplicate of row %d", n))
	}
	p.byDevice[dev.ID] = row.Row
	if !p.lk.DeviceInScope(dev.ID) {
		return skip(ReasonOutOfScope)
	}
	row.DeviceID = dev.ID
	member := false
	if row.GroupID != "" {
		if g := p.lk.GroupByName(values[ColGroup].(string)); g != nil {
			member = contains(g.MemberIDs, dev.ID)
		}
	}
	row.Fields, row.HeldBack = Diff(dev, values, member, p.opts.Replace)
	if len(row.Fields) == 0 {
		if len(row.HeldBack) == 0 {
			return skip("unchanged")
		}
		return skip("nothing to change: every value is held back")
	}
	row.Outcome = OutcomeUpdate
	return row
}

// values validates the row's field cells as the API validates them (R4) and
// returns them typed, or the reason the first invalid one skips the row.
func (p *planner) values(c map[string]string, row *Row) (map[string]any, string) {
	values := map[string]any{}
	limit := func(col string, max int) string {
		if len(c[col]) > max {
			return fmt.Sprintf("%s is over %d characters", col, max)
		}
		return ""
	}
	for _, col := range fieldColumns {
		v, ok := c[col]
		if !ok {
			continue
		}
		switch col {
		case ColDisplayName:
			if r := limit(col, model.MaxDisplayName); r != "" {
				return nil, r
			}
			values[col] = v
		case ColLocation:
			if r := limit(col, model.MaxLocation); r != "" {
				return nil, r
			}
			values[col] = v
		case ColAssetTag:
			if r := limit(col, model.MaxAssetTag); r != "" {
				return nil, r
			}
			values[col] = v
		case ColNotes:
			if r := limit(col, model.MaxNote); r != "" {
				return nil, r
			}
			values[col] = v
		case ColWebURL:
			if model.ValidWebURL(v) != nil {
				return nil, "webUrl is not a URL"
			}
			values[col] = v
		case ColDNSSync:
			b, ok := parseBool(v)
			if !ok {
				return nil, "dnsSync must be true or false"
			}
			values[col] = b
		case ColState:
			s, ok := parseState(v)
			if !ok {
				return nil, "state is not a lifecycle state"
			}
			values[col] = string(s)
		case ColGroup:
			g := p.lk.GroupByName(v)
			switch {
			case g == nil:
				return nil, fmt.Sprintf("group %q does not exist", v)
			case g.Kind == "rule":
				return nil, fmt.Sprintf("group %q is rule-based; only static groups take members", v)
			case !p.lk.GroupInScope(g.ID):
				return nil, ReasonOutOfScope
			}
			row.GroupID = g.ID
			values[col] = g.Name
		}
	}
	return values, ""
}

// Diff decides which supplied values to set on dev (nil for a new device):
// a value equal to the current one is dropped, one over a field an operator
// set is held back unless its column is in replace (R10), and a group is set
// unless dev is already a member. A state the lifecycle cannot reach from
// the device's (a new device starts "discovered") is held back with the
// reason. The import job uses it again when a row planned as a create meets
// a device that identification found already recorded.
func Diff(dev *model.Device, values map[string]any, isMember bool, replace map[string]bool) (map[string]any, []HeldBack) {
	fields := map[string]any{}
	held := []HeldBack{}
	for _, col := range fieldColumns {
		v, ok := values[col]
		if !ok {
			continue
		}
		switch {
		case col == ColGroup:
			if dev == nil || !isMember {
				fields[col] = v
			}
		case dev == nil:
			fields[col] = v
		case current(dev, col) == v:
		case !replace[col] && holdBackReason(dev, col) != "":
			held = append(held, HeldBack{Field: col, Reason: holdBackReason(dev, col)})
		default:
			fields[col] = v
		}
	}
	from := model.StateDiscovered
	if dev != nil {
		from = dev.State
	}
	return fields, checkState(from, fields, held)
}

// checkState moves a state the lifecycle does not allow from `from` out of
// fields into the held-back list; a new device's own state is dropped.
func checkState(from model.LifecycleState, fields map[string]any, held []HeldBack) []HeldBack {
	v, ok := fields[ColState]
	if !ok {
		return held
	}
	to := model.LifecycleState(v.(string))
	if to == from {
		delete(fields, ColState)
		return held
	}
	if !model.ValidTransition(from, to) {
		delete(fields, ColState)
		held = append(held, HeldBack{Field: ColState, Reason: fmt.Sprintf("cannot move from %s to %s", from, to)})
	}
	return held
}

// current is a device's present value of an operator field.
func current(d *model.Device, col string) any {
	switch col {
	case ColDisplayName:
		return d.DisplayName
	case ColLocation:
		return d.Location
	case ColAssetTag:
		return d.AssetTag
	case ColNotes:
		return d.Notes
	case ColWebURL:
		return d.WebURLOverride
	case ColDNSSync:
		return d.DNSSync
	case ColState:
		return string(d.State)
	}
	return nil
}

// holdBackReason says why a field's present value is kept over a supplied
// one, or "" when it may be overwritten: an import may overwrite its own
// value, so a typo is fixed by importing again.
func holdBackReason(d *model.Device, col string) string {
	if src, ok := d.FieldSources[col]; ok {
		switch src.Source {
		case model.SourceImport:
			return ""
		case model.SourceOperator:
			return ReasonOperatorSet
		case model.SourceDNS:
			return ReasonDNSSync
		}
		return ReasonHasValue
	}
	if hasValue(d, col) {
		return ReasonHasValue
	}
	return ""
}

// hasValue reports whether a field differs from what a new record holds: a
// non-empty text, DNS sync turned on, a lifecycle state past "discovered".
func hasValue(d *model.Device, col string) bool {
	switch col {
	case ColDNSSync:
		return d.DNSSync
	case ColState:
		return d.State != "" && d.State != model.StateDiscovered
	}
	s, _ := current(d, col).(string)
	return s != ""
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "true", "yes", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	}
	return false, false
}

func parseState(v string) (model.LifecycleState, bool) {
	s := model.LifecycleState(strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(v)))
	switch s {
	case model.StateDiscovered, model.StateOnboarded, model.StateActive, model.StateNeedsAttention, model.StateDecommissioned:
		return s, true
	}
	return "", false
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
