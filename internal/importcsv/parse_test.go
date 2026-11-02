package importcsv

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name      string
		in        string
		maxRows   int
		maxBytes  int
		sep       string
		known     []string
		ignored   []string
		lines     []int
		cells     []map[string]string
		errSubstr string
	}{
		{
			name:  "comma with BOM and normalised header",
			in:    "\ufeffAddress,Display Name,asset_tag,Asset Owner\n10.0.0.5, Front desk ,A-1,Kim\n",
			sep:   ",",
			known: []string{"address", "displayName", "assetTag"}, ignored: []string{"Asset Owner"},
			lines: []int{2},
			cells: []map[string]string{{"address": "10.0.0.5", "displayName": "Front desk", "assetTag": "A-1"}},
		},
		{
			name:  "semicolon detected from the header",
			in:    "address;location;DNS-sync\r\n10.0.0.5;Floor 2, east;yes\r\n",
			sep:   ";",
			known: []string{"address", "location", "dnsSync"}, ignored: []string{},
			lines: []int{2},
			cells: []map[string]string{{"address": "10.0.0.5", "location": "Floor 2, east", "dnsSync": "yes"}},
		},
		{
			name:  "quoted multi-line cell keeps file line numbers; blank rows dropped",
			in:    "serial,notes,web url\nS1,\"two\nlines\",http://a\n,,\n\nS2,,\n",
			sep:   ",",
			known: []string{"serial", "notes", "webUrl"}, ignored: []string{},
			lines: []int{2, 6},
			cells: []map[string]string{{"serial": "S1", "notes": "two\nlines", "webUrl": "http://a"}, {"serial": "S2"}},
		},
		{
			name:  "short rows are allowed",
			in:    "hostname,group,state\nlp-1\n",
			sep:   ",",
			known: []string{"hostname", "group", "state"}, ignored: []string{},
			lines: []int{2},
			cells: []map[string]string{{"hostname": "lp-1"}},
		},
		{name: "no key column", in: "location,notes\nFloor 2,x\n", errSubstr: "address, hostname or serial column"},
		{name: "empty file", in: "", errSubstr: "file is empty"},
		{name: "only a BOM", in: "\ufeff\n", errSubstr: "file is empty"},
		{name: "not UTF-8", in: "address,notes\n10.0.0.1,caf\xe9\n", errSubstr: "not UTF-8"},
		{name: "column twice", in: "address,Asset Tag,asset-tag\n", errSubstr: `"asset-tag" repeats`},
		{name: "too large", in: "address\n10.0.0.1\n10.0.0.2\n", maxBytes: 10, errSubstr: "over 10 bytes"},
		{name: "too many rows", in: "address\n10.0.0.1\n10.0.0.2\n10.0.0.3\n", maxRows: 2, errSubstr: "more than 2 rows"},
		{name: "bad quoting", in: "address,notes\n10.0.0.1,\"open\n", errSubstr: "line 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maxRows, maxBytes := tc.maxRows, tc.maxBytes
			if maxRows == 0 {
				maxRows = MaxRows
			}
			if maxBytes == 0 {
				maxBytes = MaxBytes
			}
			tab, err := Parse(strings.NewReader(tc.in), maxRows, maxBytes)
			if tc.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tab.Separator != tc.sep {
				t.Errorf("separator %q, want %q", tab.Separator, tc.sep)
			}
			if !reflect.DeepEqual(tab.Known, tc.known) {
				t.Errorf("known %v, want %v", tab.Known, tc.known)
			}
			if !reflect.DeepEqual(tab.Ignored, tc.ignored) {
				t.Errorf("ignored %v, want %v", tab.Ignored, tc.ignored)
			}
			var lines []int
			var cells []map[string]string
			for _, r := range tab.Records {
				lines = append(lines, r.Line)
				cells = append(cells, r.Cells)
			}
			if !reflect.DeepEqual(lines, tc.lines) {
				t.Errorf("lines %v, want %v", lines, tc.lines)
			}
			if !reflect.DeepEqual(cells, tc.cells) {
				t.Errorf("cells %v, want %v", cells, tc.cells)
			}
		})
	}
}

func TestParseSizeMessageInMegabytes(t *testing.T) {
	_, err := Parse(strings.NewReader("address\n"+strings.Repeat("10.0.0.1\n", 300000)), MaxRows, MaxBytes)
	if err == nil || err.Error() != "file is over 2 MB" {
		t.Fatalf("err = %v", err)
	}
}

func TestCanonical(t *testing.T) {
	// The page's "Download template" header uses the display names (R16).
	for in, want := range map[string]string{
		"Address": "address", "Hostname": "hostname", "Serial": "serial", "Display name": "displayName",
		"Location": "location", "Asset tag": "assetTag", "Notes": "notes", "Web address": "webUrl",
		"DNS sync": "dnsSync", "Lifecycle state": "state", "Group": "group",
	} {
		if got, ok := Canonical(in); !ok || got != want {
			t.Errorf("template column Canonical(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for in, want := range map[string]string{
		"Address": "address", "IP address": "address", "ip": "address",
		"Host name": "hostname", "SERIAL": "serial", "serial number": "serial",
		"Display name": "displayName", "name": "displayName",
		"Location": "location", "Asset tag": "assetTag", "notes": "notes", "Note": "notes",
		"Web URL": "webUrl", "web address": "webUrl", "dns_sync": "dnsSync",
		"State": "state", "lifecycle state": "state", "Group": "group",
	} {
		if got, ok := Canonical(in); !ok || got != want {
			t.Errorf("Canonical(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "make", "pagesTotal", "owner"} {
		if got, ok := Canonical(in); ok {
			t.Errorf("Canonical(%q) = %q, want unknown", in, got)
		}
	}
}

// The console's own device export (GET /api/devices.csv) imports into
// another instance (spec decision 3): every export column is either a known
// column or deliberately ignored, never both, and the ignored list names only
// export columns. internal/api's import test keeps ExportHeader equal to the
// header the export writes.
func TestExportHeaderAliases(t *testing.T) {
	ignored := map[string]bool{}
	for _, c := range ExportIgnored {
		ignored[c] = true
	}
	inExport := map[string]bool{}
	for _, c := range ExportHeader {
		inExport[c] = true
		_, known := Canonical(c)
		switch {
		case known && ignored[c]:
			t.Errorf("export column %q is both known and listed as ignored", c)
		case !known && !ignored[c]:
			t.Errorf("export column %q is neither known nor listed as ignored", c)
		}
	}
	for _, c := range ExportIgnored {
		if !inExport[c] {
			t.Errorf("ExportIgnored names %q, which the export does not write", c)
		}
	}
	tab, err := Parse(strings.NewReader(strings.Join(ExportHeader, ",")+"\n"), MaxRows, MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"displayName", "serial", "address", "hostname", "state", "location", "assetTag"}
	if !reflect.DeepEqual(tab.Known, want) {
		t.Errorf("export known columns %v, want %v", tab.Known, want)
	}
	if !reflect.DeepEqual(tab.Ignored, ExportIgnored) {
		t.Errorf("export ignored columns %v, want %v", tab.Ignored, ExportIgnored)
	}
}

// A file the console exported carries an apostrophe before formula characters;
// reading it back gives the value, not the protection.
func TestParseRemovesExportProtection(t *testing.T) {
	tbl, err := Parse(strings.NewReader("address,displayName\n10.0.0.1,\"'=Lobby\"\n"), 10, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Records) != 1 || tbl.Records[0].Cells["displayName"] != "=Lobby" {
		t.Errorf("records: %+v", tbl.Records)
	}
}
