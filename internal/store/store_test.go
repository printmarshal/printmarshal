package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
