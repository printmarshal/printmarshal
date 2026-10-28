package ipp

import (
	"context"
	"os"
	"sync"
	"testing"

	"printmarshal/internal/model"
)

// recordingTransport answers every request with a canned response and records
// the operations it saw, so a test can prove a dry run never sent a write.
type recordingTransport struct {
	mu   sync.Mutex
	resp []byte
	// respFor overrides the response per operation id.
	respFor map[uint16][]byte
	ops     []uint16
	reqs    []*Message
}

func (t *recordingTransport) Do(_ context.Context, _ string, body []byte) ([]byte, error) {
	m, err := Decode(body)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ops = append(t.ops, m.StatusCode)
	t.reqs = append(t.reqs, m)
	if r, ok := t.respFor[m.StatusCode]; ok {
		return r, nil
	}
	return t.resp, nil
}

func (t *recordingTransport) count(op uint16) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, o := range t.ops {
		if o == op {
			n++
		}
	}
	return n
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func settingMap(doc *model.ConfigDocument) map[string]model.Setting {
	m := map[string]model.Setting{}
	for _, s := range doc.Settings {
		m[s.Key] = s
	}
	return m
}

// Unit (spec §14): the mapping against a full Get-Printer-Attributes response
// recorded from a real HP M283fdw.
func TestConfigFromRealPrinter(t *testing.T) {
	m, err := Decode(loadFixture(t, "hp-m283fdw-full.ipp"))
	if err != nil {
		t.Fatal(err)
	}
	s := settingMap(&model.ConfigDocument{Settings: ConfigFrom(m)})
	want := map[string]any{
		"identity.hostname":           "NPI0A1B2C",
		"identity.location":           "Home Office",
		"identity.description":        "HP Color LaserJet MFP M283fdw",
		"print.defaultCopies":         1,
		"print.defaultDuplex":         false,
		"print.defaultColorMode":      "auto",
		"print.defaultResolution":     "600x600dpi",
		"print.defaultDocumentFormat": "application/pdf",
		"print.jobTimeoutSeconds":     120,
		"trays.defaultMediaSize":      "iso_a4_210x297mm",
		"trays.defaultTray":           "auto",
		"trays.defaultMediaType":      "stationery",
		"panel.language":              "en",
		"protocols.ipp.enabled":       true,
		"protocols.ipps.enabled":      true,
	}
	for k, v := range want {
		got, ok := s[k]
		if !ok {
			t.Errorf("%s missing", k)
			continue
		}
		if !model.ValuesEqual(got.Value, v) {
			t.Errorf("%s = %v, want %v", k, got.Value, v)
		}
		if got.Source != Name || got.Area != model.CatalogByKey[k].Area {
			t.Errorf("%s source/area = %q/%q", k, got.Source, got.Area)
		}
	}
	// This printer does not list Set-Printer-Attributes (0x0013) in
	// operations-supported, so nothing may be writable.
	for k, st := range s {
		if st.Writable {
			t.Errorf("%s writable on a device without Set-Printer-Attributes", k)
		}
	}
	if s["identity.location"].ReadOnlyReason == "" {
		t.Error("read-only settings must say why")
	}
}

// settableResponse is a simulated printer that supports Set-Printer-Attributes.
func settableResponse() *Message {
	return &Message{Version: [2]byte{2, 0}, RequestID: 1, Groups: []Group{{Tag: TagPrinter, Attributes: []Attribute{
		{Tag: TagName, Name: "printer-name", Values: []any{"SIM-1"}},
		{Tag: TagText, Name: "printer-location", Values: []any{"Basement"}},
		{Tag: TagText, Name: "printer-info", Values: []any{"Simulated"}},
		{Tag: TagInteger, Name: "copies-default", Values: []any{1}},
		{Tag: TagRange, Name: "copies-supported", Values: []any{[2]int{1, 99}}},
		{Tag: TagKeyword, Name: "sides-default", Values: []any{"one-sided"}},
		{Tag: TagKeyword, Name: "sides-supported", Values: []any{"one-sided", "two-sided-long-edge"}},
		{Tag: TagKeyword, Name: "print-color-mode-default", Values: []any{"auto"}},
		{Tag: TagKeyword, Name: "print-color-mode-supported", Values: []any{"auto", "monochrome"}},
		{Tag: TagKeyword, Name: "media-default", Values: []any{"iso_a4_210x297mm"}},
		{Tag: TagKeyword, Name: "media-supported", Values: []any{"iso_a4_210x297mm", "na_letter_8.5x11in"}},
		{Tag: TagEnum, Name: "operations-supported", Values: []any{0x0002, 0x000B, int(OpSetPrinterAttributes)}},
		{Tag: TagKeyword, Name: "printer-settable-attributes-supported", Values: []any{"printer-location", "printer-info", "copies-default", "sides-default", "print-color-mode-default", "media-default"}},
		{Tag: TagURI, Name: "printer-uri-supported", Values: []any{"ipp://10.0.0.9/ipp/print"}},
	}}}}
}

