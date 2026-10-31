package importcsv

import (
	"reflect"
	"strings"
	"testing"

	"printmarshal/internal/model"
)

// fakeLookup is an inventory in memory; resolved counts hostname lookups.
type fakeLookup struct {
	devices  []*model.Device
	groups   []*model.Group
	hosts    map[string]string
	outDev   map[string]bool
	outGroup map[string]bool
	resolved int
}

func (f *fakeLookup) DeviceByAddress(a string) *model.Device {
	for _, d := range f.devices {
		if d.Address == a {
			return d
		}
	}
	return nil
}

func (f *fakeLookup) DeviceBySerial(s string) *model.Device {
	for _, d := range f.devices {
		if d.Serial != "" && strings.EqualFold(d.Serial, s) {
			return d
		}
	}
	return nil
}

func (f *fakeLookup) GroupByName(n string) *model.Group {
	for _, g := range f.groups {
		if g.Name == n {
			return g
		}
	}
	return nil
}

func (f *fakeLookup) ResolveHostname(h string) (string, bool) {
	f.resolved++
	a, ok := f.hosts[h]
	return a, ok
}

func (f *fakeLookup) DeviceInScope(id string) bool { return !f.outDev[id] }
func (f *fakeLookup) GroupInScope(id string) bool  { return !f.outGroup[id] }

func inventory() *fakeLookup {
	return &fakeLookup{
		devices: []*model.Device{
			{ID: "dA", Address: "10.0.0.5", Serial: "SA", DisplayName: "Front desk", State: model.StateActive},
			{ID: "dB", Address: "10.0.0.6", Serial: "SB", State: model.StateDiscovered},
			{ID: "dC", Address: "10.0.0.7", Serial: "SC", State: model.StateDiscovered},
			{ID: "dD", Address: "10.0.0.10", Serial: "SD", State: model.StateDiscovered},
			{ID: "dE", Address: "10.0.0.12", Serial: "SE", Location: "Floor 1", State: model.StateDiscovered},
			{ID: "dF", Address: "10.0.0.13", Serial: "SF", Notes: "keep", DNSSync: true, State: model.StateDecommissioned},
		},
		groups: []*model.Group{
			{ID: "g1", Name: "Floor 2", Kind: "static", MemberIDs: []string{"dA"}},
			{ID: "g2", Name: "Rules", Kind: "rule", Rule: &model.GroupRule{MakeContains: "HP"}},
			{ID: "g3", Name: "Other", Kind: "static"},
		},
		hosts:    map[string]string{"lp-1": "10.0.0.9"},
		outDev:   map[string]bool{"dC": true},
		outGroup: map[string]bool{"g3": true},
	}
}

