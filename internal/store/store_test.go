package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// Spec §14 safety test: operator fields survive a scan.
func testOperatorFieldsSurviveScan(t *testing.T, s testStore) {
	ctx := context.Background()
	id := &model.Identity{Address: "10.0.0.5", Serial: "SN1", Model: "Acme 1", Make: "Acme", SysLocation: "Scanned location", Method: "snmp"}
	d, created, err := s.UpsertDevice(ctx, id, map[model.Capability]string{model.CapReadStatus: "snmp"}, []string{"scan"})
	if err != nil || !created {
		t.Fatalf("upsert: %v created=%v", err, created)
	}
	if d.Location != "Scanned location" {
		t.Errorf("initial location should come from the scan, got %q", d.Location)
	}
	name, loc, tag, notes := "Front desk", "Lobby", "A-100", "handle with care"
	if _, err := s.PatchDevice(ctx, d.ID, DevicePatch{DisplayName: &name, Location: &loc, AssetTag: &tag, Notes: &notes}); err != nil {
		t.Fatal(err)
	}
	// The device web page: a probe result is used until an operator overrides it.
	if err := s.SetDeviceWebURLProbed(ctx, d.ID, "https://10.0.0.7/"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetDevice(ctx, d.ID); got.WebURL != "https://10.0.0.7/" || got.WebURLProbed != "https://10.0.0.7/" {
		t.Fatalf("probed web url: %+v", got.WebURL)
	}
	web := "http://printer-7.example.net/admin"
	if got, err := s.PatchDevice(ctx, d.ID, DevicePatch{WebURLOverride: &web}); err != nil || got.WebURL != web || got.WebURLOverride != web {
		t.Fatalf("web url override: %v %q", err, got.WebURL)
	}
	if err := s.SetDeviceWebURLProbed(ctx, "nope", "x"); err != ErrNotFound {
		t.Fatalf("probe on unknown device: %v", err)
	}
	if _, err := s.SetDeviceState(ctx, d.ID, model.StateActive); err != nil {
		t.Fatal(err)
	}
	// Second scan: new address, new location, new firmware.
	id2 := &model.Identity{Address: "10.0.0.77", Serial: "SN1", Model: "Acme 1 rev2", Firmware: "3.0", SysLocation: "Somewhere else", Method: "ipp"}
	d2, created, err := s.UpsertDevice(ctx, id2, map[model.Capability]string{model.CapReadSupplies: "ipp"}, []string{"mdns"})
	if err != nil || created {
		t.Fatalf("second upsert: %v created=%v", err, created)
	}
	if d2.ID != d.ID {
		t.Fatal("device must be de-duplicated by serial")
	}
	if d2.DisplayName != name || d2.Location != loc || d2.AssetTag != tag || d2.Notes != notes || d2.State != model.StateActive {
		t.Errorf("operator fields overwritten: %+v", d2)
	}
	if d2.Address != "10.0.0.77" || d2.Firmware != "3.0" || d2.Model != "Acme 1 rev2" {
		t.Errorf("automatic fields not refreshed: %+v", d2)
	}
	if d2.Capabilities[model.CapReadStatus] != "snmp" || d2.Capabilities[model.CapReadSupplies] != "ipp" {
		t.Errorf("capabilities not merged: %+v", d2.Capabilities)
	}
	if len(d2.DiscoveryMethods) != 2 || len(d2.IdentifyMethods) != 2 {
		t.Errorf("methods = %v / %v", d2.DiscoveryMethods, d2.IdentifyMethods)
	}
	// DNSName, DNSNameSuspect, DNSCheckedAt, DisplayNameSource, DNSSync, and computed DNSNameChanged round-trip.
	checkedAt := d2.UpdatedAt
	dDNS, err := s.SetDeviceDNS(ctx, d.ID, "printer-lobby", false, checkedAt)
	if err != nil {
		t.Fatalf("SetDeviceDNS: %v", err)
	}
	if dDNS.DNSName != "printer-lobby" || dDNS.DNSNameSuspect || dDNS.DNSCheckedAt == nil || !dDNS.DNSCheckedAt.Equal(checkedAt) {
		t.Errorf("after SetDeviceDNS: dnsName=%q suspect=%v checkedAt=%v", dDNS.DNSName, dDNS.DNSNameSuspect, dDNS.DNSCheckedAt)
	}
	if dDNS.Hostname != "" {
		t.Errorf("SetDeviceDNS must not touch Hostname, got %q", dDNS.Hostname)
	}
	dnsNameVal := "printer-lobby"
	dnsSrc := "dns"
	syncFalse := false
	dPatched, err := s.PatchDevice(ctx, d.ID, DevicePatch{DisplayName: &dnsNameVal, DisplayNameSource: &dnsSrc, DNSSync: &syncFalse})
	if err != nil {
		t.Fatalf("PatchDevice dns source: %v", err)
	}
	if dPatched.DisplayName != "printer-lobby" || dPatched.DisplayNameSource != "dns" || dPatched.DNSSync || dPatched.DNSNameChanged {
		t.Errorf("after PatchDevice: %+v", dPatched)
	}
	// When DNS name changes while dnsSync is off and displayNameSource is "dns", DNSNameChanged is computed true.
	dChanged, err := s.SetDeviceDNS(ctx, d.ID, "printer-lobby-2", true, checkedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !dChanged.DNSNameSuspect || !dChanged.DNSNameChanged {
		t.Errorf("expected suspect=true and dnsNameChanged=true, got %+v", dChanged)
	}
	// Turning DNSSync on clears DNSNameChanged; manual DisplayName patch without DisplayNameSource clears DisplayNameSource.
	syncTrue := true
	dSync, err := s.PatchDevice(ctx, d.ID, DevicePatch{DNSSync: &syncTrue})
	if err != nil {
		t.Fatal(err)
	}
	if !dSync.DNSSync || dSync.DNSNameChanged {
		t.Errorf("expected dnsSync=true and dnsNameChanged=false, got %+v", dSync)
	}
	manual := "Manual name"
	dManual, err := s.PatchDevice(ctx, d.ID, DevicePatch{DisplayName: &manual})
	if err != nil {
		t.Fatal(err)
	}
	if dManual.DisplayName != manual || dManual.DisplayNameSource != "" {
		t.Errorf("manual displayName should clear displayNameSource, got %+v", dManual)
	}
	// Plain "first non-empty wins" for hostname on UpsertDevice.
	dHost1, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.77", Serial: "SN1", Hostname: "first-host", Method: "ipp"}, nil, []string{"mdns"})
	if err != nil {
		t.Fatal(err)
	}
	if dHost1.Hostname != "first-host" {
		t.Errorf("first hostname = %q, want first-host", dHost1.Hostname)
	}
	dHost2, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.77", Serial: "SN1", Hostname: "second-host", Method: "snmp"}, nil, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if dHost2.Hostname != "second-host" {
		t.Errorf("new non-empty hostname should win: got %q, want second-host", dHost2.Hostname)
	}
	dHost3, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.77", Serial: "SN1", Hostname: "", Method: "snmp"}, nil, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if dHost3.Hostname != "second-host" {
		t.Errorf("empty hostname should keep existing: got %q, want second-host", dHost3.Hostname)
	}
}