func encodeMsg(t *testing.T, m *Message) []byte {
	t.Helper()
	b, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var changes = []model.SettingChange{
	{Key: "identity.location", Value: "Lobby"},
	{Key: "print.defaultCopies", Value: float64(2)}, // JSON number
	{Key: "print.defaultDuplex", Value: true},
	{Key: "print.defaultColorMode", Value: "color"}, // not in print-color-mode-supported
	{Key: "identity.hostname", Value: "NEW-NAME"},   // printer-name is not in the settable list
	{Key: "trays.defaultMediaSize", Value: "iso_a4_210x297mm"},
	{Key: "energy.sleepDelayMinutes", Value: 5}, // no IPP mapping
}

// Safety (spec §14): a dry run performs no write I/O and still reports per setting.
func TestWriteConfigDryRunSendsNoSet(t *testing.T) {
	tr := &recordingTransport{resp: encodeMsg(t, settableResponse())}
	d := New(tr, Options{Endpoints: []string{"http://%s:631/ipp/print"}})
	dev := &model.Device{ID: "dev1", Address: "10.0.0.9"}
	res, err := d.WriteConfig(context.Background(), dev, model.Creds{}, changes, model.WriteOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if tr.count(OpSetPrinterAttributes) != 0 {
		t.Fatal("dry run sent Set-Printer-Attributes")
	}
	if !res.DryRun || len(res.Rollback) != 0 {
		t.Errorf("dry run result = %+v", res)
	}
	got := map[string]model.SettingResult{}
	for _, r := range res.Results {
		got[r.Key] = r
	}
	if len(got) != len(changes) {
		t.Fatalf("expected one result per change, got %d", len(got))
	}
	if got["identity.location"].Status != model.SettingWouldWrite || got["identity.location"].Before != "Basement" {
		t.Errorf("location = %+v", got["identity.location"])
	}
	if got["print.defaultCopies"].Status != model.SettingWouldWrite || got["print.defaultDuplex"].Status != model.SettingWouldWrite {
		t.Errorf("copies/duplex = %+v / %+v", got["print.defaultCopies"], got["print.defaultDuplex"])
	}
	if got["print.defaultColorMode"].Status != model.SettingFailed {
		t.Errorf("unsupported colour mode should fail validation: %+v", got["print.defaultColorMode"])
	}
	if got["identity.hostname"].Status != model.SettingSkipped || got["identity.hostname"].Error == "" {
		t.Errorf("printer-name must be skipped with a reason: %+v", got["identity.hostname"])
	}
	if got["trays.defaultMediaSize"].Status != model.SettingUnchanged {
		t.Errorf("same value must be unchanged: %+v", got["trays.defaultMediaSize"])
	}
	if got["energy.sleepDelayMinutes"].Status != model.SettingSkipped {
		t.Errorf("unmapped key must be skipped: %+v", got["energy.sleepDelayMinutes"])
	}
}

// A real write sends one Set-Printer-Attributes per setting (RFC 3380 §4.1)
// with the right attribute encoding and records a rollback.
func TestWriteConfigSetsAttributesAndRollback(t *testing.T) {
	okResp := encodeMsg(t, &Message{Version: [2]byte{2, 0}, RequestID: 1, StatusCode: StatusOK,
		Groups: []Group{{Tag: TagOperation, Attributes: []Attribute{{Tag: TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}}}}}})
	tr := &recordingTransport{resp: encodeMsg(t, settableResponse()), respFor: map[uint16][]byte{OpSetPrinterAttributes: okResp}}
	d := New(tr, Options{Endpoints: []string{"http://%s:631/ipp/print"}})
	dev := &model.Device{ID: "dev1", Address: "10.0.0.9"}
	res, err := d.WriteConfig(context.Background(), dev, model.Creds{}, changes, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n := tr.count(OpSetPrinterAttributes); n != 3 {
		t.Fatalf("expected 3 Set-Printer-Attributes requests (location, copies, duplex), got %d", n)
	}
	sent := map[string]Attribute{}
	for _, m := range tr.reqs {
		if m.StatusCode != OpSetPrinterAttributes {
			continue
		}
		if m.String("printer-uri") != "ipp://10.0.0.9:631/ipp/print" || m.String("requesting-user-name") != RequestingUser {
			t.Errorf("operation group = %+v", m.Groups[0].Attributes)
		}
		for _, g := range m.Groups {
			if g.Tag == TagPrinter {
				for _, a := range g.Attributes {
					sent[a.Name] = a
				}
			}
		}
	}
	if a := sent["printer-location"]; a.Tag != TagText || a.Values[0] != "Lobby" {
		t.Errorf("printer-location = %+v", a)
	}
	if a := sent["copies-default"]; a.Tag != TagInteger || a.Values[0] != 2 {
		t.Errorf("copies-default = %+v", a)
	}
	if a := sent["sides-default"]; a.Tag != TagKeyword || a.Values[0] != "two-sided-long-edge" {
		t.Errorf("sides-default = %+v", a)
	}
	if len(res.Rollback) != 3 {
		t.Fatalf("rollback = %+v", res.Rollback)
	}
	rb := map[string]any{}
	for _, c := range res.Rollback {
		rb[c.Key] = c.Value
	}
	if rb["identity.location"] != "Basement" || !model.ValuesEqual(rb["print.defaultCopies"], 1) || rb["print.defaultDuplex"] != false {
		t.Errorf("rollback values = %v", rb)
	}
	for _, r := range res.Results {
		if r.Key == "identity.location" && r.Status != model.SettingWritten {
			t.Errorf("location = %+v", r)
		}
	}
}

// A device that rejects the attribute (client-error-attributes-not-settable)
// yields a failed setting and no rollback entry.
func TestWriteConfigRejected(t *testing.T) {
	rejected := encodeMsg(t, &Message{Version: [2]byte{2, 0}, RequestID: 1, StatusCode: StatusClientErrorAttrsNotSettable,
		Groups: []Group{{Tag: TagUnsupported, Attributes: []Attribute{{Tag: TagUnsupportedValue, Name: "printer-location", Values: []any{""}}}}}})
	tr := &recordingTransport{resp: encodeMsg(t, settableResponse()), respFor: map[uint16][]byte{OpSetPrinterAttributes: rejected}}
	d := New(tr, Options{Endpoints: []string{"http://%s:631/ipp/print"}})
	res, err := d.WriteConfig(context.Background(), &model.Device{ID: "d", Address: "10.0.0.9"}, model.Creds{},
		[]model.SettingChange{{Key: "identity.location", Value: "Lobby"}}, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].Status != model.SettingFailed || len(res.Rollback) != 0 {
		t.Errorf("result = %+v", res)
	}
}

func TestIdentifyReportsConfigCapabilities(t *testing.T) {
	tr := &recordingTransport{resp: encodeMsg(t, settableResponse())}
	d := New(tr, Options{Endpoints: []string{"http://%s:631/ipp/print"}})
	id, err := d.Identify(context.Background(), "10.0.0.9", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	has := map[model.Capability]bool{}
	for _, c := range id.Capabilities {
		has[c] = true
	}
	if !has[model.CapReadConfig] || !has[model.CapWriteConfig] {
		t.Errorf("capabilities = %v", id.Capabilities)
	}
	// The real HP does not support the operation: read_config only.
	tr2 := &recordingTransport{resp: loadFixture(t, "hp-m283fdw-full.ipp")}
	id2, err := New(tr2, Options{Endpoints: []string{"http://%s:631/ipp/print"}}).Identify(context.Background(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	has = map[model.Capability]bool{}
	for _, c := range id2.Capabilities {
		has[c] = true
	}
	if !has[model.CapReadConfig] || has[model.CapWriteConfig] {
		t.Errorf("HP capabilities = %v", id2.Capabilities)
	}
}

func TestResolutionCodec(t *testing.T) {
	b, err := encodeResolutionString("600x1200dpi")
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := decodeResolution(b); !ok || s != "600x1200dpi" {
		t.Errorf("round trip = %q", s)
	}
	if _, err := encodeResolutionString("fast"); err == nil {
		t.Error("garbage resolution accepted")
	}
}