func mustParse(t *testing.T, csv string) *Table {
	t.Helper()
	tab, err := Parse(strings.NewReader(csv), MaxRows, MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return tab
}

const held = "set by an operator; column not marked replace"

func TestPlan(t *testing.T) {
	file := strings.Join([]string{
		"Address,Hostname,Serial,Display name,Location,Asset tag,Notes,Web address,DNS sync,Lifecycle state,Group,owner",
		"10.0.0.5,,,Reception,Floor 2,,,,,,,Kim",               // 2 update dA: location set, name held back
		"10.0.0.6,,,,,,,,yes,,Floor 2,",                        // 3 update dB: dnsSync and group
		"10.0.0.8,,,New one,,A-8,,,,active,,",                  // 4 create
		"10.0.0.5,,,,Floor 9,,,,,,,",                           // 5 duplicate address of row 2
		",,SB,,Floor 3,,,,,,,",                                 // 6 serial matches dB, already row 3's device
		",,ZZZ,,Floor 3,,,,,,,",                                // 7 serial without address, no match
		",lp-1,,,Floor 4,,,,,,,",                               // 8 hostname resolves: create
		",lp-x,,,Floor 4,,,,,,,",                               // 9 hostname does not resolve
		"10.0.0.20,,,,,,,ftp://x,,,,",                          // 10 bad URL
		"10.0.0.21,,,,,,,,maybe,,,",                            // 11 bad bool
		"10.0.0.22,,,,,,,,,broken,,",                           // 12 bad state
		"10.0.0.23,,,," + strings.Repeat("x", 201) + ",,,,,,,", // 13 location over limit
		"10.0.0.24,,,,,,,,,,Nope,",                             // 14 unknown group
		"10.0.0.25,,,,,,,,,,Rules,",                            // 15 rule group
		"10.0.0.7,,,,Floor 5,,,,,,,",                           // 16 device outside scope
		"not-an-ip,,,,,,,,,,,",                                 // 17 bad address
		"10.0.0.10,,SA,,Floor 6,,,,,,,",                        // 18 address and serial disagree
		"10.0.0.26,,,,,,,,,,Other,",                            // 19 group outside scope
		"10.0.0.12,,,,Floor 1,,,,,,,",                          // 20 unchanged
		"10.0.0.13,,,,,,new note,,no,needs_attention,,",        // 21 everything held back
		"10.0.0.27,,,,,,,,,needs_attention,,",                  // 22 create: state cannot be entered from discovered
		"10.0.0.28,,,,,,,,,,,",                                 // 23 create with nothing but the address
		"10.0.0.29,,sb,,,,,,,,,",                               // 24 duplicate serial (case-insensitive) of row 6
	}, "\n") + "\n"

	want := []Row{
		{Row: 2, Key: "10.0.0.5", Outcome: OutcomeUpdate, DeviceID: "dA", Fields: map[string]any{"location": "Floor 2"}, HeldBack: []HeldBack{{Field: "displayName", Reason: held}}},
		{Row: 3, Key: "10.0.0.6", Outcome: OutcomeUpdate, DeviceID: "dB", Fields: map[string]any{"dnsSync": true, "group": "Floor 2"}, HeldBack: []HeldBack{}},
		{Row: 4, Key: "10.0.0.8", Outcome: OutcomeCreate, Fields: map[string]any{"displayName": "New one", "assetTag": "A-8", "state": "active"}, HeldBack: []HeldBack{}},
		{Row: 5, Key: "10.0.0.5", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "duplicate of row 2"},
		{Row: 6, Key: "serial:SB", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "duplicate of row 3"},
		{Row: 7, Key: "serial:ZZZ", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "no address; scan first"},
		{Row: 8, Key: "host:lp-1", Outcome: OutcomeCreate, Fields: map[string]any{"location": "Floor 4"}, HeldBack: []HeldBack{}},
		{Row: 9, Key: "host:lp-x", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "no address: hostname lp-x did not resolve"},
		{Row: 10, Key: "10.0.0.20", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "webUrl is not a URL"},
		{Row: 11, Key: "10.0.0.21", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "dnsSync must be true or false"},
		{Row: 12, Key: "10.0.0.22", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "state is not a lifecycle state"},
		{Row: 13, Key: "10.0.0.23", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "location is over 200 characters"},
		{Row: 14, Key: "10.0.0.24", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: `group "Nope" does not exist`},
		{Row: 15, Key: "10.0.0.25", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: `group "Rules" is rule-based; only static groups take members`},
		{Row: 16, Key: "10.0.0.7", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "outside your groups"},
		{Row: 17, Key: "not-an-ip", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "address is not an IP address"},
		{Row: 18, Key: "10.0.0.10", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "address and serial belong to different devices"},
		{Row: 19, Key: "10.0.0.26", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "outside your groups"},
		{Row: 20, Key: "10.0.0.12", Outcome: OutcomeSkip, DeviceID: "dE", Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "unchanged"},
		{Row: 21, Key: "10.0.0.13", Outcome: OutcomeSkip, DeviceID: "dF", Fields: map[string]any{}, HeldBack: []HeldBack{
			{Field: "notes", Reason: held}, {Field: "dnsSync", Reason: held}, {Field: "state", Reason: held},
		}, Reason: "nothing to change: every value is held back"},
		{Row: 22, Key: "10.0.0.27", Outcome: OutcomeCreate, Fields: map[string]any{}, HeldBack: []HeldBack{{Field: "state", Reason: "cannot move from discovered to needs_attention"}}},
		{Row: 23, Key: "10.0.0.28", Outcome: OutcomeCreate, Fields: map[string]any{}, HeldBack: []HeldBack{}},
		{Row: 24, Key: "10.0.0.29", Outcome: OutcomeSkip, Fields: map[string]any{}, HeldBack: []HeldBack{}, Reason: "duplicate of row 6"},
	}

	lk := inventory()
	got := Plan(mustParse(t, file), lk, Options{})
	if got.Separator != "," || !reflect.DeepEqual(got.IgnoredColumns, []string{"owner"}) {
		t.Errorf("separator %q ignored %v", got.Separator, got.IgnoredColumns)
	}
	wantKnown := []string{"address", "hostname", "serial", "displayName", "location", "assetTag", "notes", "webUrl", "dnsSync", "state", "group"}
	if !reflect.DeepEqual(got.KnownColumns, wantKnown) {
		t.Errorf("known %v", got.KnownColumns)
	}
	if len(got.Rows) != len(want) {
		t.Fatalf("%d rows, want %d", len(got.Rows), len(want))
	}
	for i := range want {
		g := got.Rows[i]
		w := want[i]
		g.Address, g.Hostname, g.GroupID, g.Values = "", "", "", nil
		if !reflect.DeepEqual(g, w) {
			t.Errorf("row %d:\n got %+v\nwant %+v", w.Row, g, w)
		}
	}
	if got.Totals != (Totals{Create: 4, Update: 2, Skip: 17}) {
		t.Errorf("totals %+v", got.Totals)
	}
	// What apply needs and the JSON leaves out.
	if r := got.Rows[6]; r.Address != "10.0.0.9" || r.Hostname != "lp-1" {
		t.Errorf("resolved row: address %q hostname %q", r.Address, r.Hostname)
	}
	if r := got.Rows[1]; r.GroupID != "g1" {
		t.Errorf("group id %q", r.GroupID)
	}
	// R8: the same file over the same inventory gives the same plan.
	again := Plan(mustParse(t, file), inventory(), Options{})
	if !reflect.DeepEqual(got, again) {
		t.Error("plan is not deterministic")
	}
}

func TestPlanReplace(t *testing.T) {
	file := "address,display name,location,notes,dns sync,state\n" +
		"10.0.0.5,Reception,Floor 2,,,\n" +
		"10.0.0.13,,,new note,no,onboarded\n"
	got := Plan(mustParse(t, file), inventory(), Options{Replace: map[string]bool{"displayName": true, "notes": true, "dnsSync": true, "state": true}})
	if r := got.Rows[0]; r.Outcome != OutcomeUpdate || !reflect.DeepEqual(r.Fields, map[string]any{"displayName": "Reception", "location": "Floor 2"}) || len(r.HeldBack) != 0 {
		t.Errorf("replace displayName: %+v", r)
	}
	if r := got.Rows[1]; r.Outcome != OutcomeUpdate || !reflect.DeepEqual(r.Fields, map[string]any{"notes": "new note", "dnsSync": false, "state": "onboarded"}) || len(r.HeldBack) != 0 {
		t.Errorf("replace on a decommissioned device: %+v", r)
	}
	// Replace does not excuse a transition the lifecycle forbids.
	got = Plan(mustParse(t, "address,state\n10.0.0.5,onboarded\n"), inventory(), Options{Replace: map[string]bool{"state": true}})
	if r := got.Rows[0]; r.Outcome != OutcomeSkip || !reflect.DeepEqual(r.HeldBack, []HeldBack{{Field: "state", Reason: "cannot move from active to onboarded"}}) {
		t.Errorf("forbidden transition: %+v", r)
	}
}

// A group-scoped caller may not create a device its groups would not contain.
func TestPlanScopedCreateNeedsGroup(t *testing.T) {
	file := "address,group\n10.0.0.30,\n10.0.0.31,Floor 2\n"
	got := Plan(mustParse(t, file), inventory(), Options{Scoped: true})
	if r := got.Rows[0]; r.Outcome != OutcomeSkip || r.Reason != "outside your groups" {
		t.Errorf("scoped create without group: %+v", r)
	}
	if r := got.Rows[1]; r.Outcome != OutcomeCreate || r.GroupID != "g1" {
		t.Errorf("scoped create into its group: %+v", r)
	}
}

// Only rows without an address are resolved, each hostname once.
func TestPlanResolvesOnlyWhatItNeeds(t *testing.T) {
	lk := inventory()
	Plan(mustParse(t, "address,hostname\n10.0.0.40,lp-1\n,lp-1\n,lp-1\n"), lk, Options{})
	if lk.resolved != 1 {
		t.Errorf("%d lookups, want 1", lk.resolved)
	}
	if got := Hostnames(mustParse(t, "address,hostname\n10.0.0.40,a\n,b\n,b\n,c\n")); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("Hostnames = %v", got)
	}
}