// Spec §14 safety test: secrets never leave the store except via ResolveCreds.
func testCredentialSecretsStayEncrypted(t *testing.T, s testStore) {
	ctx := context.Background()
	c := &model.Credential{Name: "site", Type: model.CredSNMPCommunity}
	if err := s.CreateCredential(ctx, c, &model.Secret{Community: "s3cretComm"}); err != nil {
		t.Fatal(err)
	}
	v3 := &model.Credential{Name: "v3", Type: model.CredSNMPv3, Username: "pm", Scope: "", Priority: 5}
	if err := s.CreateCredential(ctx, v3, &model.Secret{AuthProtocol: "SHA", AuthPassword: "authpw123", PrivProtocol: "AES", PrivPassword: "privpw123"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(list)
	for _, leak := range []string{"s3cretComm", "authpw123", "privpw123"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("secret %q leaked through ListCredentials", leak)
		}
	}
	var raw []byte
	if err := s.rawRow(`SELECT secret FROM credentials WHERE id=?`, c.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "s3cretComm") {
		t.Fatal("secret stored in clear text")
	}
	creds, err := s.ResolveCreds(ctx, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(creds.Communities) != 1 || creds.Communities[0] != "s3cretComm" || creds.V3 == nil || creds.V3.AuthPassword != "authpw123" {
		t.Errorf("resolve = %+v", creds)
	}
	// Device-scoped credential wins over global.
	dev := &model.Credential{Name: "dev", Type: model.CredSNMPCommunity, Scope: "device:abc"}
	_ = s.CreateCredential(ctx, dev, &model.Secret{Community: "devComm"})
	creds, _ = s.ResolveCreds(ctx, "abc", nil)
	if creds.Communities[0] != "devComm" {
		t.Errorf("device scope should be first: %v", creds.Communities)
	}
}

func testAlertsLifecycle(t *testing.T, s testStore) {
	ctx := context.Background()
	a, raised, err := s.RaiseAlert(ctx, "d1", "supply:1", "supply", model.SeverityWarning, "Black low")
	if err != nil || !raised {
		t.Fatalf("raise: %v %v", err, raised)
	}
	a2, raised, _ := s.RaiseAlert(ctx, "d1", "supply:1", "supply", model.SeverityCritical, "Black empty")
	if raised || a2.ID != a.ID || a2.Severity != model.SeverityCritical {
		t.Errorf("re-raise should update in place: %+v raised=%v", a2, raised)
	}
	_, _, _ = s.RaiseAlert(ctx, "d1", "offline", "offline", model.SeverityCritical, "down")
	cleared, err := s.ClearAlertsExcept(ctx, "d1", "supply", nil)
	if err != nil || len(cleared) != 1 || cleared[0].ClearedAt == nil {
		t.Errorf("clear: %v %+v", err, cleared)
	}
	active, _ := s.ListAlerts(ctx, AlertFilter{DeviceID: "d1", ActiveOnly: true})
	if len(active) != 1 || active[0].Key != "offline" {
		t.Errorf("active = %+v", active)
	}
	acked, err := s.AckAlert(ctx, active[0].ID, "op")
	if err != nil || acked.AckedBy != "op" || acked.AckedAt == nil {
		t.Errorf("ack: %v %+v", err, acked)
	}
	ds, _ := s.Dashboard(ctx)
	if ds.ActiveAlerts != 1 || ds.CriticalAlerts != 1 {
		t.Errorf("dashboard = %+v", ds)
	}
}

func testGroupsAndRules(t *testing.T, s testStore) {
	ctx := context.Background()
	d1, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.1.0.5", Serial: "A", Make: "Acme", Model: "Acme X", Method: "snmp"}, nil, nil)
	d2, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.2.0.5", Serial: "B", Make: "Other", Model: "Other Y", Method: "snmp"}, nil, nil)
	g := &model.Group{Name: "static", MemberIDs: []string{d1.ID}}
	if err := s.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	devs, _ := s.ListDevices(ctx, DeviceFilter{GroupIDs: []string{g.ID}})
	if len(devs) != 1 || devs[0].ID != d1.ID {
		t.Errorf("static members = %+v", devs)
	}
	rule := &model.GroupRule{Subnet: "10.2.0.0/16"}
	if !MatchRule(rule, d2) || MatchRule(rule, d1) {
		t.Error("subnet rule mismatch")
	}
	if !MatchRule(&model.GroupRule{MakeContains: "acme"}, d1) {
		t.Error("make rule should be case-insensitive")
	}
	found, _ := s.ListDevices(ctx, DeviceFilter{Q: "other y"})
	if len(found) != 1 || found[0].ID != d2.ID {
		t.Errorf("search = %+v", found)
	}
}

func testJobsAndTelemetry(t *testing.T, s testStore) {
	ctx := context.Background()
	j := &model.Job{Type: "poll", Target: map[string]any{"all": true}, Params: map[string]any{}, Status: model.JobRunning}
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	_ = s.AddJobResult(ctx, &model.JobResult{JobID: j.ID, DeviceID: "d1", Status: "ok", Message: "idle"})
	n, _ := s.MarkInterruptedJobs(ctx)
	if n != 1 {
		t.Errorf("interrupted = %d", n)
	}
	got, _ := s.GetJob(ctx, j.ID)
	if got.Status != model.JobFailed || got.Error == "" {
		t.Errorf("job = %+v", got)
	}
	rs, _ := s.ListJobResults(ctx, j.ID)
	if len(rs) != 1 || rs[0].DeviceID != "d1" {
		t.Errorf("results = %+v", rs)
	}
	_ = s.AddSamples(ctx, []model.TelemetrySample{{DeviceID: "d1", Metric: "supply.percent", Label: "1:Black", Value: 12}, {DeviceID: "d1", Metric: "counter.total", Value: 100}})
	ts, _ := s.QueryTelemetry(ctx, TelemetryQuery{DeviceID: "d1", Metric: "supply.percent"})
	if len(ts) != 1 || ts[0].Value != 12 {
		t.Errorf("telemetry = %+v", ts)
	}
}

// Consumables: stock history is complete and atomic, links cascade, and the
// phase-2 "stock" placeholder table is absorbed on open.
func testConsumablesStockAndLinks(t *testing.T, s testStore) {
	ctx := context.Background()
	c := &model.Consumable{Name: "Black toner", Type: model.ConsumableToner, Color: "K", PartNumber: "TN-4200", CompatibleModels: []string{"LaserPrint 4200", " "}, OnHand: 3, WarningAt: 2, CriticalAt: 1}
	if err := s.CreateConsumable(ctx, c); err != nil {
		t.Fatal(err)
	}
	if c.Color != "k" || len(c.CompatibleModels) != 1 || c.Unit != "each" {
		t.Errorf("normalisation: %+v", c)
	}
	got, _ := s.GetConsumable(ctx, c.ID)
	if got.Status() != model.StockOK {
		t.Errorf("status = %s", got.Status())
	}
	// Delta below zero clamps; set records the resulting delta; events keep history.
	upd, err := s.AdjustStock(ctx, c.ID, &model.StockEvent{Actor: "op", Kind: model.StockEventRemove, Delta: -5, Reason: "used"})
	if err != nil || upd.OnHand != 0 || upd.Status() != model.StockOut {
		t.Fatalf("remove: %v %+v", err, upd)
	}
	upd, _ = s.AdjustStock(ctx, c.ID, &model.StockEvent{Actor: "op", Kind: model.StockEventSet, Resulting: 2})
	if upd.OnHand != 2 || upd.Status() != model.StockLow {
		t.Errorf("set: %+v", upd)
	}
	// Pending replacements are recorded without changing stock.
	upd, _ = s.AdjustStock(ctx, c.ID, &model.StockEvent{Actor: "system", Kind: model.StockEventReplacement, Delta: -1, Status: model.StockEventPending, DeviceID: "d1", SupplyIndex: 1})
	if upd.OnHand != 2 {
		t.Errorf("pending replacement must not change stock: %+v", upd)
	}
	events, _ := s.ListStockEvents(ctx, StockEventFilter{ConsumableID: c.ID})
	if len(events) != 4 || events[0].Status != model.StockEventPending || events[1].Delta != 2 || events[2].Delta != -3 {
		t.Errorf("events = %+v", events)
	}
	pending, _ := s.ListStockEvents(ctx, StockEventFilter{Kind: model.StockEventReplacement, Status: model.StockEventPending})
	if len(pending) != 1 {
		t.Errorf("pending = %+v", pending)
	}
	if err := s.SetStockEventStatus(ctx, pending[0].ID, model.StockEventPending, model.StockEventDismissed); err != nil {
		t.Fatal(err)
	}
	// The transition is conditional: a second actor racing on the same event loses.
	if err := s.SetStockEventStatus(ctx, pending[0].ID, model.StockEventPending, model.StockEventConfirmed); err != ErrNotFound {
		t.Fatalf("stale transition = %v, want ErrNotFound", err)
	}
	// UpdateConsumable never touches on-hand.
	c.OnHand = 99
	c.Location = "Store B"
	if err := s.UpdateConsumable(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetConsumable(ctx, c.ID)
	if got.OnHand != 2 || got.Location != "Store B" {
		t.Errorf("update must only change item fields: %+v", got)
	}
	// Filters.
	if items, _ := s.ListConsumables(ctx, ConsumableFilter{Status: "low"}); len(items) != 1 {
		t.Errorf("status filter = %+v", items)
	}
	if items, _ := s.ListConsumables(ctx, ConsumableFilter{Q: "tn-42"}); len(items) != 1 {
		t.Errorf("search = %+v", items)
	}
	if items, _ := s.ListConsumables(ctx, ConsumableFilter{Model: "4200"}); len(items) != 1 {
		t.Errorf("model filter = %+v", items)
	}
	if items, _ := s.ListConsumables(ctx, ConsumableFilter{Type: "ink"}); len(items) != 0 {
		t.Errorf("type filter = %+v", items)
	}
	// Links: manual survives auto replacement; deleting the device or item cascades.
	d, _, _ := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.9", Serial: "L1", Model: "LaserPrint 4200", Method: "snmp"}, nil, nil)
	_ = s.SetSupplyLink(ctx, &model.SupplyLink{DeviceID: d.ID, SupplyIndex: 1, ConsumableID: c.ID, Source: "manual"})
	c2 := &model.Consumable{Name: "Drum", Type: model.ConsumableDrum}
	_ = s.CreateConsumable(ctx, c2)
	if err := s.ReplaceAutoLinks(ctx, d.ID, []*model.SupplyLink{{SupplyIndex: 1, ConsumableID: c2.ID}, {SupplyIndex: 2, ConsumableID: c2.ID}}); err != nil {
		t.Fatal(err)
	}
	links, _ := s.ListSupplyLinks(ctx, SupplyLinkFilter{DeviceID: d.ID})
	if len(links) != 2 || links[0].ConsumableID != c.ID || links[0].Source != "manual" || links[1].Source != "auto" {
		t.Errorf("links = %+v", links)
	}
	_ = s.DeleteConsumable(ctx, c2.ID)
	links, _ = s.ListSupplyLinks(ctx, SupplyLinkFilter{DeviceID: d.ID})
	if len(links) != 1 {
		t.Errorf("item delete should cascade links: %+v", links)
	}
	_ = s.DeleteDevice(ctx, d.ID)
	links, _ = s.ListSupplyLinks(ctx, SupplyLinkFilter{ConsumableID: c.ID})
	if len(links) != 0 {
		t.Errorf("device delete should cascade links: %+v", links)
	}
	ds, _ := s.Dashboard(ctx)
	if ds.LowStock != 1 {
		t.Errorf("dashboard low stock = %d", ds.LowStock)
	}
}

// Issue #19: why a driver did not answer is kept from the latest scan only,
// and a scan that could not read a field never blanks the earlier value.
func testIdentifyErrorsFollowLatestScan(t *testing.T, s testStore) {
	ctx := context.Background()
	full := &model.Identity{Address: "10.0.0.19", Serial: "SN19", Make: "HP", Model: "M283", Firmware: "1.0", Method: "snmp,hp"}
	d, _, err := s.UpsertDevice(ctx, full, nil, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.IdentifyErrors) != 0 {
		t.Fatalf("clean scan should record no errors: %+v", d.IdentifyErrors)
	}
	// eSCL never identified this printer, so its failure is expected and dropped;
	// SNMP (the baseline) and HP (which identified it before) are kept.
	partial := &model.Identity{Address: "10.0.0.19", Serial: "SN19", Make: "HP", Method: "ipp", Errors: map[string]string{"snmp": "request timeout (after 1 retries)", "hp": "no snmp/ipp answer: request timeout", "escl": "http 404"}}
	d, _, err = s.UpsertDevice(ctx, partial, nil, []string{"scan"})
	if err != nil {
		t.Fatal(err)
	}
	if d.IdentifyErrors["snmp"] != "request timeout (after 1 retries)" || d.IdentifyErrors["hp"] == "" || len(d.IdentifyErrors) != 2 {
		t.Fatalf("errors not stored as expected: %+v", d.IdentifyErrors)
	}
	if strings.Join(d.IdentifyMethods, ",") != "snmp,hp,ipp" {
		t.Fatalf("methods accumulate: %v", d.IdentifyMethods)
	}
	// A brand-new device only ever records the SNMP baseline.
	fresh, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.20", Serial: "SN20", Method: "ipp", Errors: map[string]string{"snmp": "timeout", "escl": "http 404", "hp": "no snmp/ipp answer"}}, nil, []string{"scan"})
	if err != nil || len(fresh.IdentifyErrors) != 1 || fresh.IdentifyErrors["snmp"] != "timeout" {
		t.Fatalf("new device errors: %+v (%v)", fresh.IdentifyErrors, err)
	}
	if d.Model != "M283" || d.Firmware != "1.0" {
		t.Fatalf("earlier values must survive a partial read: %+v", d)
	}
	got, err := s.GetDevice(ctx, d.ID)
	if err != nil || got.IdentifyErrors["snmp"] == "" {
		t.Fatalf("errors not read back: %+v (%v)", got.IdentifyErrors, err)
	}
	d, _, err = s.UpsertDevice(ctx, full, nil, []string{"scan"})
	if err != nil || len(d.IdentifyErrors) != 0 {
		t.Fatalf("a later clean scan should clear the errors: %+v (%v)", d.IdentifyErrors, err)
	}
}

// Field sources (intent/csv-import, spec decision 4): a patch merges its
// sources into the map, never replaces it; a scan leaves the map alone; the
// list and the single read return the same map.
func testFieldSourcesRoundTrip(t *testing.T, s testStore) {
	ctx := context.Background()
	d, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.9.0.1", Serial: "FS1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.FieldSources) != 0 {
		t.Fatalf("new device has field sources: %v", d.FieldSources)
	}
	at1 := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	at2 := at1.Add(time.Hour)
	loc, name := "Floor 2", "Front desk"
	if _, err := s.PatchDevice(ctx, d.ID, DevicePatch{Location: &loc, FieldSources: map[string]model.FieldSource{"location": {Source: model.SourceImport, At: at1}}}); err != nil {
		t.Fatal(err)
	}
	d, err = s.PatchDevice(ctx, d.ID, DevicePatch{DisplayName: &name, FieldSources: map[string]model.FieldSource{"displayName": {Source: model.SourceOperator, At: at2}}})
	if err != nil {
		t.Fatal(err)
	}
	same := func(got map[string]model.FieldSource, want map[string]model.FieldSource) bool {
		if len(got) != len(want) {
			return false
		}
		for k, w := range want {
			if g, ok := got[k]; !ok || g.Source != w.Source || !g.At.Equal(w.At) {
				return false
			}
		}
		return true
	}
	want := map[string]model.FieldSource{"location": {Source: model.SourceImport, At: at1}, "displayName": {Source: model.SourceOperator, At: at2}}
	if !same(d.FieldSources, want) {
		t.Fatalf("after two patches: %v", d.FieldSources)
	}
	// A patch without sources keeps them; one with a source replaces that key only.
	notes := "n"
	if d, err = s.PatchDevice(ctx, d.ID, DevicePatch{Notes: &notes}); err != nil || !same(d.FieldSources, want) {
		t.Fatalf("patch without sources: %v %v", d.FieldSources, err)
	}
	d, err = s.PatchDevice(ctx, d.ID, DevicePatch{FieldSources: map[string]model.FieldSource{"location": {Source: model.SourceDNS, At: at2}}})
	want["location"] = model.FieldSource{Source: model.SourceDNS, At: at2}
	if err != nil || !same(d.FieldSources, want) || d.Location != loc {
		t.Fatalf("source-only patch: %v %q %v", d.FieldSources, d.Location, err)
	}
	// A scan of the same printer leaves them alone.
	if d, _, err = s.UpsertDevice(ctx, &model.Identity{Address: "10.9.0.1", Serial: "FS1", Model: "M1", Method: "snmp"}, nil, nil); err != nil || !same(d.FieldSources, want) {
		t.Fatalf("after a scan: %v %v", d.FieldSources, err)
	}
	list, err := s.ListDevices(ctx, DeviceFilter{IDs: []string{d.ID}})
	if err != nil || len(list) != 1 || !same(list[0].FieldSources, want) {
		t.Fatalf("listed: %v %v", list, err)
	}
}

// #47: a device's severity is the worse of what the printer reports and its
// active operating alerts, in every read: the device, the filter, the sort and
// the dashboard's counts.
func testDeviceSeverityCountsActiveAlerts(t *testing.T, s testStore) {
	ctx := context.Background()
	seed := func(address, serial string) *model.Device {
		d, _, err := s.UpsertDevice(ctx, &model.Identity{Address: address, Serial: serial, Method: "snmp"}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ApplyPoll(ctx, d.ID, PollUpdate{Reachable: true, At: time.Now().UTC(),
			Status: &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, At: time.Now().UTC()}}); err != nil {
			t.Fatal(err)
		}
		return d
	}
	healthy := seed("10.0.0.10", "SN-HEALTHY")
	d := seed("10.0.0.20", "SN-ALERTED")
	want := func(step string, sev model.Severity, counts map[string]int) {
		t.Helper()
		got, err := s.GetDevice(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Severity != sev {
			t.Errorf("%s: device severity = %q, want %q", step, got.Severity, sev)
		}
		ds, err := s.Dashboard(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for k, n := range counts {
			if ds.BySeverity[k] != n {
				t.Errorf("%s: bySeverity[%s] = %d, want %d (%v)", step, k, ds.BySeverity[k], n, ds.BySeverity)
			}
		}
		list, err := s.ListDevices(ctx, DeviceFilter{Severity: string(sev)})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, l := range list {
			found = found || l.ID == d.ID
		}
		if !found {
			t.Errorf("%s: filter severity=%s misses the device", step, sev)
		}
		if n, err := s.CountDevices(ctx, DeviceFilter{Severity: string(sev)}); err != nil || n != len(list) {
			t.Errorf("%s: count severity=%s = %d %v, list has %d", step, sev, n, err, len(list))
		}
	}
	want("no alerts", model.SeverityOK, map[string]int{"ok": 2})

	// Security findings have their own widget and do not change device health.
	if _, _, err := s.RaiseAlert(ctx, d.ID, "security:telnet", "security", model.SeverityCritical, "Telnet on"); err != nil {
		t.Fatal(err)
	}
	want("security finding", model.SeverityOK, map[string]int{"ok": 2, "critical": 0})

	if _, _, err := s.RaiseAlert(ctx, d.ID, "supply:1", "supply", model.SeverityWarning, "Black low (12%)"); err != nil {
		t.Fatal(err)
	}
	want("supply warning", model.SeverityWarning, map[string]int{"ok": 1, "warning": 1})

	drum, _, err := s.RaiseAlert(ctx, d.ID, "supply:5", "supply", model.SeverityCritical, "Black Drum at 5%")
	if err != nil {
		t.Fatal(err)
	}
	want("supply critical", model.SeverityCritical, map[string]int{"ok": 1, "warning": 0, "critical": 1})

	// The healthy device has the lower address; severity still sorts first.
	sorted, err := s.ListDevices(ctx, DeviceFilter{Sort: "severity"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sorted) != 2 || sorted[0].ID != d.ID || sorted[1].ID != healthy.ID {
		t.Errorf("sort by severity puts the alerted device first: %+v", sorted)
	}

	// Acknowledging records that someone saw it; the drum is still at 5%.
	if _, err := s.AckAlert(ctx, drum.ID, "op"); err != nil {
		t.Fatal(err)
	}
	want("acknowledged", model.SeverityCritical, map[string]int{"critical": 1})

	// status.severity keeps meaning what the printer reported.
	if got, _ := s.GetDevice(ctx, d.ID); got.Status == nil || got.Status.Severity != model.SeverityOK {
		t.Errorf("printer-reported severity changed: %+v", got.Status)
	}

	// An offline device reads offline whatever alerts remain from before.
	if _, err := s.ApplyPoll(ctx, d.ID, PollUpdate{Reachable: false, At: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	want("offline with old alerts", "offline", map[string]int{"offline": 1, "critical": 0})
	if ds, _ := s.Dashboard(ctx); ds.Offline != 1 {
		t.Errorf("offline count = %d", ds.Offline)
	}

	// Back online with the conditions gone: healthy again.
	if _, err := s.ApplyPoll(ctx, d.ID, PollUpdate{Reachable: true, At: time.Now().UTC(),
		Status: &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, At: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearAlertsExcept(ctx, d.ID, "supply", nil); err != nil {
		t.Fatal(err)
	}
	want("cleared", model.SeverityOK, map[string]int{"ok": 2, "critical": 0})

	// A trap counts, and what the printer itself reports still does.
	if _, _, err := s.RaiseAlert(ctx, d.ID, "trap:cold-start", "trap", model.SeverityWarning, "Device cold start"); err != nil {
		t.Fatal(err)
	}
	want("trap", model.SeverityWarning, map[string]int{"warning": 1})
	if _, err := s.ClearAlertsExcept(ctx, d.ID, "trap", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyPoll(ctx, d.ID, PollUpdate{Reachable: true, At: time.Now().UTC(),
		Status: &model.Status{Online: true, State: "down", Severity: model.SeverityCritical, At: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	want("printer reports critical", model.SeverityCritical, map[string]int{"critical": 1})
}
