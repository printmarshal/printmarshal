// Package importcsv reads a CSV list of printers and plans what importing it
// would do to the inventory (intent/csv-import). It is standard library
// only, apart from csvsafe, contacts no device and changes nothing: Parse turns the file into a
// Table, Plan turns a Table and a Lookup over the inventory into a
// PlanResult, which the API returns as the preview and the import job
// recomputes and applies.
package importcsv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"printmarshal/internal/csvsafe"
)

// Limits on one file (spec R1).
const (
	MaxRows  = 5000
	MaxBytes = 2 << 20
)

// Canonical column names: the keys the plan and the API use.
const (
	ColAddress     = "address"
	ColHostname    = "hostname"
	ColSerial      = "serial"
	ColDisplayName = "displayName"
	ColLocation    = "location"
	ColAssetTag    = "assetTag"
	ColNotes       = "notes"
	ColWebURL      = "webUrl"
	ColDNSSync     = "dnsSync"
	ColState       = "state"
	ColGroup       = "group"
)

// KnownColumns lists every column the import reads, keys first.
var KnownColumns = []string{ColAddress, ColHostname, ColSerial, ColDisplayName, ColLocation, ColAssetTag, ColNotes, ColWebURL, ColDNSSync, ColState, ColGroup}

// ExportHeader is the header of the console's device export
// (GET /api/devices.csv). The import accepts an export from another instance
// (spec decision 3); internal/api's import test keeps this list equal to
// what the export writes.
var ExportHeader = []string{"id", "name", "make", "model", "serial", "address", "hostname", "mac", "firmware", "type", "state", "status", "severity", "location", "assetTag", "pagesTotal", "lastSeen", "lastPolled"}

// ExportIgnored lists the export columns the import deliberately ignores:
// the record id of the other instance and automatic fields a scan owns.
var ExportIgnored = []string{"id", "make", "model", "mac", "firmware", "type", "status", "severity", "pagesTotal", "lastSeen", "lastPolled"}

// aliases maps a normalised header (see Normalize) to its column: the keys
// themselves, the display names the page's template uses ("Web address",
// "Lifecycle state"), the export's "name", and a few common spellings.
var aliases = map[string]string{
	"address":        ColAddress,
	"ip":             ColAddress,
	"ipaddress":      ColAddress,
	"hostname":       ColHostname,
	"host":           ColHostname,
	"serial":         ColSerial,
	"serialnumber":   ColSerial,
	"displayname":    ColDisplayName,
	"name":           ColDisplayName,
	"location":       ColLocation,
	"assettag":       ColAssetTag,
	"notes":          ColNotes,
	"note":           ColNotes,
	"weburl":         ColWebURL,
	"webaddress":     ColWebURL,
	"dnssync":        ColDNSSync,
	"state":          ColState,
	"lifecyclestate": ColState,
	"group":          ColGroup,
	"groupname":      ColGroup,
}

// Normalize lower-cases a header and drops spaces, underscores and hyphens
// (spec R2).
func Normalize(h string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '_', '-':
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(h)))
}

// Canonical returns the column a header names, or false for one the import
// does not know.
func Canonical(h string) (string, bool) {
	c, ok := aliases[Normalize(h)]
	return c, ok
}

// Table is a parsed file: the recognised columns in file order, the headers
// it ignored, and one Record per non-blank row.
type Table struct {
	Separator string
	Known     []string
	Ignored   []string
	Records   []Record
}

// Record is one row: its line in the file (the header is line 1) and its
// non-empty cells by column, trimmed.
type Record struct {
	Line  int
	Cells map[string]string
}

var bom = []byte{0xEF, 0xBB, 0xBF}

// Parse reads a UTF-8 CSV (optional byte-order mark, comma or semicolon
// separated) of at most maxRows data rows and maxBytes bytes. Errors are one
// line, fit to show the operator as they are.
func Parse(r io.Reader, maxRows, maxBytes int) (*Table, error) {
	b, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("file is unreadable: %v", err)
	}
	if len(b) > maxBytes {
		return nil, fmt.Errorf("file is over %s", sizeText(maxBytes))
	}
	b = bytes.TrimPrefix(b, bom)
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("file is empty")
	}
	if !utf8.Valid(b) {
		return nil, errors.New("file is not UTF-8 text")
	}
	sep := detectSeparator(b)
	cr := csv.NewReader(bytes.NewReader(b))
	cr.Comma = rune(sep[0])
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, csvError(err)
	}
	t := &Table{Separator: sep, Known: []string{}, Ignored: []string{}, Records: []Record{}}
	cols := make([]string, len(header))
	seen := map[string]string{}
	for i, h := range header {
		h = strings.TrimSpace(h)
		c, ok := Canonical(h)
		if !ok {
			if h != "" {
				t.Ignored = append(t.Ignored, h)
			}
			continue
		}
		if prev, dup := seen[c]; dup {
			return nil, fmt.Errorf("column %q repeats column %q", h, prev)
		}
		seen[c] = h
		cols[i] = c
		t.Known = append(t.Known, c)
	}
	if seen[ColAddress] == "" && seen[ColHostname] == "" && seen[ColSerial] == "" {
		return nil, errors.New("no key column: the file needs an address, hostname or serial column")
	}
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, csvError(err)
		}
		line, _ := cr.FieldPos(0)
		cells := map[string]string{}
		for i, v := range rec {
			if i < len(cols) && cols[i] != "" {
				if v = strings.TrimSpace(v); v != "" {
					cells[cols[i]] = csvsafe.Undo(v)
				}
			}
		}
		if len(cells) == 0 {
			continue // a blank row, or one with only ignored columns filled
		}
		if len(t.Records) >= maxRows {
			return nil, fmt.Errorf("file has more than %d rows", maxRows)
		}
		t.Records = append(t.Records, Record{Line: line, Cells: cells})
	}
	return t, nil
}

// detectSeparator picks comma or semicolon, whichever splits the header line
// into more cells (quoted text does not count); a tie is a comma.
func detectSeparator(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[:i]
	}
	commas, semis, quoted := 0, 0, false
	for _, c := range b {
		switch {
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == ',':
			commas++
		case c == ';':
			semis++
		}
	}
	if semis > commas {
		return ";"
	}
	return ","
}

func csvError(err error) error {
	var pe *csv.ParseError
	if errors.As(err, &pe) {
		return fmt.Errorf("line %d: %v", pe.StartLine, pe.Err)
	}
	return fmt.Errorf("file is unreadable: %v", err)
}

func sizeText(n int) string {
	if n >= 1<<20 && n%(1<<20) == 0 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}
