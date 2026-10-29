package ipp

import (
	"context"
	"testing"

	"printmarshal/internal/model"
)

func TestRoundTrip(t *testing.T) {
	req := NewGetPrinterAttributes("ipp://10.0.0.5:631/ipp/print", 42, []string{"printer-state", "marker-levels"})
	b, err := Encode(req)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if m.StatusCode != OpGetPrinterAttributes || m.RequestID != 42 || m.String("printer-uri") != "ipp://10.0.0.5:631/ipp/print" {
		t.Errorf("decoded = %+v", m)
	}
	if got := m.Strings("requested-attributes"); len(got) != 2 || got[1] != "marker-levels" {
		t.Errorf("1setOf = %v", got)
	}

	resp := &Message{Version: [2]byte{2, 0}, StatusCode: 0, RequestID: 42, Groups: []Group{
		{Tag: TagOperation, Attributes: []Attribute{{Tag: TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}}}},
		{Tag: TagPrinter, Attributes: []Attribute{
			{Tag: TagEnum, Name: "printer-state", Values: []any{3}},
			{Tag: TagInteger, Name: "marker-levels", Values: []any{12, -3, 100}},
			{Tag: TagBoolean, Name: "color-supported", Values: []any{true}},
			{Tag: TagRange, Name: "copies-supported", Values: []any{[2]int{1, 99}}},
			{Tag: TagBegCollection, Name: "media-col-ready", Values: []any{map[string]Attribute{
				"media-size": {Tag: TagBegCollection, Values: []any{map[string]Attribute{"x-dimension": {Tag: TagInteger, Values: []any{21000}}}}},
				"media-type": {Tag: TagKeyword, Values: []any{"stationery"}},
			}}},
			{Tag: TagKeyword, Name: "printer-state-reasons", Values: []any{"none"}},
		}},
	}}
	b, err = Encode(resp)
	if err != nil {
		t.Fatal(err)
	}
	m, err = Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := m.Int("printer-state"); n != 3 {
		t.Errorf("printer-state = %d", n)
	}
	if lv := m.Ints("marker-levels"); len(lv) != 3 || lv[1] != -3 {
		t.Errorf("marker-levels = %v", lv)
	}
	if m.String("printer-state-reasons") != "none" {
		t.Errorf("attribute after a collection was lost: %+v", m.Groups[1].Attributes)
	}
	coll, ok := m.Attr("media-col-ready").Values[0].(map[string]Attribute)
	if !ok || coll["media-type"].Values[0] != "stationery" {
		t.Errorf("collection = %#v", m.Attr("media-col-ready").Values)
	}
}

func TestPWGStrings(t *testing.T) {
	m := &Message{Groups: []Group{{Tag: TagPrinter, Attributes: []Attribute{
		{Tag: TagOctet, Name: "printer-supply", Values: []any{
			"index=1;class=supplyThatIsConsumed;type=toner;unit=percent;maxcapacity=100;level=8;colorantname=black;",
			"index=2;class=receptacleThatIsFilled;type=wasteToner;unit=percent;maxcapacity=100;level=50;",
		}},
		{Tag: TagText, Name: "printer-supply-description", Values: []any{"Black Cartridge", "Waste Box"}},
		{Tag: TagOctet, Name: "printer-input-tray", Values: []any{
			"type=sheetFeedAutoRemovableTray;mediafeed=29700;mediaxfeed=21000;maxcapacity=250;level=0;status=16;name=Tray 1;",
		}},
		{Tag: TagEnum, Name: "printer-state", Values: []any{5}},
		{Tag: TagKeyword, Name: "printer-state-reasons", Values: []any{"media-jam-error", "toner-low-warning"}},
	}}}}
	s := SuppliesFrom(m)
	if len(s) != 2 || s[0].Description != "Black Cartridge" || *s[0].Percent != 8 || s[1].Class != "filled" {
		t.Errorf("supplies = %+v", s)
	}
	tr := TraysFrom(m)
	if len(tr) != 1 || tr[0].Status != "empty" || tr[0].MediaSize != "210x297mm" {
		t.Errorf("trays = %+v", tr)
	}
	st := StatusFrom(m)
	if st.State != "down" || st.Severity != model.SeverityCritical || len(st.Errors) != 2 {
		t.Errorf("status = %+v", st)
	}
}

type fakeTransport struct{ resp []byte }

func (f fakeTransport) Do(ctx context.Context, url string, body []byte) ([]byte, error) {
	return f.resp, nil
}

func (f fakeTransport) DoAuth(ctx context.Context, url string, body []byte, _ Auth) (*Response, error) {
	return &Response{Body: f.resp}, nil
}

func TestIdentifyViaTransport(t *testing.T) {
	resp := &Message{Version: [2]byte{2, 0}, RequestID: 1, Groups: []Group{{Tag: TagPrinter, Attributes: []Attribute{
		{Tag: TagText, Name: "printer-make-and-model", Values: []any{"Acme ColorWorks 7700"}},
		{Tag: TagText, Name: "printer-device-id", Values: []any{"MFG:Acme;MDL:ColorWorks 7700;CMD:PDF,SCAN;SN:CW7700-1;"}},
		{Tag: TagURI, Name: "printer-uuid", Values: []any{"urn:uuid:11111111-2222-3333-4444-555555555555"}},
		{Tag: TagText, Name: "printer-firmware-string-version", Values: []any{"9.1.2"}},
		{Tag: TagEnum, Name: "printer-state", Values: []any{3}},
	}}}}
	b, _ := Encode(resp)
	d := New(fakeTransport{b}, DefaultOptions())
	id, err := d.Identify(context.Background(), "10.0.0.9", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "Acme" || id.Serial != "CW7700-1" || id.UUID != "11111111-2222-3333-4444-555555555555" || id.Firmware != "9.1.2" || id.DeviceType != "mfp" {
		t.Errorf("identity = %+v", id)
	}
	hasFW := false
	for _, c := range id.Capabilities {
		if c == model.CapFirmwareRead {
			hasFW = true
		}
	}
	if !hasFW {
		t.Error("firmware_read capability expected")
	}
}