func TestDiff(t *testing.T) {
	dev := &model.Device{ID: "d", DisplayName: "Kept", Location: "", State: model.StateDiscovered}
	values := map[string]any{"displayName": "New", "location": "L", "group": "Floor 2"}
	fields, heldBack := Diff(dev, values, true, nil)
	if !reflect.DeepEqual(fields, map[string]any{"location": "L"}) || !reflect.DeepEqual(heldBack, []HeldBack{{Field: "displayName", Reason: held}}) {
		t.Errorf("member: %v %v", fields, heldBack)
	}
	fields, _ = Diff(dev, values, false, map[string]bool{"displayName": true})
	if !reflect.DeepEqual(fields, map[string]any{"displayName": "New", "location": "L", "group": "Floor 2"}) {
		t.Errorf("non-member with replace: %v", fields)
	}
	// A recorded source marks a field as chosen even at its default value.
	off := &model.Device{ID: "o", State: model.StateDiscovered, FieldSources: map[string]model.FieldSource{"dnsSync": {Source: model.SourceOperator}, "state": {Source: model.SourceOperator}}}
	fields, heldBack = Diff(off, map[string]any{"dnsSync": true, "state": "active"}, false, nil)
	if len(fields) != 0 || !reflect.DeepEqual(heldBack, []HeldBack{{Field: "dnsSync", Reason: held}, {Field: "state", Reason: held}}) {
		t.Errorf("recorded sources: %v %v", fields, heldBack)
	}
	// A new device takes every value.
	fields, heldBack = Diff(nil, values, false, nil)
	if !reflect.DeepEqual(fields, values) || len(heldBack) != 0 {
		t.Errorf("create: %v %v", fields, heldBack)
	}
}

func TestParseReplace(t *testing.T) {
	got, err := ParseReplace(" Display name, location ,,dns_sync")
	if err != nil || !reflect.DeepEqual(got, map[string]bool{"displayName": true, "location": true, "dnsSync": true}) {
		t.Errorf("ParseReplace = %v, %v", got, err)
	}
	for _, bad := range []string{"address", "group", "owner"} {
		if _, err := ParseReplace(bad); err == nil {
			t.Errorf("ParseReplace(%q) accepted", bad)
		}
	}
}
